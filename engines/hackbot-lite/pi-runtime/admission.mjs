import path from "node:path";
import { createHash, randomUUID } from "node:crypto";
import { atomicJSON, readJSON, transact } from "./storage.mjs";

const namespace = "pentest-harnes/1";
const safeId = value => typeof value === "string" && /^[A-Za-z0-9][A-Za-z0-9_-]{0,100}$/.test(value);
const canonical = value => Array.isArray(value) ? value.map(canonical) : value && typeof value === "object" ? Object.fromEntries(Object.keys(value).sort().map(key => [key, canonical(value[key])])) : value;

export function taskDigest(task) {
  const keys = ["id", "phase", "question", "skill", "output", "inputs", "dependencies", "stateKeys", "targets", "network", "mutates", "bounds", "actor", "method", "success", "stop", "restoration"];
  return createHash("sha256").update(JSON.stringify(canonical(Object.fromEntries(keys.filter(key => task[key] !== undefined).map(key => [key, task[key]]))))).digest("hex");
}

export function createAttempt({ root, task, parentSessionId, parentPid, launchFile }) {
  if (!safeId(task.id) || !parentSessionId || !Number.isSafeInteger(parentPid) || parentPid <= 0) throw new Error("Invalid native worker admission identity");
  const attemptId = randomUUID();
  const record = { version: 1, taskId: task.id, attemptId, contractDigest: taskDigest(task), parentSessionId, parentPid, launchFile,
    requestId: randomUUID(), expiresAt: new Date(Date.parse(task.startedAt) + task.bounds.minutes * 60000).toISOString(), authorized: true, createdAt: new Date().toISOString() };
  atomicJSON(path.join(root, "specialists", task.id, "attempts", `${attemptId}.json`), record);
  return record;
}

export function bindingsFor(attempt) {
  return { [namespace]: Object.fromEntries(["taskId", "attemptId", "contractDigest", "parentSessionId"].map(key => [key, attempt[key]])) };
}

export function childBinding(env = process.env) {
  if (env.PI_SUBAGENT_CHILD !== "1") return null;
  let bindings;
  try { bindings = JSON.parse(env.PI_SUBAGENT_EXTENSION_BINDINGS ?? "{}"); }
  catch { throw new Error("Native specialist has malformed extension bindings"); }
  const binding = bindings[namespace];
  if (!binding || !safeId(binding.taskId) || !safeId(binding.attemptId) || !/^[a-f0-9]{64}$/.test(binding.contractDigest) || typeof binding.parentSessionId !== "string" || !binding.parentSessionId) {
    throw new Error("Native specialist requires a harness-authorized task binding; dispatch through pentest_delegate");
  }
  return Object.freeze({ ...binding });
}

export function claimAttempt({ root, launchFile, controller, binding, sessionId, sessionFile, pid = process.pid }) {
  if (!binding || !safeId(binding.taskId) || !safeId(binding.attemptId)) throw new Error("Invalid native specialist binding");
  const task = controller.state.tasks[binding.taskId];
  if (!task || task.status !== "running" || !controller.taskPhaseAllowed(task)) throw new Error("Native specialist task is not admitted in the current phase");
  if (taskDigest(task) !== binding.contractDigest) throw new Error("Native specialist contract changed after admission");
  if (task.dependencies.some(id => controller.state.tasks[id]?.status !== "verified")) throw new Error("Native specialist prerequisites are not verified");
  if (task.network !== false) for (const target of task.targets) controller.checkScope(target.url, target.method);
  if (!sessionId || !sessionFile || !Number.isSafeInteger(pid) || pid <= 0) throw new Error("Native specialist must expose an actual child session and runner PID");
  const nativeRoot = path.resolve(root, "specialists", task.id, "native");
  if (!path.resolve(sessionFile).startsWith(nativeRoot + path.sep)) throw new Error("Native child session is outside its isolated task directory");
  const file = path.join(root, "specialists", task.id, "attempts", `${binding.attemptId}.json`);
  return transact(file, undefined, attempt => {
    if (attempt.version !== 1 || attempt.authorized !== true || attempt.revoked || attempt.launchFile !== launchFile ||
        ["taskId", "attemptId", "contractDigest", "parentSessionId"].some(key => attempt[key] !== binding[key]) ||
        Date.now() >= Date.parse(attempt.expiresAt) || !Number.isFinite(Date.parse(attempt.expiresAt))) throw new Error("Native specialist admission expired, changed, or was revoked");
    try { process.kill(attempt.parentPid, 0); } catch { throw new Error("Native specialist's owning orchestrator has exited"); }
    if (attempt.parentPid === pid || sessionId === attempt.parentSessionId) throw new Error("Native specialist requires a distinct runner and child session");
    if (attempt.claim && (attempt.claim.pid !== pid || attempt.claim.sessionId !== sessionId || attempt.claim.sessionFile !== sessionFile)) throw new Error("Native specialist admission has already been claimed by another child");
    attempt.claim ??= { pid, sessionId, sessionFile, at: new Date().toISOString() };
    return { ...attempt };
  });
}

export function assertAttempt({ root, controller, binding, sessionId, pid = process.pid }) {
  const task = controller.state.tasks[binding.taskId];
  const attempt = readJSON(path.join(root, "specialists", binding.taskId, "attempts", `${binding.attemptId}.json`));
  if (!task || task.status !== "running" || !controller.taskPhaseAllowed(task) || !attempt.authorized || attempt.revoked || Date.now() >= Date.parse(attempt.expiresAt) ||
      attempt.claim?.pid !== pid || attempt.claim?.sessionId !== sessionId || taskDigest(task) !== binding.contractDigest) throw new Error("Native specialist's task admission is no longer valid");
  return attempt;
}
