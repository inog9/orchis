// Minimal stdio MCP server for local integration tests, never a live service.
import fs from "node:fs";
import readline from "node:readline";

const log = process.argv[2];
const input = readline.createInterface({ input: process.stdin });
const send = message => process.stdout.write(JSON.stringify(message) + "\n");
input.on("line", line => {
  const request = JSON.parse(line);
  if (request.id === undefined) return;
  let result;
  switch (request.method) {
    case "initialize":
      result = { protocolVersion: "2025-03-26", capabilities: { tools: {} }, serverInfo: { name: "harness-fixture", version: "1.0.0" } };
      break;
    case "ping": result = {}; break;
    case "tools/list":
      result = { tools: [{ name: "probe", description: "Return a local fixture value", inputSchema: { type: "object", properties: { value: { type: "string" } }, required: ["value"] } }] };
      break;
    case "tools/call":
      fs.appendFileSync(log, JSON.stringify({ pid: process.pid, name: request.params.name, value: request.params.arguments.value }) + "\n");
      result = { content: [{ type: "text", text: `Native MCP fixture: ${request.params.arguments.value}` }] };
      break;
    default:
      send({ jsonrpc: "2.0", id: request.id, error: { code: -32601, message: "Method not found" } });
      return;
  }
  send({ jsonrpc: "2.0", id: request.id, result });
});
