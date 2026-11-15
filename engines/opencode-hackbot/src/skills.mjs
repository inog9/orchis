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
    .replace(/, pentest-fuzzing, pentest-endpoint-fuzz-full, pentest-gadgets/g,'')
    .replace('             -> pentest-fuzzing -> pentest-endpoint-fuzz-full\n             -> pentest-gadgets -> pentest-reflection','             -> pentest-reflection')
    .replace(/the most findings\. `injection`, `fuzzing`, `endpoint-fuzz-full`, and `gadgets`\nneed the inventory and web logic from recon; `endpoint-fuzz-full` is the deep\nseeded discovery loop \(5 runs\)\. Gadgets are chained into the findings found\nearlier\. `reflection`/, 'the most findings. Injection testing uses the inventory and web logic from recon. `reflection`')
    .replace(/- \*\*Dependency confusion from JS:\*\*[\s\S]*?(?=^- \*\*WordPress:)/m,'')
    .replace('## Rules\n','## Rules\n- Exclude endpoint fuzzing, gadget hunting and dependency-confusion checks; record them as not performed.\n'));
  edit('pentest-recon/SKILL.md',s=>s.replace(section('Dependency confusion from collected assets'),'').replace('`pentest-authz`, `pentest-injection`, and `pentest-fuzzing`','`pentest-authz` and `pentest-injection`'));
  edit('pentest-js-analysis/SKILL.md',s=>s.replace(/Use \[pentest-dependency-confusion\][\s\S]*?Package strings alone do not establish exploitable dependency confusion\.\n\n/,''));
  edit('pentest-reflection/SKILL.md',s=>s.replace(' (especially after gadgets)','').replace('auth, authz, injection, fuzzing, gadgets','auth, authz, injection').replace('a chain with a proven gadget,','a chain with proven evidence,').replace('prior evidence tab, gadget, or report file','prior evidence tab or report file'));
  edit('pentest-injection/SKILL.md',s=>s.replace(/For image-only uploads, consult the\n  \[non-image content gadget\]\([^\n]+\):/,'For image-only uploads:'));
  edit('pentest/references/checklist.md',s=>s.replace(section('1c\\. Dependency confusion from collected JS'),'').replace(section('7c\\. Endpoint fuzzing'),'').replace(section('8b\\. Gadgets'),'').replace('written after gadgets','written after testing'));
  edit('pentest-js-analysis/references/collection.md',s=>s.replace('clues go to `pentest-dependency-confusion` for its separate verification loop.','record package clues as unverified inventory.'));
  edit('pentest-js-analysis/references/workflow.md',s=>s.replace('Use `pentest-gadgets` for primitive and chain classification, `pentest-authz` for','Use `pentest-authz` for'));
  edit('pentest/references/terms.md',s=>s.replace(' For gadget-specific definitions,\nexamples, evidence requirements, and false positives, use the\n[gadget reference](../../pentest-gadgets/references/gadgets.md).',''));
  edit('wordpress-pentest/reference/enumeration.md',s=>s.replace('Follow [bounded fuzzing](../../pentest-fuzzing/SKILL.md) and the\n[all-role workflow]','Follow the [all-role workflow]'));
  for (const name of ['pentest-fuzzing','pentest-endpoint-fuzz-full','pentest-gadgets','pentest-dependency-confusion']) fs.rmSync(path.join(skills,name),{recursive:true,force:true});
}
