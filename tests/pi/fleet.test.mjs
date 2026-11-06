import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { randomUUID, createHash } from "node:crypto";
import { pathToFileURL } from "node:url";
import { Controller } from "../../pi-runtime/controller.mjs";
import { atomicJSON, readJSON, transact } from "../../pi-runtime/storage.mjs";
import { createFleetBackend } from "../../pi-runtime/fleet.mjs";

const REQUEST = "subagents:rpc:v1:request";
const REPLY = "subagents:rpc:v1:reply:";
const STARTED = "subagent:async-started";
const LEAF = "pentest-harness-leaf";
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

class Bus {
  listeners = new Map();
  on(name, callback) {
    if (!this.listeners.has(name)) this.listeners.set(name, new Set());
    this.listeners.get(name).add(callback);
    return () => {
      this.listeners.get(name)?.delete(callback);
      if (!this.listeners.get(name)?.size) this.listeners.delete(name);
    };
  }
  emit(name, event) { for (const callback of [...(this.listeners.get(name) ?? [])]) callback(event); }
}

async function until(predicate, label = "fixture condition") {
  const deadline = Date.now() + 2000;
  while (!predicate()) {
    assert.ok(Date.now() < deadline, `Timed out waiting for ${label}`);
    await sleep(2);
  }
}

async function fixture(t, options = {}) {
  const directory = fs.mkdtempSync("/tmp/opencode/pentest-fleet-test-");
  const root = path.join(directory, "checkpoint");
  const workspace = path.join(directory, "workspace");
  const runtimeDir = path.join(directory, "runtime");
  const packageRoot = path.join(directory, "fake-subagents");
  for (const dir of [root, workspace, runtimeDir, path.join(packageRoot, "bundle"), path.join(workspace, ".pi/skills/pentest/references")]) fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(workspace, ".pi/skills/pentest/references/checklist.md"), "## 0. Setup\n- [ ] Local fixture\n");
  fs.writeFileSync(path.join(workspace, ".pi/skills/pentest/SKILL.md"), "Offline fixture skill\n");
  fs.writeFileSync(path.join(runtimeDir, "extension.ts"), "// Fake guard path; never loaded or executed.\n");
  fs.writeFileSync(path.join(packageRoot, "package.json"), JSON.stringify({ name: "pi-subagents", type: "module" }));
  // Public API-shaped, isolated module fixtures: no installed runner, CLI, model,
  // target traffic, or actual OS child is involved in any test in this file.
  fs.writeFileSync(path.join(packageRoot, "api.js"), `
    export const seen = [], active = new Set();
    function register(kind, input) {
      const entry = { kind, input }; seen.push(entry); active.add(entry);
      return { dispose() { active.delete(entry); } };
    }
    export const registerRequiredChildExtensions = input => register('required', input);
    export const registerSubagentCapabilityCeiling = input => register('ceiling', input);
  `);
  for (const name of ["required-child-extensions", "capability-ceiling"]) fs.writeFileSync(path.join(packageRoot, "bundle", `${name}.js`), "export * from '../api.js';\n");
  const api = await import(pathToFileURL(path.join(packageRoot, "api.js")).href);
  const controller = new Controller(root, workspace);
  const bus = new Bus();
  const requests = [], changes = [], updates = [], runs = [];
  const sessionId = randomUUID();
  const sessionFile = path.join(directory, "parent-session.jsonl");
  const context = { cwd: directory, model: { provider: "fixture", id: "billion-context", contextWindow: 1e9 },
    sessionManager: { getSessionId: () => sessionId, getSessionFile: () => sessionFile } };
  const launchFile = path.join(directory, "launch.json");
  atomicJSON(launchFile, { budgetUSD: 15, spentUSD: 0, reservations: {} });
  let sequence = 0, backend;
  const admission = {
    taskDigest: task => createHash("sha256").update(JSON.stringify({ id: task.id, actor: task.actor, bounds: task.bounds, output: task.output })).digest("hex"),
    createAttempt({ task, parentSessionId, parentPid, launchFile }) {
      const attempt = { taskId: task.id, attemptId: randomUUID(), contractDigest: admission.taskDigest(task),
        parentSessionId, parentPid, launchFile, expiresAt: new Date(Date.now() + 60000).toISOString(), requestId: randomUUID() };
      atomicJSON(path.join(root, "specialists", task.id, "attempts", `${attempt.attemptId}.json`), attempt);
      return attempt;
    },
    bindingsFor: attempt => ({ "pentest-harnes/1": Object.fromEntries(["taskId", "attemptId", "contractDigest", "parentSessionId"].map(key => [key, attempt[key]])) }),
  };
  const f = { directory, root, workspace, runtimeDir, packageRoot, api, controller, bus, context, launchFile, admission,
    requests, changes, updates, runs, sessionId, sessionFile,
    task(id = "offline", data = {}) {
      return controller.createTask({ id, question: "Review supplied local fixture", skill: ".pi/skills/pentest/SKILL.md", network: false,
        bounds: { minutes: 1 }, ...data });
    },
    reply(request, data) { bus.emit(REPLY + request.requestId, { version: 1, requestId: request.requestId, method: request.method, success: true, data }); },
    reject(request, message = "Native runner admission failed", code = "invalid_params") { bus.emit(REPLY + request.requestId, { version: 1, requestId: request.requestId, method: request.method, success: false, error: { code, message } }); },
    native(request, { start = true, correlation = true, pid = 900000 + ++sequence, claim = true } = {}) {
      const params = request.params;
      const binding = params.extensionBindings["pentest-harnes/1"];
      const reportFile = path.resolve(workspace, controller.state.tasks[binding.taskId].output);
      const runId = `native-${sequence}-${binding.taskId}`;
      const asyncDir = path.join(directory, "native-runs", runId);
      const instanceId = randomUUID();
      const childSessionId = randomUUID();
      const childSessionFile = path.join(params.sessionDir, "child.jsonl");
      const transcriptPath = path.join(asyncDir, "artifacts", "transcript.jsonl");
      fs.mkdirSync(params.sessionDir, { recursive: true });
      fs.mkdirSync(path.dirname(transcriptPath), { recursive: true });
      fs.writeFileSync(childSessionFile, JSON.stringify({ type: "session", version: 3, id: childSessionId }) + "\n");
      fs.writeFileSync(transcriptPath, JSON.stringify({ type: "fixture-transcript", simulated: true }) + "\n");
      const attemptFile = path.join(root, "specialists", binding.taskId, "attempts", `${binding.attemptId}.json`);
      if (claim) transact(attemptFile, undefined, attempt => {
        attempt.claim = { pid, sessionId: childSessionId, sessionFile: childSessionFile, at: new Date().toISOString() };
      });
      const pending = { version: 1, state: "pending", runId, runnerProcessInstanceId: instanceId };
      const status = { runId, mode: "single", state: "running", pid, sessionId: f.sessionFile ?? f.sessionId,
        toolCallId: `rpc-spawn-${request.requestId}`, startedAt: Date.now(), processTerminal: pending,
        steps: [{ agent: LEAF, status: "running", sessionFile: childSessionFile, transcriptPath, currentTool: "read",
          toolCount: 2, tokens: { input: 100, output: 20, total: 120 }, totalCost: { total: 0.001, source: "estimated" }, contextLimit: 1e9, model: params.model }] };
      const native = { request, params, runId, asyncDir, instanceId, childSessionId, childSessionFile, transcriptPath, attemptFile, reportFile, status,
        persist() { atomicJSON(path.join(asyncDir, "status.json"), status); },
        proof(overrides = {}) {
          const proof = { version: 1, state: "observed", runId, runnerProcessInstanceId: instanceId, observedAt: Date.now(),
            instances: [{ kind: "runner", processInstanceId: instanceId, closeObservedAt: Date.now(), exitCode: 0, signal: null }], ...overrides };
          atomicJSON(path.join(asyncDir, "process-terminal.json"), proof);
          return proof;
        },
        seal({ state = "complete", proof = true, report = true } = {}) {
          status.state = state;
          status.steps[0].status = state;
          status.steps[0].exitCode = state === "failed" ? 1 : 0;
          if (report) {
            fs.mkdirSync(path.dirname(reportFile), { recursive: true });
            fs.writeFileSync(reportFile, "# Local synthetic report\nNo model or target was contacted.\n");
          }
          fs.mkdirSync(path.dirname(params.output), { recursive: true });
          fs.writeFileSync(params.output, "Local native final summary. Evidence is in the worker-written task report.\n");
          native.persist();
          if (proof) native.proof();
          bus.emit("subagent:async-complete", { runId, asyncDir, state });
        },
        receipt() { f.reply(request, { text: "Native run started", details: { mode: "single", runId, asyncId: runId, asyncDir } }); },
      };
      runs.push(native);
      native.persist();
      atomicJSON(path.join(asyncDir, "process-terminal.json"), pending);
      if (start) bus.emit(STARTED, { id: runId, pid, mode: "single", agent: LEAF, asyncDir, sessionId: f.sessionFile ?? f.sessionId,
        ...(correlation ? { toolCallId: status.toolCallId } : {}) });
      return native;
    },
    async setup(extra = {}) {
      backend = await createFleetBackend({ pi: { events: bus }, context, controller, root, runtimeDir, packageRoot, admission, launchFile,
        onChange: record => changes.push(record), onUpdate: record => updates.push(record), timing: { pollMs: 3, rpcMs: 100, proofMs: 150 }, ...extra });
      f.backend = backend;
      return backend;
    },
  };
  const detachServer = bus.on(REQUEST, request => {
    requests.push(request);
    if (request.method === "ping") f.reply(request, { version: 1, methods: ["ping", "status", "spawn", "stop", "cost"],
      session: { sessionId: f.sessionId, sessionFile: f.sessionFile }, capabilities: { asyncSpawn: true, processTerminalProof: { version: 1 } } });
    else if (request.method === "spawn") {
      try { if (f.onSpawn) f.onSpawn(request); else f.reject(request, "Fixture has no spawn handler"); }
      catch (error) { f.reject(request, error.message); }
    } else if (request.method === "stop") {
      f.reply(request, { runId: request.params.runId, state: "stopping" });
      f.onStop?.(request);
    } else f.reply(request, { text: "Read-only fixture RPC" });
  });
  t.after(async () => {
    for (const run of runs) run.seal({ state: "stopped", report: false });
    if (backend) await backend.dispose().catch(() => {});
    detachServer();
    fs.rmSync(directory, { recursive: true, force: true });
  });
  return f;
}

test("public registrations use the Pi UUID, with a mandatory guard and leaf-only ceiling", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  assert.deepEqual(f.api.seen.map(entry => entry.input), [
    { sessionId: f.sessionId, extensions: [{ id: "pentest-harnes-guard-v1", path: path.join(f.runtimeDir, "extension.ts") }], requireForAllRunners: true },
    { sessionId: f.sessionId, source: "pentest-harnes", ceiling: { allowedAgents: [LEAF] } },
  ]);
  assert.equal((await backend.rpc("ping")).session.sessionFile, f.sessionFile);
  await assert.rejects(backend.rpc("spawn", {}), /task admission/);
  await assert.rejects(backend.rpc("resume", {}), /task admission/);
  await backend.dispose();
  await backend.dispose();
  assert.equal(f.api.active.size, 0);
  assert.deepEqual([...f.bus.listeners.keys()], [REQUEST]);
});

test("future live phases and unmet dependencies queue without admission or native RPC", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  f.task("future", { phase: "recon", network: true, targets: [{ url: "https://fixture.test/", method: "GET" }] });
  f.task("dependency");
  f.task("dependent", { dependencies: ["dependency"] });
  assert.equal((await backend.run("future")).status, "queued");
  assert.equal((await backend.run("dependent")).status, "queued");
  assert.equal((await backend.start("future")).status, "queued");
  assert.equal((await backend.start("dependent")).status, "queued");
  assert.equal(f.requests.length, 0);
  assert.equal(f.controller.state.tasks.future.status, "pending");
  assert.equal(fs.existsSync(path.join(f.root, "specialists")), false);
});

test("controller actor-session serialization remains an admission gate", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  f.task("first", { actor: { session: "shared-local-fixture" }, mutates: true, restoration: "Restore the local fixture" });
  f.task("second", { actor: { session: "shared-local-fixture" } });
  f.onSpawn = request => { const native = f.native(request); native.receipt(); };
  const first = backend.run("first");
  await until(() => f.runs.length);
  const second = await backend.run("second");
  assert.equal(second.status, "queued");
  assert.match(second.reason, /sharing an actor session/);
  assert.equal(f.requests.filter(request => request.method === "spawn").length, 1);
  f.runs[0].seal();
  assert.equal((await first).status, "returned");
});

test("pre-aborted dispatch and malformed per-attempt bindings cannot spawn", async t => {
  for (const variant of ["abort", "binding", "digest"]) await t.test(variant, async t => {
    const f = await fixture(t);
    const admission = { ...f.admission };
    if (variant === "binding") admission.bindingsFor = () => ({ "pentest-harnes/1": { taskId: "wrong-task" } });
    if (variant === "digest") admission.taskDigest = () => "wrong-contract";
    const backend = await f.setup({ admission });
    f.task();
    const abort = new AbortController();
    if (variant === "abort") abort.abort();
    const result = await backend.run("offline", { signal: abort.signal });
    assert.equal(result.status, variant === "abort" ? "interrupted" : "failed");
    assert.equal(f.requests.some(request => request.method === "spawn" || request.method === "stop"), false);
    assert.equal(result.execution, null);
    await backend.dispose();
    assert.equal(f.api.active.size, 0);
    assert.deepEqual([...f.bus.listeners.keys()], [REQUEST]);
  });
});

test("one task maps to one async native run, including start-before-receipt and billion-context metrics", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  const task = f.task();
  let native;
  f.onSpawn = request => {
    native = f.native(request);
    assert.equal(f.controller.state.tasks[task.id].execution.pid, native.status.pid, "correlated start captures the actual native PID before receipt");
    native.seal();
    native.receipt();
  };
  const first = backend.run(task.id, { onUpdate: record => assert.equal(record.task, task.id), source: "fixture" });
  assert.equal(backend.run(task.id), first, "duplicate dispatch shares the admitted run promise");
  const result = await first;
  assert.equal(result.status, "returned");
  assert.equal(f.requests.filter(request => request.method === "spawn").length, 1);
  const params = native.params;
  assert.equal(params.agent, LEAF);
  assert.equal(params.async, true);
  assert.equal(params.context, "fresh");
  assert.equal(params.cwd, f.workspace);
  assert.equal(params.model, "fixture/billion-context");
  assert.equal(params.sessionDir, path.join(f.root, "specialists", task.id, "native"));
  assert.ok(params.timeoutMs > 0 && params.timeoutMs <= 60000);
  assert.equal(params.output, path.join(f.workspace, "private/native-results", `${task.id}-${params.extensionBindings["pentest-harnes/1"].attemptId}.md`));
  assert.notEqual(params.output, path.join(f.workspace, task.output));
  assert.equal(params.outputMode, "file-only");
  assert.equal(params.artifacts, true);
  assert.equal(Object.hasOwn(params, "clarify"), false);
  assert.match(params.task, /You are specialist offline/);
  assert.match(params.task, /OFFLINE/);
  assert.deepEqual(readJSON(path.join(f.root, "specialists", task.id, "contract.json")), JSON.parse(params.task.split("\nContract:\n")[1]));
  const processFile = path.join(f.root, "specialists", task.id, "process.json");
  const record = readJSON(processFile);
  assert.equal(record.pid, native.status.pid);
  assert.notEqual(record.pid, process.pid);
  assert.equal(record.parentSessionId, f.sessionId);
  assert.equal(record.ownerSessionId, f.sessionFile);
  assert.equal(record.sessionId, native.childSessionId);
  assert.equal(record.sessionFile, native.childSessionFile);
  assert.equal(record.transcriptPath, native.transcriptPath);
  assert.equal(record.contextLimit, 1e9);
  assert.equal(record.responseFile, params.output);
  assert.equal(result.responseFile, params.output);
  assert.equal(result.execution.responseFile, params.output);
  assert.deepEqual(record.totalCost, { total: 0.001, source: "estimated" });
  assert.equal(record.tokens.total, 120);
  assert.equal(record.toolCalls, 2);
  assert.equal(record.lastTool, "read");
  assert.equal(result.runId, native.runId);
  assert.equal(result.execution.runId, native.runId);
  assert.equal(result.execution.sessionId, native.childSessionId);
  assert.equal(f.changes.at(-1).status, "returned");
  const events = fs.readFileSync(path.join(f.root, "specialists", task.id, "process-events.jsonl"), "utf8").trim().split("\n").map(JSON.parse);
  assert.ok(events.length >= 3);
  assert.equal(events.at(-1).processTerminal.state, "observed");
  assert.equal((await backend.run(task.id)).status, "returned");
});

test("start returns the bound receipt while one shared completion monitor keeps reconciling", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  f.task();
  f.onSpawn = request => { f.native(request, { start: false }); };
  const started = backend.start("offline", { source: "nonblocking-fixture" });
  assert.equal(backend.start("offline"), started, "duplicate start before receipt shares admission");
  const completion = backend.run("offline");
  assert.notEqual(completion, started);
  let acknowledged = false, completed = false;
  started.then(() => { acknowledged = true; });
  completion.then(() => { completed = true; });
  await until(() => f.runs.length);
  const native = f.runs[0];
  await sleep(10);
  assert.equal(acknowledged, false, "start waits for the native receipt, not merely dispatch");
  native.receipt();
  const receipt = await started;
  assert.equal(receipt.status, "running");
  assert.equal(receipt.runId, native.runId);
  assert.equal(receipt.asyncDir, native.asyncDir);
  assert.equal(receipt.pid, native.status.pid);
  assert.notEqual(receipt.pid, process.pid);
  assert.equal(receipt.sessionFile, native.childSessionFile);
  assert.equal(receipt.responseFile, native.params.output);
  assert.equal(receipt.output, "reports/offline.md");
  assert.equal(f.controller.state.tasks.offline.status, "running");
  assert.equal(completed, false);
  assert.equal(backend.start("offline"), started, "an active job keeps the same receipt");
  assert.equal(backend.run("offline"), completion);
  assert.equal(f.requests.filter(request => request.method === "spawn").length, 1);
  native.seal({ proof: false });
  await sleep(15);
  assert.equal(completed, false);
  assert.equal(f.controller.state.tasks.offline.status, "running");
  assert.equal(f.changes.some(record => record.status === "returned"), false);
  native.proof();
  await until(() => f.changes.some(record => record.status === "returned"), "asynchronous proven return notification");
  assert.equal(f.controller.state.tasks.offline.status, "returned");
  assert.equal((await completion).status, "returned");
  assert.equal(f.changes.at(-1).processTerminal.state, "observed");
  assert.equal(f.changes.at(-1).sessionId, native.childSessionId);
});

test("start-only dispatch publishes a proven terminal failure without a completion waiter", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  f.task();
  f.onSpawn = request => { const native = f.native(request); native.receipt(); };
  assert.equal((await backend.start("offline")).status, "running");
  const native = f.runs[0];
  native.status.error = "Synthetic native execution failure";
  native.seal({ state: "failed", proof: false, report: false });
  await sleep(15);
  assert.equal(f.controller.state.tasks.offline.status, "running");
  native.proof({ instances: [{ kind: "runner", processInstanceId: native.instanceId, closeObservedAt: Date.now(), exitCode: 1, signal: null }] });
  await until(() => f.changes.some(record => record.status === "failed"));
  assert.equal(f.controller.state.tasks.offline.status, "failed");
  assert.match(f.changes.at(-1).reason, /Synthetic native execution failure/);
  assert.equal(f.changes.at(-1).processTerminal.state, "observed");
  assert.equal(f.changes.at(-1).exitCode, 1);
  await backend.dispose();
  assert.equal(f.api.active.size, 0);
});

test("start resolves early terminal failures and prelaunch validation errors", async t => {
  for (const variant of ["native-rejection", "binding", "pre-aborted", "missing-model", "missing-launch", "unknown-task"]) await t.test(variant, async t => {
    const f = await fixture(t);
    const admission = variant === "binding" ? { ...f.admission, bindingsFor: () => ({}) } : f.admission;
    const backend = await f.setup({ admission, ...(variant === "missing-launch" ? { launchFile: "" } : {}) });
    f.task();
    const abort = new AbortController();
    if (variant === "pre-aborted") abort.abort();
    const result = await backend.start(variant === "unknown-task" ? "absent" : "offline", {
      signal: abort.signal, context: variant === "missing-model" ? { ...f.context, model: null } : f.context,
    });
    assert.equal(result.status, variant === "pre-aborted" ? "interrupted" : "failed");
    assert.ok(result.reason);
    assert.equal(result.execution ?? null, null);
    assert.equal(f.runs.length, 0);
    assert.equal(f.requests.some(request => request.method === "stop"), false);
    if (variant !== "native-rejection") assert.equal(f.requests.some(request => request.method === "spawn"), false);
    if (!["missing-model", "unknown-task"].includes(variant)) assert.equal(f.controller.state.tasks.offline.status, result.status);
    await backend.dispose();
    assert.equal(f.api.active.size, 0);
    assert.deepEqual([...f.bus.listeners.keys()], [REQUEST]);
  });
});

test("native final summary is private and cannot overwrite or substitute for the worker report", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  const task = f.task();
  const workerReport = "# Worker evidence report\nControl, raw evidence, verdicts and coverage retained byte-for-byte.\n";
  f.onSpawn = request => {
    const native = f.native(request);
    assert.match(request.params.task, /Write a substantive initial report to reports\/offline\.md/);
    assert.match(request.params.task, /Return a concise summary/);
    fs.mkdirSync(path.dirname(native.reportFile), { recursive: true });
    fs.writeFileSync(native.reportFile, workerReport);
    native.receipt();
    native.seal({ report: false });
  };
  const result = await backend.run(task.id);
  assert.equal(result.status, "returned");
  assert.equal(fs.readFileSync(path.join(f.workspace, task.output), "utf8"), workerReport);
  const native = f.runs[0];
  assert.notEqual(native.params.output, native.reportFile);
  assert.equal(native.params.output, result.responseFile);
  assert.equal(path.dirname(result.responseFile), path.join(f.workspace, "private/native-results"));
  assert.equal(fs.statSync(path.dirname(result.responseFile)).mode & 0o777, 0o700);
  assert.match(fs.readFileSync(result.responseFile, "utf8"), /Local native final summary/);
  f.task("summary-only");
  f.onSpawn = request => { const native = f.native(request); native.receipt(); native.seal({ report: false }); };
  const missingReport = await backend.run("summary-only");
  assert.equal(missingReport.status, "failed");
  assert.ok(fs.statSync(missingReport.responseFile).size > 0, "a final summary exists but is not the task report");
});

test("start-only cancellation retains unproven ownership and guards until retry observes proof", async t => {
  const f = await fixture(t);
  const backend = await f.setup({ timing: { pollMs: 2, rpcMs: 50, proofMs: 25 } });
  f.task();
  f.onSpawn = request => { const native = f.native(request); native.receipt(); };
  const receiptPromise = backend.start("offline");
  assert.equal((await receiptPromise).status, "running");
  await assert.rejects(backend.cancelAll("Pause after nonblocking launch"), /teardown unproven/);
  assert.equal(f.controller.state.tasks.offline.status, "running");
  assert.equal(f.changes.at(-1).teardownPending, true);
  assert.equal(f.api.active.size, 2);
  assert.equal(backend.start("offline"), receiptPromise);
  assert.equal(f.requests.filter(request => request.method === "spawn").length, 1);
  f.runs[0].seal({ state: "stopped", report: false });
  await backend.cancelAll();
  assert.equal(f.controller.state.tasks.offline.status, "interrupted");
  assert.equal(f.changes.at(-1).processTerminal.state, "observed");
  await backend.dispose();
  assert.equal(f.api.active.size, 0);
});

test("unexpected unattended monitor rejection is observed and retains ownership for reconciliation", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  f.task();
  f.onSpawn = request => { const native = f.native(request); native.receipt(); };
  assert.equal((await backend.start("offline")).status, "running");
  const returnedTask = f.controller.returnedTask;
  f.controller.returnedTask = () => { throw new Error("Synthetic checkpoint write failure"); };
  f.runs[0].seal();
  // Node's test runner also fails this test on any unhandled rejection from the
  // completion monitor, which deliberately has no run() waiter here.
  await until(() => f.changes.some(record => record.teardownPending), "observed background bookkeeping failure");
  assert.equal(f.controller.state.tasks.offline.status, "running");
  assert.match(f.changes.at(-1).reason, /Synthetic checkpoint write failure/);
  assert.equal(f.api.active.size, 2);
  f.controller.returnedTask = returnedTask;
  await backend.cancelAll();
  assert.equal(f.controller.state.tasks.offline.status, "failed");
  assert.equal(f.changes.at(-1).processTerminal.state, "observed");
  await backend.dispose();
  assert.equal(f.api.active.size, 0);
});

test("unlabelled early starts are buffered by runId; unrelated workers cannot supply provenance", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  f.task();
  f.onSpawn = request => {
    f.bus.emit(STARTED, { id: "unrelated", pid: process.pid, asyncDir: f.directory });
    const native = f.native(request, { correlation: false });
    assert.equal(f.controller.state.tasks.offline.execution, null);
    native.receipt();
    native.seal();
  };
  const result = await backend.run("offline");
  assert.equal(result.status, "returned");
  assert.equal(result.execution.pid, f.runs[0].status.pid);
});

test("status and guard claim recover PID/session when the start event is absent", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  f.task();
  f.onSpawn = request => {
    const native = f.native(request, { start: false });
    native.receipt(); native.seal();
  };
  assert.equal((await backend.run("offline")).status, "returned");
  assert.equal(f.controller.state.tasks.offline.execution.sessionFile, f.runs[0].childSessionFile);
});

test("in-memory parent sessions use their UUID for both registration and async ownership", async t => {
  const f = await fixture(t);
  f.sessionFile = null;
  f.context.sessionManager.getSessionFile = () => null;
  const backend = await f.setup();
  f.task();
  f.onSpawn = request => { const native = f.native(request); native.receipt(); native.seal(); };
  const result = await backend.run("offline");
  assert.equal(result.status, "returned");
  const record = readJSON(path.join(f.root, "specialists/offline/process.json"));
  assert.equal(record.parentSessionId, f.sessionId);
  assert.equal(record.ownerSessionId, f.sessionId);
  assert.notEqual(record.sessionId, f.sessionId);
});

test("terminal status, completion hints and child-step proofs do not establish run termination", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  f.task();
  let native;
  f.onSpawn = request => { native = f.native(request); native.receipt(); native.seal({ proof: false }); };
  let finished = false;
  const running = backend.run("offline").then(result => { finished = true; return result; });
  await until(() => native);
  native.status.steps[0].processTerminal = { ...native.proof(), childIndex: 0 };
  // Restore the pending run-level sidecar after manufacturing the child proof.
  atomicJSON(path.join(native.asyncDir, "process-terminal.json"), { version: 1, state: "pending", runId: native.runId, runnerProcessInstanceId: native.instanceId });
  native.persist();
  f.bus.emit("subagent:process-terminal", { runId: native.runId, state: "observed" });
  await sleep(20);
  assert.equal(finished, false);
  assert.equal(f.controller.state.tasks.offline.status, "running");
  native.proof({ runId: "some-other-run" });
  await sleep(15);
  assert.equal(finished, false, "proof must belong to this exact native run");
  native.proof();
  assert.equal((await running).status, "returned");
});

test("stopAll waits for authoritative termination after the stopping acknowledgement", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  f.task();
  f.onSpawn = request => { const native = f.native(request); native.receipt(); };
  const running = backend.run("offline");
  await until(() => f.runs.length);
  let stopped = false;
  const stop = backend.stopAll().then(() => { stopped = true; });
  const alsoStop = backend.stopAll();
  await until(() => f.requests.some(request => request.method === "stop"));
  await sleep(15);
  assert.equal(stopped, false);
  assert.equal(f.controller.state.tasks.offline.status, "running");
  assert.equal(f.api.active.size, 2, "mandatory registrations remain through teardown");
  f.runs[0].seal({ state: "stopped", proof: false, report: false });
  await sleep(15);
  assert.equal(stopped, false);
  f.runs[0].proof();
  await stop;
  await alsoStop;
  assert.equal(f.requests.filter(request => request.method === "stop").length, 1);
  assert.equal((await running).status, "interrupted");
  f.task("after-shutdown");
  await backend.cancelAll();
  await assert.rejects(backend.run("after-shutdown"), /shutting down/);
  await backend.dispose();
  assert.equal(f.api.active.size, 0);
  assert.deepEqual([...f.bus.listeners.keys()], [REQUEST]);
});

test("cancelAll waits for terminal/process proof, retains registrations, and admits a later run", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  f.task("before-pause", { actor: { session: "pause-resume-fixture" } });
  f.task("after-pause", { actor: { session: "pause-resume-fixture" } });
  f.onSpawn = request => { const native = f.native(request); native.receipt(); };
  const running = backend.run("before-pause");
  await until(() => f.runs.length);
  const reason = "User paused the local fixture; reconcile partial work";
  const cancellation = backend.cancelAll(reason);
  assert.equal(backend.cancelAll("Concurrent pause"), cancellation);
  let cancelled = false;
  cancellation.then(() => { cancelled = true; });
  await until(() => f.requests.some(request => request.method === "stop"));
  assert.equal((await backend.run("after-pause")).status, "queued");
  await sleep(15);
  assert.equal(cancelled, false, "a stop receipt is not completion");
  assert.equal(f.controller.state.tasks["before-pause"].status, "running");
  assert.equal(f.api.active.size, 2);
  const native = f.runs[0];
  native.seal({ state: "stopped", proof: false, report: false });
  await sleep(15);
  assert.equal(cancelled, false, "terminal status still requires observed run-level process proof");
  native.proof();
  await cancellation;
  assert.equal((await running).status, "interrupted");
  assert.equal(f.controller.state.tasks["before-pause"].reason, reason);
  assert.equal(f.requests.filter(request => request.method === "stop").length, 1);
  assert.equal(f.api.active.size, 2, "pause keeps the guard and capability ceiling registered");
  assert.deepEqual([...f.bus.listeners.keys()], [REQUEST]);
  f.onSpawn = request => { const native = f.native(request); native.receipt(); native.seal(); };
  const resumed = await backend.run("after-pause");
  assert.equal(resumed.status, "returned");
  assert.notEqual(resumed.runId, native.runId);
  assert.notEqual(f.runs[1].params.extensionBindings["pentest-harnes/1"].attemptId, native.params.extensionBindings["pentest-harnes/1"].attemptId);
  assert.equal(f.api.active.size, 2);
  await backend.cancelAll();
  await backend.dispose();
  assert.equal(f.api.active.size, 0);
});

test("unproven cancelAll retains ownership and can be retried without closing future admission", async t => {
  const f = await fixture(t);
  const backend = await f.setup({ timing: { pollMs: 2, rpcMs: 50, proofMs: 25 } });
  f.task("unproven-pause", { actor: { session: "unproven-pause-fixture" }, stateKeys: ["owned-resource"] });
  f.task("resumed", { actor: { session: "unproven-pause-fixture" }, stateKeys: ["owned-resource"] });
  f.onSpawn = request => { const native = f.native(request); native.receipt(); };
  const running = backend.run("unproven-pause");
  await until(() => f.runs.length);
  await assert.rejects(backend.cancelAll(), /teardown unproven/);
  assert.equal((await running).teardownPending, true);
  assert.equal(f.controller.state.tasks["unproven-pause"].status, "running");
  assert.equal(f.api.active.size, 2);
  assert.ok(f.bus.listeners.has(STARTED));
  assert.equal((await backend.run("unproven-pause")).teardownPending, true);
  const queued = await backend.run("resumed");
  assert.equal(queued.status, "queued");
  assert.match(queued.reason, /shared target state/);
  assert.equal(f.requests.filter(request => request.method === "spawn").length, 1);
  f.runs[0].seal({ state: "stopped", report: false });
  await backend.cancelAll();
  assert.equal(f.controller.state.tasks["unproven-pause"].status, "interrupted");
  assert.match(f.controller.state.tasks["unproven-pause"].reason, /Orchestrator paused/);
  assert.equal(readJSON(path.join(f.root, "specialists/unproven-pause/process.json")).teardownPending, undefined);
  assert.equal(f.api.active.size, 2);
  f.onSpawn = request => { const native = f.native(request); native.receipt(); native.seal(); };
  assert.equal((await backend.run("resumed")).status, "returned");
  await backend.dispose();
  assert.equal(f.api.active.size, 0);
});

test("abort before the spawn receipt stops the correlated run without spawning a replacement", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  f.task();
  const abort = new AbortController();
  f.onSpawn = request => {
    const native = f.native(request);
    abort.abort();
    // Stop can happen before the outstanding spawn request replies.
    assert.equal(f.requests.at(-1).method, "stop");
    native.seal({ state: "stopped", report: false });
    native.receipt();
  };
  assert.equal((await backend.run("offline", { signal: abort.signal })).status, "interrupted");
  assert.equal(f.requests.filter(request => request.method === "spawn").length, 1);
  assert.equal(f.requests.filter(request => request.method === "stop").length, 1);
});

test("native deadline also requests stop while spawn is awaiting its receipt", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  f.task("short", { bounds: { minutes: 0.0005 } });
  f.onSpawn = request => { f.native(request); };
  f.onStop = () => { f.runs[0].seal({ state: "stopped", report: false }); f.runs[0].receipt(); };
  const result = await backend.run("short");
  assert.equal(result.status, "interrupted");
  assert.match(result.reason, /time bound/);
  assert.ok(f.requests.find(request => request.method === "spawn").params.timeoutMs <= 30);
});

test("missing terminal proof is an explicit unresolved failure, retaining guard and task ownership", async t => {
  const f = await fixture(t);
  const backend = await f.setup({ timing: { pollMs: 2, rpcMs: 50, proofMs: 25 } });
  f.task();
  f.onSpawn = request => { const native = f.native(request); native.receipt(); native.seal({ proof: false }); };
  const result = await backend.run("offline");
  assert.equal(result.status, "failed");
  assert.equal(result.teardownPending, true);
  assert.match(result.reason, /unproven/);
  assert.equal(f.controller.state.tasks.offline.status, "running");
  await assert.rejects(backend.dispose(), /teardown unproven/);
  assert.equal(f.api.active.size, 2);
  assert.ok(f.bus.listeners.has(STARTED));
  f.runs[0].seal({ state: "stopped", report: false });
  await backend.dispose();
  assert.equal(f.controller.state.tasks.offline.status, "interrupted");
  assert.equal(f.api.active.size, 0);
});

test("native rejection is returned as failure without fabricated execution provenance", async t => {
  const f = await fixture(t);
  const backend = await f.setup();
  f.task();
  const result = await backend.run("offline");
  assert.equal(result.status, "failed");
  assert.match(result.reason, /Fixture has no spawn handler/);
  assert.equal(result.execution, null);
  assert.equal(f.controller.state.tasks.offline.status, "failed");
  assert.equal(f.requests.filter(request => request.method === "stop").length, 0);
});

test("an execution_failed reply can follow OS startup and cannot itself prove teardown", async t => {
  const f = await fixture(t);
  const backend = await f.setup({ timing: { pollMs: 2, rpcMs: 100, proofMs: 25 } });
  f.task();
  let spawnRequest;
  f.onSpawn = request => { spawnRequest = request; f.reject(request, "Runner startup handshake failed", "execution_failed"); };
  const result = await backend.run("offline");
  assert.equal(result.status, "failed");
  assert.equal(result.teardownPending, true);
  assert.equal(f.controller.state.tasks.offline.status, "running");
  assert.equal(result.execution, null);
  assert.match(result.reason, /startup handshake failed/);
  assert.equal(f.api.active.size, 2);
  // A delayed, exactly correlated package start can still establish ownership.
  const native = f.native(spawnRequest);
  native.seal({ state: "stopped", report: false });
  await backend.dispose();
  assert.equal(f.controller.state.tasks.offline.status, "failed");
  assert.equal(f.api.active.size, 0);
});

test("lost spawn receipt is not retried; correlated execution is stopped and proven before returning", async t => {
  const f = await fixture(t);
  const backend = await f.setup({ timing: { pollMs: 2, rpcMs: 25, proofMs: 100 } });
  f.task();
  f.onSpawn = request => { f.native(request); };
  f.onStop = () => f.runs[0].seal({ state: "stopped", report: false });
  const result = await backend.run("offline");
  assert.equal(result.status, "failed");
  assert.match(result.reason, /spawn RPC timed out/);
  assert.equal(result.teardownPending, undefined);
  assert.equal(f.requests.filter(request => request.method === "spawn").length, 1);
  assert.equal(f.requests.filter(request => request.method === "stop").length, 1);
  assert.equal(f.controller.state.tasks.offline.status, "failed");
});

test("report, mandatory claim and actual child session are required for a successful return", async t => {
  for (const missing of ["report", "claim", "session"]) await t.test(missing, async t => {
    const f = await fixture(t);
    const backend = await f.setup();
    f.task();
    f.onSpawn = request => {
      const native = f.native(request, { claim: missing !== "claim" });
      if (missing === "session") {
        // A child cannot claim the parent session or another task's directory.
        transact(native.attemptFile, undefined, attempt => { attempt.claim.sessionId = f.sessionId; });
      }
      native.receipt(); native.seal({ report: missing !== "report" });
    };
    const result = await backend.run("offline");
    assert.equal(result.status, "failed");
    assert.match(result.reason, missing === "report" ? /ENOENT|Artifact/ : /claim|session/);
  });
});

test("parent PID is rejected even when native termination is proven", async t => {
  const f = await fixture(t);
  const backend = await f.setup({ timing: { pollMs: 2, rpcMs: 100, proofMs: 30 } });
  f.task();
  f.onSpawn = request => {
    const native = f.native(request, { pid: process.pid });
    native.receipt(); native.seal();
  };
  const result = await backend.run("offline");
  assert.equal(result.status, "failed");
  assert.equal(result.teardownPending, undefined);
  assert.equal(result.execution, null);
  assert.match(result.reason, /actual runner PID/);
  await backend.dispose();
  assert.equal(f.controller.state.tasks.offline.status, "failed");
});

test("mismatched runner-instance proofs retain ownership until the exact runner is observed", async t => {
  const f = await fixture(t);
  const backend = await f.setup({ timing: { pollMs: 2, rpcMs: 100, proofMs: 30 } });
  f.task();
  f.onSpawn = request => {
    const native = f.native(request);
    native.receipt(); native.seal();
    native.proof({ runnerProcessInstanceId: "wrong-instance" });
  };
  const result = await backend.run("offline");
  assert.equal(result.status, "failed");
  assert.equal(result.teardownPending, true);
  assert.equal(f.controller.state.tasks.offline.status, "running");
  await assert.rejects(backend.dispose(), /teardown unproven/);
  f.runs[0].proof();
  await backend.dispose();
  assert.equal(f.controller.state.tasks.offline.status, "interrupted");
});

test("missing package, unavailable RPC, and changed parent identity fail usefully", async t => {
  const f = await fixture(t);
  await assert.rejects(f.setup({ packageRoot: path.join(f.directory, "missing-package") }), /Native Fleet unavailable/);
  assert.equal(f.api.active.size, 0);
  const backend = await f.setup({ timing: { pollMs: 2, rpcMs: 15, proofMs: 30 } });
  f.task();
  await assert.rejects(backend.run("offline", { context: { ...f.context, sessionManager: { getSessionId: () => "wrong", getSessionFile: () => f.sessionFile } } }), /parent session changed/);
  f.bus.listeners.delete(REQUEST);
  const result = await backend.run("offline");
  assert.equal(result.status, "failed");
  assert.match(result.reason, /session-ready/);
  assert.equal(f.runs.length, 0);
  assert.equal(result.execution, null);
  await backend.dispose();
  assert.equal(f.bus.listeners.size, 0);
});
