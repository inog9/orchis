import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';

function regular(file, limit) {
  const stat = fs.lstatSync(file);
  if (!stat.isFile() || stat.size < 1 || stat.size > limit) throw new Error('invalid_scanner_input');
  return fs.readFileSync(file);
}

export function copyScannerInputs(source, workspace) {
  if (!source) return null;
  const directory = fs.lstatSync(source);
  if (!directory.isDirectory()) throw new Error('invalid_scanner_directory');
  const manifest = JSON.parse(regular(path.join(source, 'manifest.json'), 8192));
  if (manifest.schema !== 1 || !Array.isArray(manifest.sources) || manifest.sources.length !== 2) throw new Error('invalid_scanner_manifest');
  const seen = new Set();
  const files = [];
  for (const entry of manifest.sources) {
    if (!['nmap.json', 'nuclei.json'].includes(entry.name) || seen.has(entry.name)) throw new Error('invalid_scanner_name');
    seen.add(entry.name);
    const data = regular(path.join(source, entry.name), 32 * 1024 * 1024);
    if (data.length !== entry.bytes || crypto.createHash('sha256').update(data).digest('hex') !== entry.sha256) throw new Error('scanner_input_changed');
    const parsed = JSON.parse(data);
    if (parsed.tool + '.json' !== entry.name || !['finished', 'failed', 'incomplete'].includes(parsed.status) || !Array.isArray(parsed.findings)) throw new Error('invalid_scanner_observations');
    files.push([entry.name, data]);
  }
  if (seen.size !== 2) throw new Error('incomplete_scanner_manifest');
  files.push(['manifest.json', regular(path.join(source, 'manifest.json'), 8192)]);
  let target = fs.realpathSync(workspace);
  for (const name of ['inputs', 'scanners']) {
    target = path.join(target, name);
    if (fs.existsSync(target)) {
      if (!fs.lstatSync(target).isDirectory()) throw new Error('invalid_scanner_destination');
    } else fs.mkdirSync(target, { mode: 0o700 });
  }
  for (const [name, data] of files) {
    const file = path.join(target, name);
    try { fs.writeFileSync(file, data, { flag: 'wx', mode: 0o600 }); }
    catch (error) {
      if (error.code !== 'EEXIST' || !regular(file, 32 * 1024 * 1024).equals(data)) throw new Error('scanner_destination_changed');
    }
  }
  return target;
}
