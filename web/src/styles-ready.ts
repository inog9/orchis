import { t as textUI } from "./i18n/runtime";
// Stylesheet gagal tidak boleh menghasilkan workspace tanpa tata letak/kontras.
// Hanya tautan stylesheet dari dokumen build yang dicoba ulang; tidak ada URL pengguna.
function styled(): boolean {
    return getComputedStyle(document.documentElement).getPropertyValue('--orchis-ui-styled').trim() === '1';
}
function reloadStylesheet(original: HTMLLinkElement, attempt: number): Promise<void> {
    return new Promise((resolve) => {
        const replacement = original.cloneNode() as HTMLLinkElement;
        const url = new URL(original.href);
        url.searchParams.set('orchis-style-retry', `${Date.now()}-${attempt}`);
        replacement.href = url.toString();
        replacement.disabled = false;
        let timer: ReturnType<typeof setTimeout>;
        const done = () => {
            clearTimeout(timer);
            replacement.onload = replacement.onerror = null;
            resolve();
        };
        replacement.onload = replacement.onerror = done;
        timer = setTimeout(done, 5000);
        original.replaceWith(replacement);
    });
}
function loaded(link: HTMLLinkElement): boolean {
    // A failed response can still expose an empty CSSStyleSheet in Chromium.
    // Build stylesheets contain rules; a sheet object alone does not prove they loaded.
    try { return !link.disabled && !!link.sheet && link.sheet.cssRules.length > 0; }
    catch { return false; }
}
export async function applicationStylesReady(): Promise<boolean> {
    const sheetsNow = () => Array.from(document.querySelectorAll<HTMLLinkElement>('link[rel="stylesheet"]'));
    if (styled() && sheetsNow().every(loaded))
        return true;
    for (let attempt = 0; attempt < 2; attempt++) {
        const sheets = sheetsNow().filter(sheet => !loaded(sheet));
        if (!sheets.length)
            break;
        await Promise.all(sheets.map(sheet => reloadStylesheet(sheet, attempt)));
        if (styled() && sheetsNow().every(loaded))
            return true;
    }
    return false;
}
// Tampilan pemulihan mandiri harus tetap terbaca ketika CSS utama tidak tersedia.
export function showStylesheetFailure(root: HTMLElement): void {
    const alert = document.createElement('section');
    alert.setAttribute('role', 'alert');
    Object.assign(alert.style, { background: '#0a0b0d', color: '#f5f5f5', font: '16px/1.5 system-ui, sans-serif', padding: '32px', minHeight: '100vh' });
    const heading = document.createElement('h1');
    heading.textContent = textUI("m_dda5524ef396");
    const message = document.createElement('p');
    message.textContent = textUI("m_921a6b47a87a");
    const reload = document.createElement('button');
    reload.textContent = textUI("m_ed72f3dfd1b5");
    Object.assign(reload.style, { background: '#e2bd79', color: '#19150e', padding: '10px 16px', border: '2px solid #f5f5f5', font: 'inherit', cursor: 'pointer' });
    reload.onclick = () => window.location.reload();
    alert.append(heading, message, reload);
    const main = document.createElement('main');
    main.append(alert);
    root.replaceChildren(main);
}
