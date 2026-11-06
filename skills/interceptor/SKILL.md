---
name: interceptor
description: "Choose the right Interceptor surface. Use interceptor-browser for page DOM, network, browser tabs, rich editors, screenshots, and browser automation. Use interceptor-desktop for native apps on this computer: element trees, windows, dialogs, background input, native screenshots, system services, and Electron or Chromium app web contents. Use interceptor-research for deep multi-source web research. Background-first by default; focus changes require explicit opt-in."
metadata:
  short-description: Choose the right Interceptor surface
---

# Interceptor

Use this as the routing skill before loading a surface-specific skill.

## Surface Decision

| Task | Skill |
|---|---|
| Page DOM, text, network, SPA state, browser monitor, screenshots of browser content | `interceptor-browser` |
| Rich browser editors (Docs/Slides/Canva), canvas/WebGL camera viewers, scene-graph reads, page-world request overrides, blob/export capture, or "cooking" a live page with overlays/HUDs | `interceptor-browser` |
| Native apps on this computer: element trees, windows, dialogs, menus, background input, native screenshots, system services | `interceptor-desktop` |
| Electron / Chromium desktop app web contents, such as Slack, VS Code, Notion, or Descript DOM/network/JS | `interceptor desktop cdp` via `interceptor-desktop` |
| Open or control a named app | `interceptor-desktop` |
| Deep web research: investigate a topic across many sources with breadth + verification | `interceptor-research` |
| Capabilities added by an installed extension (operator-supplied) | run `interceptor extensions list`, then load the extension's own skill (`interceptor-ext-<name>`) |

## The User Is On This Computer Too

Assume the person is working on this computer while you work. Stay in the background at all costs. Take focus (the app in front, the focused window, or the tab showing in a window the user is using) only when the action truly cannot be done any other way. Never take it to make your own work easier, and never to test something.

Before any focus-moving verb, go down this list and stop at the first rung that works:

1. **A background verb.** Synthetic browser input, element refs, and `--app` / `--pid` / `--window` on `interceptor desktop` input.
2. **`interceptor tab keepalive <id>`** for a page that will not render while hidden.
3. **Your own window.** `interceptor window new [url]` moves your tab group into its own background window, and `interceptor tab switch <id>` there changes only that window. Never switch tabs in a window the user is working in.
4. **Only then a focus move.** Say so first, keep it to one fast batch, and put back the frontmost app, the active tab, and the cursor.


## Core Rules

- Browser commands operate inside the cyan `interceptor` tab group. Do not use `--any-tab` unless the user explicitly authorizes acting outside that group.
- Solo browser work in a supported agent shell gets a soft per-session group automatically. For concurrent lanes, set a unique `INTERCEPTOR_SESSION_ID` or pass a unique `--group <label>` to each lane. Explicit groups are hard-scoped by default unless `--any-tab` is authorized. Close named groups with `interceptor group close <label>` when done; the idle timer is based on tab activity, so metadata polls do not keep a group alive.
- `interceptor open <url>`, `interceptor tab new <url>`, and `interceptor window new [url]` create background tabs and windows by default. Only `open --activate`, `tab new --activate`, `window new --activate`, `tab switch <id>`, and `window focus <id>` intentionally move browser focus.
- A page that will not render while hidden is not a reason to switch tabs in the user's window. `interceptor tab keepalive <id>` makes it read as visible and run its animation frames in place. A page that needs a real paint (video playback, a virtualized grid that measures itself) goes in your own window: `interceptor window new <url>`.
- The native surface is background-first by default. Only `interceptor desktop app activate <name>` and `interceptor desktop open <name> --activate` intentionally move focus.
- If multiple browser profiles are connected, run `interceptor contexts` and pass `--context <id>`.
- Prefer compound commands (`open`, `read`, `act`, `inspect`) and structured reads before screenshots.
- The zero-CDP browser rule governs the user's real browser session. For owned Electron apps, `interceptor desktop cdp` is an intentional app-control surface.
- If the extension behaves stale after a package update, run `interceptor reload --context <id>`: an unpacked copy picks up the installed files; a Chrome Web Store copy only asks the store for an update and keeps its version until the store publishes the new one. `interceptor contexts --verbose` shows which copy (store or unpacked) and version each context runs.
- `interceptor daemon stop` and `interceptor skills unadopt` are install/maintenance verbs. The daemon is shared by every agent on the machine — never stop it during normal automation; the next browser command respawns it, but every connected agent's in-flight work is dropped.

## Load A Surface Skill

- Load `interceptor-browser` for browser page content, network, tabs, scene graphs, and browser screenshots.
- Load `interceptor-desktop` for native apps, windows, dialogs, element trees, background input, system services, and Electron app CDP. It lists what this computer cannot do.
- Load `interceptor-research` for deep web research — investigating a topic across many sources with a planner loop, an on-disk source ledger, and adversarial verification (methodology layered on the browser surface). Pull the playbook any time with `interceptor research`.
