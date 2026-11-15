// Import a bounded, immutable preliminary-scanner handoff into this engagement.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';

export function importScannerInputs(source, workspace) {
 if (!source) return undefined;
 const folder = fs.lstatSync(source);
 if (!folder.isDirectory() || folder.isSymbolicLink()) throw new Error('invalid_scanner_inputs_directory');
 const readRegular = (file, max) => {
  const stat = fs.lstatSync(file);
  if (!stat.isFile() || stat.isSymbolicLink() || stat.size > max || !stat.size) throw new Error('invalid_scanner_input_file');
  return fs.readFileSync(file);
 };
 const rawManifest = readRegular(path.join(source,'manifest.json'), 8192);
 const manifest = JSON.parse(rawManifest);
 if (manifest.schema !== 1 || !Array.isArray(manifest.sources) || manifest.sources.length !== 2) throw new Error('invalid_scanner_manifest');
 const seen = new Set();
 const files = [];
 for (const entry of manifest.sources) {
  if (!['nmap.json','nuclei.json'].includes(entry.name) || seen.has(entry.name)) throw new Error('invalid_scanner_input_name');
  seen.add(entry.name);
  const data = readRegular(path.join(source,entry.name),32*1024*1024);
  if (entry.bytes !== data.length || crypto.createHash('sha256').update(data).digest('hex') !== entry.sha256) throw new Error('scanner_input_changed');
  const value = JSON.parse(data);
  if (value.tool+'.json' !== entry.name || !['finished','failed','incomplete'].includes(value.status) || !Array.isArray(value.findings)) throw new Error('invalid_scanner_observations');
  files.push([entry.name,data]);
 }
 files.push(['manifest.json',rawManifest]);
 // Check each existing parent before creating the next; never follow a workspace symlink.
 let target = fs.realpathSync(workspace);
 for (const name of ['inputs','scanners']) {
  target=path.join(target,name);
  if (fs.existsSync(target)) {
   const stat=fs.lstatSync(target);
   if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error('scanner_destination_is_not_directory');
  } else fs.mkdirSync(target,{mode:0o700});
 }
 for (const [name,data] of files) {
  const file=path.join(target,name);
  try {fs.writeFileSync(file,data,{flag:'wx',mode:0o600});}
  catch(error) {
   if(error.code!=='EEXIST')throw error;
   const previous=readRegular(file,32*1024*1024);
   if(!previous.equals(data))throw new Error('existing_scanner_inputs_changed');
  }
 }
 return target;
}
