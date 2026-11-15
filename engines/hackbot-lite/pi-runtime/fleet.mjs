import fs from "node:fs";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";
import { artifact, atomicJSON, readJSON } from "./storage.mjs";

const REQUEST = "subagents:rpc:v1:request";
const REPLY = "subagents:rpc:v1:reply:";
const STARTED = "subagent:async-started";
const HINTS = [STARTED, "subagent:async-complete", "subagent:child-status", "subagent:process-terminal"];
const LEAF = "pentest-harness-leaf";
const GUARD = "pentest-harnes-guard-v1";
const TERMINAL = new Set(["complete", "failed", "partial", "paused", "stopped", "rejected"]);
const positivePID = value => Number.isSafeInteger(value) && value > 0;
const text = value => typeof value === "string" && value.trim().length > 0;
const inside = (directory, file) => path.resolve(file).startsWith(path.resolve(directory) + path.sep);

async function publicAPI(packageRoot, name) {
  // Use only exported APIs, not src internals (including when the host is bundled).
  const require = createRequire(packageRoot ? path.join(path.resolve(packageRoot), "package.json") : import.meta.url);
  const file = packageRoot ? path.join(packageRoot, "bundle", `${name}.js`) : require.resolve(`pi-subagents/${name}`);
  return import(pathToFileURL(path.resolve(file)).href);
}

function sessionIdentity(context) {
  const id = context?.sessionManager?.getSessionId();
  const file = context?.sessionManager?.getSessionFile() ?? null;
  if (!text(id)) throw new Error("Native Fleet requires an active Pi session ID");
  // RPC spawn carries getSessionId() as asyncCtx.parentSessionId, the required-
  // extension registry key. Async ownership separately uses file ?? UUID.
  return { id, file, owner: file ?? id };
}

function sessionHeader(file) {
  const fd = fs.openSync(file, "r");
  try {
    const bytes = Buffer.alloc(8192);
    const count = fs.readSync(fd, bytes, 0, bytes.length, 0);
    const header = JSON.parse(bytes.toString("utf8", 0, count).split("\n")[0]);
    return header.type === "session" && text(header.id) ? header.id : undefined;
  } finally { fs.closeSync(fd); }
}

// Mirrors the public ProcessTerminal shape. Child-step proofs and lifecycle events
// are deliberately insufficient: only the durable run-level proof can close work.
function observedProof(proof, runId, instanceId) {
  if (proof?.version !== 1 || proof.state !== "observed" || proof.runId !== runId || proof.childIndex !== undefined ||
      !text(proof.runnerProcessInstanceId) || (instanceId && proof.runnerProcessInstanceId !== instanceId) ||
      !Number.isFinite(proof.observedAt) || !Array.isArray(proof.instances)) return false;
  const valid = item => text(item?.processInstanceId) && Number.isFinite(item.closeObservedAt) &&
    (item.exitCode === null || Number.isSafeInteger(item.exitCode)) && (item.signal === null || text(item.signal));
  const runner = proof.instances.find(item => item.kind === "runner");
  return valid(runner) && runner.processInstanceId === proof.runnerProcessInstanceId && proof.instances.every(item =>
    valid(item) && (item.kind === "runner" || (item.kind === "pi-writer" && item.processTree?.state === "observed")));
}

function promptFor(task, scratch, workspace) {
  const reportPath = path.resolve(workspace, task.output);
  return `You are specialist ${task.id}. Read the assigned SKILL.md and relevant references first. Execute only this contract; do not perform parent orchestration, delegate further, or modify central ledgers/checkpoints. Keep secrets private and use the pinned actor/session. ${task.network === false ? "This is OFFLINE work: no target requests, HTTP replay, live browser automation, or operational MCP calls. Analyze only supplied local artifacts." : "Use only the scoped target URLs/methods and request bounds; stop on the skill's challenge/error conditions."} Your scratch is ${scratch}. Write a substantive initial report to ${task.output} early, using the exact absolute path ${reportPath}, then update it as evidence accumulates so a deadline preserves honest partial coverage; its parent directory already exists. Pi also supplies a separate private final-response file. The task report must be written to ${reportPath}; verify that file exists before finishing. Keep underlying evidence in your private scratch. Return a concise summary, report path, underlying evidence paths, verdicts, coverage, restoration/final state, unresolved blockers and suggested follow-up as your final answer. Do not claim a tool worked without observing it.\nContract:\n${JSON.stringify(task, null, 2)}`;
}

function recoverNativeReport(entry, workspace) {
  const destination = path.resolve(workspace, entry.task.output);
  if (fs.existsSync(destination)) return;
  const source = entry.record.responseFile;
  const session = entry.record.sessionFile;
  if (!text(source) || !text(session) || !inside(path.join(workspace, "private", "native-results"), source)) return;
  if (!fs.lstatSync(source, { throwIfNoEntry: false })?.isFile()) return;
  const evidence = artifact(workspace, path.relative(workspace, source));
  if (evidence.bytes < 512) return;
  if (fs.statSync(session).size > 64 * 1024 * 1024) return;
  const authored = fs.readFileSync(session, "utf8").split("\n").some(line => {
    if (!line.trim()) return false;
    let event;
    try { event = JSON.parse(line); } catch { return false; }
    return event.message?.role === "assistant" && event.message.content?.some(part =>
      part.type === "toolCall" && part.name === "write" && typeof part.arguments?.path === "string" &&
      path.resolve(workspace, part.arguments.path) === source);
  });
  if (authored) fs.copyFileSync(source, destination, fs.constants.COPYFILE_EXCL);
}

/** Native pi-subagents adapter. admission and timing are local-test seams. */
export async function createFleetBackend({ pi, context, controller, root = controller.root, runtimeDir, packageRoot,
  onChange = () => {}, onUpdate = () => {}, admission, launchFile = process.env.PENTEST_HARNES_LAUNCH, timing = {} }) {
  if (!pi?.events?.on || !pi.events.emit) throw new Error("Native Fleet requires the Pi event bus");
  const identity = sessionIdentity(context);
  const pollMs = timing.pollMs ?? 100;
  const rpcMs = timing.rpcMs ?? 30000;
  const proofMs = timing.proofMs ?? 15000;
  if (![pollMs, rpcMs, proofMs].every(value => Number.isFinite(value) && value > 0)) throw new Error("Invalid native Fleet timing bounds");
  let requiredAPI, ceilingAPI;
  try {
    [requiredAPI, ceilingAPI] = await Promise.all([
      publicAPI(packageRoot, "required-child-extensions"), publicAPI(packageRoot, "capability-ceiling"),
    ]);
  } catch (error) { throw new Error(`Native Fleet unavailable: install pi-subagents with required-child-extensions and capability-ceiling APIs (${error.message})`); }
  admission ??= await import("./admission.mjs");
  if (!["taskDigest", "createAttempt", "bindingsFor"].every(key => typeof admission[key] === "function")) throw new Error("Native Fleet requires the task admission API");

  const registrations = [];
  try {
    registrations.push(requiredAPI.registerRequiredChildExtensions({ sessionId: identity.id,
      extensions: [{ id: GUARD, path: path.resolve(runtimeDir, "extension.ts") }], requireForAllRunners: true }));
    registrations.push(ceilingAPI.registerSubagentCapabilityCeiling({ sessionId: identity.id, source: "pentest-harnes",
      ceiling: { allowedAgents: [LEAF] } }));
  } catch (error) {
    for (const registration of registrations.reverse()) registration.dispose();
    throw new Error(`Native Fleet guard registration failed: ${error.message}`);
  }

  const active = new Map();
  const pendingRPC = new Set();
  let closing = false, disposed = false, teardownFlight;

  function request(method, params, requestId = randomUUID()) {
    if (disposed) return Promise.reject(new Error("Native Fleet is disposed"));
    return new Promise((resolve, reject) => {
      let timer, unsubscribe;
      const finish = (error, data) => {
        clearTimeout(timer);
        unsubscribe?.();
        pendingRPC.delete(cancel);
        if (error) reject(error); else resolve(data);
      };
      const cancel = () => finish(new Error("Native Fleet RPC disposed"));
      unsubscribe = pi.events.on(REPLY + requestId, reply => {
        if (reply?.version !== 1 || reply.requestId !== requestId || reply.method !== method) return;
        if (reply.success === true) finish(null, reply.data);
        else if (reply.success === false) finish(Object.assign(new Error(`Native Fleet ${method}: ${reply.error?.message ?? "RPC failed"}`),
          { code: reply.error?.code, definitiveReply: true }));
      });
      pendingRPC.add(cancel);
      timer = setTimeout(() => finish(new Error(`Native Fleet ${method} RPC timed out; check that pi-subagents is loaded and session-ready`)), rpcMs);
      try { pi.events.emit(REQUEST, { version: 1, requestId, method, ...(params === undefined ? {} : { params }) }); }
      catch (error) { finish(error); }
    });
  }

  function publish(entry) {
    const record = entry.record;
    atomicJSON(entry.processFile, record);
    fs.appendFileSync(path.join(entry.scratch, "process-events.jsonl"), JSON.stringify({ ...record, at: new Date().toISOString() }) + "\n", { mode: 0o600 });
    // UI callbacks cannot invalidate execution bookkeeping or trigger more workers.
    for (const callback of [onChange, onUpdate, entry.onUpdate]) {
      try { callback?.({ ...record }); } catch { /* Dashboard teardown. */ }
    }
  }

  function stop(entry, reason) {
    clearTimeout(entry.deadlineTimer);
    entry.stopReason ??= reason;
    entry.stopAt ??= Date.now();
    entry.record.status = "stopping";
    if (entry.record.runId && !entry.stopFlight) {
      entry.stopFlight = request("stop", { runId: entry.record.runId, ...(entry.record.asyncDir ? { dir: entry.record.asyncDir } : {}) })
        .then(receipt => { entry.record.stopReceipt = receipt; })
        .catch(error => { entry.record.stopError = error.message; })
        .finally(() => { entry.wake?.(); });
    }
    entry.wake?.();
  }

  function adopt(entry, runId, asyncDir, pid) {
    const record = entry.record;
    if (!text(runId)) throw new Error("Native Fleet spawn did not supply a runId");
    if (record.runId && record.runId !== runId) throw new Error("Native Fleet runId changed during spawn");
    record.runId = runId;
    if (asyncDir !== undefined) {
      if (!text(asyncDir) || !path.isAbsolute(asyncDir)) throw new Error("Native Fleet asyncDir must be an absolute path");
      if (record.asyncDir && record.asyncDir !== asyncDir) throw new Error("Native Fleet asyncDir changed during spawn");
      record.asyncDir = asyncDir;
    }
    if (pid !== undefined) {
      if (!positivePID(pid) || pid === process.pid) throw new Error("Native Fleet requires an actual runner PID distinct from the parent");
      if (record.pid && record.pid !== pid) throw new Error("Native Fleet runner PID changed");
      if (!record.pid) {
        controller.recordTaskProcess(entry.task.id, { pid, parentPid: process.pid, startedAt: record.startedAt, model: record.model, sessionDir: record.sessionDir });
        record.pid = pid;
      }
    }
    if (entry.stopReason) stop(entry, entry.stopReason);
  }

  function listen(entry) {
    const buffered = [];
    const acceptStart = event => {
      if (event.mode !== undefined && event.mode !== "single") throw new Error("Native Fleet expected one native leaf");
      if (event.agent !== undefined && event.agent !== LEAF) throw new Error("Native Fleet started an unexpected agent");
      if (event.sessionId !== undefined && event.sessionId !== identity.owner) throw new Error("Native Fleet async owner session mismatch");
      adopt(entry, event.id, event.asyncDir, event.pid);
      publish(entry);
    };
    entry.replayStarts = () => {
      for (const event of buffered.splice(0)) if (event.id === entry.record.runId) acceptStart(event);
    };
    entry.unsubscribers = HINTS.map(name => pi.events.on(name, event => {
      if (!event || typeof event !== "object") return;
      try {
        if (name === STARTED) {
          if (event.toolCallId === entry.toolCallId || event.requestId === entry.attempt.requestId || event.id === entry.record.runId) acceptStart(event);
          else if (!entry.record.runId && buffered.length < 64) buffered.push(event);
        }
        // Events are wake-up hints only. Never consume their terminal/progress data.
        if (event.id === entry.record.runId || event.runId === entry.record.runId) entry.wake?.();
      } catch (error) { entry.failure ??= error.message; stop(entry, "Native Fleet identity/provenance failure"); }
    }));
  }

  function readNative(entry) {
    const record = entry.record;
    if (!record.asyncDir) return {};
    const status = readJSON(path.join(record.asyncDir, "status.json"), null);
    if (!status) return {};
    if (status.runId !== record.runId || status.mode !== "single" ||
        (status.toolCallId !== undefined && status.toolCallId !== entry.toolCallId) ||
        (status.sessionId !== undefined && status.sessionId !== identity.owner) ||
        (status.steps?.length > 1) || (status.steps?.[0]?.agent !== undefined && status.steps[0].agent !== LEAF)) {
      throw new Error("Native Fleet authoritative status identity mismatch");
    }
    if (!entry.instanceId && text(status.processTerminal?.runnerProcessInstanceId)) entry.instanceId = status.processTerminal.runnerProcessInstanceId;
    const proof = readJSON(path.join(record.asyncDir, "process-terminal.json"), null) ?? status.processTerminal;
    record.processTerminal = proof;
    const terminal = TERMINAL.has(status.state);
    if (terminal) entry.terminalAt ??= Date.now();
    return { status, proof, terminal, proven: observedProof(proof, record.runId, entry.instanceId) };
  }

  function progress(entry, status) {
    const record = entry.record;
    adopt(entry, status.runId, record.asyncDir, status.pid);
    const step = status.steps?.[0] ?? {};
    const attempt = readJSON(entry.attemptFile, null);
    if (!attempt || ["taskId", "attemptId", "contractDigest", "parentSessionId", "parentPid", "launchFile", "requestId", "expiresAt"].some(key => attempt[key] !== entry.attempt[key])) throw new Error("Native Fleet admission record changed or disappeared");
    const claim = attempt?.claim;
    if (claim) {
      if (!positivePID(claim.pid) || claim.pid === process.pid || (record.pid && claim.pid !== record.pid)) throw new Error("Native Fleet guard claim PID mismatch");
      adopt(entry, status.runId, record.asyncDir, claim.pid);
      if (!text(claim.sessionId) || claim.sessionId === identity.id || !text(claim.sessionFile)) throw new Error("Native Fleet guard claim lacks a distinct child session");
      record.guardClaim = { pid: claim.pid, sessionId: claim.sessionId, sessionFile: claim.sessionFile, at: claim.at };
    }
    const file = claim?.sessionFile ?? step.sessionFile ?? status.sessionFile;
    if (file !== undefined) {
      if (!text(file) || !path.isAbsolute(file) || !inside(record.sessionDir, file) || file === identity.file) throw new Error("Native Fleet child session escaped its isolated sessionDir");
      if (record.sessionFile && record.sessionFile !== file) throw new Error("Native Fleet child sessionFile changed");
      if (claim?.sessionFile && step.sessionFile && claim.sessionFile !== step.sessionFile) throw new Error("Native Fleet guard/status child session mismatch");
      record.sessionFile = file;
      const headerId = fs.existsSync(file) ? sessionHeader(file) : undefined;
      if (headerId && claim?.sessionId && headerId !== claim.sessionId) throw new Error("Native Fleet child session header/claim mismatch");
      const sessionId = claim?.sessionId ?? headerId;
      if (sessionId) {
        if (sessionId === identity.id || (record.sessionId && record.sessionId !== sessionId)) throw new Error("Native Fleet child session ID mismatch");
        record.sessionId = sessionId;
      }
    }
    Object.assign(record, {
      nativeState: status.state, lastTool: step.currentTool ?? status.currentTool ?? step.recentTools?.at(-1)?.tool ?? record.lastTool,
      toolCalls: step.toolCount ?? status.toolCount ?? record.toolCalls,
      transcriptPath: step.transcriptPath ?? record.transcriptPath ?? record.sessionFile,
      tokens: status.totalTokens ?? step.tokens, totalCost: status.totalCost ?? step.totalCost,
      contextLimit: step.contextLimit, resolvedModel: step.resolvedModel, timedOut: Boolean(status.timedOut || step.timedOut),
    });
  }

  function finish(entry, status, reason, native = {}) {
    const record = entry.record;
    const runner = native.proof?.instances?.find(item => item.kind === "runner");
    const metadata = { exitCode: runner?.exitCode ?? native.status?.steps?.[0]?.exitCode ?? null,
      finishedAt: new Date().toISOString(), runId: record.runId, asyncDir: record.asyncDir,
      sessionId: record.sessionId, sessionFile: record.sessionFile, transcriptPath: record.transcriptPath,
      toolCalls: record.toolCalls, lastTool: record.lastTool, tokens: record.tokens, totalCost: record.totalCost,
      contextLimit: record.contextLimit, responseFile: record.responseFile, processTerminal: native.proof };
    controller.returnedTask(entry.task.id, status, reason, metadata);
    Object.assign(record, metadata, { status, reason });
    delete record.teardownPending;
    publish(entry);
    entry.finished = true;
    clearTimeout(entry.deadlineTimer);
    entry.unsubscribers?.forEach(unsubscribe => unsubscribe?.());
    entry.signal?.removeEventListener("abort", entry.abort);
    active.delete(entry.task.id);
    return result(entry, status, reason);
  }

  function result(entry, status, reason, teardownPending = false) {
    return { task: entry.task.id, status, reason, output: entry.task.output, scratch: entry.scratch,
      execution: controller.state.tasks[entry.task.id].execution, runId: entry.record.runId, asyncDir: entry.record.asyncDir,
      pid: entry.record.pid, attemptId: entry.record.attemptId, requestId: entry.record.requestId,
      sessionDir: entry.record.sessionDir, sessionFile: entry.record.sessionFile, transcriptPath: entry.record.transcriptPath,
      responseFile: entry.record.responseFile,
      ...(teardownPending ? { teardownPending: true } : {}) };
  }

  async function monitor(entry) {
    let previous;
    for (;;) {
      if (!entry.stopReason && Date.now() >= entry.deadline) stop(entry, "Task time bound reached; reconcile partial work");
      let native = {};
      try {
        native = readNative(entry);
        if (native.status) progress(entry, native.status);
      }
      catch (error) { entry.failure ??= error.message; stop(entry, "Native Fleet evidence/provenance failure"); }
      if (native.terminal && native.proven) {
        const record = entry.record;
        let status = "failed", reason = entry.failure ?? native.status.error ?? `Native specialist ended ${native.status.state}`;
        if (!entry.failure && (entry.stopReason || record.timedOut || native.status.stopped || ["stopped", "paused"].includes(native.status.state))) {
          status = "interrupted"; reason = entry.stopReason ?? "Native specialist stopped; reconcile partial work";
        } else if (!entry.failure && native.status.state === "complete") {
          try {
            if (!record.pid || !record.sessionId || !record.sessionFile || !record.guardClaim) throw new Error("Native specialist lacks actual PID, child session, or mandatory guard claim");
            if (native.status.steps?.[0]?.exitCode && native.status.steps[0].exitCode !== 0) throw new Error("Native specialist exited unsuccessfully");
            if (native.proof.instances.some(item => item.exitCode !== 0 || item.signal !== null)) throw new Error("Native specialist process proof records an unsuccessful exit");
            recoverNativeReport(entry, controller.workspace);
            artifact(controller.workspace, entry.task.output);
            status = "returned"; reason = "Awaiting lead evidence verification";
          } catch (error) { reason = error.message; }
        }
        return finish(entry, status, reason, native);
      }
      const serialized = JSON.stringify(entry.record);
      if (serialized !== previous) { publish(entry); previous = serialized; }
      if ((entry.stopAt && Date.now() - entry.stopAt >= proofMs) || (entry.terminalAt && Date.now() - entry.terminalAt >= proofMs)) {
        stop(entry, entry.stopReason ?? "Native terminal status lacks observed run-level process proof");
        const reason = `${entry.failure ?? entry.stopReason}; teardown is unproven. Keep the guard registered and reconcile native run ${entry.record.runId ?? entry.attempt.requestId}.`;
        entry.record.reason = reason;
        entry.record.teardownPending = true;
        publish(entry);
        // Retain ownership/listeners/registrations: neither a timeout nor a stop
        // acknowledgement permits returnedTask or silently forgetting a worker.
        return result(entry, "failed", reason, true);
      }
      await new Promise(resolve => {
        const timer = setTimeout(wake, pollMs);
        function wake() { clearTimeout(timer); entry.wake = undefined; resolve(); }
        entry.wake = wake;
      });
    }
  }

  async function execute(entry) {
    const task = entry.task;
    try {
      controller.startTask(task.id);
      entry.record.startedAt = task.startedAt;
      entry.deadline = Date.parse(task.startedAt) + task.bounds.minutes * 60000;
      const checkDeadline = () => {
        if (Date.now() >= entry.deadline) stop(entry, "Task time bound reached; reconcile partial work");
        else entry.deadlineTimer = setTimeout(checkDeadline, Math.min(2147483647, entry.deadline - Date.now()));
      };
      checkDeadline();
      if (!text(launchFile)) throw new Error("Native Fleet requires PENTEST_HARNES_LAUNCH for shared budget enforcement");
      const ping = await request("ping");
      if (ping?.version !== 1 || ping.session?.sessionId !== identity.id || (ping.session.sessionFile ?? null) !== identity.file ||
          ping.capabilities?.asyncSpawn !== true || ping.capabilities?.processTerminalProof?.version !== 1 || !ping.methods?.includes("stop")) {
        throw new Error("Native Fleet is not session-ready or lacks async spawn/stop/process-terminal proof support");
      }
      entry.attempt = await admission.createAttempt({ root, task, parentSessionId: identity.id, parentPid: process.pid, launchFile });
      if (entry.attempt.taskId !== task.id || entry.attempt.parentSessionId !== identity.id || entry.attempt.parentPid !== process.pid ||
          entry.attempt.contractDigest !== admission.taskDigest(task) || !text(entry.attempt.attemptId) ||
          !/^[a-zA-Z0-9_-]+$/.test(entry.attempt.attemptId) || !text(entry.attempt.requestId) || /[\r\n]/.test(entry.attempt.requestId)) {
        throw new Error("Native Fleet received an invalid admission attempt");
      }
      const extensionBindings = admission.bindingsFor(entry.attempt);
      const binding = extensionBindings?.["pentest-harnes/1"];
      if (!binding || ["taskId", "attemptId", "contractDigest", "parentSessionId"].some(key => binding[key] !== entry.attempt[key])) throw new Error("Native Fleet requires per-attempt guard bindings");
      entry.attemptFile = path.join(entry.scratch, "attempts", `${entry.attempt.attemptId}.json`);
      entry.toolCallId = `rpc-spawn-${entry.attempt.requestId}`;
      Object.assign(entry.record, { attemptId: entry.attempt.attemptId, requestId: entry.attempt.requestId, toolCallId: entry.toolCallId,
        contractDigest: entry.attempt.contractDigest,
        responseFile: path.resolve(controller.workspace, "private", "native-results", `${task.id}-${entry.attempt.attemptId}.md`) });
      atomicJSON(path.join(entry.scratch, "contract.json"), task);
      fs.mkdirSync(entry.record.sessionDir, { recursive: true, mode: 0o700 });
      fs.mkdirSync(path.dirname(entry.record.responseFile), { recursive: true, mode: 0o700 });
      listen(entry);
      publish(entry);
      // A pre-aborted call has no native run to stop and creates no worker.
      if (entry.stopReason || entry.signal?.aborted || Date.now() >= entry.deadline) return finish(entry, "interrupted", entry.stopReason ?? "Specialist interrupted before native spawn");
      entry.spawnSent = true;
      const receipt = await request("spawn", { agent: LEAF, task: promptFor(task, entry.scratch, controller.workspace), async: true, context: "fresh",
        cwd: controller.workspace, sessionDir: entry.record.sessionDir, timeoutMs: Math.max(1, Math.floor(entry.deadline - Date.now())),
        extensionBindings, output: entry.record.responseFile, outputMode: "file-only", artifacts: true,
        model: entry.record.model }, entry.attempt.requestId);
      const details = receipt?.details;
      if (!details || details.mode !== "single" || !text(details.asyncDir) || (details.asyncId && details.asyncId !== details.runId)) throw new Error("Native Fleet returned an invalid single-run receipt");
      adopt(entry, details.runId, details.asyncDir);
      entry.replayStarts();
      const native = readNative(entry);
      if (native.status) progress(entry, native.status);
      entry.record.status = entry.stopReason ? "stopping" : "running";
      publish(entry);
      entry.resolveReceipt(result(entry, entry.record.status, "Native launch receipt bound; completion is published through onChange"));
      return await monitor(entry);
    } catch (error) {
      entry.failure ??= error.message;
      // execution_failed can be a *runner startup* error after OS launch. Only
      // protocol/admission rejections are known to precede the executor.
      const rejectedBeforeExecution = error.definitiveReply && ["invalid_request", "invalid_params", "unsupported_version", "unsupported_method", "no_active_session"].includes(error.code);
      if (!entry.spawnSent || (rejectedBeforeExecution && !entry.record.runId)) return finish(entry, "failed", entry.failure);
      // A missing/malformed receipt can follow a successful launch. Never retry
      // spawn, and retain the start listener to discover and stop that exact run.
      stop(entry, "Native spawn/control failure; reconcile partial work");
      return await monitor(entry);
    }
  }

  function dispatch(taskId, options, completionWait) {
    const task = controller.state.tasks[taskId];
    const prelaunchError = error => {
      if (completionWait) return Promise.reject(error);
      const failed = { task: taskId, status: "failed", reason: error.message, output: task?.output, execution: task?.execution };
      try { onChange(failed); } catch { /* UI teardown. */ }
      return Promise.resolve(failed);
    };
    if (closing || disposed) return prelaunchError(new Error("Native Fleet is shutting down"));
    if (active.has(taskId)) return completionWait ? active.get(taskId).promise : active.get(taskId).receiptPromise;
    if (!task) return prelaunchError(new Error(`Unknown specialist task ${taskId}`));
    if (task.status !== "pending") return Promise.resolve({ task: task.id, status: task.status, output: task.output, reason: task.reason,
      execution: task.execution, runId: task.execution?.runId, responseFile: task.execution?.responseFile });
    const readiness = teardownFlight
      ? { ready: false, reason: "Native Fleet cancellation is in progress; await cancelAll before dispatching" }
      : controller.taskReadiness(task);
    if (!readiness.ready) {
      const queued = { task: task.id, status: "queued", phase: task.phase, currentPhase: controller.state.phase, reason: readiness.reason,
        nextAction: "Continue independent lead/worker work; dispatch this taskId when its phase/dependencies are ready" };
      try { onChange(queued); } catch { /* UI teardown. */ }
      return Promise.resolve(queued);
    }
    const ctx = options.context ?? context;
    let current;
    try { current = sessionIdentity(ctx); } catch (error) { return prelaunchError(error); }
    if (current.id !== identity.id || current.file !== identity.file) return prelaunchError(new Error("Native Fleet parent session changed; recreate the backend"));
    if (!text(ctx.model?.provider) || !text(ctx.model?.id)) return prelaunchError(new Error("Select a Pi model before dispatching native specialists"));
    const scratch = path.join(root, "specialists", task.id);
    const entry = { task, scratch, processFile: path.join(scratch, "process.json"), onUpdate: options.onUpdate, signal: options.signal,
      record: { task: task.id, source: options.source ?? "delegate", backend: "pi-subagents", status: "starting", parentPid: process.pid,
        parentSessionId: identity.id, ownerSessionId: identity.owner, model: `${ctx.model.provider}/${ctx.model.id}`,
        sessionDir: path.join(scratch, "native"), toolCalls: 0 } };
    entry.abort = () => stop(entry, "Specialist interrupted; reconcile target state before retry");
    entry.receiptPromise = new Promise(resolve => { entry.resolveReceipt = resolve; });
    options.signal?.addEventListener("abort", entry.abort, { once: true });
    active.set(taskId, entry);
    entry.promise = execute(entry);
    // start() callers never await the completion promise. Observe both outcomes
    // so early terminal results settle their receipt and background failures do
    // not become unhandled rejections or silently discard process ownership.
    entry.promise.then(entry.resolveReceipt, error => {
      entry.failure ??= error.message;
      if (!entry.finished) {
        stop(entry, "Native Fleet bookkeeping failed; reconcile partial work");
        entry.record.reason = entry.failure;
        entry.record.teardownPending = true;
        try { publish(entry); } catch { /* Preserve ownership if logging is unavailable. */ }
      }
      entry.resolveReceipt(result(entry, "failed", entry.failure, !entry.finished));
    });
    return completionWait ? entry.promise : entry.receiptPromise;
  }

  // Both APIs share admission and exactly one native execution/monitor per task.
  function run(taskId, options = {}) {
    return dispatch(taskId, options, true);
  }

  function start(taskId, options = {}) {
    return dispatch(taskId, options, false);
  }

  async function teardown(reason) {
    const entries = [...active.values()];
    for (const entry of entries) stop(entry, reason);
    await Promise.all(entries.map(async entry => {
      await entry.promise.catch(() => {}); // The observer above records unexpected failures.
      if (!entry.finished && entry.record.teardownPending) {
        // A subsequent cancellation/shutdown can observe proof written after timeout.
        const native = readNative(entry);
        if (native.terminal && native.proven) await monitor(entry);
      }
      await entry.stopFlight;
    }));
    if (active.size) throw new Error(`Native Fleet teardown unproven for: ${[...active.keys()].join(", ")}; mandatory guard remains registered`);
  }

  // Pause stops the current jobs while retaining the registrations and backend.
  // New pending work queues during cancellation and may be admitted afterward.
  function cancelAll(reason = "Orchestrator paused; reconcile partial work") {
    if (disposed) return Promise.resolve();
    if (!teardownFlight) {
      teardownFlight = Promise.resolve().then(() => teardown(reason));
      // Coalesce pause/shutdown hooks, but permit a retry after missing proof.
      teardownFlight.then(() => { teardownFlight = undefined; }, () => { teardownFlight = undefined; });
    }
    return teardownFlight;
  }

  function stopAll() {
    closing = true;
    return cancelAll("Orchestrator shutdown; reconcile partial work");
  }

  async function dispose() {
    if (disposed) return;
    await stopAll();
    if (disposed) return;
    disposed = true;
    for (const cancel of [...pendingRPC]) cancel();
    for (const registration of registrations.reverse()) registration.dispose();
  }

  // Expose read-only public RPCs; arbitrary spawn/resume would bypass task admission.
  function rpc(method, params) {
    if (!["ping", "status", "cost"].includes(method)) return Promise.reject(new Error(`Native Fleet RPC ${method} must go through task admission/run/stopAll`));
    return request(method, params);
  }
  return { start, run, cancelAll, stopAll, dispose, rpc };
}
