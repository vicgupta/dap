# Theme

DAP has three appearance modes: **System** (the default), **Light**, and **Dark**.

System follows the operating system through `prefers-color-scheme` and updates when that setting changes. Light and Dark stay put. The control is in the top bar on every screen: a sun for Light, a moon for Dark, and a monitor for System. The button's tooltip and accessible name are `Theme: Light`, `Theme: Dark`, or `Theme: System`. Clicking it opens a menu with all three choices. Arrow keys move, Enter selects, Escape closes.

## Where the choice is stored

- The browser keeps `localStorage["dap.theme"]` (`system`, `light`, or `dark`). `theme-boot.js` reads it and sets `<html data-theme>` before the stylesheet loads, so the first paint matches.
- The server keeps the same value in `config.json` as `"theme"`. `GET` and `PUT /api/settings` read and write `theme` next to `layout`. A body may send either key. A browser with no saved choice takes the server value. A browser that already chose writes that choice back, so the next browser on this machine starts there.

## Plugin pages

A plugin UI is a different loopback origin, so it cannot read the core page's `localStorage`. DAP passes the mode in three ways:

1. The link and the `/plugins/<name>/` redirect include `?theme=system|light|dark` and `?density=comfortable|compact`.
2. The sandbox sets `<html data-theme="...">`, `<html data-density="...">`, and the matching meta tags from that query, or from `config.json` when the query is absent.
3. The sandbox serves `/theme.css`, `/theme-boot.js`, `/layout.css`, and `/layout-boot.js`.

`theme-boot.js` applies the query first, then `localStorage` on that origin, then the attribute already on `<html>`. Include the script before any stylesheet:

```html
<html lang="en" data-theme="system">
<head>
  <script src="/theme-boot.js"></script>
  <!--DAP_BOOT-->
  <script src="/layout-boot.js"></script>
  <link rel="stylesheet" href="/theme.css" />
  <link rel="stylesheet" href="/layout.css" />
  <link rel="stylesheet" href="/app.css" />
</head>
```

Style with the custom properties from `/theme.css` (`--bg`, `--surface`, `--ink`, `--muted`, `--accent`, `--good`, `--warn`, `--danger`, `--field`, `--control-border`, and the rest in that file). Do not hardcode light-only colors. `color-scheme` is set with the tokens, so form controls and scrollbars follow the resolved palette. When the mode is `system`, the media query updates those tokens live.

A window that is already open does not see a later change made in the core. The next open or reload does. System mode still tracks the OS inside the open window.

The same window also accepts a `message` event `{ "type": "dap.theme", "theme": "dark" }` from a `http://127.0.0.1` or `http://localhost` origin. The core opens plugin pages with `rel="noreferrer"`, so it uses the query string rather than `postMessage`.

`plugins/workflow/ui` and `plugins/inbox-followups/ui` follow this contract. `hello-watch` has no web UI. Density is described in [layout.md](layout.md).
