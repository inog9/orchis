// The main real-Pi test contacts only a local mock model and stdio MCP fixtures.
// The opt-in user-config startup check initializes configured MCP connections
// but invokes no model, inbox, browser, or SMS tools.
import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import http from "node:http";
import readline from "node:readline";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { fileURLToPath } from "node:url";

const binary = process.env.PENTEST_HARNES_TEST_BIN;
const pi = process.env.PENTEST_HARNES_TEST_PI;
const enabled = Boolean(binary && pi);
const nativePackage = process.env.PENTEST_HARNES_TEST_SUBAGENTS_DIR ?? "";
const askPackage = process.env.PENTEST_HARNES_TEST_ASK_DIR ?? "";

test("real Pi: full lead tools, parallel specialists, visible questions, native MCP and shared budget", { skip: !enabled, timeout: 90000 }, async t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "pentest-pi-smoke-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const agentDir = path.join(root, "agent");
  const project = path.join(root, "project");
  fs.mkdirSync(agentDir);
  fs.mkdirSync(project);
  const config = path.join(root, "opencode.json");
  fs.writeFileSync(config, '{"mcp":{}}');
  const mcpCalls = path.join(root, "mcp-calls.jsonl");
  const mcpServer = { command: process.execPath, args: [fileURLToPath(new URL("./native-mcp-fixture.mjs", import.meta.url)), mcpCalls] };
  fs.writeFileSync(path.join(agentDir, "mcp.json"), JSON.stringify({ mcpServers: { fixture: { ...mcpServer, exposure: "direct" }, indirect: { ...mcpServer, exposure: "codemode" } } }));
  let requests = 0;
  let targetRequests = 0;
  let nativeInspection = "";
  let parallelReceipt;
  let questionAnswer;
  const completions = [];
  const contract = { id: "smoke-worker", phase: "setup", question: "Write an offline fixture report", skill: ".pi/skills/pentest-auth/SKILL.md", output: "reports/smoke-worker.md", method: "Offline fixture only; no target traffic", success: "A nonempty report", stop: "After the report", restoration: "No mutations", inputs: [], dependencies: [], stateKeys: [], targets: [{ url: "https://fixture.test/me", method: "GET" }], mutates: false, bounds: { requests: 1, minutes: 1, rate: 1 }, actor: { account: "anonymous", role: "anonymous", tenant: "none", permissions: "public", identityEvidence: "not applicable", session: "smoke-isolated" } };
  const server = http.createServer(async (request, result) => {
    if (request.url !== "/v1/chat/completions") {
      targetRequests++;
      result.writeHead(200, { "content-type": "text/plain" });
      result.end("This local fixture endpoint must never be requested by primary lead/offline work");
      return;
    }
    let text = "";
    for await (const chunk of request) text += chunk;
    const input = JSON.parse(text);
    requests++;
    const messages = input.messages;
    const messageText = item => typeof item.content === "string" ? item.content : (item.content ?? []).map(part => part.text ?? "").join("\n");
    const contractMessage = messages.find(item => item.role === "user" && messageText(item).includes("\nContract:\n"));
    let childContract = null;
    if (contractMessage) {
      const tail = messageText(contractMessage).split("\nContract:\n").at(-1);
      for (let end = tail.lastIndexOf("}"); end >= 0; end = tail.lastIndexOf("}", end - 1)) {
        try { childContract = JSON.parse(tail.slice(0, end + 1)); break; } catch {}
      }
      if (!childContract) throw new Error("Fixture could not read the actual specialist contract");
    }
    const child = Boolean(childContract);
    const done = id => messages.some(item => item.role === "tool" && item.tool_call_id === id);
    if (done("parallel-delegate")) parallelReceipt = JSON.parse(messageText(messages.find(item => item.role === "tool" && item.tool_call_id === "parallel-delegate")));
    if (done("answer-question")) questionAnswer = messageText(messages.find(item => item.role === "tool" && item.tool_call_id === "answer-question"));
    if (done("fleet-inspect")) nativeInspection = messageText(messages.find(item => item.role === "tool" && item.tool_call_id === "fleet-inspect"));
    let call, content;
    const autoLoop = messages.some(item => item.role === "user" && JSON.stringify(item.content).includes("AUTO_LOOP_SMOKE"));
    if (autoLoop && !child) {
      if (!done("auto-run")) call = { id: "auto-run", name: "pentest_workflow", args: { action: "run" } };
      else if (messages.some(item => JSON.stringify(item.content).includes("Continue from checkpoint revision")) && !done("auto-pause")) call = { id: "auto-pause", name: "pentest_workflow", args: { action: "pause", data: { reason: "Offline auto-continuation verified; user fixture input needed" } } };
      else content = done("auto-pause") ? "AUTO_LOOP_COMPLETE" : "FIRST_AUTO_TURN_COMPLETE";
    } else if (child && childContract.network === false) {
      if (!done("offline-skill")) call = { id: "offline-skill", name: "read", args: { path: childContract.skill } };
      else if (!done("offline-request")) call = { id: "offline-request", name: "pentest_http", args: { url: `http://127.0.0.1:${server.address().port}/forbidden-offline`, method: "GET" } };
      else if (!done("offline-evidence")) call = { id: "offline-evidence", name: "write", args: { path: `private/${childContract.id}.txt`, content: "Local fixture prerequisite review; no target traffic.\n" } };
      else if (!done("offline-report")) call = { id: "offline-report", name: "write", args: { path: childContract.output, content: `# Offline setup specialist\nRead the assigned skill. Evidence: private/${childContract.id}.txt. Target requests forbidden.\n` } };
      else content = "OFFLINE_SPECIALIST_COMPLETE";
    } else if (child) {
      if (!done("worker-native")) call = { id: "worker-native", name: "mcp__fixture__probe", args: { value: "worker-native" } };
      else if (!done("worker-native-excess")) call = { id: "worker-native-excess", name: "codemode", args: { code: 'return await tools.mcp__indirect__probe({value:"worker-over-budget"});' } };
      else if (!done("worker-write")) call = { id: "worker-write", name: "write", args: { path: "reports/smoke-worker.md", content: "# Offline specialist report\nFixture evidence only; no target requests were made.\n" } };
      else content = "SPECIALIST_COMPLETE";
    } else if (!done("lead-shell")) {
      call = { id: "lead-shell", name: "bash", args: { command: "node -e \"require('node:fs').writeFileSync('lead-full-tools','working')\"" } };
    } else if (askPackage && !done("visible-question")) {
      call = { id: "visible-question", name: "pentest_ask", args: { question: "Continue the offline question fixture?", reason: "Local UI decision fixture" } };
    } else if (askPackage && !done("answer-question")) {
      const prepared = JSON.parse(messageText(messages.find(item => item.role === "tool" && item.tool_call_id === "visible-question")));
      call = { id: "answer-question", name: "ask_user_question", args: { questions: prepared.questions } };
    } else if (!done("native-before-scope")) {
      call = { id: "native-before-scope", name: "mcp__fixture__probe", args: { value: "before-scope" } };
    } else if (!done("queue-future")) {
      call = { id: "queue-future", name: "pentest_delegate", args: { task: { id: "future-auth", phase: "auth", question: "Test authentication fixture later", skill: ".pi/skills/pentest-auth/SKILL.md", network: true, targets: [{ url: "https://fixture.test/me", method: "POST" }], bounds: { requests: 1, rate: 1, minutes: 1 } } } };
    } else if (!done("scope-write")) {
      call = { id: "scope-write", name: "write", args: { path: "private/scope.md", content: "Explicit offline fixture authorization; no real target requests.\n" } };
    } else if (!done("scope-register")) {
      call = { id: "scope-register", name: "pentest_workflow", args: { action: "scope", data: { target: "fixture", origins: ["https://fixture.test", `http://127.0.0.1:${server.address().port}`], methods: ["GET"], authorization: "Offline test fixture", window: "This test only", actions: "No actual target calls", evidence: ["private/scope.md"] } } };
    } else if (!done("checklist-batch")) {
      call = { id: "checklist-batch", name: "pentest_workflow", args: { action: "checklist", data: { items: [
        { id: "C001", status: "tested", reason: "Fixture authorization and local origins documented", evidence: ["private/scope.md"] },
        { id: "C002", status: "not_applicable", reason: "The offline fixture has no user accounts or target authorization cells", evidence: ["private/scope.md"] },
      ] } } };
    } else if (nativePackage && !done("parallel-delegate")) {
      const scopeResult = JSON.parse(messageText(messages.find(item => item.role === "tool" && item.tool_call_id === "scope-register")));
      assert.equal(scopeResult.tasks.length, 1, "Scope registration must not manufacture inventory tasks");
      call = { id: "parallel-delegate", name: "pentest_delegate", args: { tasks: Array.from({ length: 4 }, (_, i) => ({ id: `parallel-${i}`, phase: "recon", question: "Write the assigned offline fixture report", skill: ".pi/skills/pentest-recon/SKILL.md", network: false })) } };
    } else if (nativePackage && [0, 1, 2, 3].some(i => !done(`wait-parallel-${i}`))) {
      const i = [0, 1, 2, 3].find(i => !done(`wait-parallel-${i}`));
      call = { id: `wait-parallel-${i}`, name: "pentest_workflow", args: { action: "wait", data: { id: `parallel-${i}` } } };
    } else if (!done("lead-http")) call = { id: "lead-http", name: "pentest_http", args: { url: `http://127.0.0.1:${server.address().port}/forbidden-lead`, method: "GET" } };
    else if (!done("native-parent")) call = { id: "native-parent", name: "mcp__fixture__probe", args: { value: "lead-without-verification" } };
    else if (!done("delegate")) call = { id: "delegate", name: "pentest_delegate", args: { task: contract } };
    else if (nativePackage && !done("wait-worker")) call = { id: "wait-worker", name: "pentest_workflow", args: { action: "wait", data: { id: "smoke-worker" } } };
    else if (nativePackage && !done("fleet-inspect")) {
      const receipt = JSON.parse(messageText(messages.find(item => item.role === "tool" && item.tool_call_id === "delegate")));
      call = { id: "fleet-inspect", name: "subagent", args: { action: "status", id: receipt.runId, options: { view: "transcript", lines: 100 } } };
    }
    else if (!done("begin-verify")) call = { id: "begin-verify", name: "pentest_workflow", args: { action: "begin_verification", data: { id: "smoke-worker", reason: "Verify the returned specialist's fixture evidence", requests: 1, maxToolCalls: 3, minutes: 1 } } };
    else if (!done("native-nested")) call = { id: "native-nested", name: "codemode", args: { code: 'return await tools.mcp__indirect__probe({value:"parent-codemode"});' } };
    else if (!done("verify-excess")) call = { id: "verify-excess", name: "mcp__fixture__probe", args: { value: "lead-over-verification-budget" } };
    else if (!done("verify-shell")) call = { id: "verify-shell", name: "bash", args: { command: "node -e \"require('node:fs').writeFileSync('private/verification-ok','verified')\"" } };
    else if (!done("end-verify")) call = { id: "end-verify", name: "pentest_workflow", args: { action: "end_verification" } };
    else if (!done("fixture-pause")) call = { id: "fixture-pause", name: "pentest_workflow", args: { action: "pause", data: { reason: "Offline orchestration fixture completed" } } };
    else content = "DELEGATION_COMPLETE";
    completions.push({ child, tool: call?.name, content });
    result.writeHead(200, { "content-type": "text/event-stream" });
    const chunk = delta => ({ id: `mock-${requests}`, object: "chat.completion.chunk", created: 1, model: "fixture", choices: [{ index: 0, delta, finish_reason: null }] });
    if (call) result.write(`data: ${JSON.stringify(chunk({ role: "assistant", tool_calls: [{ index: 0, id: call.id, type: "function", function: { name: call.name, arguments: JSON.stringify(call.args) } }] }))}\n\n`);
    else result.write(`data: ${JSON.stringify(chunk({ role: "assistant", content }))}\n\n`);
    result.write(`data: ${JSON.stringify({ ...chunk({}), choices: [{ index: 0, delta: {}, finish_reason: call ? "tool_calls" : "stop" }] })}\n\n`);
    result.end(`data: ${JSON.stringify({ ...chunk({}), choices: [], usage: { prompt_tokens: 100, completion_tokens: 10, total_tokens: 110 } })}\n\ndata: [DONE]\n\n`);
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const modelConfig = cost => ({ providers: { harness: { baseUrl: `http://127.0.0.1:${server.address().port}/v1`, api: "openai-completions", apiKey: "offline-test-key", models: [{ id: "fixture", name: "Fixture", reasoning: false, input: ["text"], contextWindow: 131072, maxTokens: 1024, cost }] } } });
  fs.writeFileSync(path.join(agentDir, "models.json"), JSON.stringify(modelConfig({ input: 1, output: 1, cacheRead: 0, cacheWrite: 0 })));
  fs.writeFileSync(path.join(agentDir, "settings.json"), JSON.stringify({ defaultProvider: "harness", defaultModel: "fixture", packages: [nativePackage, askPackage].filter(Boolean), skills: [] }));

  function start(extra) {
    const child = spawn(binary, ["--dir", project, "--pi", pi, "--opencode-config", config, "--offline", "--rpc", ...extra], { env: { ...process.env, PI_CODING_AGENT_DIR: agentDir, OPENCODE_CONFIG: config, PENTEST_HARNES_SUBAGENTS_DIR: nativePackage, BILI_NATIVE_PI: "0", ACP_AUTO_UPDATE: "0", PI_TELEMETRY: "0" }, stdio: ["pipe", "pipe", "pipe"] });
    const records = [];
    const diagnostics = [];
    let stderr = "";
    child.stderr.on("data", data => { stderr += data; });
    const lines = readline.createInterface({ input: child.stdout });
    lines.on("line", line => { try {
      const record = JSON.parse(line);
      records.push(record);
      if (askPackage && record.type === "extension_ui_request" && record.method === "select") child.stdin.write(JSON.stringify({ type: "extension_ui_response", id: record.id, value: record.options[0] }) + "\n");
    } catch { diagnostics.push(line); } });
    const send = message => child.stdin.write(JSON.stringify(message) + "\n");
    const wait = async predicate => {
      const deadline = Date.now() + 30000;
      while (Date.now() < deadline) {
        const match = records.find(predicate);
        if (match) return match;
        if (child.exitCode !== null) throw new Error(`Pi exited ${child.exitCode}: ${stderr}\n${diagnostics.join("\n")}\n${JSON.stringify(records).slice(-5000)}`);
        await new Promise(resolve => setTimeout(resolve, 25));
      }
      throw new Error(`Timed out waiting for Pi: ${stderr}\n${JSON.stringify(records).slice(-8000)}`);
    };
    t.after(() => { if (child.exitCode === null) child.kill("SIGTERM"); });
    return { child, records, diagnostics, send, wait, close: async () => { child.stdin.end(); await Promise.race([once(child, "exit"), new Promise((_, reject) => setTimeout(() => { child.kill("SIGTERM"); reject(new Error("Pi did not exit on EOF")); }, 10000).unref())]); } };
  }

  const first = start(["--new", "--name", "Offline smoke"]);
  first.send({ id: "commands", type: "get_commands" });
  const commands = await first.wait(event => event.type === "response" && event.id === "commands");
  assert.ok(commands.success);
  assert.deepEqual(first.diagnostics, [], "RPC stdout was not JSON-lines");
  assert.ok(commands.data.commands.some(command => command.name === "pentest-run"), JSON.stringify(commands));
  const wordpressSkill = commands.data.commands.find(command => command.name === "skill:wordpress-pentest");
  assert.ok(wordpressSkill, "Pi did not parse/load the bundled WordPress skill");
  first.send({ id: "prompt", type: "prompt", message: "DELEGATE_SMOKE: execute the offline fixture sequence." });
  await first.wait(event => event.type === "agent_end");
  const sessionsDir = path.join(project, ".pentest-harnes", "sessions");
  const sessionID = fs.readdirSync(sessionsDir)[0];
  const sessionRoot = path.join(sessionsDir, sessionID);
  assert.equal(wordpressSkill.sourceInfo?.path ?? wordpressSkill.path,
    path.join(sessionRoot, "workspace", ".pi", "skills", "wordpress-pentest", "SKILL.md"));
  const state = JSON.parse(fs.readFileSync(path.join(sessionRoot, "state.json"), "utf8"));
  assert.equal(state.tasks["smoke-worker"]?.status, "returned", JSON.stringify(first.records).slice(-8000));
  assert.equal(state.tasks["future-auth"]?.status, "pending");
  assert.equal(state.checklist.find(item => item.id === "C001").status, "tested");
  assert.equal(state.checklist.find(item => item.id === "C002").status, "not_applicable");
  assert.equal(fs.readFileSync(path.join(sessionRoot, "events.jsonl"), "utf8").split("\n").filter(line => line.includes('"event":"checklist"')).length, 1);
  assert.equal(fs.existsSync(path.join(sessionRoot, "specialists", "future-auth")), false, "Future-phase work ran before its gate");
  assert.equal(Object.values(state.tasks).some(task => task.bootstrap || task.id.startsWith("setup-inventory")), false, "No automatic inventory workers should exist");
  assert.equal(targetRequests, 1, "Only the lead's deliberate loopback request is allowed; offline workers must remain offline");
  assert.equal(fs.readFileSync(path.join(sessionRoot, "workspace", "lead-full-tools"), "utf8"), "working");
  if (askPackage) {
    assert.ok(first.records.some(record => record.type === "extension_ui_request" && record.method === "select" && record.title.includes("offline question fixture")), `Question dialog was never shown: ${questionAnswer}`);
    assert.match(questionAnswer, /Continue offline/);
  }
  if (nativePackage) {
    assert.equal(parallelReceipt.specialists.length, 4);
    assert.equal(new Set(parallelReceipt.specialists.map(worker => worker.pid)).size, 4);
    const events = fs.readFileSync(path.join(sessionRoot, "events.jsonl"), "utf8").trim().split("\n").map(JSON.parse);
    let running = 0, peak = 0;
    for (const { event } of events) {
      if (event.startsWith("task-started:")) peak = Math.max(peak, ++running);
      if (event.startsWith("task-returned:")) running--;
    }
    assert.ok(peak >= 4, `Workers did not overlap: peak ${peak}`);
  }
  assert.equal(fs.readFileSync(path.join(sessionRoot, "workspace", "private", "verification-ok"), "utf8"), "verified");
  const executed = Object.values(state.tasks).filter(task => task.status === "returned");
  for (const task of executed) {
    assert.ok(Number.isSafeInteger(task.execution.pid) && task.execution.pid > 0);
    assert.notEqual(task.execution.pid, task.execution.parentPid);
    assert.equal(task.execution.exitCode, 0);
    assert.ok(task.execution.toolCalls > 0);
    const processRecord = JSON.parse(fs.readFileSync(path.join(sessionRoot, "specialists", task.id, "process.json"), "utf8"));
    assert.equal(processRecord.pid, task.execution.pid);
    assert.equal(processRecord.status, "returned");
    if (nativePackage) {
      assert.equal(processRecord.backend, "pi-subagents");
      assert.ok(processRecord.runId && processRecord.sessionFile && processRecord.guardClaim);
    } else assert.ok(processRecord.browserSession.includes(task.id));
  }
  assert.equal(new Set(executed.map(task => task.execution.pid)).size, executed.length);
  if (nativePackage) assert.match(nativeInspection, /worker-native|smoke-worker\.md/, "Native Fleet could not inspect the actual worker transcript");
  assert.ok(completions.some(item => item.child && item.content === "SPECIALIST_COMPLETE"), JSON.stringify(completions));
  const nativeCalls = fs.readFileSync(mcpCalls, "utf8").trim().split("\n").map(JSON.parse);
  assert.deepEqual(nativeCalls.map(call => call.value).sort(), ["before-scope", "lead-without-verification", "parent-codemode", "worker-native"].sort(), "Lead tools or specialist/optional replay budgets failed");
  assert.equal(JSON.parse(fs.readFileSync(path.join(sessionRoot, "specialists", "smoke-worker", "requests.json"), "utf8")).count, 1);
  const toolEvents = fs.readFileSync(path.join(sessionRoot, "tool-events.jsonl"), "utf8").trim().split("\n").map(JSON.parse);
  assert.ok(toolEvents.some(event => event.tool === "mcp__indirect__probe"), "Nested native MCP calls bypassed the harness tool pipeline");
  first.send({ id: "auto", type: "prompt", message: "AUTO_LOOP_SMOKE: start then automatically continue and pause at the fixture exception." });
  await first.wait(event => event.type === "agent_end" && first.records.filter(item => item.type === "agent_end").length >= 3);
  const paused = JSON.parse(fs.readFileSync(path.join(sessionRoot, "state.json"), "utf8"));
  assert.equal(paused.mode, "paused", JSON.stringify(first.records).slice(-8000));
  assert.match(paused.pauseReason, /auto-continuation verified/);
  assert.ok(completions.some(item => item.content === "AUTO_LOOP_COMPLETE"));
  const ledgerDir = path.join(sessionRoot, "launches");
  const originalLedger = fs.readdirSync(ledgerDir)[0];
  const original = JSON.parse(fs.readFileSync(path.join(ledgerDir, originalLedger), "utf8"));
  assert.ok(original.spentUSD > 0);
  assert.equal(Object.keys(original.events).length, requests, "specialist usage was omitted or double-counted");
  assert.equal(Object.keys(original.reservations).length, 0);
  await first.close();
  assert.equal(fs.existsSync(path.join(sessionRoot, "launch.lock")), false);

  // Simulate a worker whose orchestrator has died. It must stop before paying
  // for another model request; its partial contract is reconciled on resume.
  const crashedState = JSON.parse(fs.readFileSync(path.join(sessionRoot, "state.json"), "utf8"));
  crashedState.tasks["orphan-worker"] = { ...contract, id: "orphan-worker", status: "running", startedAt: new Date().toISOString(), output: "reports/orphan-worker.md" };
  fs.writeFileSync(path.join(sessionRoot, "state.json"), JSON.stringify(crashedState));
  const requestsBeforeOrphan = requests;
  const orphan = spawn(pi, ["--approve", "--offline", "--print", "--mode", "json", "--session-dir", path.join(sessionRoot, "specialists", "orphan-worker"), "--extension", path.join(sessionRoot, "runtime", "extension.ts"), "--provider", "harness", "--model", "fixture", "An orphan worker must not issue another model request."], { cwd: path.join(sessionRoot, "workspace"), env: { ...process.env, PI_CODING_AGENT_DIR: agentDir, PENTEST_HARNES_SESSION: sessionRoot, PENTEST_HARNES_LAUNCH: path.join(ledgerDir, originalLedger), PENTEST_HARNES_TASK: "orphan-worker", PENTEST_HARNES_PARENT_PID: String(first.child.pid), PI_TELEMETRY: "0" }, stdio: ["ignore", "ignore", "ignore"] });
  t.after(() => { if (orphan.exitCode === null) orphan.kill("SIGTERM"); });
  await once(orphan, "exit");
  assert.equal(requests, requestsBeforeOrphan);
  assert.match(JSON.parse(fs.readFileSync(path.join(sessionRoot, "specialists", "orphan-worker", "interrupted.json"), "utf8")).reason, /Orchestrator exited/);

  const brokenExtension = path.join(root, "broken.ts");
  fs.writeFileSync(brokenExtension, "throw new Error('fixture extension load failure');\n");
  const broken = spawn(pi, ["--offline", "--print", "--extension", brokenExtension, "--pentest-runtime", "--provider", "harness", "--model", "fixture", "A missing workflow extension must not dispatch this prompt."], { cwd: root, env: { ...process.env, PI_CODING_AGENT_DIR: agentDir, PI_TELEMETRY: "0" }, stdio: "ignore" });
  t.after(() => { if (broken.exitCode === null) broken.kill("SIGTERM"); });
  const [brokenCode] = await once(broken, "exit");
  assert.notEqual(brokenCode, 0);
  assert.equal(requests, requestsBeforeOrphan, "failed extension loading silently dispatched a model request");

  // A resumed launch retains the task state and history but gets a fresh ledger.
  // Unknown zero pricing must abort BEFORE another model HTTP request is sent.
  const unpriced = modelConfig({ input: 0, output: 0, cacheRead: 0, cacheWrite: 0 });
  unpriced.providers.harness.models.push({ ...unpriced.providers.harness.models[0], id: "alternate" });
  fs.writeFileSync(path.join(agentDir, "models.json"), JSON.stringify(unpriced));
  const previousRequests = requests;
  const second = start(["--session", sessionID]);
  second.send({ id: "state", type: "get_state" });
  const restored = await second.wait(event => event.type === "response" && event.id === "state");
  assert.equal(restored.data.sessionId, sessionID);
  const newLedgerName = fs.readdirSync(ledgerDir).find(name => name !== originalLedger && name.endsWith(".json"));
  assert.ok(newLedgerName);
  const resumedLedgerPath = path.join(ledgerDir, newLedgerName);
  const resumedLedger = JSON.parse(fs.readFileSync(resumedLedgerPath, "utf8"));
  assert.equal(resumedLedger.budgetMode, "estimate");
  // Exercise the retained strict mode explicitly; normal launches now use the
  // user's requested estimate-only accounting with no monetary cutoff.
  fs.writeFileSync(resumedLedgerPath, JSON.stringify({ ...resumedLedger, budgetMode: "strict" }));
  second.send({ id: "blocked-prompt", type: "prompt", message: "This request must be blocked because pricing is unknown." });
  await second.wait(event => event.type === "agent_end");
  assert.equal(requests, previousRequests, "unknown pricing still dispatched a paid model request");
  const resumedState = JSON.parse(fs.readFileSync(path.join(sessionRoot, "state.json"), "utf8"));
  assert.equal(resumedState.tasks["smoke-worker"].status, "returned");
  assert.equal(resumedState.tasks["orphan-worker"].status, "interrupted");
  assert.equal(resumedState.mode, "paused");
  assert.equal(JSON.parse(fs.readFileSync(path.join(ledgerDir, newLedgerName), "utf8")).spentUSD, 0);

  second.send({ id: "switch-model", type: "set_model", provider: "harness", modelId: "alternate" });
  const switched = await second.wait(event => event.type === "response" && event.id === "switch-model");
  assert.ok(switched.success);

  // Pi treats hook exceptions as diagnostics. Even if writing the pause
  // checkpoint fails, the guard must abort the provider request first.
  const statePath = path.join(sessionRoot, "state.json");
  const savedState = fs.readFileSync(statePath, "utf8");
  fs.unlinkSync(statePath);
  fs.mkdirSync(statePath);
  try {
    const previousEnds = second.records.filter(item => item.type === "agent_end").length;
    second.send({ id: "io-failure", type: "prompt", message: "A checkpoint I/O failure must not bypass the budget guard." });
    await second.wait(event => event.type === "agent_end" && second.records.filter(item => item.type === "agent_end").length > previousEnds);
    assert.equal(requests, previousRequests);
    assert.ok(second.records.some(event => JSON.stringify(event).includes("checkpoint write failed")));
  } finally {
    fs.rmdirSync(statePath);
    fs.writeFileSync(statePath, savedState);
  }
  await second.close();
});

test("real Pi: a transient lead model error resumes from checkpoint without user input", { skip: !enabled, timeout: 55000 }, async t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "pentest-lead-recovery-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const agent = path.join(root, "agent"), project = path.join(root, "project");
  fs.mkdirSync(agent); fs.mkdirSync(project);
  fs.writeFileSync(path.join(agent, "settings.json"), JSON.stringify({ defaultProvider: "harness", defaultModel: "fixture", packages: [] }));
  fs.writeFileSync(path.join(agent, "mcp.json"), JSON.stringify({ mcpServers: {} }));
  const config = path.join(root, "opencode.json");
  fs.writeFileSync(config, '{"mcp":{}}');
  let attempts = 0;
  const server = http.createServer(async (request, result) => {
    for await (const _ of request) { /* drain the local mock request */ }
    attempts++;
    if (attempts === 2) {
      result.writeHead(503, { "content-type": "application/json" });
      result.end('{"error":{"message":"transient local fixture outage"}}');
      return;
    }
    const call = attempts === 1 ? { id: "start-run", name: "pentest_workflow", args: { action: "run" } }
      : attempts === 3 ? { id: "recovered-pause", name: "pentest_workflow", args: { action: "pause", data: { reason: "Lead auto-recovered from local model error" } } } : null;
    result.writeHead(200, { "content-type": "text/event-stream" });
    const delta = part => ({ id: `recover-${attempts}`, object: "chat.completion.chunk", created: 1, model: "fixture", choices: [{ index: 0, delta: part, finish_reason: null }] });
    if (call) result.write(`data: ${JSON.stringify(delta({ role: "assistant", tool_calls: [{ index: 0, id: call.id, type: "function", function: { name: call.name, arguments: JSON.stringify(call.args) } }] }))}\n\n`);
    else result.write(`data: ${JSON.stringify(delta({ role: "assistant", content: "Recovered and paused fixture cleanly." }))}\n\n`);
    result.write(`data: ${JSON.stringify({ ...delta({}), choices: [{ index: 0, delta: {}, finish_reason: call ? "tool_calls" : "stop" }] })}\n\n`);
    result.end(`data: ${JSON.stringify({ ...delta({}), choices: [], usage: { prompt_tokens: 50, completion_tokens: 5, total_tokens: 55 } })}\n\ndata: [DONE]\n\n`);
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  fs.writeFileSync(path.join(agent, "models.json"), JSON.stringify({ providers: { harness: { baseUrl: `http://127.0.0.1:${server.address().port}/v1`, api: "openai-completions", apiKey: "offline-fixture-key", models: [{ id: "fixture", name: "Fixture", reasoning: false, input: ["text"], contextWindow: 131072, maxTokens: 1024, cost: { input: 1, output: 1, cacheRead: 0, cacheWrite: 0 } }] } } }));
  const child = spawn(binary, ["--dir", project, "--pi", pi, "--opencode-config", config, "--new", "--offline", "--rpc"],
    { env: { ...process.env, PI_CODING_AGENT_DIR: agent, OPENCODE_CONFIG: config, PENTEST_HARNES_SUBAGENTS_DIR: "", BILI_NATIVE_PI: "0", ACP_AUTO_UPDATE: "0", PI_TELEMETRY: "0" }, stdio: ["pipe", "pipe", "pipe"] });
  t.after(() => { if (child.exitCode === null) child.kill("SIGTERM"); });
  let stderr = "";
  child.stderr.on("data", data => { stderr += data; });
  const records = [];
  readline.createInterface({ input: child.stdout }).on("line", line => { try { records.push(JSON.parse(line)); } catch {} });
  child.stdin.write(JSON.stringify({ id: "start", type: "prompt", message: "RECOVERY_SMOKE: start then handle one local provider failure." }) + "\n");
  const deadline = Date.now() + 35000;
  let state;
  while (Date.now() < deadline) {
    const dirs = path.join(project, ".pentest-harnes/sessions");
    if (fs.existsSync(dirs)) {
      const names = fs.readdirSync(dirs);
      if (names.length) {
        const file = path.join(dirs, names[0], "state.json");
        if (fs.existsSync(file)) state = JSON.parse(fs.readFileSync(file, "utf8"));
      }
    }
    if (state?.pauseReason === "Lead auto-recovered from local model error") break;
    if (child.exitCode !== null) throw new Error(`Pi exited early: ${stderr}`);
    await new Promise(resolve => setTimeout(resolve, 30));
  }
  assert.equal(state?.pauseReason, "Lead auto-recovered from local model error", `attempts=${attempts}; stderr=${stderr}; events=${JSON.stringify(records).slice(-3500)}`);
  assert.equal(state.mode, "paused");
  assert.equal(attempts, 4, "transient model error did not trigger one retry and a final pause response");
  assert.ok(records.some(record => JSON.stringify(record).includes("Lead model error; retrying from saved checkpoint")));
  child.stdin.end();
  await once(child, "exit");
});

test("real user Pi config: native Fleet, compression and todo load without model/inbox/SMS tools", { skip: !enabled || process.env.PENTEST_HARNES_TEST_USER_CONFIG !== "1", timeout: 75000 }, async t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "pentest-user-config-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const userEnv = { ...process.env, PI_TELEMETRY: "0" };
  if (process.env.BILI_NATIVE_PI !== "0") {
    // billion-context deliberately suppresses native bootstrap inside node:test.
    // This opt-in process is a real startup integration check, not a unit fixture.
    for (const key of ["NODE_TEST_CONTEXT", "BILI_PROVIDER_REWRITES", "BILLION_CONTEXT_PROXY", "BILLION_CONTEXT_ATTACH"]) delete userEnv[key];
    Object.assign(userEnv, { BILI_NATIVE_PI: "1", BILLION_CONTEXT_PLUGIN: "1", BILI_PI_INLINE: "0" });
  }
  const child = spawn(binary, ["--dir", root, "--pi", pi, "--new", "--offline", "--rpc"], { env: userEnv, stdio: ["pipe", "pipe", "pipe"] });
  t.after(() => { if (child.exitCode === null) child.kill("SIGTERM"); });
  let stderr = "";
  child.stderr.on("data", data => { stderr += data; });
  const records = [];
  const lines = readline.createInterface({ input: child.stdout });
  lines.on("line", line => { try { records.push(JSON.parse(line)); } catch {} });
  child.stdin.write(JSON.stringify({ id: "commands", type: "get_commands" }) + "\n");
  const deadline = Date.now() + 30000;
  while (!records.some(item => item.type === "response" && item.id === "commands")) {
    if (Date.now() >= deadline || child.exitCode !== null) throw new Error(`Integration startup failed: ${stderr}`);
    await new Promise(resolve => setTimeout(resolve, 25));
  }
  const commands = records.find(item => item.type === "response" && item.id === "commands");
  assert.ok(commands.success);
  const mcpCommand = commands.data.commands.find(command => command.name === "mcp");
  assert.ok(mcpCommand);
  assert.doesNotMatch(JSON.stringify(mcpCommand), /pi-mcp-adapter/);
  assert.ok(commands.data.commands.some(command => command.name === "pentest-run"));
  assert.ok(commands.data.commands.some(command => command.name === "pentest-agents"));
  assert.ok(commands.data.commands.some(command => command.name === "subagents-fleet"), "Native Fleet inspector command is unavailable");
  assert.ok(commands.data.commands.some(command => command.name === "acp"), "billion-context command did not load");
  child.stdin.write(JSON.stringify({ id: "integrations", type: "prompt", message: "/pentest-integrations" }) + "\n");
  let integration;
  const integrationDeadline = Date.now() + 20000;
  while (!integration && Date.now() < integrationDeadline) {
    for (const record of records) {
      const message = record.message ?? record.text;
      if (typeof message === "string" && message.startsWith("PENTEST_INTEGRATIONS_JSON:")) integration = JSON.parse(message.slice("PENTEST_INTEGRATIONS_JSON:".length));
    }
    if (!integration) await new Promise(resolve => setTimeout(resolve, 25));
  }
  assert.ok(integration, "Integration status command did not respond");
  assert.equal(integration.nativeFleet, true, integration.fleetError);
  assert.equal(integration.todo, true, "Pix Todo tool did not register");
  assert.equal(integration.estimatedSpending, true);
  if (process.env.BILI_NATIVE_PI !== "0") {
    // The package's session_start registration is fire-and-forget and can miss
    // a proxy still booting. /acp proves readiness without a paid model request;
    // provider hooks register tools on the first actual request.
    const readyDeadline = Date.now() + 30000;
    while (!records.some(record => JSON.stringify(record).includes("compression armed")) && Date.now() < readyDeadline) {
      child.stdin.write(JSON.stringify({ id: "compression-ready", type: "prompt", message: "/acp" }) + "\n");
      await new Promise(resolve => setTimeout(resolve, 500));
    }
    const diagnostics = records.filter(record => record.type === "extension_ui_request" || record.type === "message_end").slice(-6).map(record => JSON.stringify(record).slice(0, 1400)).join("\n");
    assert.ok(records.some(record => JSON.stringify(record).includes("compression armed")), `Compression proxy did not become ready: ${stderr}\n${diagnostics}`);
  }
  assert.doesNotMatch(stderr, /Failed to load extension|Duplicate tool/i);
  const sessions = path.join(root, ".pentest-harnes", "sessions");
  const session = path.join(sessions, fs.readdirSync(sessions)[0]);
  for (const name of ["interceptor", "interceptor-browser", "interceptor-research", "wordpress-pentest"]) {
    const skill = commands.data.commands.find(command => command.name === `skill:${name}`);
    assert.ok(skill, `Pi did not load ${name}`);
    assert.equal(skill.sourceInfo?.path ?? skill.path, path.join(session, "workspace", ".pi", "skills", name, "SKILL.md"), `Pi did not use the bundled ${name} skill`);
  }
  const configFile = path.join(session, "workspace", ".pi", "mcp.json");
  const servers = JSON.parse(fs.readFileSync(configFile, "utf8")).mcpServers;
  for (const required of ["burp", "caido", "textbee"]) assert.ok(Object.keys(servers).some(name => name.includes(required)), `missing ${required}`);
  if (process.platform !== "win32") assert.equal(fs.statSync(configFile).mode & 0o777, 0o600);
  const launches = path.join(session, "launches");
  assert.equal(JSON.parse(fs.readFileSync(path.join(launches, fs.readdirSync(launches)[0]), "utf8")).spentUSD, 0);
  child.stdin.end();
  await once(child, "exit");
});
