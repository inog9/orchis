import fs from "node:fs";
import path from "node:path";
import crypto from "node:crypto";

export function atomicJSON(file, value) {
  fs.mkdirSync(path.dirname(file), { recursive: true, mode: 0o700 });
  const temporary = `${file}.${process.pid}.${crypto.randomUUID()}.tmp`;
  try {
    fs.writeFileSync(temporary, JSON.stringify(value, null, 2) + "\n", { mode: 0o600, flag: "wx" });
    fs.renameSync(temporary, file);
  } finally {
    if (fs.existsSync(temporary)) fs.unlinkSync(temporary);
  }
}

export function readJSON(file, fallback) {
  try { return JSON.parse(fs.readFileSync(file, "utf8")); }
  catch (error) {
    if (error.code === "ENOENT" && fallback !== undefined) return structuredClone(fallback);
    throw new Error(`Cannot read checkpoint ${file}: ${error.message}`);
  }
}

// Cross-process lock for the shared launch ledger and traffic scheduler. A live
// owner's lock is never stolen. Recover dead owners, including interrupted writes.
export function transact(file, fallback, operation) {
  const lock = file + ".lock";
  fs.mkdirSync(path.dirname(file), { recursive: true, mode: 0o700 });
  const deadline = Date.now() + 10000;
  for (;;) {
    try {
      fs.mkdirSync(lock, { mode: 0o700 });
      fs.writeFileSync(path.join(lock, "owner"), String(process.pid), { mode: 0o600 });
      break;
    } catch (error) {
      if (error.code !== "EEXIST") throw error;
      try {
        const owner = Number(fs.readFileSync(path.join(lock, "owner"), "utf8"));
        try { process.kill(owner, 0); }
        catch (error) {
          if (error.code === "ESRCH") { fs.rmSync(lock, { recursive: true }); continue; }
        }
      } catch (error) {
        if (error.code !== "ENOENT") throw error;
      }
      if (Date.now() >= deadline) throw new Error(`Checkpoint is locked: ${file}`);
      Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 10);
    }
  }
  try {
    const value = readJSON(file, fallback);
    const result = operation(value);
    atomicJSON(file, value);
    return result;
  } finally { fs.rmSync(lock, { recursive: true }); }
}

export function artifact(workspace, relative) {
  if (typeof relative !== "string" || !relative || path.isAbsolute(relative)) throw new Error("Evidence needs a workspace-relative file path");
  const resolved = path.resolve(workspace, relative);
  const root = fs.realpathSync(workspace);
  const actual = fs.realpathSync(resolved);
  if (!actual.startsWith(root + path.sep)) throw new Error(`Artifact escapes workspace: ${relative}`);
  const stat = fs.statSync(actual);
  if (!stat.isFile() || stat.size === 0) throw new Error(`Artifact must be a nonempty file: ${relative}`);
  if (stat.size > 64 * 1024 * 1024) throw new Error("Reference a bounded evidence extract or hash ledger for artifacts over 64 MiB");
  return { path: path.relative(root, actual), sha256: crypto.createHash("sha256").update(fs.readFileSync(actual)).digest("hex"), bytes: stat.size };
}

export function assertArtifacts(workspace, entries) {
  for (const entry of entries) {
    if (artifact(workspace, entry.path).sha256 !== entry.sha256) throw new Error(`Evidence changed; reverify ${entry.path}`);
  }
}

export function privateText(file, text) {
  fs.mkdirSync(path.dirname(file), { recursive: true, mode: 0o700 });
  fs.writeFileSync(file, text, { mode: 0o600 });
}
