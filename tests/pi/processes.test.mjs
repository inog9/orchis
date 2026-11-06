import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { terminateSpecialist } from "../../pi-runtime/specialists.mjs";

function running(pid) {
  try {
    process.kill(pid, 0);
    if (process.platform === "linux") {
      const stat = fs.readFileSync(`/proc/${pid}/stat`, "utf8");
      if (stat.slice(stat.lastIndexOf(")") + 2).startsWith("Z ")) return false;
    }
    return true;
  } catch { return false; }
}

test("cancellation retains group teardown after the immediate worker exits", { skip: process.platform === "win32", timeout: 10000 }, async t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "pentest-process-stop-"));
  const marker = path.join(root, "descendant.pid");
  const fixture = path.join(root, "worker.mjs");
  // A local subprocess fixture, not a model/agent. The descendant deliberately
  // survives its parent's SIGTERM and owns no inherited stdout/stderr handles.
  fs.writeFileSync(fixture, `import fs from "node:fs"; import {spawn} from "node:child_process";
const child=spawn(process.execPath,["-e","process.on('SIGTERM',()=>{});process.send('ready');setInterval(()=>{},1000);"],{stdio:["ignore","ignore","ignore","ipc"]});
child.on("message",()=>fs.writeFileSync(process.argv[2],String(child.pid)));
process.on("SIGTERM",()=>process.exit(0));
setInterval(()=>{},1000);
`);
  const worker = spawn(process.execPath, [fixture, marker], { detached: true, stdio: "ignore" });
  let descendant;
  t.after(() => {
    try { process.kill(-worker.pid, "SIGKILL"); } catch {}
    if (descendant && running(descendant)) try { process.kill(descendant, "SIGKILL"); } catch {}
    fs.rmSync(root, { recursive: true, force: true });
  });
  const deadline = Date.now() + 5000;
  while (!fs.existsSync(marker)) {
    assert.ok(Date.now() < deadline, "Subprocess fixture did not start");
    await new Promise(resolve => setTimeout(resolve, 10));
  }
  descendant = Number(fs.readFileSync(marker, "utf8"));
  assert.ok(running(descendant));
  const exited = once(worker, "close");
  const stop = terminateSpecialist(worker, { killAfterMs: 300 });
  assert.equal(terminateSpecialist(worker), stop, "Concurrent cancellation must share one teardown");
  let complete = false;
  stop.then(() => { complete = true; });
  await exited;
  assert.ok(running(descendant), "Fixture did not reproduce a surviving descendant");
  await new Promise(resolve => setTimeout(resolve, 40));
  assert.equal(complete, false, "Teardown stopped tracking as soon as the immediate worker exited");
  await stop;
  const stoppedAt = Date.now() + 2000;
  while (running(descendant) && Date.now() < stoppedAt) await new Promise(resolve => setTimeout(resolve, 10));
  assert.equal(running(descendant), false, "Owned descendant survived hard teardown");
});
