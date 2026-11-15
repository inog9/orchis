#!/usr/bin/env node
// Service-friendly RPC client. Public stdout is lifecycle JSONL only; complete
// model/tool traffic is retained in the engagement's private job directory.
import fs from "node:fs";
import { exportResult } from "./export-result.mjs";
import { importScannerInputs } from "./scanner-inputs.mjs";
import os from "node:os";
import path from "node:path";
import crypto from "node:crypto";
import { spawn } from "node:child_process";
import { pathToFileURL } from "node:url";

const modes = new Set(["idle", "running", "paused", "complete", "limited"]);
const phases = new Set(["setup", "exploration", "recon", "auth", "authz", "injection", "fuzzing", "endpoint-fuzz-full", "gadgets", "reflection", "evidence", "report"]);
const taskStatuses = ["pending", "running", "returned", "verified", "blocked", "interrupted", "cancelled", "failed"];
const autoRun = `Execute the engagement request below now. Use the supplied targets and explicitly authorized actions as scope; do not widen authorization. Record the stated scope through pentest_workflow, then run the workflow and perform useful authorized exploration and login/account verification. Continue through the gated phases, delegation, verification, reflection, evidence, and both reports. Do not stop at a plan or after one agent turn. Preserve all controller gates and actor/session contracts; never assume that this service invocation registers scope or grants missing authorization. On a resumed session, inspect the checkpoint and reconcile interrupted work before resuming. Ask only for an actual blocker; record unavailable coverage honestly.\n\nEngagement request:\n`;

export function parseArguments(args) {
  const values = { dir: "/data", harness: "pentest-harnes", startupTimeoutMs: 120000, shutdownTimeoutMs: 60000, pollMs: 500 };
  const flags = new Map([["--inputs-dir", "inputsDir"], ["--prompt-file", "promptFile"], ["--dir", "dir"], ["--session", "session"], ["--name", "name"], ["--pi", "pi"], ["--harness", "harness"], ["--startup-timeout-ms", "startupTimeoutMs"], ["--shutdown-timeout-ms", "shutdownTimeoutMs"], ["--poll-ms", "pollMs"]]);
  for (let i = 0; i < args.length; i++) {
    const key = flags.get(args[i]);
    if (!key || args[i + 1] === undefined) throw new Error("invalid_arguments");
    values[key] = args[++i];
  }
  if (!values.promptFile || (values.session && values.name)) throw new Error("invalid_arguments");
  for (const key of ["startupTimeoutMs", "shutdownTimeoutMs", "pollMs"]) {
    if (!/^\d+$/.test(String(values[key])) || Number(values[key]) < 1) throw new Error("invalid_arguments");
    values[key] = Number(values[key]);
  }
  values.dir = path.resolve(values.dir);
  return values;
}

export async function runJob(options) {
  const jobId = crypto.randomUUID();
  const emit = (event, detail = {}) => process.stdout.write(JSON.stringify({ schema: 1, event, at: new Date().toISOString(), jobId, ...(metadata && { sessionId: metadata.sessionId }), ...detail }) + "\n");
  let metadata;
  let prompt;
  try { prompt = fs.readFileSync(options.promptFile === "-" ? 0 : options.promptFile, "utf8"); }
  catch { emit("failed", { reason: "prompt_read_failed" }); return 1; }
  if (!prompt.trim()) { emit("failed", { reason: "empty_prompt" }); return 1; }
  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), "pentest-job-"));
  fs.chmodSync(temporary, 0o700);
  const statusFile = path.join(temporary, "session.json");
  let logs = temporary;
  const append = (name, data) => fs.appendFileSync(path.join(logs, name), data, { mode: 0o600 });
  const rpcLog = (direction, record) => append("rpc.jsonl", JSON.stringify({ at: new Date().toISOString(), direction, record }) + "\n");
  const args = ["--rpc", "--dir", options.dir];
  if (options.session) args.push("--session", options.session); else args.push("--new");
  if (options.name) args.push("--name", options.name);
  if (options.pi) args.push("--pi", options.pi);
  emit("starting");
  const child = spawn(options.harness, args, {
    env: { ...process.env, PENTEST_HARNES_JOB_STATUS_FILE: statusFile, PENTEST_HARNES_UNATTENDED: "1" },
    stdio: ["pipe", "pipe", "pipe"],
    // A private POSIX process group lets the final watchdog stop an unresponsive
    // launcher and its Pi descendants without signalling the service's group.
    detached: process.platform !== "win32",
  });
  let stopping, finish;
  let readyRPC = false, prompted = false, ended = false, baselineRevision, lastSummary;
  let stdout = "", stateErrors = 0;
  const startedAt = Date.now();
  const result = new Promise(resolve => { finish = resolve; });
  let shutdownTimer, terminateTimer;
  let closed = false;
  const paths = () => metadata ? { root: metadata.root, workspace: metadata.workspace, logs } : { logs };
  const send = record => {
    if (closed || child.stdin.destroyed || !child.stdin.writable) return;
    rpcLog("in", record);
    child.stdin.write(JSON.stringify(record) + "\n");
  };
  const stop = (event, code, reason) => {
    if (stopping) return;
    stopping = { event, code, reason };
    emit("stopping", { reason });
    if (code === 0 || code === 2) {
      child.stdin.end();
      terminateTimer = setTimeout(() => child.kill("SIGTERM"), Math.min(30000, options.shutdownTimeoutMs / 2));
    } else {
      // Extension commands are handled by RPC prompt even during a running turn.
      // Never fabricate an answer to an engagement authorization question.
      if (readyRPC) send({ id: "job-pause", type: "prompt", message: "/pentest-pause" });
      terminateTimer = setTimeout(() => child.kill("SIGTERM"), Math.min(1000, options.shutdownTimeoutMs / 4));
    }
    shutdownTimer = setTimeout(() => {
      stopping.forced = true;
      if (stopping.code !== 130) stopping = { event: "failed", code: 1, reason: "shutdown_timeout", forced: true };
      try {
        if (process.platform !== "win32") process.kill(-child.pid, "SIGKILL");
        else child.kill("SIGKILL");
      } catch (error) { if (error.code !== "ESRCH") append("stderr.log", String(error) + "\n"); }
    }, options.shutdownTimeoutMs);
  };
  const cancel = () => stop("cancelled", 130, "cancel_requested");
  process.once("SIGTERM", cancel);
  process.once("SIGINT", cancel);
  child.stdin.on("error", error => { append("stderr.log", String(error) + "\n"); });
  child.stderr.on("data", data => append("stderr.log", data));
  child.on("error", error => {
    append("stderr.log", String(error) + "\n");
    if (!stopping) stopping = { event: "failed", code: 1, reason: "launcher_start_failed" };
  });
  child.stdout.setEncoding("utf8");
  child.stdout.on("data", chunk => {
    // Strict LF framing; readline also treats U+2028/U+2029 as boundaries,
    // although those characters may legitimately occur inside model JSON.
    stdout += chunk;
    if (stdout.length > 32 * 1024 * 1024) { stop("failed", 1, "rpc_record_too_large"); return; }
    let newline;
    while ((newline = stdout.indexOf("\n")) !== -1) {
      const line = stdout.slice(0, newline).replace(/\r$/, "");
      stdout = stdout.slice(newline + 1);
      if (!line) continue;
      let record;
      try { record = JSON.parse(line); } catch {
        append("rpc-invalid.log", line + "\n");
        stop("failed", 1, "invalid_rpc_output");
        continue;
      }
      rpcLog("out", record);
      if (record.type === "response" && record.id === "job-state") {
        if (record.success !== true) stop("failed", 1, "rpc_handshake_failed");
        else readyRPC = true;
      }
      if (record.type === "response" && record.id === "job-prompt" && record.success !== true) stop("failed", 1, "prompt_rejected");
      if (record.type === "response" && record.id === "job-pause") child.kill("SIGTERM");
      if (record.type === "agent_end" && prompted) ended = true;
      if (record.type === "extension_ui_request" && ["select", "confirm", "input", "editor", "custom"].includes(record.method)) {
        send({ type: "extension_ui_response", id: record.id, cancelled: true });
        stop("input_required", 3, "interactive_request");
      }
    }
  });

  function readMetadata() {
    if (metadata || !fs.existsSync(statusFile)) return;
    const value = JSON.parse(fs.readFileSync(statusFile, "utf8"));
    if (!/^[a-zA-Z0-9-]{1,80}$/.test(value.sessionId ?? "") || !path.isAbsolute(value.root ?? "") || value.workspace !== path.join(value.root, "workspace")) throw new Error("invalid_session_metadata");
    metadata = value;
    const destination = path.join(metadata.root, "private", "jobs", jobId);
    fs.mkdirSync(destination, { recursive: true, mode: 0o700 });
    for (const name of fs.readdirSync(temporary)) {
      if (name !== "session.json") fs.copyFileSync(path.join(temporary, name), path.join(destination, name));
    }
    logs = destination;
    // Keep the prompt available for private incident/restart diagnosis even if
    // the model fails before accepting it. It is never exposed on public stdout.
    fs.writeFileSync(path.join(logs, "request.txt"), prompt, { mode: 0o600 });
  }

  function tick() {
    if (stopping || closed) return;
    try {
      readMetadata();
      if (!prompted && metadata && readyRPC) {
        const statePath = path.join(metadata.root, "state.json");
        baselineRevision = fs.existsSync(statePath) ? JSON.parse(fs.readFileSync(statePath, "utf8")).revision : undefined;
        const scannerInputs = importScannerInputs(options.inputsDir, metadata.workspace);
        emit("started", { ...paths(), ...(scannerInputs && { scannerInputs }) });
        prompted = true;
        send({ id: "job-prompt", type: "prompt", message: autoRun + prompt });
      }
      if (!prompted) {
        if (Date.now() - startedAt > options.startupTimeoutMs) stop("failed", 1, "startup_timeout");
        return;
      }
      const statePath = path.join(metadata.root, "state.json");
      if (!fs.existsSync(statePath)) return;
      const state = JSON.parse(fs.readFileSync(statePath, "utf8"));
      if (!modes.has(state.mode)) throw new Error("invalid_workflow_state");
      const counts = Object.fromEntries(taskStatuses.map(status => [status, Object.values(state.tasks ?? {}).filter(task => task?.status === status).length]));
      const summary = { mode: state.mode, phase: phases.has(state.phase) ? state.phase : "unknown", revision: Number.isSafeInteger(state.revision) ? state.revision : 0, tasks: counts };
      const encoded = JSON.stringify(summary);
      if (encoded !== lastSummary) { emit("state", summary); lastSummary = encoded; }
      stateErrors = 0;
      // A resumed session may already be paused/terminal. Let its new prompt
      // reconcile state before interpreting the previous launch's disposition.
      if (state.revision === baselineRevision && !ended) return;
      if (state.mode === "complete") stop("complete", 0, "workflow_complete");
      else if (state.mode === "limited") stop("limited", 2, "workflow_limited");
      else if (state.mode === "paused" || (state.mode === "idle" && ended)) stop("input_required", 3, state.mode === "paused" ? "paused" : "workflow_not_running");
      // agent_end while running is deliberately ignored: Fleet completions,
      // retries and controller follow-up turns may arrive after it.
    } catch (error) {
      append("stderr.log", String(error) + "\n");
      if (++stateErrors >= 3) stop("failed", 1, "invalid_job_state");
    }
  }

  const interval = setInterval(tick, options.pollMs);
  child.on("close", (code, signal) => {
    closed = true;
    clearInterval(interval);
    clearTimeout(shutdownTimer);
    clearTimeout(terminateTimer);
    process.removeListener("SIGTERM", cancel);
    process.removeListener("SIGINT", cancel);
    // A launcher may finish immediately after writing its private metadata,
    // before the first polling tick has attached the logs to the session.
    try { readMetadata(); } catch (error) { append("stderr.log", String(error) + "\n"); }
    if (!stopping) stopping = { event: "failed", code: 1, reason: code === 0 && !signal ? "launcher_exited_before_delivery" : "launcher_failed" };
    else if ([0, 2].includes(stopping.code) && (code !== 0 || signal)) stopping = { event: "failed", code: 1, reason: "launcher_shutdown_failed" };
    if (metadata) {
      fs.rmSync(temporary, { recursive: true, force: true });
    }
    let resultFile;
    if (metadata) {
      try { resultFile = exportResult(metadata, stopping.event, stopping.reason); }
      catch (error) {
        append("stderr.log", String(error) + "\n");
        stopping = { event: "failed", code: 1, reason: "result_export_failed" };
      }
    }
    if (metadata) fs.writeFileSync(path.join(logs, "result.json"), JSON.stringify({ schema: 1, jobId, sessionId: metadata.sessionId, ...stopping, launcherCode: code, launcherSignal: signal }), { mode: 0o600 });
    emit(stopping.event, { ...(resultFile && { resultFile }), reason: stopping.reason, ...(stopping.forced && { forced: true }), ...paths() });
    finish(stopping.code);
  });
  send({ id: "job-state", type: "get_state" });
  tick();
  return result;
}

if (process.argv[1] && import.meta.url === pathToFileURL(fs.realpathSync(process.argv[1])).href) {
  try { process.exitCode = await runJob(parseArguments(process.argv.slice(2))); }
  catch { process.stdout.write(JSON.stringify({ schema: 1, event: "failed", at: new Date().toISOString(), jobId: crypto.randomUUID(), reason: "invalid_arguments" }) + "\n"); process.exitCode = 1; }
}
