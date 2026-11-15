import { Budget } from "../../pi-runtime/budget.mjs";
const [ledger, root] = process.argv.slice(2);
try {
  const id = new Budget(ledger, root).reserve({ provider: "fixture", id: "concurrent", contextWindow: 2000000, maxTokens: 2000000, cost: { input: 1, output: 1, cacheRead: 0, cacheWrite: 0 } }, {});
  process.stdout.write(JSON.stringify({ reserved: true, id }) + "\n");
} catch (error) {
  process.stdout.write(JSON.stringify({ reserved: false, message: error.message }) + "\n");
}
