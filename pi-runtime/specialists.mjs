import fs from "node:fs";
import path from "node:path";
import readline from "node:readline";
import { spawn } from "node:child_process";
import { artifact, atomicJSON, readJSON } from "./storage.mjs";

const stopping = new WeakMap();

function groupAlive(child) {
  if (!child.pid) return false;
  try { process.kill(process.platform === "win32" ? child.pid : -child.pid, 0); return true; }
  catch (error) { return error.code !== "ESRCH"; }
}

function signalGroup(child, signal) {
  try {
    if (process.platform !== "win32" && child.pid) process.kill(-child.pid, signal);
    else child.kill(signal);
  } catch (error) { if (error.code !== "ESRCH") child.kill(signal); }
}

// Signal Pi first so its print-mode shutdown closes tracked shell processes and
// native MCP servers (which may own separate process groups). Keep escalation
// alive until the owned group is gone, even if the immediate Pi child exits.
export function terminateSpecialist(child, options = {}) {
  if (!child?.pid) return Promise.resolve();
  if (stopping.has(child)) return stopping.get(child);
  let finish;
  const done = new Promise(resolve => { finish = resolve; });
  stopping.set(child, done);
  if (process.platform === "win32") {
    const killer = spawn("taskkill", ["/PID", String(child.pid), "/T", "/F"], { stdio: "ignore" });
    killer.once("error", () => { child.kill(); finish(); });
    killer.once("close", finish);
    return done;
  }
  let finished = false;
  const complete = () => {
    if (finished) return;
    finished = true;
    clearInterval(poll); clearTimeout(escalation);
    finish();
  };
  const check = () => { if (!groupAlive(child)) complete(); };
  const poll = setInterval(check, 25);
  const escalation = setTimeout(() => { signalGroup(child, "SIGKILL"); complete(); }, options.killAfterMs ?? 5000);
  child.once("close", check);
  let alreadyFinishing = false;
  try { alreadyFinishing = Boolean(child.pentestFinishing?.()); } catch {}
  if (!alreadyFinishing) try { child.kill("SIGTERM"); } catch { check(); }
  check();
  return done;
}

export function browserSession(root, taskId) {
  return `pentest-${path.basename(root)}-${taskId}`;
}

export async function runSpecialist({ controller, root, runtimeDir, taskId, context, signal, children, onChange = () => {}, onUpdate = () => {}, source = "delegate" }) {
  const task = controller.state.tasks[taskId];
  if (!task) throw new Error(`Unknown specialist task ${taskId}`);
  if (task.status !== "pending") return { task: task.id, status: task.status, output: task.output, execution: task.execution };
  const readiness = controller.taskReadiness(task);
  if (!readiness.ready) {
    onChange({ task: task.id, status: "queued", reason: readiness.reason });
    return { task: task.id, status: "queued", phase: task.phase, currentPhase: controller.state.phase, reason: readiness.reason, nextAction: "Continue independent lead/worker work; dispatch this taskId when its phase/dependencies are ready" };
  }
  if (!context.model) throw new Error("Select a Pi model before dispatching specialists");

  controller.startTask(task.id);
  const scratch = path.join(root, "specialists", task.id);
  const processFile = path.join(scratch, "process.json");
  let child, closed, lines, transcript, errors, timer, final = "", timedOut = false, interrupted = false;
  let record = { task: task.id, source, status: "starting", parentPid: process.pid, startedAt: task.startedAt, model: `${context.model.provider}/${context.model.id}`, sessionDir: scratch, browserSession: browserSession(root, task.id), toolCalls: 0 };
  const publish = () => {
    atomicJSON(processFile, record);
    fs.appendFileSync(path.join(scratch, "process-events.jsonl"), JSON.stringify({ ...record, at: new Date().toISOString() }) + "\n", { mode: 0o600 });
    try { onChange({ ...record }); } catch { /* UI teardown must not corrupt process bookkeeping. */ }
  };
  const terminate = () => {
    if (!child) return;
    return terminateSpecialist(child);
  };
  const abort = () => { interrupted = true; terminate(); };

  try {
    fs.mkdirSync(scratch, { recursive: true, mode: 0o700 });
    atomicJSON(path.join(scratch, "contract.json"), task);
    publish();
    const args = ["--approve", "--print", "--mode", "json", "--name", `specialist:${task.id}`, "--session-dir", scratch,
      "--extension", "builtin:mcp", "--extension", "builtin:codemode", "--extension", "builtin:tool-search",
      "--extension", path.join(runtimeDir, "extension.ts"), "--pentest-runtime", "--append-system-prompt", path.join(runtimeDir, "SPECIALIST.md"),
      "--provider", context.model.provider, "--model", context.model.id];
    for (const skill of (process.env.PENTEST_HARNES_EXTERNAL_SKILLS ?? "").split(path.delimiter).filter(Boolean)) args.push("--skill", skill);
    args.push(`You are specialist ${task.id}. Read the assigned SKILL.md and relevant references first. Execute only this contract; do not perform parent orchestration or modify central ledgers/checkpoints. Keep secrets private and use the pinned actor/session. ${task.network === false ? "This is OFFLINE work: no target requests, HTTP replay, live browser automation, or operational MCP calls. Analyze only supplied local artifacts." : "Use only the scoped target URLs/methods and request bounds; stop on the skill's challenge/error conditions."} Your scratch is ${scratch}. Write the report to ${task.output}, with actual evidence paths, coverage, verdicts and blockers. Do not claim a tool worked without observing it.\nContract:\n${JSON.stringify(task, null, 2)}`);
    child = spawn(process.env.PENTEST_HARNES_PI ?? "pi", args, { cwd: context.cwd,
      env: { ...process.env, PENTEST_HARNES_TASK: task.id, PENTEST_HARNES_PARENT_PID: String(process.pid), INTERCEPTOR_SESSION_ID: record.browserSession },
      detached: process.platform !== "win32", stdio: ["ignore", "pipe", "pipe"] });
    child.pentestFinishing = () => {
      if (record.agentEndedAt) return true;
      const stopping = readJSON(path.join(scratch, "interrupted.json"), null);
      return stopping && Date.parse(stopping.at) >= Date.parse(task.startedAt);
    };
    children.add(child);
    closed = new Promise((resolve, reject) => { child.once("error", reject); child.once("close", (code, signal) => resolve({ code, signal })); });
    // The close promise is also observed while awaiting OS spawn acknowledgement.
    closed.catch(() => {});
    transcript = fs.createWriteStream(path.join(scratch, "transcript.jsonl"), { flags: "a", mode: 0o600 });
    errors = fs.createWriteStream(path.join(scratch, "stderr.log"), { flags: "a", mode: 0o600 });
    const logFailure = error => { record.logError = error.message; terminate(); };
    transcript.on("error", logFailure); errors.on("error", logFailure);
    child.stdout.pipe(transcript); child.stderr.pipe(errors);
    lines = readline.createInterface({ input: child.stdout });
    lines.on("line", line => {
      let event;
      try { event = JSON.parse(line); } catch { return; }
      if (event.type === "session" && event.id) record.sessionId = event.id;
      if (event.type === "agent_end") record.agentEndedAt = new Date().toISOString();
      if (event.type === "message_end" && event.message?.role === "assistant") final = (event.message.content ?? []).filter(item => item.type === "text").map(item => item.text).join("\n");
      if (event.type === "tool_execution_start") {
        record.lastTool = event.toolName; record.toolCalls++;
        try { publish(); onUpdate({ ...record }); } catch { terminate(); }
      }
    });
    await new Promise((resolve, reject) => { child.once("spawn", resolve); child.once("error", reject); });
    controller.recordTaskProcess(task.id, { pid: child.pid, parentPid: process.pid, startedAt: task.startedAt, model: record.model, sessionDir: scratch });
    record = { ...record, pid: child.pid, status: "running" };
    publish(); onUpdate({ ...record });
    const deadline = Date.parse(task.startedAt) + task.bounds.minutes * 60000;
    timer = setInterval(() => { if (Date.now() >= deadline) { timedOut = true; terminate(); } }, Math.max(10, Math.min(1000, task.bounds.minutes * 60000)));
    signal?.addEventListener("abort", abort, { once: true });
    if (signal?.aborted) abort();
    const exit = await closed;
    if (process.platform !== "win32" && groupAlive(child)) {
      if (!interrupted && !timedOut) record.cleanupRequired = true;
      interrupted = true; terminate();
    }
    if (stopping.has(child)) {
      record.status = "stopping"; publish();
      await stopping.get(child);
    }
    const interruption = readJSON(path.join(scratch, "interrupted.json"), null);
    const workerInterrupted = interruption && Date.parse(interruption.at) >= Date.parse(task.startedAt);
    const failed = exit.code !== 0 || timedOut || interrupted || workerInterrupted || record.logError;
    const reason = workerInterrupted ? interruption.reason : timedOut ? "Task time bound reached; reconcile partial work" : record.cleanupRequired ? "Owned subprocesses remained after worker exit; forced teardown and reconciliation required" : interrupted ? "Specialist interrupted; reconcile target state before retry" : record.logError ? "Specialist evidence logging failed" : failed ? `Specialist exited ${exit.code}; inspect private transcript and stderr` : "Awaiting lead evidence verification";
    const status = failed ? "interrupted" : "returned";
    if (!failed) artifact(context.cwd, task.output);
    const finishedAt = new Date().toISOString();
    controller.returnedTask(task.id, status, reason, { exitCode: exit.code, finishedAt, toolCalls: record.toolCalls, lastTool: record.lastTool, sessionId: record.sessionId });
    record = { ...record, status, reason, exitCode: exit.code, finishedAt };
    publish();
    return { task: task.id, status, output: task.output, execution: controller.state.tasks[task.id].execution, scratch, reason, summary: final.slice(-16000) };
  } catch (error) {
    terminate();
    if (child && stopping.has(child)) await stopping.get(child);
    if (closed) await Promise.race([closed.catch(() => {}), new Promise(resolve => setTimeout(resolve, 1000).unref())]);
    if (controller.state.tasks[task.id]?.status === "running") controller.returnedTask(task.id, "failed", error.message);
    record = { ...record, status: "failed", reason: error.message, finishedAt: new Date().toISOString() };
    try { publish(); } catch {}
    return { task: task.id, status: "failed", reason: error.message, scratch, execution: controller.state.tasks[task.id]?.execution };
  } finally {
    clearInterval(timer);
    signal?.removeEventListener("abort", abort);
    if (child && stopping.has(child)) await stopping.get(child);
    if (child) children.delete(child);
    lines?.close(); transcript?.end(); errors?.end();
  }
}
