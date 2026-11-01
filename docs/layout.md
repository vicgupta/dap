# Card layout

Home and the decision inbox remember a layout each. The control is the icon group in the page header. It is a radiogroup: Tab moves to the current choice, arrow keys change it, and the button name is the view or the density (`Grid`, `List`, `Board`, `Split`, `Comfortable`, `Compact`).

| | Home default | Inbox default |
| --- | --- | --- |
| View | Grid | List |
| Density | Comfortable | Comfortable |

Compact is the tighter density on both screens.

Board on Home groups by health: Needs you, Attention, Down, Healthy. Board in the inbox groups by plugin. Split is inbox only: a scannable list with Approve and Reject, and the open card beside it. Below 800px the split shows one pane. Approve, Edit, Reject, and Snooze stay written out on every card, including when the card is collapsed.

Drag a card's handle to reorder that view. With the handle focused, Up and Down do the same. Pin keeps the card in front. The S M L control on a Home card sets its grid span. A plugin can declare a preferred size and a floor in `plugin.yaml`; see [plugins.md](plugins.md). The person can pick anything at or above that floor.

## Where the choice is stored

- The browser keeps `localStorage["dap.layout"]`. `layout-boot.js` reads it and sets `data-density`, `data-home-view`, and `data-inbox-view` before the stylesheet loads.
- The server keeps the same document in `config.json` as `"layout"`. `GET` and `PUT /api/settings` read and write `{ "layout": { "home": {}, "inbox": {} } }`. A browser with no saved choice takes the server value. A browser that already chose writes that choice back.
- A `PUT` that omits `layout` does not change it. Other keys in `config.json` are left in place.

Each view object is `view`, `density`, and optional `order`, `pinned`, `collapsed`, and `sizes` (Home tile spans keyed by plugin name).

## Plugin pages

A plugin UI is a different loopback origin, so it cannot read the core page's `localStorage`. DAP passes the **Home** density, because that is the dashboard the plugin's tile sits on. The same three channels theme uses:

1. The link and the `/plugins/<name>/` redirect include `?density=comfortable|compact`.
2. The sandbox sets `<html data-density="...">` and `<meta name="dap-density" content="...">` from that query, or from the saved Home density when the query is absent.
3. The sandbox serves `/layout.css` and `/layout-boot.js`.

`layout-boot.js` applies the query first, then `localStorage` on that origin, then `<meta name="dap-layout">` when the page is the core UI, then the attribute already on `<html>`. Include the script before any stylesheet:

```html
<html lang="en">
<head>
  <!--DAP_BOOT-->
  <script src="/layout-boot.js"></script>
  <link rel="stylesheet" href="/layout.css" />
  <link rel="stylesheet" href="/app.css" />
</head>
```

`/layout.css` defines `--card-pad`, `--card-gap`, `--card-title`, `--card-radius`, `--stack-gap`, and `--layout-gap`. Use those for card spacing. Colors stay on the theme tokens (`--bg`, `--surface`, `--ink`, `--muted`, `--line`, `--accent`, and the rest). Do not hide an approve or reject control in compact density.

A window that is already open does not see a later change made in the core. The next open or reload does.

The same window also accepts a `message` event `{ "type": "dap.layout", "density": "compact" }` from a `http://127.0.0.1` or `http://localhost` origin. The core opens plugin pages with `rel="noreferrer"`, so it uses the query string rather than `postMessage`.

`plugins/workflow/ui` and `plugins/inbox-followups/ui` follow this contract.
