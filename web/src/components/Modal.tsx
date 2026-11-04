import { t as textUI, useLocale, fv as formatUI } from "../i18n/runtime";
import { useEffect, useRef, type ReactNode } from 'react';
const PURPOSES: Record<string, string> = {
    get 'add-asset-title'() {
        return textUI("m_05f84f557eac");
    },
    get 'edit-asset-title'() {
        return textUI("m_495b2e69211a");
    },
    get 'group-title'() {
        return textUI("m_e8cc3631e8b6");
    },
    get 'import-title'() {
        return textUI("m_7ce64ba2d992");
    },
    get 'install-title'() {
        return textUI("m_4ce98aa56c04");
    },
    get 'atomic-title'() {
        return textUI("m_b978c053a094");
    },
    get 'org-form-title'() {
        return textUI("m_5be9c44085a7");
    },
    get 'new-scenario-title'() {
        return textUI("m_9208cec9f252");
    },
    get 'targets-title'() {
        return textUI("m_0f96d6090777");
    },
    get 'launch-title'() {
        return textUI("m_6a5576b39c0c");
    },
    get 'person-title'() {
        return textUI("m_474ab9dc0c27");
    },
    get 'team-title'() {
        return textUI("m_67001ffbbebe");
    },
    get 'members-title'() {
        return textUI("m_9b3f29470e16");
    },
    get 'user-title'() {
        return textUI("m_02c2ed3855a5");
    },
    get 'reset-title'() {
        return textUI("m_da453a572437");
    },
    get 'tok-title'() {
        return textUI("m_538c9112f488");
    },
    get 'ntok-title'() {
        return textUI("m_472918856e3f");
    },
    get 'stok-title'() {
        return textUI("m_91e68ba98806");
    },
    get 'tag-title'() {
        return textUI("m_6497eb33d446");
    },
    get 'palette-title'() {
        return textUI("m_e031bb9b4b88");
    },
};
const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';
// Dialog modal: Esc menutup, Tab berputar di dalamnya, dan fokus dikembalikan ke pemicu saat ditutup.
export function Modal({ labelledBy, onClose, children, width }: {
    labelledBy: string;
    onClose: () => void;
    children: ReactNode;
    width?: number;
}) {
    useLocale();
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
            if (e.key !== 'Tab' || !node)
                return;
            const items = [...node.querySelectorAll<HTMLElement>(FOCUSABLE)].filter((el) => el.offsetParent !== null);
            if (items.length === 0)
                return;
            const first = items[0]!;
            const last = items[items.length - 1]!;
            if (e.shiftKey && document.activeElement === first) {
                e.preventDefault();
                last.focus();
            }
            else if (!e.shiftKey && document.activeElement === last) {
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
    return (<div className="modal" role="presentation" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="dialog" role="dialog" aria-modal="true" aria-labelledby={labelledBy} aria-describedby={PURPOSES[labelledBy] ? `${labelledBy}-purpose` : undefined} ref={ref} style={width ? { width } : undefined}>
        {formatUI(PURPOSES[labelledBy] && <p id={`${labelledBy}-purpose`} className="dialog-purpose">{formatUI(PURPOSES[labelledBy])}</p>)}
        {formatUI(children)}
      </div>
    </div>);
}
