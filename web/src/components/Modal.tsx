import { useEffect, useRef, type ReactNode } from 'react';

const PURPOSES: Record<string,string> = {
 'add-asset-title':'Register a target. Adding an asset does not run a scan; set its addresses and obtain testing permission first.',
 'edit-asset-title':'Update this asset’s target addresses, labels and testing permission. Past report snapshots stay unchanged.',
 'group-title':'Choose rules that automatically include matching assets. A group is a target selector, not a company organisation.',
 'import-title':'Preview asset rows before importing. Importing does not run checks or grant permission to test.',
 'install-title':'Install one endpoint instance. Each instance creates its own asset; scan authorization remains an administrator action.',
 'atomic-title':'Save one check for one authorized target. You can run it and inspect its findings afterward.',
 'org-form-title':'Keep a company’s assets, assessments and reports together. Creating an organisation starts no scans.',
 'new-scenario-title':'Create a reusable testing plan. Add its chain of checks and targets before launching a simulation.',
 'targets-title':'Choose assets for every action in this scenario. Groups are resolved at launch; testing permission is checked again.',
 'launch-title':'Start a recorded run using a frozen copy of this plan and its targets. Review the scope before launching.',
 'person-title':'Register an exercise participant. This does not create a platform sign-in account.',
 'team-title':'Group exercise participants. Teams do not grant access to the platform or permission to scan assets.',
 'members-title':'Choose existing participants to include in this exercise team.',
 'user-title':'Create or update a sign-in account and its access role. Exercise participants are managed under People.',
 'reset-title':'Give this one-time password setup link to the named account owner. Closing the dialog hides it.',
 'tok-title':'Generate a credential for one scan worker. Give it to that worker; do not share it in reports.',
 'ntok-title':'Create a personal credential for scripts. It inherits your current account permissions.',
 'stok-title':'Copy this credential now and store it safely. It will not be displayed again.',
 'tag-title':'Create a shared label for assets and automatic group rules.',
 'palette-title':'Find a destination by its name or purpose. Arrow keys select, Enter opens, Escape closes.',
};

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

// Dialog modal: Esc menutup, Tab berputar di dalamnya, dan fokus dikembalikan ke pemicu saat ditutup.
export function Modal({ labelledBy, onClose, children, width }: { labelledBy: string; onClose: () => void; children: ReactNode; width?: number }) {
  const ref = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  closeRef.current = onClose;

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const node = ref.current;
    node?.querySelector<HTMLElement>('input, select, textarea, button')?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        closeRef.current();
        return;
      }
      if (e.key !== 'Tab' || !node) return;
      const items = [...node.querySelectorAll<HTMLElement>(FOCUSABLE)].filter((el) => el.offsetParent !== null);
      if (items.length === 0) return;
      const first = items[0]!;
      const last = items[items.length - 1]!;
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('keydown', onKey);
      previous?.focus?.();
    };
  }, []);

  return (
    <div className="modal" role="presentation" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="dialog" role="dialog" aria-modal="true" aria-labelledby={labelledBy} aria-describedby={PURPOSES[labelledBy]?`${labelledBy}-purpose`:undefined} ref={ref} style={width ? { width } : undefined}>
        {PURPOSES[labelledBy]&&<p id={`${labelledBy}-purpose`} className="dialog-purpose">{PURPOSES[labelledBy]}</p>}
        {children}
      </div>
    </div>
  );
}
