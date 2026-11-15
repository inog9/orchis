import test from "node:test";
import crypto from "node:crypto";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawn, execFileSync } from "node:child_process";
import { once } from "node:events";
import { fileURLToPath } from "node:url";
import { parseArguments } from "../../docker/run-job.mjs";

const runner = fileURLToPath(new URL("../../docker/run-job.mjs", import.meta.url));
const secret = "authorized-fixture-password-do-not-print";

function fixture(t, scenario, { resumed = false, stdin = false, scannerInputs = false } = {}) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "pentest-job-test-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const harness = path.join(dir, "harness.mjs");
  fs.writeFileSync(harness, `#!${process.execPath}
import fs from "node:fs";
import path from "node:path";
import { spawn } from "node:child_process";
import crypto from "node:crypto";
let buffer = "";
const root = ${JSON.stringify(path.join(dir, "session"))};
const scenario = ${JSON.stringify(scenario)};
fs.mkdirSync(path.join(root,"workspace"),{recursive:true});
fs.writeFileSync(path.join(root,"args.json"),JSON.stringify({args:process.argv.slice(2),unattended:process.env.PENTEST_HARNES_UNATTENDED}));
const state = { mode:${JSON.stringify(resumed ? "paused" : "running")}, phase:"setup",revision:4,tasks:{worker:{status:"running"}}, pauseReason:${JSON.stringify(secret)} };
const outputs = ['fixture_PENTEST_REPORT.md','fixture_VALIDATION_REASON.md'].map(name=>{
 const content='Fixture report: no target requests';fs.writeFileSync(path.join(root,'workspace',name),content);
 return {path:name,sha256:crypto.createHash('sha256').update(content).digest('hex')};
});
state.phases={report:{completedAt:'2026-11-07T00:00:00Z',outputs}};
const save = () => fs.writeFileSync(path.join(root,"state.json"),JSON.stringify(state));
save();
if(scenario==="force-cancel"){
 process.on("SIGTERM",()=>{});
 const descendant=spawn(process.execPath,["-e",'process.on("SIGTERM",()=>{});setInterval(()=>{},1000)'],{stdio:"ignore"});
 fs.writeFileSync(path.join(root,"descendant.pid"),String(descendant.pid));
}
fs.writeFileSync(process.env.PENTEST_HARNES_JOB_STATUS_FILE,JSON.stringify({sessionId:"job-fixture",root,workspace:path.join(root,"workspace")}));
const emit = record => process.stdout.write(JSON.stringify(record)+"\\n");
process.stderr.write(${JSON.stringify(secret + " diagnostic\n")});
process.stdin.setEncoding("utf8");
process.stdin.on("data",chunk=>{
 buffer+=chunk;let newline;
 while((newline=buffer.indexOf("\\n"))!==-1){
  const record=JSON.parse(buffer.slice(0,newline));buffer=buffer.slice(newline+1);
  fs.appendFileSync(path.join(root,"received.jsonl"),JSON.stringify(record)+"\\n");
  if(record.type==="get_state"){
   emit({type:"extension_ui_request",method:"notify",message:${JSON.stringify(secret)}});
   if(scenario!=="no-handshake") emit({type:"response",id:record.id,command:"get_state",success:scenario!=="bad-handshake",data:{isStreaming:false}});
  }
  if(record.type==="prompt" && record.message==="/pentest-pause"){
   state.mode="paused";state.revision++;save();
   fs.writeFileSync(path.join(root,"pause-received"),"yes");
   emit({type:"response",id:record.id,success:true});
  } else if(record.type==="prompt"){
   fs.writeFileSync(path.join(root,"prompt.txt"),record.message);
   emit({type:"response",id:record.id,success:scenario!=="prompt-rejected"});
   if(scenario==="prompt-rejected") continue;
   if(scenario==="unicode") process.stdout.write(JSON.stringify({type:"message_update",text:"valid separator\\u2028and paragraph\\u2029"})+"\\n");
   if(scenario==="interactive") setTimeout(()=>emit({type:"extension_ui_request",id:"private-question",method:"select",title:${JSON.stringify(secret)},options:["Continue","Pause"]}),20);
   else {
    if(${JSON.stringify(resumed)}) {state.mode="running";state.revision++;save();}
    setTimeout(()=>emit({type:"agent_end",messages:[{text:${JSON.stringify(secret)}}]}),15);
    if(scenario!=="running" && scenario!=="force-cancel") setTimeout(()=>{state.mode=scenario==="limited"?"limited":scenario==="paused"?"paused":"complete";state.phase="report";state.revision++;state.tasks.worker.status="verified";save();},150);
   }
  }
 }
});
process.stdin.on("end",()=>{fs.writeFileSync(path.join(root,"stdin-ended"),"yes");process.exit(0)});
setInterval(()=>{},1000);
`, { mode: 0o755 });
  const promptFile = path.join(dir, "request.txt");
  fs.writeFileSync(promptFile, `Test https://fixture.test with supplied credentials ${secret}`);
  const args = [runner, "--harness", harness, "--dir", dir, "--prompt-file", stdin ? "-" : promptFile, "--poll-ms", "20", "--startup-timeout-ms", scenario==="no-handshake"?"300":"3000", "--shutdown-timeout-ms", "500"];
  if (scannerInputs) {
    const source=path.join(dir,'scanner-source');fs.mkdirSync(source);
    const sources=['nmap','nuclei'].map(tool=>{const name=tool+'.json',data=Buffer.from(JSON.stringify({tool,status:'finished',findings:[{title:tool+' seed'}]}));fs.writeFileSync(path.join(source,name),data);return {name,bytes:data.length,sha256:crypto.createHash('sha256').update(data).digest('hex')}});
    fs.writeFileSync(path.join(source,'manifest.json'),JSON.stringify({schema:1,sources}));args.push('--inputs-dir',source);
  }
  if (resumed) args.push("--session", "job-fixture");
  else args.push("--name", "Offline job test", "--pi", "/test/pi");
  const child = spawn(process.execPath, args, { stdio: ["pipe", "pipe", "pipe"] });
  let output = "", errorOutput = "";
  const events = [];
  let partial = "";
  child.stdout.setEncoding("utf8");
  child.stdout.on("data", chunk => {
    output += chunk; partial += chunk;
    let newline;
    while ((newline = partial.indexOf("\n")) !== -1) {
      events.push(JSON.parse(partial.slice(0, newline))); partial = partial.slice(newline + 1);
    }
  });
  child.stderr.on("data", chunk => errorOutput += chunk);
  if (stdin) child.stdin.end(fs.readFileSync(promptFile)); else child.stdin.end();
  t.after(() => { if (child.exitCode === null && child.signalCode === null) child.kill("SIGKILL"); });
  const completion = once(child, "close").then(([code]) => ({ code, output, errorOutput, events, root:path.join(dir,"session") }));
  return { child, completion, events, dir, root:path.join(dir,"session") };
}

test("job argument contract", () => {
  const args = parseArguments(["--prompt-file", "-", "--session", "existing"]);
  assert.equal(args.dir, "/data");
  assert.equal(args.harness, "pentest-harnes");
  assert.equal(args.shutdownTimeoutMs, 60000);
  assert.throws(() => parseArguments([]));
  assert.throws(() => parseArguments(["--prompt-file", "-", "--poll-ms", "0"]));
  assert.throws(() => parseArguments(["--prompt-file", "-", "--session", "x", "--name", "y"]));
});

test("job waits past agent_end for workflow completion and keeps private logs", { timeout: 5000, skip:process.platform==="win32" }, async t => {
  const { completion } = fixture(t,"complete");
  const result = await completion;
  assert.equal(result.code,0,result.output+result.errorOutput);
  assert.equal(result.events.at(-1).event,"complete");
  assert.ok(result.events.some(event=>event.event==="state" && event.mode==="running"));
  assert.ok(fs.existsSync(path.join(result.root,"stdin-ended")));
  assert.ok(!result.output.includes(secret));
  assert.equal(result.errorOutput,"");
  const started=result.events.find(event=>event.event==="started");
  const privateRPC=fs.readFileSync(path.join(started.logs,"rpc.jsonl"),"utf8");
  assert.ok(privateRPC.includes(secret));
  assert.equal(fs.statSync(path.join(started.logs,"rpc.jsonl")).mode & 0o777,0o600);
  assert.equal(fs.statSync(started.logs).mode & 0o777,0o700);
  const received=fs.readFileSync(path.join(result.root,"received.jsonl"),"utf8").trim().split("\n").map(JSON.parse);
  assert.equal(received[0].type,"get_state");
  assert.ok(received.find(record=>record.id==="job-prompt").message.includes("Record the stated scope through pentest_workflow"));
  const env=JSON.parse(fs.readFileSync(path.join(result.root,"args.json"),"utf8"));
  assert.equal(env.unattended,"1");
  assert.ok(env.args.includes("--rpc") && env.args.includes("--new") && env.args.includes("/test/pi"));
});

for (const [scenario, code, event] of [["limited",2,"limited"],["paused",3,"input_required"],["interactive",3,"input_required"],["bad-handshake",1,"failed"],["prompt-rejected",1,"failed"],["no-handshake",1,"failed"]]) {
  test(`job ${scenario} has stable exit semantics without disclosing private text`,{timeout:5000,skip:process.platform==="win32"},async t=>{
    const result=await fixture(t,scenario).completion;
    assert.equal(result.code,code,result.output+result.errorOutput);
    assert.equal(result.events.at(-1).event,event);
    assert.ok(!result.output.includes(secret));
    if(scenario==="interactive"){
      const records=fs.readFileSync(path.join(result.root,"received.jsonl"),"utf8").trim().split("\n").map(JSON.parse);
      assert.ok(records.some(record=>record.type==="extension_ui_response" && record.cancelled===true));
      assert.ok(records.some(record=>record.message==="/pentest-pause"));
    }
  });
}

test("resumed paused state gets its new prompt before terminal evaluation",{timeout:5000,skip:process.platform==="win32"},async t=>{
  const result=await fixture(t,"complete",{resumed:true}).completion;
  assert.equal(result.code,0,result.output);
  assert.ok(fs.readFileSync(path.join(result.root,"prompt.txt"),"utf8").includes(secret));
  const args=JSON.parse(fs.readFileSync(path.join(result.root,"args.json"),"utf8")).args;
  assert.ok(args.includes("--session") && !args.includes("--new"));
});

test("stdin prompt and Unicode separators preserve JSONL framing",{timeout:5000,skip:process.platform==="win32"},async t=>{
  const result=await fixture(t,"unicode",{stdin:true}).completion;
  assert.equal(result.code,0,result.output);
  assert.equal(result.events.at(-1).event,"complete");
});

test("SIGTERM asks the controller to pause before stopping the launcher",{timeout:5000,skip:process.platform==="win32"},async t=>{
  const job=fixture(t,"running");
  const until=Date.now()+2000;
  while(!job.events.some(event=>event.event==="started")){
    if(Date.now()>until) assert.fail("job never started");
    await new Promise(resolve=>setTimeout(resolve,10));
  }
  job.child.kill("SIGTERM");
  const result=await job.completion;
  assert.equal(result.code,130,result.output);
  assert.equal(result.events.at(-1).event,"cancelled");
  assert.ok(fs.existsSync(path.join(result.root,"pause-received")));
});

test("installed symlink entrypoint runs the CLI and returns a job identifier",{skip:process.platform==="win32",timeout:3000},async t=>{
  const dir=fs.mkdtempSync(path.join(os.tmpdir(),"pentest-job-link-"));
  t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));
  const link=path.join(dir,"pentest-job");
  fs.symlinkSync(runner,link);
  const child=spawn(process.execPath,[link],{stdio:["ignore","pipe","pipe"]});
  let output="";
  child.stdout.on("data",data=>output+=data);
  const [code]=await once(child,"close");
  assert.equal(code,1);
  const event=JSON.parse(output);
  assert.equal(event.reason,"invalid_arguments");
  assert.match(event.jobId,/^[0-9a-f-]{36}$/);
});

test("hard cancellation stops the launcher's process group",{timeout:5000,skip:process.platform==="win32"},async t=>{
  const job=fixture(t,"force-cancel");
  const until=Date.now()+3000;
  while(!job.events.some(event=>event.event==="started")){
    if(Date.now()>until) assert.fail("job never started");
    await new Promise(resolve=>setTimeout(resolve,10));
  }
  const pid=Number(fs.readFileSync(path.join(job.root,"descendant.pid"),"utf8"));
  job.child.kill("SIGTERM");
  const result=await job.completion;
  assert.equal(result.code,130,result.output);
  assert.equal(result.events.at(-1).forced,true);
  let alive=true;
  for(let attempt=0;attempt<30;attempt++){
    try{
      process.kill(pid,0);
      const status=execFileSync("ps",["-o","stat=","-p",String(pid)],{encoding:"utf8"}).trim();
      if(!status || status.startsWith("Z")){alive=false;break;}
    }catch{alive=false;break;}
    await new Promise(resolve=>setTimeout(resolve,20));
  }
  if(alive) process.kill(pid,"SIGKILL");
  assert.equal(alive,false,"unresponsive descendant survived forced job cancellation");
});

test('scanner handoff is present in the actual Pi workspace before engagement delivery',async t=>{
 const result=await fixture(t,'complete',{scannerInputs:true}).completion;
 assert.equal(result.code,0,result.output);
 const started=result.events.find(e=>e.event==='started');
 assert.equal(started.scannerInputs,path.join(fs.realpathSync(result.root),'workspace','inputs','scanners'));
 assert.equal(JSON.parse(fs.readFileSync(path.join(started.scannerInputs,'nuclei.json'))).findings[0].title,'nuclei seed');
});
