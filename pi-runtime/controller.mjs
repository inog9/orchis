import fs from "node:fs";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { artifact, assertArtifacts, atomicJSON, privateText, readJSON } from "./storage.mjs";

export const PHASES = ["setup", "exploration", "recon", "auth", "authz", "injection", "fuzzing", "endpoint-fuzz-full", "gadgets", "reflection", "evidence", "report"];
export const VERDICTS = ["VALIDATED", "INVALID", "INFORMATIONAL", "NEEDS_MORE_EVIDENCE", "BLOCKED"];
export const FAMILIES = ["siblings", "roles", "tenants", "methods", "failure-paths", "chains"];
const PRIMARY_SKILLS = { exploration: ["pentest-exploration"], recon: ["pentest-recon", "pentest-js-analysis", "pentest-dependency-confusion"], auth: ["pentest-auth"], authz: ["pentest-authz"], injection: ["pentest-injection"], fuzzing: ["pentest-fuzzing"], "endpoint-fuzz-full": ["pentest-endpoint-fuzz-full"], gadgets: ["pentest-gadgets"], reflection: ["pentest-reflection"], evidence: ["pentest-evidence"], report: ["pentest-report"] };
const settled = value => ["tested", "blocked", "not_applicable"].includes(value.status);
const slug = value => typeof value === "string" && /^[a-zA-Z0-9][a-zA-Z0-9_-]{0,100}$/.test(value);
const required = (value, label) => { if (typeof value !== "string" || !value.trim()) throw new Error(`${label} is required`); return value; };
const cell = value => String(value ?? "").replaceAll("|", "\\|").replaceAll("\n", " ");
const refs = entries => entries.map(entry => `${entry.path} (${entry.sha256.slice(0, 12)})`).join(", ");
const object = value => value !== null && typeof value === "object" && !Array.isArray(value);
const positivePID = value => Number.isSafeInteger(value) && value > 0;
const verificationOnly = reason => new Error(`Optional verification contract: ${reason}`);

export function parseChecklist(text) {
  const items = [];
  let phase = "setup", current;
  for (const line of text.split(/\r?\n/)) {
    if (line.startsWith("## ")) {
      const section = line.slice(3).split(".")[0];
      phase = ({ "0": "setup", "0b": "exploration", "1": "recon", "1b": "recon", "1c": "recon", "2": "auth", "3": "authz", "4": "injection", "5": "injection", "6": "injection", "7": "auth", "7b": "fuzzing", "7c": "endpoint-fuzz-full", "8": "recon", "8b": "gadgets", "8c": "reflection", "8d": "reflection", "9": "report" })[section];
      if (!phase) throw new Error(`Unmapped checklist section: ${line}`);
      current = undefined;
    } else if (/^- \[[ x~]\] /.test(line)) {
      current = { id: `C${String(items.length + 1).padStart(3, "0")}`, title: line.slice(6), phase, status: "pending", evidence: [] };
      items.push(current);
    } else if (current && /^\s+\S/.test(line)) current.title += " " + line.trim();
  }
  if (!items.length) throw new Error("The bundled checklist is empty");
  return items;
}

export class Controller {
  constructor(root, workspace) {
    this.root = root;
    this.workspace = workspace;
    this.maxParallelAgents = Number(process.env.PENTEST_HARNES_MAX_AGENTS ?? 8);
    if (!Number.isSafeInteger(this.maxParallelAgents) || this.maxParallelAgents < 1) throw new Error("PENTEST_HARNES_MAX_AGENTS must be a positive integer");
    this.file = path.join(root, "state.json");
    this.state = readJSON(this.file, null);
    if (!this.state) {
      this.state = { schema: 1, revision: 0, phase: "setup", mode: "idle", scope: null, verification: null, phases: {}, tasks: {}, hypotheses: {}, roles: {}, cells: {}, assets: {}, candidates: {}, reflections: [], checklist: parseChecklist(fs.readFileSync(path.join(workspace, ".pi/skills/pentest/references/checklist.md"), "utf8")), createdAt: new Date().toISOString() };
      this.save("created");
    } else if (this.state.schema !== 1) throw new Error("Unsupported checkpoint schema; refusing to reset engagement state");
  }
  save(event) {
    this.state.revision++;
    this.state.updatedAt = new Date().toISOString();
    atomicJSON(this.file, this.state);
    fs.appendFileSync(path.join(this.root, "events.jsonl"), JSON.stringify({ at: this.state.updatedAt, revision: this.state.revision, event, phase: this.state.phase, mode: this.state.mode }) + "\n", { mode: 0o600 });
    this.render();
  }
  markPhaseChanged(phase) {
    this.state.phaseRevision ??= {};
    this.state.phaseRevision[phase] = this.state.revision + 1;
  }
  render() {
    const state = this.state;
    const checklist = ["# Engagement checklist", "", "`[x]` tested; `[~]` blocked; `[N/A]` justified not applicable; `[ ]` pending.", "", "| ID | Phase | Item | State | Evidence / reason |", "| --- | --- | --- | --- | --- |"];
    for (const item of state.checklist) checklist.push(`| ${item.id} | ${item.phase} | ${cell(item.title)} | ${{ tested: "[x]", blocked: "[~]", not_applicable: "[N/A]", pending: "[ ]" }[item.status]} | ${cell(refs(item.evidence) + " " + (item.reason ?? "") + (item.attempts ? `; attempts: ${item.attempts}; client action: ${item.nextAction}` : ""))} |`);
    privateText(path.join(this.workspace, "reports/checklist.md"), checklist.join("\n") + "\n");
    const hypotheses = ["# Hypothesis ledger", "", "| ID | Hypothesis | Basis | Bounded test | State / verdict | Evidence / blocker |", "| --- | --- | --- | --- | --- | --- |"];
    for (const item of Object.values(state.hypotheses)) hypotheses.push(`| ${cell(item.id)} | ${cell(item.question)} | ${cell(item.basis)} | ${cell(item.test)} | ${cell(item.verdict ?? item.status)} | ${cell(refs(item.evidence?.length ? item.evidence : (item.basisEvidence ?? [])) + " " + (item.reason ?? "") + (item.nextAction ? `; next action: ${item.nextAction}` : ""))} |`);
    privateText(path.join(this.workspace, "reports/hypotheses.md"), hypotheses.join("\n") + "\n");
    const tasks = ["# Task ledger", "", "| ID | Phase | Status | Output | Dependencies | Execution | Blocker |", "| --- | --- | --- | --- | --- | --- | --- |"];
    for (const task of Object.values(state.tasks)) tasks.push(`| ${cell(task.id)} | ${task.phase} | ${task.status} | ${cell(task.output)} | ${cell(task.dependencies.join(", "))} | ${cell(task.execution ? JSON.stringify(task.execution) : "")} | ${cell(task.reason)} |`);
    privateText(path.join(this.workspace, "reports/tasks.md"), tasks.join("\n") + "\n");
  }
  evidence(paths) {
    if (!Array.isArray(paths) || !paths.length) throw new Error("At least one actual evidence file is required");
    return [...new Map(paths.map(relative => {
      const entry = artifact(this.workspace, relative);
      return [entry.path, entry];
    })).values()];
  }
  summary() {
    const s = this.state;
    const counts = entries => Object.fromEntries(["pending", "tested", "blocked", "not_applicable"].map(status => [status, entries.filter(item => item.status === status).length]));
    return { phase: s.phase, mode: s.mode, revision: s.revision,
      scope: s.scope && { target: s.scope.target, origins: s.scope.origins, methods: s.scope.methods, expiresAt: s.scope.expiresAt },
      verification: s.verification ?? null, checklist: counts(s.checklist),
      tasks: Object.values(s.tasks).slice(-30).map(({ id, phase, status, output, reason, network, bootstrap, execution, retryOf, supersededBy }) =>
        ({ id, phase, status, output, network, bootstrap, retryOf, supersededBy,
          ...(reason ? { reason: reason.slice(0, 180) } : {}),
          ...(execution ? { execution: { pid: execution.pid, exitCode: execution.exitCode, runId: execution.runId } } : {}) })),
      totalTasks: Object.keys(s.tasks).length, openHypotheses: Object.values(s.hypotheses).filter(item => !item.verdict).map(item => item.id).slice(0, 30),
      coverage: { roles: Object.keys(s.roles).length, authorizationCells: counts(Object.values(s.cells)), assets: counts(Object.values(s.assets)) },
      candidates: Object.values(s.candidates).map(({ id, verdict, score }) => ({ id, verdict, score })).slice(-30),
      lastReflection: s.reflections.at(-1) && { phase: s.reflections.at(-1).phase, taskId: s.reflections.at(-1).taskId },
      pauseReason: s.pauseReason, checkpoint: this.file };
  }
  // Inject only routing state into the model. Full execution proofs and large
  // inventories stay on disk and remain available through status/read tools.
  promptSummary() {
    const s = this.summary();
    return { phase: s.phase, mode: s.mode, revision: s.revision,
      scope: s.scope && { target: s.scope.target, origins: s.scope.origins, methods: s.scope.methods },
      checklist: s.checklist,
      tasks: s.tasks.map(({ id, phase, status, output, reason, network, execution }) =>
        ({ id, phase, status, output, network, ...(reason ? { reason: reason.slice(0, 180) } : {}),
          ...(execution?.pid ? { pid: execution.pid } : {}) })),
      totalTasks: s.totalTasks, openHypotheses: s.openHypotheses,
      coverage: s.coverage, lastReflection: s.lastReflection && { phase: s.lastReflection.phase, taskId: s.lastReflection.taskId },
      pauseReason: s.pauseReason, checkpoint: s.checkpoint };
  }
  recover() {
    let interrupted = false;
    for (const task of Object.values(this.state.tasks)) {
      if (task.status === "running") {
        task.status = "interrupted";
        task.reason = task.mutates ? "Interrupted mutation: reconcile actual target state and restoration before retrying" : "Interrupted specialist: inspect its transcript and artifacts before re-dispatch";
        this.markPhaseChanged(task.phase);
        interrupted = true;
      }
    }
    if (interrupted || this.state.mode === "running") {
      this.state.verification = null;
      this.state.mode = "paused";
      this.state.pauseReason = interrupted ? "Interrupted tasks require reconciliation" : "Session restored; use /pentest-run to continue";
      this.save("recovered");
    }
  }
  checkScope(url, method = "GET") {
    const scope = this.state.scope;
    if (!scope) throw new Error("Record engagement authorization, hosts, actions, and window before target requests");
    const parsed = new URL(url);
    if (!["http:", "https:"].includes(parsed.protocol) || parsed.username || parsed.password) throw new Error("Only HTTP(S) URLs without embedded credentials are supported");
    if (!scope.origins.includes(parsed.origin)) throw new Error(`Origin is outside scope: ${parsed.origin}`);
    if (!scope.methods.includes(method.toUpperCase())) throw new Error(`Method ${method} is not permitted by the engagement`);
    if (scope.expiresAt && Date.now() >= Date.parse(scope.expiresAt)) throw new Error("The engagement test window has expired");
  }
  taskPhaseAllowed(task) {
    if (task.phase === this.state.phase) return true;
    if (PHASES.indexOf(task.phase) <= PHASES.indexOf(this.state.phase)) return false;
    if (task.network === false) return true;
    // Public read-only discovery need not wait for accounts or setup checklist
    // disposition. Mutations and authenticated/live validation stay phase-gated.
    return !task.mutates && task.actor?.account === "anonymous" && task.targets.every(target => ["GET", "HEAD", "OPTIONS"].includes(target.method.toUpperCase()));
  }
  taskReady(task) {
    if (!task) throw new Error("Unknown task");
    if (!this.taskPhaseAllowed(task)) throw new Error(`Task phase ${task.phase} does not match current phase ${this.state.phase}`);
    if (task.dependencies.some(id => this.state.tasks[id]?.status !== "verified")) throw new Error("Task dependencies are not verified");
    if (task.network !== false) {
      if (!this.state.scope) throw new Error("Set the authorized scope first");
      for (const endpoint of task.targets) this.checkScope(endpoint.url, endpoint.method);
    }
    const running = Object.values(this.state.tasks).filter(item => item.status === "running");
    if (running.length >= this.maxParallelAgents) throw new Error(`Specialist concurrency limit reached (${this.maxParallelAgents}); raise PENTEST_HARNES_MAX_AGENTS for more workers`);
    if (running.some(item => (task.mutates || item.mutates) && item.actor.session === task.actor.session)) throw new Error("Mutations sharing an actor session must be serialized");
    for (const item of running) if (task.stateKeys.some(key => item.stateKeys.includes(key))) throw new Error("Tasks touch shared target state; serialize them");
  }
  taskReadiness(taskOrId) {
    try {
      this.taskReady(typeof taskOrId === "string" ? this.state.tasks[taskOrId] : taskOrId);
      return { ready: true };
    } catch (error) { return { ready: false, reason: error.message }; }
  }
  createTask(data) {
    const task = this.prepareTask(data);
    this.state.tasks[task.id] = task;
    this.markPhaseChanged(task.phase);
    this.save(`task-created:${task.id}`);
    return task;
  }
  createTasks(items) {
    if (!Array.isArray(items) || !items.length) throw new Error("Provide a nonempty tasks array");
    // Validate the entire batch before changing the checkpoint.
    const original = this.state;
    const tasks = [];
    this.state = structuredClone(original);
    try {
      for (const data of items) {
        const task = this.prepareTask(data);
        this.state.tasks[task.id] = task;
        tasks.push(task);
      }
    } finally { this.state = original; }
    for (const task of tasks) {
      this.state.tasks[task.id] = task;
      this.markPhaseChanged(task.phase);
    }
    this.save(`tasks-created:${tasks.map(task => task.id).join(",")}`);
    return tasks;
  }
  prepareTask(data) {
    if (!object(data)) throw new Error("Task contract must be an object");
    if (!slug(data.id) || this.state.tasks[data.id]) throw new Error("Task needs a new unique alphanumeric ID");
    const task = structuredClone({ phase: this.state.phase, output: `reports/${data.id}.md`, inputs: [], dependencies: [], stateKeys: [], targets: [], mutates: false,
      method: "Follow the assigned skill within this contract", success: "Answer the assigned question with bounded, reproducible evidence or an explicit blocker",
      report: "Write the assigned output with evidence paths, coverage, verdicts, and unresolved blockers",
      stop: "Stop at the first applicable budget, challenge, scope boundary, or sufficient result",
      restoration: "Read-only task; leave local and target state unchanged", ...Object.fromEntries(Object.entries(data).filter(([, value]) => value !== undefined)) });
    for (const field of ["question", "skill", "output", "method", "success", "report", "stop", "restoration"]) required(task[field], field);
    if (!PHASES.includes(task.phase)) throw new Error("Task phase is invalid");
    if (typeof task.mutates !== "boolean") throw new Error("mutates must be boolean");
    if (task.mutates) required(data.restoration, "Explicit restoration for mutations");
    if (task.bootstrap !== undefined && typeof task.bootstrap !== "boolean") throw new Error("bootstrap must be boolean");
    if (path.isAbsolute(task.output)) throw new Error("Task output must be a unique reports/ or recon/ path");
    task.output = path.posix.normalize(task.output.replaceAll("\\", "/"));
    if (!task.output.startsWith("reports/") && !task.output.startsWith("recon/")) throw new Error("Task output must be a unique reports/ or recon/ path");
    // Resolve existing ancestors too: aliases through symlinks must not bypass isolation.
    const outputPath = relative => {
      let ancestor = path.resolve(this.workspace, relative), suffix = [];
      while (!fs.existsSync(ancestor)) { suffix.unshift(path.basename(ancestor)); ancestor = path.dirname(ancestor); }
      return path.join(fs.realpathSync(ancestor), ...suffix);
    };
    const output = outputPath(task.output);
    if (!output.startsWith(fs.realpathSync(this.workspace) + path.sep)) throw new Error("Task output escapes workspace");
    if (Object.values(this.state.tasks).some(item => outputPath(item.output) === output)) throw new Error("Each task must have a unique output path");
    if (fs.existsSync(output) || fs.lstatSync(path.resolve(this.workspace, task.output), { throwIfNoEntry: false })) throw new Error("Task output already exists; preserve it and choose a new path");
    if (path.basename(task.skill) !== "SKILL.md") throw new Error("Task skill must reference an actual SKILL.md file");
    artifact(this.workspace, task.skill);
    for (const field of ["inputs", "dependencies", "stateKeys"]) {
      if (!Array.isArray(task[field]) || task[field].some(value => typeof value !== "string" || !value.trim())) throw new Error(`${field} must be an array of nonempty strings`);
    }
    if (task.dependencies.some(id => !slug(id) || id === task.id)) throw new Error("Dependencies must be other task IDs");
    if (!Array.isArray(task.targets)) throw new Error("targets must be an array");
    for (const endpoint of task.targets) {
      if (!object(endpoint)) throw new Error("Targets require explicit exact URLs and methods");
      required(endpoint.url, "target.url"); required(endpoint.method, "target.method");
      const url = new URL(endpoint.url);
      if (!/^https?:\/\//i.test(endpoint.url) || !["http:", "https:"].includes(url.protocol) || url.username || url.password || url.hash || endpoint.url.includes("*") || !/^[A-Za-z]+$/.test(endpoint.method)) throw new Error("Targets require explicit exact HTTP(S) URLs and methods");
    }
    if (task.bounds !== undefined && !object(task.bounds)) throw new Error("bounds must be an object");
    task.network = typeof data.network === "boolean" ? data.network : task.targets.length > 0 && task.bounds?.requests !== 0;
    task.bounds = { requests: 20, rate: 1, minutes: 10, ...Object.fromEntries(Object.entries(task.bounds ?? {}).filter(([, value]) => value !== undefined)) };
    if (!task.network) Object.assign(task.bounds, { requests: 0, rate: 0 });
    if (!Number.isFinite(task.bounds.minutes) || task.bounds.minutes <= 0) throw new Error("Finite positive time bounds are required");
    if (task.network && (!task.targets.length || !Number.isSafeInteger(task.bounds.requests) || task.bounds.requests <= 0 || !Number.isFinite(task.bounds.rate) || task.bounds.rate <= 0 || task.bounds.rate > 50)) throw new Error("Network tasks require explicit targets and finite positive request/rate bounds (integer requests, rate <= 50)");
    if (task.actor !== undefined && !object(task.actor)) throw new Error("actor must be an object");
    const account = task.actor?.account ?? (task.network ? "anonymous" : "local");
    if (!["anonymous", "local"].includes(account)) {
      // Authenticated identities must come entirely from explicit, evidenced input.
      for (const key of ["account", "role", "tenant", "permissions", "identityEvidence", "session"]) required(task.actor?.[key], `actor.${key}`);
      this.evidence([task.actor.identityEvidence]);
    } else {
      const local = account === "local";
      if (local && task.network) throw new Error("Local artifact actors cannot make network requests");
      task.actor = { account, role: account, tenant: "none", permissions: local ? "local artifacts" : "unauthenticated", identityEvidence: "not-applicable", session: `${account}-${randomUUID()}`, ...Object.fromEntries(Object.entries(task.actor ?? {}).filter(([, value]) => value !== undefined)) };
      for (const key of ["account", "role", "tenant", "permissions", "identityEvidence", "session"]) required(task.actor[key], `actor.${key}`);
      if (task.actor.role !== account) throw new Error(`${local ? "Local" : "Anonymous"} actor cannot claim an authenticated role`);
    }
    Object.assign(task, { status: "pending", evidence: [], execution: null, createdAt: new Date().toISOString() });
    return task;
  }
  startTask(id) {
    const task = this.state.tasks[id];
    if (!task || task.status !== "pending") throw new Error("Only pending, reconciled tasks may start");
    this.taskReady(task);
    // Workers must be able to write their unique contracted report on the
    // first attempt, even if recon/ has never been used in this session.
    const parent = path.resolve(this.workspace, path.dirname(task.output));
    fs.mkdirSync(parent, { recursive: true, mode: 0o700 });
    if (!fs.realpathSync(parent).startsWith(fs.realpathSync(this.workspace) + path.sep)) throw new Error("Task output parent escapes workspace");
    task.status = "running";
    task.execution = null;
    task.startedAt = new Date().toISOString();
    this.markPhaseChanged(task.phase);
    this.save(`task-started:${id}`);
    return task;
  }
  recordTaskProcess(id, record) {
    const task = this.state.tasks[id];
    if (!task || task.status !== "running") throw new Error("Process execution can only be recorded for a running task");
    if (!object(record) || !positivePID(record.pid) || !positivePID(record.parentPid) || record.pid === record.parentPid) throw new Error("Execution requires distinct positive safe-integer pid and parentPid");
    const startedAt = record.startedAt ?? new Date().toISOString();
    if (typeof startedAt !== "string" || !Number.isFinite(Date.parse(startedAt))) throw new Error("Invalid execution startedAt");
    for (const key of ["model", "sessionDir"]) if (record[key] !== undefined) required(record[key], `execution.${key}`);
    task.execution = { pid: record.pid, parentPid: record.parentPid, startedAt, ...(record.model === undefined ? {} : { model: record.model }), ...(record.sessionDir === undefined ? {} : { sessionDir: record.sessionDir }) };
    this.markPhaseChanged(task.phase);
    this.save(`task-process:${id}`);
    return task;
  }
  returnedTask(id, outcome, reason, metadata = {}) {
    const task = this.state.tasks[id];
    if (!task || task.status !== "running") throw new Error("Only running tasks can return");
    if (!["returned", "failed", "interrupted"].includes(outcome)) throw new Error("Invalid task outcome");
    if (!object(metadata)) throw new Error("Execution metadata must be an object");
    if (metadata.exitCode !== undefined && metadata.exitCode !== null && !Number.isSafeInteger(metadata.exitCode)) throw new Error("Invalid execution exitCode");
    if (metadata.finishedAt !== undefined && (typeof metadata.finishedAt !== "string" || !Number.isFinite(Date.parse(metadata.finishedAt)))) throw new Error("Invalid execution finishedAt");
    if (["pid", "parentPid", "startedAt"].some(key => metadata[key] !== undefined && metadata[key] !== task.execution?.[key])) throw new Error("Return metadata cannot establish or change process provenance");
    task.status = outcome;
    task.reason = reason;
    task.finishedAt = new Date().toISOString();
    if (task.execution) task.execution = { ...task.execution, ...metadata, finishedAt: metadata.finishedAt ?? task.finishedAt };
    this.markPhaseChanged(task.phase);
    this.save(`task-returned:${id}`);
  }
  requireExecution(task) {
    if (positivePID(task.execution?.pid)) return;
    const transcript = path.join(this.root, "specialists", task.id, "transcript.jsonl");
    if (fs.existsSync(transcript) && fs.statSync(transcript).isFile() && fs.readFileSync(transcript, "utf8").trim()) return;
    throw new Error("Task requires actual specialist execution provenance: recordTaskProcess after OS spawn or a nonempty legacy specialist transcript");
  }
  verificationTask() {
    const grant = this.state.verification;
    if (!grant) throw verificationOnly("begin_verification requires a returned specialist task and a bounded reason");
    if (!Number.isFinite(Date.parse(grant.expiresAt)) || Date.now() >= Date.parse(grant.expiresAt)) throw verificationOnly("verification grant expired; begin a new bounded verification");
    const task = this.state.tasks[grant.taskId];
    if (!task || task.phase !== this.state.phase || !["returned", "verified"].includes(task.status)) throw verificationOnly("verification task must be returned or verified in the current phase");
    try { this.requireExecution(task); } catch (error) { throw verificationOnly(error.message); }
    return { task, grant };
  }
  consumeVerificationTool(network = false) {
    const { task, grant } = this.verificationTask();
    if (grant.toolCalls >= grant.maxToolCalls) throw verificationOnly("verification tool-call budget exhausted");
    if (network) {
      if (task.network === false) throw verificationOnly("offline verification forbids network requests");
      if (grant.requestsUsed >= grant.requests) throw verificationOnly("verification request budget exhausted");
      for (const endpoint of task.targets) this.checkScope(endpoint.url, endpoint.method);
    }
    grant.toolCalls++;
    if (network) grant.requestsUsed++;
    this.save(`verification-tool:${task.id}`);
    return task;
  }
  unresolvedCoverage(phase) {
    const gaps = [];
    for (const role of Object.values(this.state.roles)) {
      for (const area of phase === "authz" ? ["account", "crawl", "authz"] : ["account", "crawl"]) {
        if (!settled(role[area])) gaps.push(`role ${role.id}: ${area}`);
      }
    }
    if (phase === "authz") {
      for (const entry of Object.values(this.state.cells)) if (!settled(entry)) gaps.push(`authorization cell ${entry.id}`);
    }
    if (phase === "recon") for (const entry of Object.values(this.state.assets)) if (!settled(entry)) gaps.push(`JS/map ${entry.id}`);
    return gaps;
  }
  verifyCoverage() {
    for (const role of Object.values(this.state.roles)) {
      for (const area of ["account", "crawl", "authz"]) if (settled(role[area])) assertArtifacts(this.workspace, role[area].evidence);
      if (role.policy !== "UNKNOWN") assertArtifacts(this.workspace, role.policyEvidence);
    }
    for (const entry of Object.values(this.state.cells)) {
      if (settled(entry)) assertArtifacts(this.workspace, entry.evidence);
      if (entry.policy !== "UNKNOWN") assertArtifacts(this.workspace, entry.policyEvidence);
    }
    for (const entry of Object.values(this.state.assets)) {
      if (settled(entry)) assertArtifacts(this.workspace, entry.evidence);
      if (entry.status === "tested" && artifact(this.workspace, entry.file).sha256 !== entry.id) throw new Error(`Reviewed asset bytes changed: ${entry.id}`);
    }
  }
  apply(action, data = {}) {
    const before = structuredClone(this.state);
    try { return this.applyAction(action, data); }
    catch (error) { this.state = before; throw error; }
  }
  updateChecklistItem(data) {
    const item = this.state.checklist.find(entry => entry.id === data.id);
    if (!item) throw new Error("Unknown checklist ID");
    if (!PHASES.includes(item.phase) || PHASES.indexOf(item.phase) > PHASES.indexOf(this.state.phase)) throw new Error("Cannot complete future-phase checklist items");
    if (!["tested", "blocked", "not_applicable", "pending"].includes(data.status)) throw new Error("Invalid checklist state");
    if (data.status !== "pending") {
      item.evidence = this.evidence(data.evidence);
      required(data.reason, "Reason / verification notes");
    }
    if (data.status === "blocked") { required(data.attempts, "Attempts"); required(data.nextAction, "Required client action"); }
    Object.assign(item, { status: data.status, reason: data.reason, attempts: data.attempts, nextAction: data.nextAction });
    return item.phase;
  }
  applyAction(action, data = {}) {
    const s = this.state;
    if (action === "status") return data.full ? structuredClone(s) : this.summary();
    if (action === "checklist") {
      if (!data.id && data.items === undefined) return s.checklist;
      const items = data.items === undefined ? [data] : data.items;
      if (!Array.isArray(items) || !items.length || items.length > 50 || (data.items !== undefined && data.id)) throw new Error("Checklist batch requires 1–50 items; use either id or items");
      const ids = items.map(item => item.id);
      if (ids.some(id => typeof id !== "string") || new Set(ids).size !== ids.length) throw new Error("Checklist batch IDs must be unique");
      const phases = new Set(items.map(item => this.updateChecklistItem(item)));
      for (const phase of phases) this.markPhaseChanged(phase);
    } else if (action === "scope") {
      if (s.scope) throw new Error("Scope is already pinned; use scope_update with evidence for changes");
      this.setScope(data);
    } else if (action === "scope_update") {
      const prior = s.scope;
      if (!prior) throw new Error("Record the target with scope before scope_update");
      this.setScope({ ...prior, ...data,
        origins: data.origins ? [...new Set([...(data.replaceOrigins ? [] : prior.origins), ...data.origins])] : prior.origins,
        evidence: data.evidence ?? prior.evidence.map(item => item.path) });
      if (JSON.stringify([prior.origins, prior.methods, prior.actions]) !== JSON.stringify([s.scope.origins, s.scope.methods, s.scope.actions])) {
        this.reopen(PHASES[Math.min(PHASES.indexOf(s.phase), PHASES.indexOf("exploration"))], "Engagement scope changed");
        this.markPhaseChanged(s.phase);
      }
    } else if (action === "run") {
      if (Object.values(s.tasks).some(task => task.status === "interrupted")) throw new Error("Reconcile interrupted tasks first");
      const inflightDir = path.join(this.root, "private", "inflight");
      if (fs.existsSync(inflightDir) && fs.readdirSync(inflightDir).some(name => {
        const record = readJSON(path.join(inflightDir, name));
        return record.pid !== process.pid && record.calls?.length;
      })) throw new Error("Reconcile interrupted tool calls using reconcile_tools before continuing");
      s.verification = null;
      s.mode = "running"; delete s.pauseReason;
    } else if (action === "pause") {
      s.mode = "paused"; s.pauseReason = required(data.reason, "Pause reason");
      s.verification = null;
    } else if (action === "begin_verification") {
      const task = s.tasks[data.id ?? data.taskId];
      if (!task || task.phase !== s.phase || !["returned", "verified"].includes(task.status)) throw new Error("Verification requires a returned or verified current-phase specialist task");
      this.requireExecution(task);
      const reason = required(data.reason, "Verification reason");
      const maxToolCalls = data.maxToolCalls ?? 12;
      const minutes = data.minutes ?? Math.min(5, task.bounds.minutes);
      const requests = data.requests ?? (task.network === false ? 0 : Math.min(3, task.bounds.requests));
      if (!Number.isSafeInteger(maxToolCalls) || maxToolCalls < 1 || maxToolCalls > 50) throw new Error("Verification maxToolCalls must be an integer from 1 to 50");
      if (!Number.isFinite(minutes) || minutes <= 0 || minutes > task.bounds.minutes || !Number.isFinite(Date.now() + minutes * 60000)) throw new Error("Verification minutes must be positive and within the task time bound");
      if (!Number.isSafeInteger(requests) || requests < 0 || requests > task.bounds.requests || (task.network === false && requests !== 0)) throw new Error("Verification requests must be a nonnegative integer within the task network allowance (zero for offline tasks)");
      const now = Date.now();
      if (!Number.isFinite(new Date(now + minutes * 60000).getTime())) throw new Error("Verification expiry is outside the supported time range");
      s.verification = { taskId: task.id, reason, startedAt: new Date(now).toISOString(), expiresAt: new Date(now + minutes * 60000).toISOString(), maxToolCalls, toolCalls: 0, requests, requestsUsed: 0 };
    } else if (action === "end_verification") {
      s.verification = null;
    } else if (action === "verify_task") {
      const task = s.tasks[data.id];
      if (!task || !["returned", "verified"].includes(task.status)) throw new Error("Only returned specialist tasks can be verified or reverified");
      this.requireExecution(task);
      task.evidence = this.evidence([task.output, ...(data.evidence ?? [])]);
      task.verification = required(data.notes ?? data.verification, "Verification notes against actual raw evidence");
      if (task.evidence.length < 2) throw new Error("Task report alone is insufficient; provide underlying evidence");
      data.considerations ??= [];
      if (object(data.considerations)) data.considerations = Object.entries(data.considerations).map(([family, value]) => ({ family, ...value }));
      if (!Array.isArray(data.considerations)) throw new Error("considerations must be an array or a family-keyed object");
      data.hypotheses ??= [];
      if (!Array.isArray(data.hypotheses)) throw new Error("hypotheses must be an array of registered IDs");
      for (const id of data.hypotheses) if (!s.hypotheses[id]) throw new Error(`Register hypothesis ${id} before referencing it`);
      if (task.mutates) {
        task.restorationEvidence = this.evidence(data.restorationEvidence);
        task.finalIdentity = required(data.finalIdentity, "Final server identity/state verification");
      }
      task.considerations = data.considerations;
      task.hypotheses = data.hypotheses;
      task.status = "verified";
      if (s.verification?.taskId === task.id) s.verification = null;
    } else if (action === "block_task") {
      const task = s.tasks[data.id];
      if (!task || task.status === "running") throw new Error("Task must exist and not be running");
      task.evidence = this.evidence(data.evidence);
      task.reason = required(data.reason, "Blocker"); task.attempts = required(data.attempts, "Attempts"); task.nextAction = required(data.nextAction, "Unblock action"); task.status = "blocked";
    } else if (action === "reconcile_task") {
      const task = s.tasks[data.id];
      if (!task || (!["interrupted", "failed"].includes(task.status) && !(data.disposition === "retry" && task.status === "returned"))) throw new Error("Task must be interrupted/failed, or unverified returned work with an explicit retry disposition");
      if (data.disposition !== undefined && data.disposition !== "retry") throw new Error("Reconciliation disposition must be retry or omitted");
      const reconciliation = { notes: required(data.notes, "Observed final target state and reconciliation"), evidence: this.evidence(data.evidence) };
      if (data.disposition === "retry") {
        const contract = Object.fromEntries(["phase", "question", "skill", "method", "success", "report", "stop", "restoration", "inputs", "dependencies", "stateKeys", "targets", "mutates", "bounds", "actor", "network", "bootstrap"].filter(key => task[key] !== undefined).map(key => [key, task[key]]));
        const id = `${task.id.slice(0, 58)}-retry-${randomUUID()}`;
        reconciliation.disposition = "retry";
        // Prepare without saving: validation must succeed before either task is changed.
        const replacement = this.prepareTask({ ...contract, id, output: `${task.output.startsWith("recon/") ? "recon" : "reports"}/${id}.md`, retryOf: task.id,
          inputs: [...new Set([...task.inputs, ...reconciliation.evidence.map(entry => entry.path)])], reconciliation });
        s.tasks[id] = replacement;
        task.status = "blocked";
        task.supersededBy = id;
        task.reason = `Superseded by explicit retry ${id}: ${reconciliation.notes}`;
        task.attempts = `Prior specialist attempt reconciled against observed state: ${reconciliation.notes}`;
        task.nextAction = `Dispatch pending replacement ${id} only after readiness checks, within its copied bounds and restoration contract`;
        task.evidence = [...new Map([...(task.evidence ?? []), ...reconciliation.evidence].map(entry => [entry.path, entry])).values()];
        const oldReport = path.posix.normalize(task.output.replaceAll("\\", "/"));
        const referencesReport = input => path.posix.normalize(input.replaceAll("\\", "/")) === oldReport;
        for (const dependent of Object.values(s.tasks)) {
          // Only queued contracts are mutable; the replacement retains its original evidence inputs.
          if (dependent.status !== "pending" || dependent.id === id) continue;
          const dependenciesUpdated = dependent.dependencies.includes(task.id);
          const inputsUpdated = dependent.inputs.some(referencesReport);
          if (!dependenciesUpdated && !inputsUpdated) continue;
          dependent.dependencies = dependent.dependencies.map(dependency => dependency === task.id ? id : dependency);
          dependent.inputs = dependent.inputs.map(input => referencesReport(input) ? replacement.output : input);
          dependent.retryRebinds ??= [];
          dependent.retryRebinds.push({ fromTaskId: task.id, toTaskId: id, fromOutput: task.output, toOutput: replacement.output, dependenciesUpdated, inputsUpdated, revision: s.revision + 1 });
          this.markPhaseChanged(dependent.phase);
        }
      } else if (fs.lstatSync(path.resolve(this.workspace, task.output), { throwIfNoEntry: false })) {
        reconciliation.report = artifact(this.workspace, task.output);
        this.requireExecution(task);
        task.status = "returned";
      } else {
        task.status = "pending";
      }
      task.reconciliation = reconciliation;
      if (s.verification?.taskId === task.id) s.verification = null;
    } else if (action === "hypothesis") {
      if (!slug(data.id) || s.hypotheses[data.id]) throw new Error("Hypothesis needs a new unique ID");
      for (const field of ["question", "basis", "test", "phase"]) required(data[field], field);
      if (!PHASES.includes(data.phase)) throw new Error("Hypothesis phase is invalid");
      const basisEvidence = this.evidence(data.evidence);
      s.hypotheses[data.id] = { ...data, basisEvidence, evidence: [], status: "pending" };
    } else if (action === "resolve_hypothesis") {
      const hypothesis = s.hypotheses[data.id];
      if (!hypothesis || !VERDICTS.includes(data.verdict)) throw new Error("Unknown hypothesis or invalid verdict");
      hypothesis.evidence = this.evidence(data.evidence);
      hypothesis.reason = required(data.reason, "Evidence-based reason");
      if (["BLOCKED", "NEEDS_MORE_EVIDENCE"].includes(data.verdict)) required(data.nextAction, "Next action");
      Object.assign(hypothesis, { verdict: data.verdict, status: "resolved", nextAction: data.nextAction });
    } else if (action === "role") {
      if (!slug(data.id)) throw new Error("Role needs a stable ID");
      if (!s.roles[data.id]) {
        s.roles[data.id] = { id: data.id, label: required(data.label, "Role label"), policy: data.policy ?? "UNKNOWN", account: { status: "pending", evidence: [] }, crawl: { status: "pending", evidence: [] }, authz: { status: "pending", evidence: [] } };
        if (PHASES.indexOf(s.phase) > PHASES.indexOf("exploration")) this.reopen("exploration", `New role ${data.id}`);
      }
      if (data.policy !== undefined) {
        s.roles[data.id].policy = required(data.policy, "Sourced policy or UNKNOWN");
        if (data.policy !== "UNKNOWN") s.roles[data.id].policyEvidence = this.evidence(data.policyEvidence);
      }
      if (data.area) {
        if (!["account", "crawl", "authz"].includes(data.area)) throw new Error("Role area must be account, crawl, or authz");
        s.roles[data.id][data.area] = this.coverage(data);
      }
    } else if (action === "authz_cell") {
      if (!slug(data.id) || !s.roles[data.role]) throw new Error("Cell needs a stable ID and a discovered role");
      for (const field of ["target", "tenant", "action"]) required(data[field], field);
      s.cells[data.id] = { ...data, ...this.coverage(data), policy: data.policy ?? "UNKNOWN" };
      if (s.cells[data.id].policy !== "UNKNOWN") s.cells[data.id].policyEvidence = this.evidence(data.policyEvidence);
      if (s.phases.authz && data.status === "pending") this.reopen("authz", `New authorization cell ${data.id}`);
    } else if (action === "asset") {
      if (!/^[a-f0-9]{64}$/.test(data.id)) throw new Error("Asset ID must be its SHA-256 hash");
      if (!Array.isArray(data.urls) || !data.urls.length) throw new Error("Record JS/map provenance URLs");
      if (data.status === "tested" && artifact(this.workspace, required(data.file, "Reviewed original file")).sha256 !== data.id) throw new Error("Asset hash does not match the reviewed bytes");
      const previous = s.assets[data.id];
      s.assets[data.id] = { ...data, ...this.coverage(data), urls: [...new Set([...(previous?.urls ?? []), ...data.urls])] };
      if (!previous && PHASES.indexOf(s.phase) > PHASES.indexOf("recon")) this.reopen("recon", `New JS/map asset ${data.id}`);
    } else if (action === "candidate") {
      if (!slug(data.id) || !VERDICTS.includes(data.verdict)) throw new Error("Candidate needs a stable ID and a valid verdict");
      required(data.reason, "Verdict reason");
      if (["BLOCKED", "NEEDS_MORE_EVIDENCE"].includes(data.verdict)) required(data.nextAction, "Required follow-up or client action");
      const evidence = this.evidence(data.evidence);
      if (data.verdict === "VALIDATED") {
        if (!data.reproduced || typeof data.replay !== "string" || !data.replay.trim()) throw new Error("Validated finding requires reproduced behavior and a named replay handle");
        for (const key of ["request", "response", "control", "identity", "impact"]) artifact(this.workspace, required(data.proof?.[key], `proof.${key}`));
        if (data.mutates) for (const key of ["before", "after", "restoration", "finalIdentity"]) artifact(this.workspace, required(data.proof?.[key], `proof.${key}`));
        data.proofEvidence = this.evidence(Object.values(data.proof));
        required(data.vector, "Observed-impact CVSS 3.1 vector");
        data.score = cvss31(data.vector);
        required(data.scoreRationale, "CVSS metric rationale");
      }
      s.candidates[data.id] = { ...data, evidence };
    } else if (action === "reflection") {
      if (data.phase !== s.phase) throw new Error("Reflection must review the current phase");
      const task = s.tasks[data.taskId];
      if (!task || task.phase !== s.phase || task.status !== "verified" || !task.skill.endsWith("/pentest-reflection/SKILL.md")) throw new Error("Reflection requires a verified delegated pentest-reflection task for this phase");
      for (const key of ["missing", "coverage", "ambiguities", "furtherAction", "noise"]) required(data[key], `reflection.${key}`);
      if (!Array.isArray(data.hypotheses)) throw new Error("List follow-up hypothesis IDs (possibly empty)");
      for (const id of data.hypotheses) if (!s.hypotheses[id]) throw new Error(`Unknown hypothesis ${id}`);
      s.reflections.push({ ...data, evidence: this.evidence([task.output, ...(data.evidence ?? [])]), revision: s.revision + 1, at: new Date().toISOString() });
    } else if (action === "reopen") {
      this.reopen(data.phase, required(data.reason, "Reason for reopening coverage"));
    } else if (action === "reconcile_tools") {
      if (Object.values(s.tasks).some(task => task.status === "running")) throw new Error("Wait for or interrupt running specialists before reconciling tool calls");
      required(data.notes, "Reconciliation of interrupted tool calls and actual target state");
      const evidence = this.evidence(data.evidence);
      const dir = path.join(this.root, "private", "inflight");
      if (fs.existsSync(dir)) for (const name of fs.readdirSync(dir)) {
        const file = path.join(dir, name);
        const record = readJSON(file);
        if (record.pid !== process.pid && record.calls.length) {
          atomicJSON(file, { ...record, calls: [], reconciledAt: new Date().toISOString(), notes: data.notes, evidence });
        }
      }
    } else if (action === "advance") {
      this.advance();
    } else throw new Error(`Unknown workflow action ${action}`);
    if (!["reflection", "advance", "pause", "run", "begin_verification", "end_verification", "scope_update", "checklist"].includes(action)) {
      this.markPhaseChanged(["verify_task", "block_task", "reconcile_task"].includes(action) ? s.tasks[data.id].phase : s.phase);
    }
    this.save(action);
    return this.summary();
  }
  coverage(data) {
    if (!["pending", "tested", "blocked", "not_applicable"].includes(data.status)) throw new Error("Invalid coverage status");
    if (data.status === "pending") return { status: "pending", evidence: [] };
    const evidence = this.evidence(data.evidence);
    const reason = required(data.reason, "Coverage result/reason");
    if (data.status === "blocked") { required(data.attempts, "Attempts"); required(data.nextAction, "Required client action"); }
    return { status: data.status, evidence, reason, attempts: data.attempts, nextAction: data.nextAction };
  }
  setScope(data) {
    const inputs = data.origins ?? data.hosts ?? (data.target ? [data.target] : []);
    const entries = typeof inputs === "string" ? [inputs] : inputs;
    if (!Array.isArray(entries) || !entries.length) throw new Error("Provide the user's target host or URL in target/origins");
    const origins = [...new Set(entries.map(value => {
      const url = new URL(value.includes("://") ? value : `https://${value}`);
      if (!["http:", "https:"].includes(url.protocol) || url.username || url.password) throw new Error("Scope hosts must be HTTP(S) without embedded credentials");
      return url.origin;
    }))];
    const methods = (data.methods ?? ["GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH", "DELETE"]).map(method => method.toUpperCase());
    if (!methods.length || methods.some(method => !["GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH", "DELETE"].includes(method))) throw new Error("Invalid permitted method");
    if (data.expiresAt && !Number.isFinite(Date.parse(data.expiresAt))) throw new Error("Invalid scope expiry timestamp");
    const target = String(data.target ?? new URL(origins[0]).hostname).replace(/[^a-zA-Z0-9_-]/g, "-").slice(0, 100);
    this.state.scope = { target, authorization: data.authorization || "User's engagement request", window: data.window || "Current engagement until stopped", actions: data.actions || "User-requested web/API testing; no DoS or destructive actions", origins, methods, expiresAt: data.expiresAt ?? null, evidence: data.evidence?.length ? this.evidence(data.evidence) : [] };
    this.state.verification = null;
  }
  reopen(phase, reason) {
    if (!PHASES.includes(phase)) throw new Error("Unknown phase to reopen");
    this.state.verification = null;
    for (const key of PHASES.slice(PHASES.indexOf(phase))) delete this.state.phases[key];
    for (const item of this.state.checklist) if (PHASES.indexOf(item.phase) >= PHASES.indexOf(phase) && item.status === "tested") { item.status = "pending"; item.reason = `Reopened: ${reason}`; }
    this.state.phase = phase;
    this.state.reopenReason = reason;
  }
  advance() {
    const s = this.state, phase = s.phase;
    if (!s.scope) throw new Error("Scope has not been established");
    const pending = s.checklist.filter(item => item.phase === phase && !settled(item));
    if (pending.length) throw new Error(`Unresolved checklist items: ${pending.map(item => item.id).join(", ")}`);
    for (const item of s.checklist.filter(item => item.phase === phase)) assertArtifacts(this.workspace, item.evidence);
    const tasks = Object.values(s.tasks).filter(task => task.phase === phase);
    if (phase !== "setup" && !tasks.length) throw new Error("Every phase needs bounded specialist work or an explicitly blocked specialist task");
    if (tasks.some(task => !["verified", "blocked"].includes(task.status))) throw new Error("Phase has unverified, pending, or interrupted specialist work");
    for (const skill of PRIMARY_SKILLS[phase] ?? []) {
      if (!tasks.some(task => task.skill.endsWith(`/${skill}/SKILL.md`) && ["verified", "blocked"].includes(task.status))) throw new Error(`Phase requires verified or explicitly blocked specialist coverage for ${skill}`);
    }
    for (const task of tasks) assertArtifacts(this.workspace, task.evidence);
    const open = Object.values(s.hypotheses).filter(item => !item.verdict && PHASES.indexOf(item.phase) <= PHASES.indexOf(phase));
    if (open.length) throw new Error(`Resolve or explicitly block hypotheses first: ${open.map(item => item.id).join(", ")}`);
    if (["recon", "authz"].includes(phase)) {
      const gaps = this.unresolvedCoverage(phase);
      if (gaps.length) throw new Error(`Unresolved coverage: ${gaps.join(", ")}`);
      if (!Object.keys(s.roles).length) throw new Error("Inventory at least the anonymous role, or all discovered authenticated roles");
      if (phase === "authz" && !Object.keys(s.cells).length) throw new Error("Register relevant authorization cells, including explicitly blocked cells");
      this.verifyCoverage();
    }
    const outputs = {
      exploration: ["reports/accounts.md", "reports/role-matrix.md", "reports/app-overview.md", "reports/triage.md"],
      recon: ["recon/api-inventory.md", "recon/web-logic.md", "recon/business-logic.md", "recon/js-history.md", "recon/js-lazy.md"],
      authz: ["reports/authz-matrix.md"],
      evidence: ["reports/validation-ledger.md"],
      report: [`${s.scope.target}_PENTEST_REPORT.md`, `${s.scope.target}_VALIDATION_REASON.md`],
    }[phase] ?? [];
    for (const output of outputs) artifact(this.workspace, output);
    if (phase === "exploration") {
      const overview = fs.readFileSync(path.join(this.workspace, "reports/app-overview.md"), "utf8");
      if (overview.trim().split(/\s+/).length > 500) throw new Error("App overview exceeds 500 words");
      const triage = fs.readFileSync(path.join(this.workspace, "reports/triage.md"), "utf8");
      if (!/\bCONTINUE\b/.test(triage)) throw new Error("Exploration triage is SKIP/BLOCKED; pause with evidence and required next action");
    }
    const reflection = [...s.reflections].reverse().find(item => item.phase === phase);
    if (!reflection) throw new Error("Reflect after this phase before advancing");
    if (reflection.revision < (s.phaseRevision?.[phase] ?? 0)) throw new Error("Coverage changed since the last reflection; reflect again");
    assertArtifacts(this.workspace, reflection.evidence);
    if (phase === "report") {
      const unresolved = s.checklist.filter(item => !settled(item));
      if (unresolved.length || Object.values(s.hypotheses).some(item => !item.verdict)) throw new Error("Checklist or hypothesis loop is not exhausted");
      const pendingCoverage = [...this.unresolvedCoverage("authz"), ...Object.values(s.assets).filter(item => !settled(item)).map(item => `JS/map ${item.id}`)];
      if (pendingCoverage.length) throw new Error(`Coverage must be tested or explicitly blocked: ${pendingCoverage.join(", ")}`);
      this.verifyCoverage();
      for (const candidate of Object.values(s.candidates)) {
        assertArtifacts(this.workspace, candidate.evidence);
        if (candidate.verdict === "VALIDATED") assertArtifacts(this.workspace, candidate.proofEvidence);
      }
      const limited = s.checklist.some(item => item.status === "blocked") || Object.values(s.hypotheses).some(item => ["BLOCKED", "NEEDS_MORE_EVIDENCE"].includes(item.verdict)) || Object.values(s.candidates).some(item => ["BLOCKED", "NEEDS_MORE_EVIDENCE"].includes(item.verdict)) || Object.values(s.roles).some(role => role.policy === "UNKNOWN" || [role.account, role.crawl, role.authz].some(item => item.status !== "tested" && item.status !== "not_applicable")) || Object.values(s.cells).some(item => item.policy === "UNKNOWN" || !["tested", "not_applicable"].includes(item.status)) || Object.values(s.assets).some(item => !["tested", "not_applicable"].includes(item.status));
      s.mode = limited ? "limited" : "complete";
    } else s.phase = PHASES[PHASES.indexOf(phase) + 1];
    s.phases[phase] = { completedAt: new Date().toISOString(), reflection: s.reflections.indexOf(reflection), outputs: outputs.map(output => artifact(this.workspace, output)) };
    s.verification = null;
  }
}

export function cvss31(vector) {
  const parts = vector.split("/");
  if (parts.shift() !== "CVSS:3.1") throw new Error("Use a CVSS:3.1 base vector");
  const values = Object.fromEntries(parts.map(part => part.split(":")));
  const choices = { AV: { N: .85, A: .62, L: .55, P: .2 }, AC: { L: .77, H: .44 }, PR: { N: .85, L: values.S === "C" ? .68 : .62, H: values.S === "C" ? .5 : .27 }, UI: { N: .85, R: .62 }, S: { U: 1, C: 1 }, C: { N: 0, L: .22, H: .56 }, I: { N: 0, L: .22, H: .56 }, A: { N: 0, L: .22, H: .56 } };
  if (parts.length !== 8 || Object.keys(values).length !== 8 || Object.entries(choices).some(([key, options]) => options[values[key]] === undefined)) throw new Error("Invalid CVSS 3.1 base metrics");
  const iss = 1 - (1 - choices.C[values.C]) * (1 - choices.I[values.I]) * (1 - choices.A[values.A]);
  const impact = values.S === "U" ? 6.42 * iss : 7.52 * (iss - .029) - 3.25 * (iss - .02) ** 15;
  if (impact <= 0) return 0;
  const exploitability = 8.22 * choices.AV[values.AV] * choices.AC[values.AC] * choices.PR[values.PR] * choices.UI[values.UI];
  const base = Math.min((values.S === "C" ? 1.08 : 1) * (impact + exploitability), 10);
  return Math.ceil((Math.round(base * 100000) / 100000) * 10) / 10;
}
