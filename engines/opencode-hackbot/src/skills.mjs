import fs from 'node:fs';
import path from 'node:path';

// Limit the installed workflow, not the target scope or existing findings.
export function restrictSkills(skills) {
  const edit = (file, transform) => {
    const p=path.join(skills,file), old=fs.readFileSync(p,'utf8'), next=transform(old);
    if (old === next) throw Error(`Expected skill section missing: ${file}`);
    fs.writeFileSync(p,next);
  };
  const section = heading => new RegExp(`^## ${heading}\\n[\\s\\S]*?(?=^## |$(?![\\s\\S]))`, 'm');
  edit('pentest/SKILL.md', s => s
    .replace(/, pentest-fuzzing, pentest-endpoint-fuzz-full/g,'')
    .replace('             -> pentest-fuzzing -> pentest-endpoint-fuzz-full\n             -> pentest-reflection','             -> pentest-reflection')
    .replace(/the most findings\. `injection`, `fuzzing`, and `endpoint-fuzz-full`\nneed the inventory and web logic from recon; `endpoint-fuzz-full` is the deep\nseeded discovery loop \(5 runs\)\. `reflection`/, 'the most findings. Injection testing uses the inventory and web logic from recon. `reflection`')
    .replace(/- \*\*Dependency confusion from JS:\*\*[\s\S]*?(?=^- If a WAF)/m,'')
    .replace('## Rules\n','## Rules\n- Exclude endpoint fuzzing, gadget hunting and dependency-confusion checks; record them as not performed.\n'));
  edit('pentest-recon/SKILL.md',s=>s.replace(section('Dependency confusion from collected assets'),'').replace('`pentest-authz`, `pentest-injection`, and `pentest-fuzzing`','`pentest-authz` and `pentest-injection`'));
  edit('pentest-js-analysis/SKILL.md',s=>s.replace(/Use \[pentest-dependency-confusion\][\s\S]*?Package strings alone do not establish exploitable dependency confusion\.\n\n/,''));
  edit('pentest-reflection/SKILL.md',s=>s.replace(' (especially after gadgets)','').replace('auth, authz, injection, fuzzing, gadgets','auth, authz, injection').replace('a chain with a proven gadget,','a chain with proven evidence,').replace('prior evidence tab, gadget, or report file','prior evidence tab or report file'));
  edit('pentest/references/checklist.md',s=>s.replace(section('1c\\. Dependency confusion from collected JS'),'').replace(section('7c\\. Endpoint fuzzing'),''));
  edit('pentest-js-analysis/references/collection.md',s=>s.replace('clues go to `pentest-dependency-confusion` for its separate verification loop.','record package clues as unverified inventory.'));
  for (const name of ['pentest-fuzzing','pentest-endpoint-fuzz-full','pentest-gadgets','pentest-dependency-confusion']) fs.rmSync(path.join(skills,name),{recursive:true,force:true});
}
