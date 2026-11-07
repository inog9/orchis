import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import http from "node:http";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { fileURLToPath } from "node:url";

const harness = process.env.PENTEST_HARNES_TEST_BIN;
const pi = process.env.PENTEST_HARNES_TEST_PI;

test("service job: real Pi autostarts, pauses cleanly, and keeps prompt private", { skip: !(harness && pi), timeout: 45000 }, async t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "pentest-job-smoke-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const agent = path.join(root, "agent");
  const project = path.join(root, "project");
  fs.mkdirSync(agent);
  fs.mkdirSync(project);
  const config = path.join(root, "opencode.json");
  fs.writeFileSync(config, '{"mcp":{}}');
  let requests = 0;
  let sawRunning = false;
  const server = http.createServer(async (request, response) => {
    for await (const _ of request) { /* consume local model input */ }
    requests++;
    const sessions = path.join(project, ".pentest-harnes", "sessions");
    const session = fs.readdirSync(sessions)[0];
    const state = JSON.parse(fs.readFileSync(path.join(sessions, session, "state.json")));
    if (requests === 1) sawRunning = state.mode === "running" && state.scope === null;
    const call = requests === 1;
    const chunk = delta => ({ id: `job-${requests}`, object: "chat.completion.chunk", created: 1, model: "fixture", choices: [{ index: 0, delta, finish_reason: null }] });
    response.writeHead(200, { "content-type": "text/event-stream" });
    response.write(`data: ${JSON.stringify(chunk(call ? { role: "assistant", tool_calls: [{ index: 0, id: "job-pause", type: "function", function: { name: "pentest_workflow", arguments: JSON.stringify({ action: "pause", data: { reason: "Local fixture requires supplied account" } }) } }] } : { role: "assistant", content: "Paused local fixture." }))}\n\n`);
    response.write(`data: ${JSON.stringify({ ...chunk({}), choices: [{ index: 0, delta: {}, finish_reason: call ? "tool_calls" : "stop" }] })}\n\n`);
    response.end(`data: ${JSON.stringify({ ...chunk({}), choices: [], usage: { prompt_tokens: 100, completion_tokens: 10, total_tokens: 110 } })}\n\ndata: [DONE]\n\n`);
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  fs.writeFileSync(path.join(agent, "models.json"), JSON.stringify({ providers: { harness: { baseUrl: `http://127.0.0.1:${server.address().port}/v1`, api: "openai-completions", apiKey: "fixture-only", models: [{ id: "fixture", name: "Fixture", reasoning: false, input: ["text"], contextWindow: 131072, maxTokens: 1024, cost: { input: 1, output: 1, cacheRead: 0, cacheWrite: 0 } }] } } }));
  fs.writeFileSync(path.join(agent, "settings.json"), JSON.stringify({ defaultProvider: "harness", defaultModel: "fixture", packages: [], skills: [] }));
  const child = spawn(process.execPath, [fileURLToPath(new URL("../../docker/run-job.mjs", import.meta.url)), "--harness", harness, "--pi", pi, "--dir", project, "--prompt-file", "-"], { env: { ...process.env, PI_CODING_AGENT_DIR: agent, OPENCODE_CONFIG: config, PENTEST_HARNES_STACK: "", PENTEST_HARNES_SUBAGENTS_DIR: "", PI_TELEMETRY: "0", ACP_AUTO_UPDATE: "0" }, stdio: ["pipe", "pipe", "pipe"] });
  t.after(() => { if (child.exitCode === null) child.kill("SIGKILL"); });
  let output = "", errors = "";
  child.stdout.on("data", chunk => { output += chunk; });
  child.stderr.on("data", chunk => { errors += chunk; });
  child.stdin.end("Offline local fixture. Do not make target requests. SECRET_JOB_MARKER_173.");
  const [code] = await once(child, "exit");
  assert.equal(code, 3, errors + output);
  assert.equal(sawRunning, true, "workflow did not auto-start before first model request");
  assert.ok(requests > 0);
  assert.doesNotMatch(output, /SECRET_JOB_MARKER_173|fixture-only|Local fixture requires supplied account/);
  const events = output.trim().split("\n").map(line => JSON.parse(line));
  assert.ok(events.some(event => event.event === "started"));
  assert.equal(events.at(-1).event, "input_required");
});
