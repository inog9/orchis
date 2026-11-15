import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import { Controller, FAMILIES, PHASES, cvss31 } from "../../pi-runtime/controller.mjs";
import { atomicJSON, artifact } from "../../pi-runtime/storage.mjs";
import { Budget, scheduleRequest } from "../../pi-runtime/budget.mjs";
import { isIdleShellPolling } from "../../pi-runtime/orchestration.mjs";

function fixture(t, { withScope = true } = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "pentest-controller-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const workspace = path.join(root, "workspace");
  const write = (relative, text = "Observed request and response evidence\n") => {
    const file = path.join(workspace, relative);
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, text);
    return relative;
  };
  write(".pi/skills/pentest/references/checklist.md", fs.readFileSync(new URL("../../skills/pentest/references/checklist.md", import.meta.url), "utf8"));
  write(".pi/skills/pentest-auth/SKILL.md", "# Authentication\n");
  write(".pi/skills/pentest-reflection/SKILL.md", "# Reflection\n");
  for (const skill of ["pentest-exploration", "pentest-recon", "pentest-js-analysis", "pentest-dependency-confusion", "pentest-authz", "pentest-injection", "pentest-fuzzing", "pentest-endpoint-fuzz-full", "pentest-evidence", "pentest-report"]) write(`.pi/skills/${skill}/SKILL.md`, `# ${skill}\n`);
  write("private/evidence.txt");
  const controller = new Controller(root, workspace);
  const scope = { target: "fixture", origins: ["https://app.example.test"], methods: ["GET", "PATCH"], authorization: "Explicit fixture authorization", window: "Until checklist completion", actions: "Test-owned records only", evidence: ["private/evidence.txt"] };
  if (withScope) controller.apply("scope", scope);
  return { root, workspace, write, controller, scope };
}

function task(id = "auth-one") {
  return { id, phase: "setup", question: "Does this owned session have the expected behavior?", skill: ".pi/skills/pentest-auth/SKILL.md", output: `reports/${id}.md`, method: "One bounded observed request", success: "Replayable control and result", stop: "Stop at budget, challenge, or result", restoration: "Read-only, no restoration required", inputs: [], dependencies: [], stateKeys: [], targets: [{ url: "https://app.example.test/me", method: "GET" }], mutates: false, bounds: { requests: 3, minutes: 1, rate: 1 }, actor: { account: "anonymous", role: "anonymous", tenant: "none", permissions: "public", identityEvidence: "not applicable", session: id } };
}

function considerations() { return FAMILIES.map(family => ({ family, reason: "Reviewed against the fixture; no additional applicable surface", action: "not_applicable" })); }

// Controller fixtures simulate the parent's post-spawn record; these are not OS children.
function startSimulated(controller, id) {
  controller.startTask(id);
  controller.recordTaskProcess(id, { pid: process.pid + 1000000 + controller.state.revision, parentPid: process.pid, startedAt: new Date().toISOString(), model: "fixture-only", sessionDir: `specialists/${id}` });
}

function settlePhase(controller, write, phase, blocked = false) {
  for (const item of controller.state.checklist.filter(item => item.phase === phase)) controller.apply("checklist", { id: item.id, status: blocked ? "blocked" : "tested", reason: "Evidence checked", evidence: ["private/evidence.txt"], attempts: "Fixture attempt", nextAction: "Client supplies missing fixture" });
  if (phase !== "setup") {
    const assignment = { ...task(`phase-${phase}`), phase, skill: `.pi/skills/${phase === "auth" ? "pentest-auth" : `pentest-${phase}`}/SKILL.md` };
    controller.createTask(assignment);
    startSimulated(controller, assignment.id);
    write(assignment.output);
    controller.returnedTask(assignment.id, "returned");
    controller.apply("verify_task", { id: assignment.id, notes: "Report and actual underlying evidence checked", evidence: ["private/evidence.txt"], considerations: considerations(), hypotheses: [] });
    if (phase === "recon") for (const skill of ["pentest-js-analysis", "pentest-dependency-confusion"]) {
      const additional = { ...task(`phase-${skill}`), phase, skill: `.pi/skills/${skill}/SKILL.md` };
      controller.createTask(additional);
      startSimulated(controller, additional.id);
      write(additional.output);
      controller.returnedTask(additional.id, "returned");
      controller.apply("verify_task", { id: additional.id, notes: "Checked collected-asset analysis", evidence: ["private/evidence.txt"], considerations: considerations(), hypotheses: [] });
    }
  }
  reflect(controller, write, phase);
}

function reflect(controller, write, phase) {
  const id = `reflect-${phase}-${controller.state.revision}`;
  const assignment = { ...task(id), phase, skill: ".pi/skills/pentest-reflection/SKILL.md" };
  controller.createTask(assignment);
  startSimulated(controller, id);
  write(assignment.output, "Missing / coverage / ambiguity / follow-ups / noise reviewed\n");
  controller.returnedTask(id, "returned");
  controller.apply("verify_task", { id, notes: "Read delegated reflection and checked its evidence", evidence: ["private/evidence.txt"], considerations: considerations(), hypotheses: [] });
  controller.apply("reflection", { taskId: id, phase, missing: "None beyond documented limits", coverage: "Fixture", ambiguities: "None", furtherAction: "No valuable untested gap", noise: "None", hypotheses: [], evidence: ["private/evidence.txt"] });
}

test("mandatory checklist is parsed including multiline continuation and phase mapping", t => {
  const { controller } = fixture(t);
  assert.ok(controller.state.checklist.length > 150);
  assert.ok(controller.state.checklist.some(item => item.title.includes("window") && item.phase === "setup"));
  assert.ok(controller.state.checklist.some(item => item.title.includes("No verified Admin") && item.title.includes("PARTIAL/BLOCKED") && item.phase === "authz"));
  assert.ok(controller.state.checklist.some(item => item.title.includes("inventory is exhausted") && item.phase === "reflection"));
});

test("one checklist batch updates distinct rows atomically with individual evidence and reasons", t => {
  const { controller, root, workspace, write } = fixture(t);
  write("private/second-control.txt", "Distinct second control\n");
  const [first, second] = controller.state.checklist.filter(item => item.phase === "setup");
  const revision = controller.state.revision;
  const items = [
    { id: first.id, status: "tested", reason: "Observed the local authorization fixture", evidence: ["private/evidence.txt"] },
    { id: second.id, status: "blocked", reason: "No fixture account exists", evidence: ["private/second-control.txt"], attempts: "Inspected the supplied role file", nextAction: "Provide an owned fixture account" },
  ];
  controller.apply("checklist", { items });
  assert.equal(controller.state.revision, revision + 1);
  assert.equal(controller.state.phaseRevision.setup, revision + 1);
  assert.equal(controller.state.checklist.find(item => item.id === first.id).status, "tested");
  assert.equal(controller.state.checklist.find(item => item.id === second.id).status, "blocked");
  assert.deepEqual(controller.state.checklist.find(item => item.id === second.id).evidence.map(item => item.path), ["private/second-control.txt"]);
  const events = fs.readFileSync(path.join(root, "events.jsonl"), "utf8").trim().split("\n");
  assert.equal(events.filter(item => JSON.parse(item).event === "checklist").length, 1);
  const before = structuredClone(controller.state);
  assert.throws(() => controller.apply("checklist", { items: [items[0], { ...items[1], nextAction: "" }] }), /client action/i);
  assert.deepEqual(controller.state, before, "a later invalid row left an earlier row changed");
  assert.throws(() => controller.apply("checklist", { items: [items[0], items[0]] }), /unique/);
  assert.throws(() => controller.apply("checklist", { items: [] }), /1–50/);
  assert.deepEqual(new Controller(root, workspace).state.checklist, JSON.parse(JSON.stringify(before.checklist)));
});

test("lead shell polling is blocked only while workers run; useful bash remains available", () => {
  for (const command of ["sleep 120", "sleep 0.5; cd /tmp", " sleep 2 && true"]) {
    assert.equal(isIdleShellPolling("bash", { command }, true), true, command);
    assert.equal(isIdleShellPolling("bash", { command }, false), false, command);
  }
  for (const command of ["python3 analyze.py", "curl --max-time 10 https://example.test", "sleepy-tool", "cd /tmp && ls"]) {
    assert.equal(isIdleShellPolling("bash", { command }, true), false, command);
  }
  assert.equal(isIdleShellPolling("read", { command: "sleep 120" }, true), false);
});

test("phase gates reject missing checklist, stale evidence, and stale reflections transactionally", t => {
  const { controller, write } = fixture(t);
  assert.throws(() => controller.apply("advance"), /Unresolved checklist/);
  assert.equal(controller.state.phase, "setup");
  settlePhase(controller, write, "setup");
  write("private/evidence.txt", "changed evidence");
  assert.throws(() => controller.apply("advance"), /Evidence changed/);
  for (const item of controller.state.checklist.filter(item => item.phase === "setup")) controller.apply("checklist", { id: item.id, status: "tested", reason: "Reverified changed evidence", evidence: ["private/evidence.txt"] });
  for (const assignment of Object.values(controller.state.tasks)) controller.apply("verify_task", { id: assignment.id, notes: "Reverified changed evidence", evidence: ["private/evidence.txt"], considerations: considerations(), hypotheses: [] });
  assert.throws(() => controller.apply("advance"), /reflect again/);
  reflect(controller, write, "setup");
  controller.apply("advance");
  assert.equal(controller.state.phase, "exploration");
  const before = structuredClone(controller.state);
  assert.throws(() => controller.apply("checklist", { id: controller.state.checklist.find(item => item.phase === "exploration").id, status: "blocked", evidence: ["private/evidence.txt"], reason: "missing", attempts: "tried" }), /client action/i);
  assert.deepEqual(controller.state, before, "rejected mutation changed in-memory checkpoint");
});

test("task bookkeeping invalidates reflections only for the task phase", t => {
  const { controller, write } = fixture(t);
  settlePhase(controller, write, "setup");
  const reflection = controller.state.reflections.at(-1);
  const setupRevision = controller.state.phaseRevision.setup;
  controller.createTask({ ...task("future-recon"), phase: "recon" });
  assert.equal(controller.state.phaseRevision.recon, controller.state.revision);
  assert.equal(controller.state.phaseRevision.setup, setupRevision);
  controller.apply("block_task", { id: "future-recon", evidence: ["private/evidence.txt"], reason: "Local fixture cannot test targets", attempts: "Inspected supplied artifacts", nextAction: "Supply authorized target prerequisites" });
  assert.equal(controller.state.phaseRevision.recon, controller.state.revision);
  assert.equal(controller.state.phaseRevision.setup, setupRevision);
  assert.ok(reflection.revision >= setupRevision, "future work leaves the current reflection fresh");
  const reconRevision = controller.state.phaseRevision.recon;
  let previous = setupRevision;
  for (const change of [
    () => controller.createTask(task("late-setup")),
    () => controller.startTask("late-setup"),
    () => controller.recordTaskProcess("late-setup", { pid: process.pid + 1000000, parentPid: process.pid }),
    () => controller.returnedTask("late-setup", "returned", "Simulated fixture execution complete", { exitCode: 0 }),
  ]) {
    change();
    assert.equal(controller.state.phaseRevision.setup, controller.state.revision);
    assert.ok(controller.state.phaseRevision.setup > previous);
    assert.ok(controller.state.phaseRevision.setup > reflection.revision);
    assert.equal(controller.state.phaseRevision.recon, reconRevision);
    previous = controller.state.phaseRevision.setup;
  }
  write("reports/late-setup.md");
  controller.apply("verify_task", { id: "late-setup", notes: "Checked simulated task report and evidence", evidence: ["private/evidence.txt"], considerations: considerations(), hypotheses: [] });
  assert.throws(() => controller.apply("advance"), /reflect again/);
  reflect(controller, write, "setup");
  controller.apply("advance");
  assert.equal(controller.state.phase, "exploration");
});

test("specialist verification needs actual evidence without a rigid follow-up questionnaire", t => {
  const { controller, write } = fixture(t);
  const assignment = task();
  controller.createTask(assignment);
  startSimulated(controller, assignment.id);
  write(assignment.output);
  controller.returnedTask(assignment.id, "returned");
  assert.throws(() => controller.apply("verify_task", { id: assignment.id, notes: "Only summary" }), /report alone/);
  assert.throws(() => controller.apply("verify_task", { id: assignment.id, notes: "Duplicate summary is not raw evidence", evidence: [assignment.output, `./${assignment.output}`] }), /report alone/);
  controller.apply("verify_task", { id: assignment.id, notes: "Checked raw evidence and control", evidence: ["private/evidence.txt"] });
  assert.equal(controller.state.tasks[assignment.id].status, "verified");
});

test("mutations and shared sessions serialize; recovery does not replay interrupted mutations", t => {
  const { controller, root, workspace, write } = fixture(t);
  const mutation = { ...task("mutation"), mutates: true, restoration: "Restore own record and verify fresh identity" };
  controller.createTask(mutation);
  controller.startTask(mutation.id);
  controller.createTask({ ...task("other"), actor: mutation.actor });
  assert.throws(() => controller.startTask("other"), /serialized/);
  const restored = new Controller(root, workspace);
  restored.recover();
  assert.equal(restored.state.mode, "paused");
  assert.equal(restored.state.tasks.mutation.status, "interrupted");
  assert.throws(() => restored.apply("run"), /Reconcile/);
  assert.throws(() => restored.startTask("mutation"), /reconciled/);
  restored.apply("reconcile_task", { id: "mutation", notes: "Read server state; mutation was completed and restored", evidence: ["private/evidence.txt"] });
  assert.equal(restored.state.tasks.mutation.status, "pending");
  write(mutation.output);
  startSimulated(restored, "mutation");
  restored.returnedTask("mutation", "returned");
  assert.throws(() => restored.apply("verify_task", { id: "mutation", notes: "checked", evidence: ["private/evidence.txt"], considerations: considerations(), hypotheses: [] }), /evidence file/);
});

test("scope updates preserve unresolved setup gates and reopen exploration only from later phases", t => {
  const { controller, scope } = fixture(t);
  controller.createTask(task("unfinished-setup"));
  const checklist = structuredClone(controller.state.checklist);
  controller.apply("scope_update", { ...scope, actions: "Updated authorization; test-owned records only" });
  assert.equal(controller.state.phase, "setup");
  assert.equal(controller.state.tasks["unfinished-setup"].status, "pending");
  assert.deepEqual(controller.state.checklist, checklist);
  assert.equal(controller.state.phases.setup, undefined);
  assert.throws(() => controller.apply("advance"), /Unresolved checklist/);
  assert.equal(controller.state.phase, "setup");
  controller.state.phase = "auth";
  controller.apply("scope_update", scope);
  assert.equal(controller.state.phase, "exploration");
});

test("reconciliation rejects empty, non-file, escaping, and dangling reports transactionally", t => {
  for (const kind of ["empty", "directory", "outside", "dangling"]) {
    const { controller, root, workspace, write } = fixture(t);
    const assignment = controller.createTask(task(`invalid-${kind}`));
    startSimulated(controller, assignment.id);
    controller.returnedTask(assignment.id, "failed", "Interrupted fixture output");
    const output = path.join(workspace, assignment.output);
    if (kind === "empty") write(assignment.output, "");
    else if (kind === "directory") fs.mkdirSync(output);
    else {
      const destination = path.join(root, "outside-report.md");
      if (kind === "outside") fs.writeFileSync(destination, "Outside workspace");
      fs.symlinkSync(destination, output);
    }
    const before = structuredClone(controller.state);
    const events = fs.readFileSync(path.join(root, "events.jsonl"), "utf8");
    assert.throws(() => controller.apply("reconcile_task", { id: assignment.id, notes: "Inspected final state; output is incomplete", evidence: ["private/evidence.txt"] }), /nonempty file|escapes|ENOENT/, kind);
    assert.deepEqual(controller.state, before, kind);
    assert.deepEqual(JSON.parse(fs.readFileSync(controller.file, "utf8")), before, kind);
    assert.equal(fs.readFileSync(path.join(root, "events.jsonl"), "utf8"), events, kind);
    assert.equal(controller.state.tasks[assignment.id].status, "failed");
  }
});

test("a genuine recovered report requires process provenance and still awaits evidence verification", t => {
  const { controller, root, write } = fixture(t);
  const assignment = controller.createTask(task("recovered-report"));
  controller.startTask(assignment.id);
  controller.returnedTask(assignment.id, "interrupted", "Fixture stopped before recording a PID");
  write(assignment.output, "Completed specialist report with raw evidence references");
  const data = { id: assignment.id, notes: "Checked final state and recovered report", evidence: ["private/evidence.txt"] };
  const before = structuredClone(controller.state);
  assert.throws(() => controller.apply("reconcile_task", data), /execution provenance/);
  assert.deepEqual(controller.state, before);
  assert.deepEqual(JSON.parse(fs.readFileSync(controller.file, "utf8")), before);
  const dir = path.join(root, "specialists", assignment.id);
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, "transcript.jsonl"), '{"fixture":"legacy completed execution"}\n');
  controller.apply("reconcile_task", data);
  const recovered = controller.state.tasks[assignment.id];
  assert.equal(recovered.status, "returned");
  assert.equal(recovered.reconciliation.notes, data.notes);
  assert.equal(recovered.reconciliation.report.path, assignment.output);
  assert.equal(recovered.reconciliation.report.sha256.length, 64);
  controller.apply("verify_task", { id: assignment.id, notes: "Checked recovered report and raw evidence", evidence: ["private/evidence.txt"], considerations: considerations(), hypotheses: [] });
  assert.equal(controller.state.tasks[assignment.id].status, "verified");
});

test("explicit retries preserve partial reports and bounded mutation contracts without replaying them", t => {
  for (const partial of ["", "Incomplete specialist report; retained for investigation\n"]) {
    const { controller, workspace, write } = fixture(t);
    const contract = { ...task("partial-mutation"), mutates: true, restoration: "Restore the owned record; verify final server identity", stateKeys: ["owned-record"], inputs: ["private/evidence.txt"] };
    controller.createTask(contract);
    startSimulated(controller, contract.id);
    controller.returnedTask(contract.id, "failed", "Fixture specialist stopped");
    write(contract.output, partial);
    const originalExecution = structuredClone(controller.state.tasks[contract.id].execution);
    const revision = controller.state.revision;
    const notes = "Observed server state: owned record restored; fresh identity still matches the authorized tester";
    write("private/reconciliation.txt", notes);
    const summary = controller.apply("reconcile_task", { id: contract.id, disposition: "retry", notes, evidence: ["private/evidence.txt", "private/reconciliation.txt"] });
    const original = controller.state.tasks[contract.id];
    const replacement = controller.state.tasks[original.supersededBy];
    assert.equal(controller.state.revision, revision + 1, "both jobs are committed in one reconciliation save");
    assert.equal(original.status, "blocked");
    assert.match(original.reason, /Superseded by explicit retry/);
    assert.ok(original.attempts && original.nextAction && original.evidence.length);
    assert.deepEqual(original.execution, originalExecution);
    assert.equal(replacement.retryOf, original.id);
    assert.notEqual(replacement.id, original.id);
    assert.notEqual(replacement.output, original.output);
    assert.equal(replacement.status, "pending");
    assert.equal(replacement.execution, null);
    assert.equal(replacement.startedAt, undefined);
    assert.equal(replacement.finishedAt, undefined);
    assert.equal(fs.existsSync(path.join(workspace, replacement.output)), false);
    assert.equal(fs.readFileSync(path.join(workspace, original.output), "utf8"), partial);
    for (const key of Object.keys(contract).filter(key => !["id", "output", "inputs"].includes(key))) assert.deepEqual(replacement[key], contract[key], key);
    assert.deepEqual(replacement.inputs, ["private/evidence.txt", "private/reconciliation.txt"]);
    assert.equal(replacement.reconciliation.notes, notes);
    assert.equal(replacement.reconciliation.disposition, "retry");
    assert.equal(replacement.reconciliation.evidence[0].path, "private/evidence.txt");
    assert.equal(summary.tasks.find(item => item.id === original.id).supersededBy, replacement.id);
    assert.equal(summary.tasks.find(item => item.id === replacement.id).retryOf, original.id);
    assert.throws(() => controller.startTask(original.id), /pending/);
    settlePhase(controller, write, "setup");
    assert.throws(() => controller.apply("advance"), /unverified, pending/);
    startSimulated(controller, replacement.id);
    write(replacement.output, "Completed replacement report with restored state evidence");
    controller.returnedTask(replacement.id, "returned");
    controller.apply("verify_task", { id: replacement.id, notes: "Checked replacement control, evidence, and restored state", evidence: ["private/evidence.txt"], restorationEvidence: ["private/evidence.txt"], finalIdentity: "Authorized tester identity and original record state confirmed", considerations: considerations(), hypotheses: [] });
    reflect(controller, write, "setup");
    controller.apply("advance");
    assert.equal(controller.state.phase, "exploration", "retired task does not deadlock completion after replacement verification");
    assert.equal(fs.readFileSync(path.join(workspace, original.output), "utf8"), partial);
  }
});

test("pending dependents follow two retries and become ready only after the latest replacement is verified", t => {
  const { controller, root, workspace, write } = fixture(t);
  const offline = (id, fields = {}) => ({ ...task(id), network: false, targets: [], bounds: { requests: 0, rate: 0, minutes: 2 }, ...fields });
  let source = controller.createTask(offline("retry-source"));
  startSimulated(controller, source.id);
  write(source.output, "Original partial report\n");
  controller.returnedTask(source.id, "failed", "Fixture interrupted");
  const dependentID = "queued-dependent";
  controller.createTask(offline(dependentID, { dependencies: [source.id], inputs: [`./${source.output}`, "private/evidence.txt"] }));
  controller.createTask(offline("future-dependent", { phase: "auth", dependencies: [source.id], inputs: [source.output] }));
  controller.createTask(offline("input-only", { inputs: [source.output] }));
  const immutable = {};
  for (const status of ["returned", "verified", "running"]) {
    const reference = controller.createTask(offline(`reference-${status}`, { inputs: [source.output] }));
    startSimulated(controller, reference.id);
    if (status !== "running") {
      write(reference.output);
      controller.returnedTask(reference.id, "returned", "Fixture report complete");
      if (status === "verified") controller.apply("verify_task", { id: reference.id, notes: "Checked fixture report and raw evidence", evidence: ["private/evidence.txt"], considerations: considerations(), hypotheses: [] });
    }
    immutable[reference.id] = structuredClone(controller.state.tasks[reference.id]);
  }
  const preservedReports = new Map();
  for (let attempt = 0; attempt < 2; attempt++) {
    const retiredID = source.id;
    const retiredOutput = source.output;
    preservedReports.set(retiredOutput, fs.readFileSync(path.join(workspace, retiredOutput), "utf8"));
    const evidence = ["private/evidence.txt", retiredOutput];
    const revision = controller.state.revision;
    controller.apply("reconcile_task", { id: retiredID, disposition: "retry", notes: "Offline final state reconciled; partial output preserved", evidence });
    source = controller.state.tasks[controller.state.tasks[retiredID].supersededBy];
    assert.equal(controller.state.revision, revision + 1, "retirement, replacement, and rebinds share one save");
    const dependent = controller.state.tasks[dependentID];
    assert.deepEqual(dependent.dependencies, [source.id]);
    assert.deepEqual(dependent.inputs, [source.output, "private/evidence.txt"]);
    assert.deepEqual(controller.state.tasks["future-dependent"].dependencies, [source.id]);
    assert.deepEqual(controller.state.tasks["future-dependent"].inputs, [source.output]);
    assert.equal(controller.state.phaseRevision.auth, controller.state.revision);
    assert.deepEqual(controller.state.tasks["input-only"].inputs, [source.output]);
    assert.equal(dependent.retryRebinds.length, attempt + 1);
    assert.deepEqual(dependent.retryRebinds.at(-1), { fromTaskId: retiredID, toTaskId: source.id, fromOutput: retiredOutput, toOutput: source.output, dependenciesUpdated: true, inputsUpdated: true, revision: controller.state.revision });
    assert.match(controller.taskReadiness(dependentID).reason, /dependencies are not verified/);
    assert.throws(() => controller.startTask(dependentID), /dependencies are not verified/);
    for (const [id, contract] of Object.entries(immutable)) assert.deepEqual(controller.state.tasks[id], contract);
    assert.ok(source.inputs.includes(retiredOutput), "replacement retains the partial report as reconciliation evidence input");
    assert.deepEqual(source.reconciliation.evidence.map(entry => entry.path), evidence);
    assert.deepEqual(controller.state.tasks[retiredID].reconciliation.evidence.map(entry => entry.path), evidence);
    startSimulated(controller, source.id);
    write(source.output, attempt === 0 ? "First retry partial report\n" : "Latest completed report\n");
    controller.returnedTask(source.id, attempt === 0 ? "failed" : "returned", "Simulated fixture outcome");
    assert.match(controller.taskReadiness(dependentID).reason, /dependencies are not verified/);
  }
  controller.apply("verify_task", { id: source.id, notes: "Checked latest replacement report and raw evidence", evidence: ["private/evidence.txt"], considerations: considerations(), hypotheses: [] });
  assert.deepEqual(controller.taskReadiness(dependentID), { ready: true });
  assert.deepEqual(controller.state.tasks[dependentID].inputs, [source.output, "private/evidence.txt"]);
  const restored = new Controller(root, workspace);
  assert.deepEqual(restored.taskReadiness(dependentID), { ready: true });
  assert.deepEqual(restored.state.tasks[dependentID], controller.state.tasks[dependentID]);
  for (const [id, contract] of Object.entries(immutable)) assert.deepEqual(controller.state.tasks[id], contract);
  for (const [output, content] of preservedReports) assert.equal(fs.readFileSync(path.join(workspace, output), "utf8"), content);
});

test("retry preparation failures leave the original checkpoint and partial report untouched", t => {
  const { controller, root, workspace, write } = fixture(t);
  const assignment = controller.createTask(task("retry-validation"));
  startSimulated(controller, assignment.id);
  controller.returnedTask(assignment.id, "failed", "Fixture specialist stopped");
  write(assignment.output, "Keep original partial report");
  controller.createTask({ ...task("pending-validation-dependent"), dependencies: [assignment.id], inputs: [assignment.output] });
  fs.unlinkSync(path.join(workspace, assignment.skill));
  const before = structuredClone(controller.state);
  const events = fs.readFileSync(path.join(root, "events.jsonl"), "utf8");
  assert.throws(() => controller.apply("reconcile_task", { id: assignment.id, disposition: "retry", notes: "Observed state is unchanged", evidence: ["private/evidence.txt"] }), /ENOENT/);
  assert.deepEqual(controller.state, before);
  assert.deepEqual(JSON.parse(fs.readFileSync(controller.file, "utf8")), before);
  assert.equal(fs.readFileSync(path.join(root, "events.jsonl"), "utf8"), events);
  assert.equal(fs.readFileSync(path.join(workspace, assignment.output), "utf8"), "Keep original partial report");
});

test("explicit retry can retire an already returned partial report without accepting it as evidence", t => {
  const { controller, workspace, write } = fixture(t);
  const assignment = controller.createTask(task("legacy-partial"));
  startSimulated(controller, assignment.id);
  write(assignment.output, "");
  controller.returnedTask(assignment.id, "returned", "Legacy runtime returned a partial report");
  controller.apply("reconcile_task", { id: assignment.id, disposition: "retry", notes: "Checked observed final state; returned report is empty", evidence: ["private/evidence.txt"] });
  const original = controller.state.tasks[assignment.id];
  assert.equal(original.status, "blocked");
  assert.equal(controller.state.tasks[original.supersededBy].status, "pending");
  assert.equal(fs.readFileSync(path.join(workspace, assignment.output), "utf8"), "");
});

test("reconciliation without an output retains compatible pending redispatch at the unused path", t => {
  const { controller, workspace } = fixture(t);
  const assignment = controller.createTask(task("missing-output"));
  controller.startTask(assignment.id);
  controller.returnedTask(assignment.id, "failed", "Fixture spawn failed");
  controller.apply("reconcile_task", { id: assignment.id, notes: "No process executed and observed state is unchanged", evidence: ["private/evidence.txt"] });
  assert.equal(Object.keys(controller.state.tasks).length, 1);
  assert.equal(controller.state.tasks[assignment.id].status, "pending");
  assert.equal(controller.state.tasks[assignment.id].output, assignment.output);
  assert.equal(fs.existsSync(path.join(workspace, assignment.output)), false);
  assert.equal(controller.state.phaseRevision.setup, controller.state.revision);
  controller.startTask(assignment.id);
  assert.equal(controller.state.tasks[assignment.id].status, "running");
});

test("new role reopens coverage; intermediate roles and pending authz cells remain gaps", t => {
  const { controller } = fixture(t);
  controller.state.phase = "endpoint-fuzz-full";
  controller.state.phases.authz = { completedAt: "fixture" };
  controller.apply("role", { id: "approver", label: "Approver", policy: "UNKNOWN" });
  assert.equal(controller.state.phase, "exploration");
  assert.equal(controller.state.phases.authz, undefined);
  assert.ok(controller.unresolvedCoverage("authz").some(gap => gap.includes("approver")));
  controller.apply("authz_cell", { id: "approve-peer", role: "approver", target: "peer record", tenant: "own tenant", action: "approve", status: "pending" });
  assert.ok(controller.unresolvedCoverage("authz").some(gap => gap.includes("approve-peer")));
});

test("hypothesis loop requires verdicts; blocked remains visibly partial", t => {
  const { controller, write, workspace } = fixture(t);
  controller.apply("hypothesis", { id: "sibling", question: "Sibling boundary?", basis: "Observed fixture", test: "One owned sibling", phase: "setup", evidence: ["private/evidence.txt"] });
  settlePhase(controller, write, "setup");
  assert.throws(() => controller.apply("advance"), /hypotheses first/);
  controller.apply("resolve_hypothesis", { id: "sibling", verdict: "BLOCKED", reason: "No authorized peer", evidence: ["private/evidence.txt"], nextAction: "Client supplies a peer account" });
  const item = controller.state.checklist.find(item => item.phase === "setup");
  controller.apply("checklist", { id: item.id, status: "blocked", reason: "No peer", attempts: "Requested account", nextAction: "Client provisions account", evidence: ["private/evidence.txt"] });
  assert.match(fs.readFileSync(path.join(workspace, "reports/checklist.md"), "utf8"), /\[~\].*Client provisions account/);
  assert.match(fs.readFileSync(path.join(workspace, "reports/hypotheses.md"), "utf8"), /BLOCKED/);
});

test("full phase loop can deliver limited coverage without marking blocked checklist items passed", t => {
  const { controller, write } = fixture(t);
  controller.apply("role", { id: "anonymous", label: "Anonymous" });
  for (const area of ["account", "crawl", "authz"]) controller.apply("role", { id: "anonymous", area, status: "tested", reason: "Verified unauthenticated fixture", evidence: ["private/evidence.txt"] });
  controller.apply("authz_cell", { id: "public", role: "anonymous", target: "public record", tenant: "none", action: "read", status: "blocked", reason: "Fixture restriction", attempts: "One read", nextAction: "Client supplies dynamic app", evidence: ["private/evidence.txt"] });
  for (const output of ["reports/accounts.md", "reports/role-matrix.md", "reports/app-overview.md", "reports/triage.md", "recon/api-inventory.md", "recon/web-logic.md", "recon/business-logic.md", "recon/js-history.md", "recon/js-lazy.md", "reports/authz-matrix.md", "reports/validation-ledger.md", "fixture_PENTEST_REPORT.md", "fixture_VALIDATION_REASON.md"]) write(output, output.endsWith("triage.md") ? "CONTINUE: dynamic fixture" : "Verified fixture artifact with explicit limits");
  for (const phase of PHASES) {
    assert.equal(controller.state.phase, phase);
    settlePhase(controller, write, phase, phase === "authz");
    controller.apply("advance");
  }
  assert.equal(controller.state.mode, "limited");
  assert.ok(controller.state.checklist.some(item => item.status === "blocked"));
});

test("CVSS scores are calculated and missing runtime proof cannot validate candidates", t => {
  assert.equal(cvss31("CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"), 9.8);
  assert.equal(cvss31("CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:N/A:N"), 6.5);
  assert.equal(cvss31("CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:N"), 0);
  assert.throws(() => cvss31("CVSS:3.1/AV:N"), /Invalid/);
  const { controller } = fixture(t);
  assert.throws(() => controller.apply("candidate", { id: "candidate", verdict: "VALIDATED", reason: "Static clue", evidence: ["private/evidence.txt"] }), /reproduced/);
});

test("shared budget reservations enforce $15, deduplicate usage, and pause unknown pricing", t => {
  const { root } = fixture(t);
  const ledger = path.join(root, "launch.json");
  atomicJSON(ledger, { budgetUSD: 15, spentUSD: 0, reservations: {}, events: {} });
  const first = new Budget(ledger, root), second = new Budget(ledger, root);
  const model = { provider: "test", id: "model", contextWindow: 1000000, maxTokens: 1000000, cost: { input: 1, output: 1, cacheRead: 0, cacheWrite: 0 } };
  const a = first.reserve(model, {}), b = second.reserve(model, {});
  assert.equal(first.status().reservedUSD, 4);
  first.settle(a, { usage: { input: 100, output: 20, cacheRead: 0, cacheWrite: 0, cost: { total: .5 } } });
  first.settle(a, { usage: { input: 100, output: 20, cacheRead: 0, cacheWrite: 0, cost: { total: .5 } } });
  assert.equal(second.status().spentUSD, .5);
  second.settle(b, {});
  assert.equal(first.status().spentUSD, 2.5, "unmetered request was silently free");
  assert.throws(() => first.reserve({ ...model, contextWindow: 20000000 }, {}), /cannot fund/);
  const zero = { ...model, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } };
  assert.throws(() => first.reserve(zero, {}), /Zero\/unknown pricing/);
  first.setPricing(zero, [0, 0, 0, 0]);
  assert.ok(first.reserve(zero, {}));
  const fresh = path.join(root, "second-launch.json");
  atomicJSON(fresh, { budgetUSD: 15, spentUSD: 0, reservations: {}, events: {} });
  assert.equal(new Budget(fresh, root).status().spentUSD, 0);
  assert.equal(first.status().spentUSD, 2.5);
});

test("managed request scheduler paces concurrent callers and honors cancellation", async t => {
  const { root } = fixture(t);
  const times = [];
  await Promise.all(Array.from({ length: 8 }, async () => { await scheduleRequest(root, 50); times.push(Date.now()); }));
  times.sort((a, b) => a - b);
  assert.ok(times.at(-1) - times[0] >= 130, `requests were not paced: ${times}`);
  const abort = new AbortController(); abort.abort();
  await assert.rejects(scheduleRequest(root, 50, abort.signal), /aborted/);
  await assert.rejects(scheduleRequest(root, 51), /no more than 50/);
});

test("separate processes cannot race past the shared launch budget", async t => {
  const { root } = fixture(t);
  const ledger = path.join(root, "launch-race.json");
  atomicJSON(ledger, { budgetUSD: 15, spentUSD: 0, reservations: {}, events: {} });
  const attempts = await Promise.all(Array.from({ length: 4 }, () => new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [fileURLToPath(new URL("./budget-worker.mjs", import.meta.url)), ledger, root]);
    let output = "";
    child.stdout.on("data", chunk => { output += chunk; });
    child.once("error", reject);
    child.once("close", code => { if (code !== 0) reject(new Error(`worker exited ${code}`)); else resolve(JSON.parse(output)); });
  })));
  assert.equal(attempts.filter(attempt => attempt.reserved).length, 3);
  assert.equal(attempts.filter(attempt => !attempt.reserved).length, 1);
  assert.equal(new Budget(ledger, root).status().reservedUSD, 12);
});

test("artifact paths cannot escape workspace and corrupted state is never reset", t => {
  const { controller, root, workspace } = fixture(t);
  fs.writeFileSync(path.join(root, "outside.txt"), "outside");
  assert.throws(() => artifact(workspace, "../outside.txt"), /escapes/);
  fs.writeFileSync(controller.file, "{broken");
  assert.throws(() => new Controller(root, workspace), /Cannot read checkpoint/);
  assert.equal(fs.readFileSync(controller.file, "utf8"), "{broken");
});

test("unreconciled orchestrator tool calls prevent automatic resume", t => {
  const { controller, root } = fixture(t);
  const record = path.join(root, "private", "inflight", "old-process.json");
  atomicJSON(record, { pid: process.pid + 1000000, task: null, calls: [{ id: "uncertain", name: "bash" }] });
  assert.throws(() => controller.apply("run"), /interrupted tool calls/);
  controller.apply("reconcile_tools", { notes: "Checked actual server state and restored the tester-owned record", evidence: ["private/evidence.txt"] });
  controller.apply("run");
  assert.equal(controller.state.mode, "running");
});

test("managed scope rejects sibling hosts, methods, and expired windows", t => {
  const { controller, scope } = fixture(t);
  controller.checkScope("https://app.example.test/me", "GET");
  assert.throws(() => controller.checkScope("https://app.example.test.attacker.invalid/me", "GET"), /outside scope/);
  assert.throws(() => controller.checkScope("https://app.example.test/me", "DELETE"), /not permitted/);
  controller.apply("scope_update", { ...scope, expiresAt: "2000-01-01T00:00:00Z" });
  assert.throws(() => controller.checkScope("https://app.example.test/me", "GET"), /expired/);
});

test("scope accepts the stated hostname or URL without an optional-details questionnaire", t => {
  const { controller } = fixture(t, { withScope: false });
  controller.apply("run");
  assert.equal(controller.state.mode, "running", "offline orchestration can start immediately");
  controller.apply("scope", { target: "example.test" });
  assert.deepEqual(controller.state.scope.origins, ["https://example.test"]);
  controller.checkScope("https://example.test/api", "POST");
  controller.apply("scope_update", { origins: ["https://example.test/app/path", "api.example.test"] });
  assert.deepEqual(controller.state.scope.origins, ["https://example.test", "https://api.example.test"]);
  controller.checkScope("https://api.example.test/v1", "PATCH");
});

test("partial scope updates preserve authorization and previously recorded hosts", t => {
  const { controller, scope } = fixture(t);
  const before = structuredClone(controller.state.scope);
  controller.apply("scope_update", { origins: ["https://api.example.test/v1"] });
  assert.deepEqual(controller.state.scope.origins, [...before.origins, "https://api.example.test"]);
  for (const field of ["target", "authorization", "window", "actions", "methods", "evidence"]) assert.deepEqual(controller.state.scope[field], before[field], field);
  const revision = controller.state.phaseRevision.setup;
  controller.apply("scope_update", { origins: ["https://api.example.test"] });
  assert.equal(controller.state.phaseRevision.setup, revision, "a repeated origin update is not new coverage");
  controller.apply("scope_update", { ...scope, replaceOrigins: true });
  assert.deepEqual(controller.state.scope.origins, before.origins);
});

test("starting a task creates its private report directory before the worker exists", t => {
  const { controller, workspace } = fixture(t);
  const assignment = controller.createTask({ id: "new-area", network: false, phase: "recon",
    question: "Review local source", skill: ".pi/skills/pentest-recon/SKILL.md", output: "recon/new-area/report.md" });
  const dir = path.join(workspace, "recon/new-area");
  assert.equal(fs.existsSync(dir), false);
  controller.startTask(assignment.id);
  assert.equal(fs.statSync(dir).isDirectory(), true);
  assert.equal(fs.existsSync(path.join(dir, "report.md")), false);
});

test("prompt checkpoint is bounded while full status keeps execution evidence", t => {
  const { controller } = fixture(t);
  const assignment = controller.createTask({ id: "context", network: false, question: "Review local source", skill: ".pi/skills/pentest-recon/SKILL.md" });
  const veryLarge = "private-run-proof-".repeat(3000);
  controller.state.tasks[assignment.id].execution = { pid: 12345, transcriptPath: veryLarge, processTerminal: { proof: veryLarge } };
  const full = JSON.stringify(controller.apply("status", { full: true }));
  const short = JSON.stringify(controller.promptSummary());
  const routine = JSON.stringify(controller.summary());
  assert.ok(full.length > 100000);
  assert.ok(short.length < 10000);
  assert.ok(routine.length < 10000, "routine workflow results must not embed native process proofs");
  assert.equal(short.includes("private-run-proof"), false);
  assert.equal(controller.promptSummary().tasks[0].pid, 12345);
});

test("batch contracts validate atomically and admit eight independent workers", t => {
  const { controller } = fixture(t);
  const before = structuredClone(controller.state);
  assert.throws(() => controller.createTasks([task("batch-a"), task("batch-a")]), /unique/);
  assert.deepEqual(controller.state, before);
  const tasks = controller.createTasks(Array.from({ length: 8 }, (_, i) => task(`batch-${i}`)));
  for (const task of tasks) controller.startTask(task.id);
  assert.equal(Object.values(controller.state.tasks).filter(task => task.status === "running").length, 8);
});

test("future offline reviews run during setup but still wait for actual dependencies", t => {
  const { controller, write } = fixture(t, { withScope: false });
  returnedFixture(controller, write, "source-input", true);
  const review = controller.createTask({ id: "offline-recon", phase: "recon", network: false,
    question: "Review the supplied source", skill: ".pi/skills/pentest-recon/SKILL.md", dependencies: ["source-input"] });
  assert.match(controller.taskReadiness(review.id).reason, /dependencies/);
  controller.apply("verify_task", { id: "source-input", notes: "Checked the local input", evidence: ["private/evidence.txt"] });
  assert.equal(controller.taskReadiness(review.id).ready, true);
  controller.startTask(review.id);
  assert.equal(controller.state.scope, null);
  assert.equal(controller.state.phase, "setup");
  assert.equal(controller.state.tasks[review.id].phase, "recon");
  assert.equal(controller.state.tasks[review.id].status, "running");
});

test("public read-only discovery starts during setup, while future writes remain gated", t => {
  const { controller } = fixture(t);
  const publicRead = controller.createTask({ ...task("future-public"), phase: "recon" });
  assert.equal(controller.taskReadiness(publicRead.id).ready, true);
  controller.startTask(publicRead.id);
  const writeTask = controller.createTask({ ...task("future-write"), phase: "recon", targets: [{ url: "https://app.example.test/orders", method: "PATCH" }], mutates: true, restoration: "Restore test-owned order" });
  assert.match(controller.taskReadiness(writeTask.id).reason, /phase recon/);
  const authTask = controller.createTask({ ...task("future-authenticated"), phase: "auth", actor: { account: "tester", role: "viewer", tenant: "local", permissions: "self", identityEvidence: "private/evidence.txt", session: "tester-isolated" } });
  assert.match(controller.taskReadiness(authTask.id).reason, /phase auth/);
});

test("minimal offline contracts run target-free in setup without scope and with zero network budgets", t => {
  const { controller } = fixture(t, { withScope: false });
  const minimal = { id: "offline", question: "Review supplied local artifacts", skill: ".pi/skills/pentest-reflection/SKILL.md" };
  const assignment = controller.createTask(minimal);
  assert.equal(assignment.phase, "setup");
  assert.equal(assignment.output, "reports/offline.md");
  assert.equal(assignment.network, false);
  assert.equal(assignment.mutates, false);
  assert.deepEqual(assignment.targets, []);
  assert.deepEqual(assignment.bounds, { requests: 0, rate: 0, minutes: 10 });
  for (const field of ["inputs", "dependencies", "stateKeys"]) assert.deepEqual(assignment[field], []);
  for (const field of ["method", "success", "report", "stop", "restoration"]) assert.ok(assignment[field].trim());
  assert.deepEqual({ ...assignment.actor, session: "unique" }, { account: "local", role: "local", tenant: "none", permissions: "local artifacts", identityEvidence: "not-applicable", session: "unique" });
  assert.deepEqual(controller.taskReadiness(assignment), { ready: true });
  controller.startTask(assignment.id);
  const explicit = controller.createTask({ ...minimal, id: "offline-explicit", network: false, bounds: { requests: 0, rate: 0, minutes: 2 } });
  assert.deepEqual(explicit.bounds, { requests: 0, rate: 0, minutes: 2 });
  assert.notEqual(explicit.actor.session, assignment.actor.session);
  controller.startTask(explicit.id);
});

test("network defaults preserve explicit contracts, infer offline zero requests, and never invent authenticated identities", t => {
  const { controller } = fixture(t);
  const minimal = { id: "network", question: "Check anonymous surface", skill: ".pi/skills/pentest-auth/SKILL.md", targets: task().targets };
  const assignment = controller.createTask(minimal);
  assert.equal(assignment.network, true);
  assert.deepEqual(assignment.bounds, { requests: 20, rate: 1, minutes: 10 });
  assert.deepEqual({ ...assignment.actor, session: "unique" }, { account: "anonymous", role: "anonymous", tenant: "none", permissions: "unauthenticated", identityEvidence: "not-applicable", session: "unique" });
  const original = task("preserved");
  const preserved = controller.createTask(original);
  for (const key of Object.keys(original)) assert.deepEqual(preserved[key], original[key], key);
  const offline = controller.createTask({ ...minimal, id: "zero-requests", bounds: { requests: 0, rate: 0 } });
  assert.equal(offline.network, false);
  assert.equal(offline.actor.account, "local");
  assert.deepEqual(offline.bounds, { requests: 0, rate: 0, minutes: 10 });
  assert.throws(() => controller.createTask({ ...minimal, id: "missing-identity", actor: { account: "tester-owned" } }), /actor.role/);
  assert.throws(() => controller.createTask({ ...minimal, id: "false-anonymous", actor: { role: "admin" } }), /authenticated role/);
  assert.throws(() => controller.createTask({ ...minimal, id: "false-local", network: false, actor: { role: "admin" } }), /authenticated role/);
  const actor = { account: "tester-owned", role: "reviewer", tenant: "test-tenant", permissions: "read owned records", identityEvidence: "private/evidence.txt", session: "isolated-reviewer" };
  const authenticated = controller.createTask({ ...minimal, id: "authenticated", actor });
  assert.deepEqual(authenticated.actor, actor);
  assert.throws(() => controller.createTask({ ...minimal, id: "absent-evidence", actor: { ...actor, identityEvidence: "private/missing.txt" } }), /ENOENT/);
});

test("contract validation is structural, preserves files, and canonicalizes output uniqueness", t => {
  const { controller, workspace, root, write } = fixture(t);
  const minimal = { id: "structural", question: "Local inspection", skill: ".pi/skills/pentest-auth/SKILL.md" };
  const reject = (fields, pattern) => {
    const before = structuredClone(controller.state);
    assert.throws(() => controller.createTask({ ...minimal, ...fields }), pattern);
    assert.deepEqual(controller.state, before);
    assert.deepEqual(JSON.parse(fs.readFileSync(controller.file, "utf8")), before);
  };
  reject({ mutates: true }, /Explicit restoration/);
  reject({ mutates: "false" }, /boolean/);
  reject({ inputs: "private/evidence.txt" }, /inputs/);
  reject({ dependencies: [null] }, /dependencies/);
  reject({ stateKeys: [{}] }, /stateKeys/);
  reject({ phase: "unknown" }, /phase/);
  reject({ targets: {} }, /targets/);
  reject({ targets: [{ url: "https://app.example.test/me" }] }, /target.method/);
  reject({ targets: [{ url: "https://*.example.test/me", method: "GET" }] }, /exact/);
  reject({ network: true }, /explicit targets/);
  reject({ network: true, targets: task().targets, bounds: { requests: 0 } }, /positive/);
  reject({ network: true, targets: task().targets, bounds: { requests: 1.5 } }, /integer/);
  reject({ network: true, targets: task().targets, bounds: { rate: 51 } }, /rate/);
  for (const minutes of [0, -1, Infinity, "10"]) reject({ bounds: { minutes } }, /time bounds/);
  reject({ skill: "private/evidence.txt" }, /SKILL.md/);
  reject({ skill: ".pi/skills/missing/SKILL.md" }, /ENOENT/);
  reject({ output: "reports/../../outside.md" }, /output/);
  write("reports/existing.md", "Preserve user content");
  reject({ output: "reports/existing.md" }, /already exists/);
  assert.equal(fs.readFileSync(path.join(workspace, "reports/existing.md"), "utf8"), "Preserve user content");
  const portable = controller.createTask({ ...minimal, id: "first-path", output: "reports\\nested\\..\\unique.md" });
  assert.equal(portable.output, "reports/unique.md");
  reject({ output: "reports/nested/../unique.md" }, /unique output/);
  reject({ output: "reports/unique.md" }, /unique output/);
  fs.symlinkSync(path.join(workspace, "reports"), path.join(workspace, "recon"), "dir");
  reject({ output: "recon/unique.md" }, /unique output/);
  fs.symlinkSync(root, path.join(workspace, "reports/outside"), "dir");
  reject({ output: "reports/outside/escape.md" }, /escapes/);
});

test("future-phase, dependency, and network scope checks happen at readiness/start without changing queued tasks", t => {
  const { controller, write, scope } = fixture(t, { withScope: false });
  const future = controller.createTask({ ...task("queued-recon"), phase: "recon", dependencies: ["setup-control"], mutates: true, restoration: "Restore own fixture" });
  controller.createTask({ ...task("queued-auth"), phase: "auth" });
  const before = structuredClone(controller.state);
  assert.match(controller.taskReadiness(future.id).reason, /phase recon/);
  assert.throws(() => controller.startTask(future.id), /phase recon/);
  assert.deepEqual(controller.state, before);
  controller.state.phase = "recon";
  assert.match(controller.taskReadiness(future).reason, /dependencies/);
  controller.state.phase = "setup";
  returnedFixture(controller, write, "setup-control", true);
  controller.apply("verify_task", { id: "setup-control", notes: "Verified offline prerequisite evidence", evidence: ["private/evidence.txt"], considerations: considerations(), hypotheses: [] });
  controller.state.phase = "recon";
  assert.match(controller.taskReadiness(future).reason, /scope/);
  assert.equal(future.status, "pending");
  assert.deepEqual(controller.taskReadiness("missing"), { ready: false, reason: "Unknown task" });
  controller.apply("scope", scope);
  assert.deepEqual(controller.taskReadiness(future.id), { ready: true });
  controller.startTask(future.id);
  const outside = controller.createTask({ ...task("outside"), phase: "recon", targets: [{ url: "https://outside.example.test/me", method: "GET" }] });
  assert.match(controller.taskReadiness(outside.id).reason, /outside scope/);
  assert.throws(() => controller.startTask(outside.id), /outside scope/);
});

test("readiness serializes shared sessions and state and enforces the specialist concurrency cap", t => {
  const { controller } = fixture(t);
  controller.createTask(task("first"));
  controller.startTask("first");
  const shared = controller.createTask({ ...task("shared"), actor: task("first").actor });
  assert.equal(controller.taskReadiness(shared.id).ready, true, "read-only session sharing can run in parallel");
  controller.state.tasks.first.mutates = true;
  assert.match(controller.taskReadiness(shared.id).reason, /sharing an actor session/);
  controller.state.tasks.first.mutates = false;
  controller.state.tasks.first.stateKeys = ["owned-object"];
  const conflicting = controller.createTask({ ...task("conflicting"), stateKeys: ["owned-object"] });
  assert.match(controller.taskReadiness(conflicting.id).reason, /shared target state/);
  for (let i = 2; i <= controller.maxParallelAgents; i++) {
    controller.createTask(task(`parallel-${i}`));
    controller.startTask(`parallel-${i}`);
  }
  controller.createTask(task("overflow"));
  assert.match(controller.taskReadiness("overflow").reason, /concurrency limit reached/);
});

test("scope and repeated run commands do not create generic inventory tasks", t => {
  const { controller, root, workspace, scope } = fixture(t, { withScope: false });
  controller.apply("scope", scope);
  controller.apply("run");
  controller.apply("run");
  assert.deepEqual(controller.state.tasks, {});
  const restored = new Controller(root, workspace);
  restored.apply("run");
  assert.deepEqual(restored.state.tasks, {});
});

test("legacy inventory tasks can still be explicitly reconciled without regenerating them", t => {
  const { controller, root, workspace, write } = fixture(t);
  let bootstrap = controller.createTask({ ...task("legacy-inventory"), bootstrap: true, network: false });
  for (let attempt = 0; attempt < 2; attempt++) {
    startSimulated(controller, bootstrap.id);
    controller.returnedTask(bootstrap.id, "failed", "Fixture bootstrap interrupted");
    write(bootstrap.output, "");
    controller.apply("reconcile_task", { id: bootstrap.id, disposition: "retry", notes: "Offline artifacts untouched; partial report is empty", evidence: ["private/evidence.txt"] });
    const replacement = controller.state.tasks[bootstrap.supersededBy];
    assert.equal(replacement.bootstrap, true);
    assert.equal(replacement.network, false);
    assert.equal(replacement.retryOf, bootstrap.id);
    const restored = new Controller(root, workspace);
    assert.equal(restored.state.tasks[replacement.id].retryOf, bootstrap.id);
    assert.equal(fs.readFileSync(path.join(workspace, bootstrap.output), "utf8"), "");
    bootstrap = replacement;
  }
});

test("verification and grants require recorded process provenance or an actual nonempty legacy transcript", t => {
  const { controller, root, write } = fixture(t);
  const assignment = controller.createTask({ ...task("no-spawn"), execution: { pid: 1234 } });
  assert.equal(assignment.execution, null, "creation cannot supply execution provenance");
  assert.throws(() => controller.recordTaskProcess(assignment.id, { pid: 1234, parentPid: 1 }), /running task/);
  controller.startTask(assignment.id);
  const before = structuredClone(controller.state);
  for (const record of [{ pid: 0, parentPid: 1 }, { pid: 1, parentPid: 1 }, { pid: 1.2, parentPid: 2 }, { pid: 2, parentPid: -1 }, { pid: 2, parentPid: 1, startedAt: "invalid" }]) {
    assert.throws(() => controller.recordTaskProcess(assignment.id, record), /Execution|execution/);
    assert.deepEqual(controller.state, before);
  }
  write(assignment.output);
  controller.returnedTask(assignment.id, "returned");
  const verify = { id: assignment.id, notes: "Inspected raw evidence", evidence: ["private/evidence.txt"], considerations: considerations(), hypotheses: [] };
  assert.throws(() => controller.apply("verify_task", verify), /execution provenance/);
  assert.throws(() => controller.apply("begin_verification", { id: assignment.id, reason: "Inspect report" }), /execution provenance/);
  const dir = path.join(root, "specialists", assignment.id);
  fs.mkdirSync(dir, { recursive: true });
  const transcript = path.join(dir, "transcript.jsonl");
  fs.writeFileSync(transcript, "\n");
  assert.throws(() => controller.apply("verify_task", verify), /execution provenance/);
  fs.writeFileSync(transcript, '{"fixture":"legacy session evidence"}\n');
  controller.apply("begin_verification", { taskId: assignment.id, reason: "Inspect legacy transcript and evidence" });
  controller.apply("verify_task", verify);
  assert.equal(controller.state.tasks[assignment.id].status, "verified");
  assert.equal(controller.state.verification, null);
});

test("process records persist and returned metadata merges without fabricating or changing provenance", t => {
  const { controller, root, workspace } = fixture(t);
  controller.createTask(task("recorded"));
  startSimulated(controller, "recorded");
  const execution = structuredClone(controller.state.tasks.recorded.execution);
  assert.ok(execution.pid > 0);
  assert.notEqual(execution.pid, execution.parentPid);
  assert.deepEqual(new Controller(root, workspace).state.tasks.recorded.execution, execution);
  const before = structuredClone(controller.state);
  assert.throws(() => controller.returnedTask("recorded", "returned", "Finished", { pid: execution.pid + 1 }), /provenance/);
  assert.deepEqual(controller.state, before);
  const finishedAt = new Date().toISOString();
  controller.returnedTask("recorded", "returned", "Fixture complete", { exitCode: 0, finishedAt });
  assert.deepEqual(controller.state.tasks.recorded.execution, { ...execution, exitCode: 0, finishedAt });
  assert.equal(controller.summary().tasks[0].execution.pid, controller.state.tasks.recorded.execution.pid);
  assert.deepEqual(controller.apply("status", { full: true }).tasks.recorded.execution, controller.state.tasks.recorded.execution);
  assert.match(fs.readFileSync(path.join(workspace, "reports/tasks.md"), "utf8"), /parentPid.*exitCode/);
  assert.throws(() => controller.recordTaskProcess("recorded", execution), /running task/);
});

function returnedFixture(controller, write, id = "verify-network", offline = false) {
  controller.createTask({ ...task(id), ...(offline ? { network: false, targets: [], bounds: { requests: 0, rate: 0, minutes: 2 } } : {}) });
  startSimulated(controller, id);
  write(`reports/${id}.md`);
  controller.returnedTask(id, "returned", "Awaiting evidence verification", { exitCode: 0 });
  return controller.state.tasks[id];
}

test("optional verification contracts retain bounded replay semantics", t => {
  const { controller, write } = fixture(t);
  assert.throws(() => controller.verificationTask(), /Optional verification contract/);
  assert.throws(() => controller.consumeVerificationTool(), /begin_verification/);
  const assignment = returnedFixture(controller, write);
  const invalid = [
    [{ reason: "" }, /reason/], [{ maxToolCalls: 0 }, /maxToolCalls/], [{ maxToolCalls: 51 }, /maxToolCalls/], [{ maxToolCalls: 1.5 }, /maxToolCalls/],
    [{ minutes: 0 }, /minutes/], [{ minutes: 2 }, /minutes/], [{ requests: 4 }, /requests/], [{ requests: -1 }, /requests/], [{ requests: .5 }, /requests/],
  ];
  for (const [fields, pattern] of invalid) {
    const before = structuredClone(controller.state);
    assert.throws(() => controller.apply("begin_verification", { id: assignment.id, reason: "Check raw control", ...fields }), pattern);
    assert.deepEqual(controller.state, before);
  }
  const phaseRevision = controller.state.phaseRevision?.setup;
  controller.apply("begin_verification", { taskId: assignment.id, reason: "Replay one control" });
  const { task: active, grant } = controller.verificationTask();
  assert.equal(active.id, assignment.id);
  assert.equal(grant.maxToolCalls, 12);
  assert.equal(grant.requests, 3);
  assert.equal(grant.toolCalls, 0);
  assert.equal(grant.requestsUsed, 0);
  assert.equal(Date.parse(grant.expiresAt) - Date.parse(grant.startedAt), 60000);
  assert.equal(controller.state.phaseRevision?.setup, phaseRevision, "grants do not alter coverage freshness");
  grant.expiresAt = "2000-01-01T00:00:00Z";
  assert.throws(() => controller.consumeVerificationTool(), /verification grant expired/);
  assert.equal(grant.toolCalls, 0);
  controller.apply("begin_verification", { id: assignment.id, reason: "Inspect control" });
  controller.state.phase = "auth";
  assert.throws(() => controller.verificationTask(), /current phase/);
  assert.throws(() => controller.apply("begin_verification", { id: assignment.id, reason: "Wrong phase" }), /current-phase/);
  controller.state.phase = "setup";
  controller.state.tasks[assignment.id].execution = null;
  assert.throws(() => controller.verificationTask(), /execution provenance/);
});

test("verification requests and tools have separate persisted caps and offline grants forbid network", t => {
  const { controller, write, root, workspace } = fixture(t);
  const assignment = returnedFixture(controller, write);
  controller.apply("begin_verification", { id: assignment.id, reason: "One bounded replay", maxToolCalls: 3, requests: 1 });
  const phaseRevisions = structuredClone(controller.state.phaseRevision);
  assert.equal(controller.consumeVerificationTool(true).id, assignment.id);
  let before = structuredClone(controller.state);
  assert.throws(() => controller.consumeVerificationTool(true), /request budget exhausted/);
  assert.deepEqual(controller.state, before);
  controller.consumeVerificationTool();
  controller.consumeVerificationTool();
  before = structuredClone(controller.state);
  assert.throws(() => controller.consumeVerificationTool(), /tool-call budget exhausted/);
  assert.deepEqual(controller.state, before);
  assert.deepEqual(new Controller(root, workspace).state.verification, { ...controller.state.verification, toolCalls: 3, requestsUsed: 1 });
  assert.deepEqual(controller.state.phaseRevision, phaseRevisions, "verification-tool consumption is not a coverage change");
  assert.equal(fs.existsSync(path.join(root, "specialists", assignment.id, "requests.json")), false, "lead counters are separate from worker counters");
  const offline = returnedFixture(controller, write, "verify-offline", true);
  assert.throws(() => controller.apply("begin_verification", { id: offline.id, reason: "Review local output", requests: 1 }), /zero for offline/);
  controller.apply("begin_verification", { id: offline.id, reason: "Review local output", maxToolCalls: 1 });
  assert.equal(controller.state.verification.requests, 0);
  before = structuredClone(controller.state);
  assert.throws(() => controller.consumeVerificationTool(true), /offline verification forbids network/);
  assert.deepEqual(controller.state, before);
  controller.consumeVerificationTool();
  assert.equal(controller.state.verification.toolCalls, 1);
  assert.equal(controller.state.verification.requestsUsed, 0);
});

test("verification replays check current scope and grants clear on workflow boundaries", t => {
  const { controller, write, scope } = fixture(t);
  const assignment = returnedFixture(controller, write);
  const begin = () => controller.apply("begin_verification", { id: assignment.id, reason: "Inspect current evidence" });
  begin();
  controller.state.scope.expiresAt = "2000-01-01T00:00:00Z";
  const before = structuredClone(controller.state);
  assert.throws(() => controller.consumeVerificationTool(true), /window has expired/);
  assert.deepEqual(controller.state, before);
  controller.state.scope.expiresAt = null;
  for (const [action, data] of [["end_verification", {}], ["pause", { reason: "Fixture pause" }], ["run", {}], ["reopen", { phase: "setup", reason: "Reconcile coverage" }], ["scope_update", scope]]) {
    begin();
    controller.apply(action, data);
    assert.equal(controller.state.verification, null, action);
    if (controller.state.phase !== "setup") controller.state.phase = "setup";
  }
  begin();
  controller.apply("verify_task", { id: assignment.id, notes: "Checked report and raw evidence", evidence: ["private/evidence.txt"], considerations: considerations(), hypotheses: [] });
  assert.equal(controller.state.verification, null);
  settlePhase(controller, write, "setup");
  begin();
  controller.apply("advance");
  assert.equal(controller.state.phase, "exploration");
  assert.equal(controller.state.verification, null);
});

test("old checkpoints without a verification field load with no lead execution grant", t => {
  const { controller, root, workspace } = fixture(t);
  const legacy = structuredClone(controller.state);
  delete legacy.verification;
  atomicJSON(controller.file, legacy);
  const restored = new Controller(root, workspace);
  assert.equal(restored.summary().verification, null);
  assert.throws(() => restored.verificationTask(), /begin_verification/);
});
