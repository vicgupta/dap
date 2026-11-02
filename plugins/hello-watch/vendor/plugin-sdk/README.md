# @dap/plugin-sdk

Typed helpers for a DAP plugin. The package is not published yet. A scaffold copies this directory and imports `dist/index.js`.

```ts
import { serve, type PluginHandler } from "../vendor/plugin-sdk/dist/index.js";

const plugin: PluginHandler = {
  async initialize() {
    return { ready: true, interface: "dap.plugin.v1", health: { status: "ok", message: "ready" } };
  },
  async watch() {
    return { observations: [], tile: { title: "Plugin", status: "ok", lines: [] } };
  },
  async propose() {
    return { cards: [] };
  },
  async act() {
    return { status: "done", summary: "Recorded." };
  },
  async report() {
    return { metrics: [] };
  },
  async shutdown() {
    return { ok: true };
  },
};

serve(plugin);
```

`Host` (created for you inside `serve` after `initialize`) calls the scoped plugin API: config is on `init.config`, scratch state is `putState` / `state`, secrets are `secretGet` / `secretSet` / `secretDelete`, and mail is `mailThreads` / `mailSend`. `mailSend` still requires an approved card. The plugin never sees an OAuth token.

`shutdown` is optional. DAP sends `{"reason":"quit"}` when the person quits. If the handler omits it, `serve` still replies `{ "ok": true }`.

`readBoot`, `applyTheme`, and `pluginFetch` are for a plugin page. They read `dap-token`, `dap-api`, `?theme=`, and the theme tokens from `/theme.css`. Pair that stylesheet with `/layout.css` so `data-density` tightens spacing.
