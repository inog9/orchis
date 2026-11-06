import fs from "node:fs";
import path from "node:path";
import crypto from "node:crypto";
import { fileURLToPath } from "node:url";
import { Type } from "typebox";
import { createMcpExtension, createToolSearchExtension } from "@earendil-works/pi-coding-agent";
import { Controller, PHASES } from "./controller.mjs";
import { Budget, scheduleRequest } from "./budget.mjs";
import { atomicJSON, privateText, readJSON, transact } from "./storage.mjs";
import { runSpecialist, terminateSpecialist, browserSession } from "./specialists.mjs";
import { createFleetBackend } from "./fleet.mjs";
import { childBinding, claimAttempt, assertAttempt } from "./admission.mjs";
import { isIdleShellPolling } from "./orchestration.mjs";

const runtimeDir = path.dirname(fileURLToPath(import.meta.url));
const response = value => ({ content: [{ type: "text", text: typeof value === "string" ? value : JSON.stringify(value, null, 2) }], details: {} });
const networkTool = name => name === "pentest_http" || name.startsWith("mcp__") || ["list_mcp_resources", "list_mcp_resource_templates", "read_mcp_resource"].includes(name);
const taskSchema = Type.Object({
  id: Type.String({ description: "Unique bounded task ID, e.g. setup-audit or role-viewer-crawl" }),
  question: Type.String({ description: "One concrete specialist question with a clear success criterion" }),
  skill: Type.String({ description: "Workspace-relative skill path, e.g. .pi/skills/pentest-exploration/SKILL.md" }),
  phase: Type.Optional(Type.String({ enum: PHASES, description: "Defaults to current phase. Independent offline review may start in an earlier phase; future live work queues." })),
  network: Type.Optional(Type.Boolean({ description: "False for offline source review/reflection. Offline work accepts empty targets and zero request/rate budgets." })),
  targets: Type.Optional(Type.Array(Type.Object({ url: Type.String(), method: Type.String() }), { description: "Required for live network work; omit or use [] for local-only work" })),
  output: Type.Optional(Type.String({ description: "Defaults to reports/<id>.md; must be a new unique report path" })),
  inputs: Type.Optional(Type.Array(Type.String())), dependencies: Type.Optional(Type.Array(Type.String())), stateKeys: Type.Optional(Type.Array(Type.String())),
  method: Type.Optional(Type.String()), success: Type.Optional(Type.String()), stop: Type.Optional(Type.String()), restoration: Type.Optional(Type.String()),
  mutates: Type.Optional(Type.Boolean({ description: "Defaults false; true requires an explicit restoration plan" })),
  bounds: Type.Optional(Type.Object({ requests: Type.Optional(Type.Integer({ minimum: 0 })), minutes: Type.Optional(Type.Number({ exclusiveMinimum: 0 })), rate: Type.Optional(Type.Number({ minimum: 0, maximum: 50 })) })),
  actor: Type.Optional(Type.Object({ account: Type.Optional(Type.String()), role: Type.Optional(Type.String()), tenant: Type.Optional(Type.String()), permissions: Type.Optional(Type.String()), identityEvidence: Type.Optional(Type.String()), session: Type.Optional(Type.String()) }, { description: "Local/anonymous defaults are safe. Authenticated actors require all explicit server-verified identity/session fields." })),
});

export default function (pi) {
  const root = process.env.PENTEST_HARNES_SESSION;
  const launch = process.env.PENTEST_HARNES_LAUNCH;
  const binding = childBinding();
  const taskID = binding?.taskId ?? process.env.PENTEST_HARNES_TASK;
  const fleetPackage = process.env.PENTEST_HARNES_SUBAGENTS_DIR;
  if (!root || !launch) throw new Error("Launch this extension through pentest-harnes");
  let controller, budget, reservation, activeContext, watchdog, workerStopTimer, fleet, fleetError, childAdmission, childSessionId;
  let budgetBlocked = false;
  let leadErrorRetries = 0;
  const children = new Set();
  const terminateChild = terminateSpecialist;
  const activity = new Map();
  let dispatching = false;
  const inFlight = new Map();
  const inflightFile = path.join(root, "private", "inflight", `${process.pid}.json`);
  const checkpointTools = () => atomicJSON(inflightFile, { pid: process.pid, task: taskID ?? null, calls: [...inFlight.values()], updatedAt: new Date().toISOString() });
  const countRequest = task => {
    if (!task) return;
    transact(path.join(root, "specialists", task.id, "requests.json"), { count: 0 }, counts => {
      if (counts.count >= task.bounds.requests) throw new Error("Specialist managed-request budget exhausted");
      counts.count++;
    });
  };
  const refresh = () => {
    if (taskID) controller.state = readJSON(controller.file);
    return controller;
  };
  const workerStopReason = () => {
    if (!taskID || !controller) return undefined;
    refresh();
    const task = controller.state.tasks[taskID];
    const parent = childAdmission?.parentPid ?? Number(process.env.PENTEST_HARNES_PARENT_PID);
    try {
      if (!Number.isSafeInteger(parent) || parent <= 0) throw new Error("Missing parent PID");
      process.kill(parent, 0);
    } catch { return "Orchestrator exited; specialist must not continue orphaned work"; }
    if (!task || task.status !== "running") return "Specialist contract is no longer running";
    if (binding) try { assertAttempt({ root, controller, binding, sessionId: childSessionId }); } catch (error) { return error.message; }
    if (Date.now() >= Date.parse(task.startedAt) + task.bounds.minutes * 60000) return "Specialist task time bound reached";
    return undefined;
  };
  const notify = (ctx, text, kind = "info") => { if (ctx.hasUI) ctx.ui.notify(text, kind); };
  pi.events.on("rpiv:ask-user:prompt", payload => {
    if (taskID || !activeContext?.hasUI) return;
    const questions = payload.questions.map(item => item.question);
    activeContext.ui.setStatus("pentest-question", "WAITING FOR YOUR ANSWER");
    activeContext.ui.setWidget("pentest-question", ["WAITING FOR YOUR ANSWER", ...questions]);
    notify(activeContext, questions.join("\n"), "warning");
  });
  pi.events.on("rpiv:ask-user:blocked", payload => {
    if (taskID || payload.active || !activeContext?.hasUI) return;
    activeContext.ui.setStatus("pentest-question", undefined);
    activeContext.ui.setWidget("pentest-question", undefined);
  });
  const pause = (ctx, reason) => {
    budgetBlocked = true;
    // Tell the parent that a worker is already entering normal disposal, so it
    // does not SIGTERM Pi after print mode removes its signal handlers.
    let checkpointError;
    if (taskID) {
      try { atomicJSON(path.join(root, "specialists", taskID, "interrupted.json"), { reason, at: new Date().toISOString() }); }
      catch (error) { checkpointError = error; }
    }
    // Pi treats thrown hook errors as diagnostics; always abort even if writing
    // the checkpoint failed, before returning to the provider request hook.
    ctx.abort();
    if (taskID && !binding && !workerStopTimer) workerStopTimer = setTimeout(() => {
      if (process.platform !== "win32") process.kill(-process.pid, "SIGKILL");
      else process.exit(1);
    }, 5000).unref();
    for (const child of children) terminateChild(child);
    if (!taskID && fleet) void fleet.cancelAll("Orchestrator paused; reconcile partial work").catch(() => {});
    try {
      if (!taskID && controller) controller.apply("pause", { reason });
    } catch (error) { notify(ctx, `Request aborted; checkpoint write failed: ${error.message}`, "error"); }
    if (checkpointError) notify(ctx, `Request aborted; checkpoint write failed: ${checkpointError.message}`, "error");
    notify(ctx, reason, "warning");
  };
  const updateStatus = ctx => {
    if (!ctx.hasUI || !budget || !controller) return;
    const status = budget.status();
    const tasks = Object.values(controller.state.tasks);
    const running = tasks.filter(task => task.status === "running");
    const queued = tasks.filter(task => task.status === "pending");
    ctx.ui.setStatus("pentest", `${controller.state.phase} · ${controller.state.mode} · agents ${running.length} running/${queued.length} queued · $${status.spentUSD.toFixed(3)}${status.estimated ? " estimated" : "/$15"}`);
    if (!taskID && fleet) { ctx.ui.setWidget("pentest-agents", undefined); return; }
    if (!taskID) ctx.ui.setWidget("pentest-agents", [
      `Specialists: ${running.length} running · ${queued.length} queued · ${tasks.filter(task => task.status === "returned").length} awaiting verification`,
      ...[...running, ...tasks.filter(task => ["returned", "failed", "interrupted"].includes(task.status)).slice(-2)].slice(0, 4).map(task => {
        const progress = activity.get(task.id);
        return `${task.id} · ${task.status} · PID ${task.execution?.pid ?? (task.status === "running" ? "starting" : "not recorded")} · ${progress?.lastTool ?? task.execution?.lastTool ?? task.phase}`;
      }),
    ]);
  };
  const dispatch = async (id, ctx, signal, onUpdate, source = "delegate") => {
    if (fleetPackage && !fleet) throw new Error(fleetError ?? "Native Fleet is not ready; restart the harness after pi-subagents loads");
    const observer = { context: ctx, signal, source,
      onUpdate: record => onUpdate?.(response({ specialist: record.task, status: record.status, pid: record.pid, lastTool: record.lastTool, toolCalls: record.toolCalls, runId: record.runId })),
    };
    const result = fleet ? await fleet.start(id, observer) : await runSpecialist({ controller, root, runtimeDir, taskId: id, context: ctx, signal, children, source,
      onChange: record => { activity.set(record.task, record); updateStatus(ctx); },
      onUpdate: record => onUpdate?.(response({ specialist: record.task, status: record.status, pid: record.pid, lastTool: record.lastTool, toolCalls: record.toolCalls })),
    });
    if (["failed", "interrupted"].includes(result.status)) notify(ctx, `Specialist ${id} ${result.status}: ${result.reason}. Reconcile this task; other work can continue.`, "warning");
    updateStatus(ctx);
    return result;
  };

  pi.on("session_start", async (_event, ctx) => {
    controller = new Controller(root, ctx.cwd);
    budget = new Budget(launch, root);
    activeContext = ctx;
    if (taskID) {
      const task = controller.state.tasks[taskID];
      if (!task) throw new Error("Specialist has no registered task contract");
      if (binding) {
        childSessionId = ctx.sessionManager.getSessionId();
        childAdmission = claimAttempt({ root, launchFile: launch, controller, binding, sessionId: childSessionId, sessionFile: ctx.sessionManager.getSessionFile() });
        process.env.INTERCEPTOR_SESSION_ID = browserSession(root, taskID);
        pi.events.emit("subagent:acknowledge-extension", { id: "pentest-harnes-guard-v1" });
      }
      let stopping = false;
      watchdog = setInterval(() => {
        if (stopping || budgetBlocked) return;
        const reason = workerStopReason();
        if (!reason) return;
        stopping = true;
        // Aborting lets print mode dispose native MCP clients (up to 2.5s).
        // A second SIGTERM at 500ms would interrupt that disposal; the shared
        // pause path instead uses only a bounded 5s hard fallback.
        pause(ctx, reason);
        try {
          if (reservation) { budget.settle(reservation, {}); reservation = undefined; }
        } catch (error) { notify(ctx, `Specialist aborted; checkpoint write failed: ${error.message}`, "error"); }
      }, 500);
      watchdog.unref();
    }
    if (!taskID) {
      controller.recover();
      const directory = path.dirname(inflightFile);
      if (fs.existsSync(directory)) for (const name of fs.readdirSync(directory)) {
        const record = readJSON(path.join(directory, name));
        if (record.calls?.length) {
          controller.apply("pause", { reason: `Interrupted tool calls from PID ${record.pid}; inspect private/inflight and use reconcile_tools after checking target state` });
        }
      }
      notify(ctx, `Orchestrator ready with full tools and up to ${controller.maxParallelAgents} parallel workers. /pentest-agents · /pentest-status · /pentest-run · /pentest-pause.`);
      if (fleetPackage) {
        try {
          fleet = await createFleetBackend({ pi, context: ctx, controller, root, runtimeDir, packageRoot: fleetPackage,
            onChange: record => {
              const previous = activity.get(record.task)?.status;
              activity.set(record.task, record); updateStatus(ctx);
              if (previous !== record.status && ["returned", "failed", "interrupted"].includes(record.status) && controller.state.mode === "running") {
                pi.sendMessage({ customType: "pentest-worker-result", content: `Specialist ${record.task} ${record.status}. Native run: ${record.runId}; report: ${controller.state.tasks[record.task]?.output}. Read the actual report and underlying evidence; do not count worker completion as verification.`, display: true }, { triggerTurn: true, deliverAs: "followUp" });
              }
            } });
          notify(ctx, "Native Fleet connected: /subagents-fleet to select/click a worker and read its live transcript. /pentest-agents shows harness task IDs.");
        } catch (error) {
          fleetError = error.message;
          controller.apply("pause", { reason: `Native Fleet initialization failed: ${fleetError}` });
          notify(ctx, `Native Fleet unavailable: ${fleetError}`, "error");
        }
      }
    }
    updateStatus(ctx);
  });

  pi.on("before_agent_start", async (event, ctx) => {
    activeContext = ctx;
    refresh();
    const routing = taskID ? "" : "\n\nDELEGATE FIRST: After brief setup, launch concrete exploration/API/source-review assignments with pentest_delegate({tasks:[...]}), in parallel when independent. Keep full lead tools for quick checks, unblocking, evidence integration, reflection and hypotheses. Do not spend the whole turn performing a crawl or source audit yourself while useful workers are absent or queued. Offline reviews can start now even when labelled recon/auth; only their actual prerequisites matter. Do not create generic inventory audits. Do not ask for optional account credentials while independent workers or public/offline tasks remain; if credentials never arrive, give a precise limited report instead of an indefinite input wait.";
    return { systemPrompt: event.systemPrompt + (taskID ? `\n\nROLE: execution specialist ${taskID}. Execute the assigned contract and write evidence; do not act as the lead or delegate further.` : `\n\nROLE: lead orchestrator with full tools, including bash, browser and HTTP/MCP. Up to ${controller.maxParallelAgents} workers may run concurrently. Record scope from the user's stated targets and observed API origins; do not demand repeated confirmation or every account before available work. Ask real blocking questions visibly using ask_user_question, with Continue offline / Pause choices when appropriate. Continue independent work while other prerequisites are missing. Offline tasks accept network:false and may start ahead of the phase label.`) + routing, message: { customType: "pentest-checkpoint", content: `Current routing checkpoint (full details available through pentest_workflow status/read):\n${JSON.stringify(controller.promptSummary())}\n${taskID ? `Specialist ${taskID}: execute only the assigned contract; return artifacts without editing central state.` : "Delegate useful exploration/API/source-review work first; integrate evidence and keep going without routine approval."}`, display: false } };
  });
  pi.on("model_select", async (_event, ctx) => {
    budgetBlocked = false;
    updateStatus(ctx);
  });

  pi.on("before_provider_request", async (event, ctx) => {
    try {
      if (budgetBlocked) { ctx.abort(); return; }
      const workerStop = workerStopReason();
      if (workerStop) throw new Error(workerStop);
      if (reservation) budget.settle(reservation, {});
      if (!ctx.model) throw new Error("No Pi model selected");
      reservation = budget.reserve(ctx.model, event.payload);
    } catch (error) { pause(ctx, error.message); }
  });
  pi.on("message_end", async (event, ctx) => {
    if (event.message.role !== "assistant") return;
    if (reservation) {
      budget.settle(reservation, event.message);
      reservation = undefined;
    }
    if (!budget.status().estimated && budget.status().spentUSD >= 15) pause(ctx, "The $15 launch budget is exhausted. Session checkpointed; relaunch to start a fresh launch budget.");
    updateStatus(ctx);
  });

  pi.on("tool_call", async (event, ctx) => {
    const workerStop = workerStopReason();
    if (workerStop) { pause(ctx, workerStop); return { block: true, reason: workerStop }; }
    if (budgetBlocked || (!budget.status().estimated && budget.status().spentUSD >= 15)) return { block: true, reason: "Launch budget is paused/exhausted" };
    if (["subagents_enable", "subagent_launch", "acp_delegate", "acp_delegate_wait", "acp_delegate_cancel"].includes(event.toolName) ||
        (event.toolName === "subagent" && (taskID || !["status", "guide", "doctor", "list"].includes(event.input.action)))) return { block: true, reason: "Harness worker launches require an admitted task. Use pentest_delegate; use /subagents-fleet to inspect, steer, or stop existing workers." };
    if (taskID && (["pentest_delegate", "ask_user_question"].includes(event.toolName) || (event.toolName === "pentest_workflow" && !["status", "checklist"].includes(event.input.action)))) return { block: true, reason: "Specialists return artifacts and blockers; only the orchestrator changes workflow state or asks the user" };
    if (!taskID && isIdleShellPolling(event.toolName, event.input,
      Object.values(refresh().state.tasks).some(task => task.status === "running")))
      return { block: true, reason: "Workers finish asynchronously and wake the lead. End this turn instead of sleeping/polling, or use pentest_workflow wait for one worker if its result is required now." };
    if (!taskID && networkTool(event.toolName) && controller.state.verification) {
      try { controller.consumeVerificationTool(true); }
      catch (error) { return { block: true, reason: error.message }; }
    }
    if (taskID && networkTool(event.toolName) && refresh().state.tasks[taskID]?.network === false) return { block: true, reason: "This specialist has an offline, zero-request contract. Return local artifact evidence; do not make live requests." };
    // Native MCP routes direct and codemode-nested calls through this hook.
    if (event.toolName.startsWith("mcp__") || ["list_mcp_resources", "list_mcp_resource_templates", "read_mcp_resource"].includes(event.toolName)) {
      const state = refresh().state;
      if (taskID && !state.scope) return { block: true, reason: "Record engagement scope before specialist network calls" };
      if (state.scope?.expiresAt && Date.now() >= Date.parse(state.scope.expiresAt)) return { block: true, reason: "The authorized test window has expired" };
      const task = taskID ? state.tasks[taskID] : null;
      if (taskID) countRequest(task);
      await scheduleRequest(root, task?.bounds.rate ?? 50, ctx.signal);
    }
  });
  pi.on("tool_execution_start", async (event) => {
    inFlight.set(event.toolCallId, { id: event.toolCallId, name: event.toolName, startedAt: new Date().toISOString() });
    checkpointTools();
  });
  pi.on("tool_execution_end", async (event) => {
    inFlight.delete(event.toolCallId);
    checkpointTools();
    fs.appendFileSync(path.join(root, "tool-events.jsonl"), JSON.stringify({ at: new Date().toISOString(), task: taskID ?? null, id: event.toolCallId, tool: event.toolName, isError: event.isError }) + "\n", { mode: 0o600 });
  });
  pi.on("tool_result", async (event, ctx) => {
    if (taskID || event.toolName !== "ask_user_question") return;
    if (event.isError || event.details?.cancelled || event.details?.answers?.some(answer => answer.answer === "Pause")) {
      controller.apply("pause", { reason: "User paused/dismissed the question or the question UI was unavailable" });
      notify(ctx, "Paused. Reply in chat with how to continue, or use /pentest-run.", "warning");
    }
  });

  pi.on("agent_end", async (event, ctx) => {
    if (taskID || !controller) return;
    const last = [...event.messages].reverse().find(message => message.role === "assistant");
    if (["error", "aborted"].includes(last?.stopReason)) {
      if (last.stopReason === "error" && controller.state.mode === "running" && ++leadErrorRetries <= 2) {
        notify(ctx, `Lead model error; retrying from saved checkpoint (${leadErrorRetries}/2). Workers remain active.`, "warning");
        pi.sendMessage({ customType: "pentest-continue", content: `Recover from the previous lead model error. Read the saved checkpoint, reconcile any uncertain tool effects, keep existing workers running, and continue independent tasks. Retry ${leadErrorRetries}/2.`, display: false }, { triggerTurn: true, deliverAs: "followUp" });
        return;
      }
      if (controller.state.mode === "running") controller.apply("pause", { reason: `Lead ${last.stopReason} after ${leadErrorRetries} retries; inspect error and resume from checkpoint` });
      if (fleet) await fleet.cancelAll(`Lead ${last.stopReason}; reconcile any partial specialist work`);
      return;
    }
    leadErrorRetries = 0;
    if (!budgetBlocked && controller.state.mode === "running" && !ctx.hasPendingMessages()) {
      if (!dispatching) {
        dispatching = true;
        try {
          const ready = Object.values(controller.state.tasks).filter(task => task.status === "pending" && controller.taskReadiness(task).ready).slice(0, controller.maxParallelAgents);
          if (ready.length) await Promise.all(ready.map(task => dispatch(task.id, ctx, ctx.signal, undefined, "queued-dispatch")));
        } finally { dispatching = false; }
      }
      const tasks = Object.values(controller.state.tasks);
      if (fleet && tasks.some(task => task.status === "running") && !tasks.some(task => task.status === "returned")) return;
      if (controller.state.mode === "running") pi.sendMessage({ customType: "pentest-continue", content: `Continue from checkpoint revision ${controller.state.revision}, phase ${controller.state.phase}. Dispatch useful exploration/API/source-review workers in parallel before extended lead execution. Independent offline reviews can run ahead of the phase label; only future live work remains gated. Read reports, verify evidence, reflect and generate/test valuable hypotheses. Ask visibly with ask_user_question only when genuinely blocked.`, display: false }, { triggerTurn: true, deliverAs: "followUp" });
    }
  });
  pi.on("session_shutdown", async () => {
    clearInterval(watchdog);
    if (!taskID && fleet) await fleet.dispose();
    await Promise.allSettled([...children].map(child => terminateChild(child)));
    if (reservation && budget) { budget.settle(reservation, {}); reservation = undefined; }
  });
  pi.on("session_before_switch", async () => ({ cancel: true }));
  pi.on("session_before_fork", async () => ({ cancel: true }));

  pi.registerTool({
    name: "pentest_workflow", label: "Pentest workflow", description: "Workflow actions: status, checklist, scope, scope_update, run, pause, wait, verify_task, block_task, reconcile_task, reconcile_tools, hypothesis, resolve_hypothesis, role, authz_cell, asset, candidate, reflection, reopen, advance. Checklist read: {action:'checklist'}; atomic update of 1–50 checklist rows in ONE call: {action:'checklist',data:{items:[{id,status,reason,evidence,attempts,nextAction},...]}}. Each row keeps its own reason and evidence; a failure rolls back the entire batch. Minimal scope: {target:'example.com'}; optional origins, methods, authorization, window, actions and evidence. verify_task: {id,notes,evidence:[paths]}; considerations and hypotheses are optional. begin_verification/end_verification are optional replay contracts, never required for lead tools. Native wait with data.id waits for a specific worker. run can continue offline without scope. reconcile_task disposition:'retry' preserves partial output.",
    parameters: Type.Object({ action: Type.String(), data: Type.Optional(Type.Record(Type.String(), Type.Unknown())) }),
    async execute(_id, params, signal, onUpdate, ctx) {
      if (taskID && !["status", "checklist"].includes(params.action)) throw new Error("Only the lead orchestrator may change workflow state");
      if (taskID && params.action === "checklist" && params.data?.id) throw new Error("Specialists may only read the checklist");
      if (!taskID && params.action === "wait") {
        if (!fleet || !params.data?.id) throw new Error("Native wait requires an existing specialist task ID");
        const result = await fleet.run(params.data.id, { context: ctx, signal, onUpdate: record => onUpdate?.(response(record)) });
        return response({ ...controller.summary(), specialist: result });
      }
      let result = refresh().apply(params.action, params.data);
      if (!taskID && params.action === "pause" && fleet) await fleet.cancelAll(params.data?.reason ?? "Workflow paused");
      if (!taskID && params.action === "scope") controller.apply("run");
      if (!taskID && params.action === "scope") result = controller.summary();
      if (!taskID && params.action === "begin_verification") process.env.INTERCEPTOR_SESSION_ID = browserSession(root, controller.state.verification.taskId);
      updateStatus(activeContext);
      return response(result);
    },
  });

  pi.registerTool({
    name: "pentest_delegate", label: "Delegate specialists", description: "Launch multiple independent specialists in parallel with tasks:[{id,question,skill,network:false},...], or use task/taskId for one. taskIds dispatches queued jobs in parallel. Offline reviews can start ahead of the phase label; future live work and unmet dependencies queue. Each worker has its own PID, session, report and bounds; verify returned evidence separately.",
    parameters: Type.Object({ task: Type.Optional(taskSchema), taskId: Type.Optional(Type.String()), tasks: Type.Optional(Type.Array(taskSchema)), taskIds: Type.Optional(Type.Array(Type.String())) }),
    async execute(_id, params, signal, onUpdate, ctx) {
      if (taskID) throw new Error("Specialists may not nest delegation");
      if ([params.task, params.taskId, params.tasks, params.taskIds].filter(value => value !== undefined).length !== 1) throw new Error("Provide exactly one of task, taskId, tasks or taskIds");
      const tasks = params.tasks ? controller.createTasks(params.tasks) : params.task ? [controller.createTask(params.task)] : (params.taskIds ?? [params.taskId]).map(id => controller.state.tasks[id]);
      if (!tasks.length || tasks.some(task => !task)) throw new Error("Provide existing task IDs or a nonempty tasks array");
      const results = await Promise.all(tasks.map(task => dispatch(task.id, ctx, signal, onUpdate)));
      return response(params.tasks || params.taskIds ? { specialists: results } : results[0]);
    },
  });

  pi.registerTool({
    name: "pentest_http", label: "Scoped captured request", description: "One scoped HTTP(S) request with shared rate scheduling and private request/response evidence. Does not follow redirects or retry. Prefer configured Burp/Caido MCP for named replay tabs. Shell/browser traffic and requests issued internally by MCP server operations use those tools' own limits.",
    parameters: Type.Object({ url: Type.String(), method: Type.Optional(Type.String()), headers: Type.Optional(Type.Record(Type.String(), Type.String())), body: Type.Optional(Type.String()), rate: Type.Optional(Type.Number({ minimum: .01, maximum: 50 })) }),
    async execute(_id, params, signal, _onUpdate, ctx) {
      const method = (params.method ?? "GET").toUpperCase();
      const state = refresh().state;
      controller.checkScope(params.url, method);
      const task = taskID ? state.tasks[taskID] : state.verification ? controller.verificationTask().task : null;
      if (task?.network === false) throw new Error("Offline tasks cannot make HTTP requests");
      if (task && !task.targets.some(target => new URL(target.url).origin === new URL(params.url).origin && new URL(target.url).pathname === new URL(params.url).pathname && target.method.toUpperCase() === method)) throw new Error("Request is outside this specialist's target/method contract");
      if (taskID) countRequest(task);
      await scheduleRequest(root, Math.min(params.rate ?? 50, task?.bounds.rate ?? 50), signal);
      controller.checkScope(params.url, method);
      const name = `${Date.now()}-${crypto.randomUUID()}`;
      const directory = path.join(ctx.cwd, "private", "evidence", taskID ?? "orchestrator");
      const requestPath = path.join(directory, `${name}.request.json`);
      const responsePath = path.join(directory, `${name}.response.txt`);
      atomicJSON(requestPath, { url: params.url, method, headers: params.headers ?? {}, body: params.body ?? null, at: new Date().toISOString() });
      const timeout = AbortSignal.timeout(30000);
      const combined = signal ? AbortSignal.any([signal, timeout]) : timeout;
      const result = await fetch(params.url, { method, headers: params.headers, body: params.body, redirect: "manual", signal: combined });
      const reader = result.body?.getReader();
      const chunks = [];
      let bytes = 0;
      if (reader) for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        bytes += value.length;
        if (bytes > 8 * 1024 * 1024) { await reader.cancel(); throw new Error("Response exceeds bounded 8 MiB evidence capture; use the proxy/static collection workflow"); }
        chunks.push(value);
      }
      const body = Buffer.concat(chunks);
      privateText(responsePath, `HTTP ${result.status} ${result.statusText}\n${[...result.headers].map(([key, value]) => `${key}: ${value}`).join("\n")}\n\n`);
      fs.appendFileSync(responsePath, body);
      if (result.status === 429 || (result.status >= 500 && !taskID)) {
        if (!taskID) controller.apply("pause", { reason: `HTTP ${result.status}; respect Retry-After and skill stop rules before continuing` });
      }
      return response({ status: result.status, bytes, request: path.relative(ctx.cwd, requestPath), response: path.relative(ctx.cwd, responsePath), retryAfter: result.headers.get("retry-after"), redirected: result.status >= 300 && result.status < 400 });
    },
  });

  pi.registerTool({
    name: "pentest_ask", label: "Prepare engagement question", description: "Compatibility helper: prepares a visible ask_user_question with Continue offline / Pause choices. Follow its returned instruction to show that tool immediately. Prefer ask_user_question directly; ask only for real blockers.",
    parameters: Type.Object({ question: Type.String(), reason: Type.String() }),
    async execute(_id, params, _signal, _onUpdate, ctx) {
      if (taskID) throw new Error("Record the blocker in your specialist report for the lead to resolve");
      notify(ctx, params.question, "warning");
      if (ctx.hasUI && pi.getAllTools().some(tool => tool.name === "ask_user_question")) {
        return response({ needsUser: true, questions: [{ question: params.question, header: "Continue?", options: [
          { label: "Continue offline", description: "Continue available local analysis and reflection while live prerequisites remain unresolved." },
          { label: "Pause", description: "Stop here until I provide more information." },
        ] }], instruction: "Call ask_user_question now with these questions. Do not pause or wait silently before displaying the dialog." });
      }
      controller.apply("pause", { reason: params.reason });
      pi.sendMessage({ customType: "pentest-question", content: `USER INPUT NEEDED: ${params.question}\n${params.reason}\nReply in chat to continue. For the question dialog install npm:@juicesharp/rpiv-ask-user-question and restart Pi.`, display: true });
      return response({ needsUser: true, question: params.question, reason: params.reason, instruction: "Ask the question visibly in your next message and stop. Do not wait inside a hidden input dialog." });
    },
  });

  pi.registerCommand("pentest-status", { description: "Show saved phase, task/checklist coverage, and this launch's shared budget", handler: async (_args, ctx) => notify(ctx, JSON.stringify({ ...refresh().summary(), budget: budget.status() }, null, 2)) });
  pi.registerCommand("pentest-integrations", { description: "Show active Fleet, context-compression, todo, and spending integrations without a model call", handler: async (_args, ctx) => {
    const names = new Set(pi.getAllTools().map(tool => tool.name));
    notify(ctx, "PENTEST_INTEGRATIONS_JSON:" + JSON.stringify({ nativeFleet: Boolean(fleet), fleetError: fleetError ?? null, todo: names.has("todo"),
      contextTools: ["compress", "decompress", "search_context", "acp_status", "acp_cache"].filter(name => names.has(name)),
      compressionState: names.has("compress") ? "tools-ready" : pi.getCommands().some(command => command.name === "acp") ? "loaded-awaiting-provider-hook" : "not-loaded",
      estimatedSpending: budget.status().estimated, askUserQuestion: names.has("ask_user_question"), maxParallelAgents: controller.maxParallelAgents, leadTools: "full", workerInspector: fleet ? "/subagents-fleet" : null }));
  } });
  pi.registerCommand("pentest-agents", { description: "Show real specialist PIDs, queue states, progress, and report paths", handler: async (_args, ctx) => {
    notify(ctx, JSON.stringify(Object.values(refresh().state.tasks).map(task => ({ id: task.id, phase: task.phase, status: task.status, pid: task.execution?.pid, runId: task.execution?.runId, progress: activity.get(task.id)?.lastTool ?? task.execution?.lastTool, output: task.output, readiness: task.status === "pending" ? controller.taskReadiness(task) : undefined })), null, 2) + (fleet ? "\nOpen /subagents-fleet, then click/select a worker row to view its live transcript. Enter opens the child inspector when available." : ""));
  } });
  pi.registerCommand("pentest-pause", { description: "Pause automatic continuation and interrupt active work", handler: async (_args, ctx) => {
    ctx.abort();
    const stopped = [...children].map(child => terminateChild(child));
    if (fleet) stopped.push(fleet.cancelAll("Paused by user; reconcile partial work"));
    try { if (!taskID) controller.apply("pause", { reason: "Paused by user" }); }
    catch (error) { notify(ctx, `Work aborted; checkpoint write failed: ${error.message}`, "error"); }
    await Promise.allSettled(stopped);
    notify(ctx, "Paused; reconcile interrupted mutations before retrying.");
  } });
  pi.registerCommand("pentest-run", { description: "Continue authorized work from the persistent checklist", handler: async (_args, ctx) => {
    try {
      if (taskID) throw new Error("Only the orchestrator resumes engagements");
      budgetBlocked = false;
      controller.apply("run");
      if (controller.state.mode !== "running") return;
      pi.sendUserMessage("Continue the authorized engagement from the persistent checkpoint. Follow the mandatory reflection and hypothesis loops until the checklist has evidence-backed dispositions or a genuine blocker needs the user.");
    } catch (error) { notify(ctx, error.message, "warning"); }
  } });
  pi.registerCommand("pentest-pricing", { description: "Confirm USD/million-token prices: INPUT OUTPUT CACHE_READ CACHE_WRITE", handler: async (args, ctx) => {
    try {
      if (!ctx.model) throw new Error("Select a model first");
      budget.setPricing(ctx.model, args.trim().split(/\s+/).filter(Boolean).map(Number));
      budgetBlocked = false;
      notify(ctx, `Pricing saved for ${ctx.model.provider}/${ctx.model.id}. ${budget.status().estimated ? "Spending is estimated; there is no hard monetary cutoff." : "Strict launch budget remains $15."}`);
    } catch (error) { notify(ctx, error.message, "error"); }
  } });
  // The launcher passes this flag. A skipped or failed extension load makes
  // Pi reject the unknown flag rather than silently running without gates.
  pi.registerFlag("pentest-runtime", { description: "Require the persistent pentest workflow extension", type: "boolean", default: false });
  // SDK children do not load CLI builtin factories automatically. Install the
  // official native implementations after this guard's admission hook; no
  // replacement adapter and no restrictive tools allowlist are needed.
  if (binding) { createMcpExtension()(pi); createToolSearchExtension()(pi); }
}
