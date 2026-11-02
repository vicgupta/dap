const DAP = readBoot();

function readBoot() {
  if (window.__DAP__ && window.__DAP__.token) return window.__DAP__;
  const meta = (name) => {
    const el = document.querySelector(`meta[name="${name}"]`);
    return el ? el.content : "";
  };
  return { token: meta("dap-token"), version: meta("dap-version") };
}

const state = {
  route: routeFromHash(),
  selected: 0,
  selectedId: "",
  error: "",
  notice: "",
  mode: null,
  installPath: "",
  preview: null,
  connecting: "",
  data: {},
  create: { description: "", name: "", type: "", lang: "ts", ui: false, agent: "", phase: "form", log: "", report: null, path: "", edit: "" },
  quitConfirm: null,
  quitPending: false,
  quitting: false,
  theme: document.documentElement.getAttribute("data-theme") || "system",
  themeOpen: false,
  layout: initialLayout(),
  chat: {
    provider: "",
    providers: [],
    sessions: [],
    current: "",
    session: null,
    cards: {},
    draft: "",
    streaming: false,
    loaded: false,
    focusComposer: false,
  },
  dragging: false,
  splitOpen: false,
  forceScroll: false,
  scrollReset: false,
  focusSelector: "",
};

const THEME_MODES = [
  { id: "light", label: "Light" },
  { id: "dark", label: "Dark" },
  { id: "system", label: "System" },
];

const ICONS = {
  home: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M2.5 7.2 8 2.8l5.5 4.4V13a1 1 0 0 1-1 1h-3.2V9.6H6.7V14H3.5a1 1 0 0 1-1-1V7.2Z"/></svg>',
  inbox: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M2.5 8.5 4 3.5h8l1.5 5M2.5 8.5h3l.8 1.6h3.4l.8-1.6h3V12a1 1 0 0 1-1 1h-9a1 1 0 0 1-1-1V8.5Z"/></svg>',
  activity: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M2 8h2.2l1.4-3.2L8 12l1.6-4H14"/></svg>',
  guard: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M8 2.2 13 4.2v3.6c0 3-2.1 4.8-5 6-2.9-1.2-5-3-5-6V4.2L8 2.2Z"/></svg>',
  plugins: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M6 2.5h4V5H6zM3 6.5h4V9H3zM9 6.5h4V9H9zM6 10h4v2.5H6z"/><path d="M8 5v1.5M5 9v1.5h6V9"/></svg>',
  create: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M8 2.5v11M2.5 8h11"/></svg>',
  connections: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="4" cy="8" r="1.6"/><circle cx="12" cy="5" r="1.6"/><circle cx="12" cy="11" r="1.6"/><path d="M5.5 7.4 10.4 5.6M5.5 8.6l4.9 2"/></svg>',
  chat: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M3 3.5h10a1 1 0 0 1 1 1V10a1 1 0 0 1-1 1H7.2L4.2 13.2V11H3a1 1 0 0 1-1-1V4.5a1 1 0 0 1 1-1Z"/></svg>',
  power: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="M8 2.2v5.2"/><path d="M5.05 3.7a5 5 0 1 0 5.9 0"/></svg>',
  layoutList: '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M3 4.5h10M3 8h10M3 11.5h10"/></svg>',
  layoutGrid: '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><rect x="2.5" y="2.5" width="4.5" height="4.5" rx="1"/><rect x="9" y="2.5" width="4.5" height="4.5" rx="1"/><rect x="2.5" y="9" width="4.5" height="4.5" rx="1"/><rect x="9" y="9" width="4.5" height="4.5" rx="1"/></svg>',
  layoutBoard: '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><rect x="2" y="2.5" width="3.5" height="11" rx="1"/><rect x="6.25" y="2.5" width="3.5" height="8" rx="1"/><rect x="10.5" y="2.5" width="3.5" height="6" rx="1"/></svg>',
  layoutSplit: '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><rect x="2" y="2.5" width="12" height="11" rx="1.5"/><path d="M7 2.5v11"/></svg>',
  densityComfy: '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M3 3.5h10M3 8h10M3 12.5h10"/></svg>',
  densityCompact: '<svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M3 3h10M3 6.2h10M3 9.4h10M3 12.6h10"/></svg>',
  drag: '<svg aria-hidden="true" width="14" height="14" viewBox="0 0 16 16" fill="currentColor"><circle cx="6" cy="3.5" r="1.15"/><circle cx="10" cy="3.5" r="1.15"/><circle cx="6" cy="8" r="1.15"/><circle cx="10" cy="8" r="1.15"/><circle cx="6" cy="12.5" r="1.15"/><circle cx="10" cy="12.5" r="1.15"/></svg>',
  pin: '<svg aria-hidden="true" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M6.2 2.4h3.6l-.5 3.6 2.2 2.2H4.5l2.2-2.2-.5-3.6Z"/><path d="M8 8.4v5"/></svg>',
  chevron: '<svg aria-hidden="true" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M4 6.2 8 10.2 12 6.2"/></svg>',
  sun: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="8" cy="8" r="2.3" fill="currentColor" stroke="none"/><path d="M8 1.5v1.8M8 12.7v1.8M1.5 8h1.8M12.7 8h1.8M3.3 3.3l1.3 1.3M11.4 11.4l1.3 1.3M12.7 3.3l-1.3 1.3M4.6 11.4l-1.3 1.3"/></svg>',
  moon: '<svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true"><path fill="currentColor" d="M9.4 1.7a5.5 5.5 0 1 0 4.6 8.9A4.6 4.6 0 0 1 9.4 1.7Z"/></svg>',
  monitor: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="2" y="2.4" width="12" height="8.2" rx="1.3"/><path d="M6 13.6h4M8 10.6v3"/></svg>',
  check: '<svg class="check" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M3.2 8.2 6.4 11.4 12.8 4.6"/></svg>',
};

function routeFromHash() {
  const h = (location.hash || "#/").replace(/^#\/?/, "");
  const path = h.split("?")[0];
  if (["chat", "inbox", "activity", "guardrails", "plugins", "connections", "create"].includes(path)) return path;
  return "home";
}

function hashQuery() {
  const q = (location.hash || "").split("?")[1] || "";
  return new URLSearchParams(q);
}

function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}

async function api(path, opts = {}) {
  const headers = {
    Authorization: "Bearer " + DAP.token,
    "Content-Type": "application/json",
    ...(opts.headers || {}),
  };
  const res = await fetch(path, {
    method: opts.method || "GET",
    headers,
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok || data.ok === false) {
    throw new Error(data.error || "Request failed");
  }
  return data;
}

function fmtTime(iso) {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return new Intl.DateTimeFormat(undefined, {
    month: "short", day: "numeric", hour: "numeric", minute: "2-digit",
  }).format(d);
}

function fmtClock(iso) {
  if (!iso) return "—";
  const d = new Date(iso);
  return new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit" }).format(d);
}

function modeLabel(mode) {
  if (mode === "ask_first") return "Ask first";
  if (mode === "allowed") return "Allowed";
  if (mode === "never") return "Never";
  return mode || "";
}

function initialLayout() {
  const Layout = window.DAPLayout;
  if (!Layout) {
    return { home: { view: "grid", density: "comfortable", order: [], pinned: [], collapsed: [] }, inbox: { view: "list", density: "comfortable", order: [], pinned: [], collapsed: [] } };
  }
  if (window.__DAP_LAYOUT__) return Layout.normalize(window.__DAP_LAYOUT__);
  try {
    const raw = localStorage.getItem("dap.layout");
    if (raw) return Layout.normalize(JSON.parse(raw));
  } catch (e) { /* private mode */ }
  const root = document.documentElement;
  return Layout.normalize({
    home: { view: root.getAttribute("data-home-view"), density: root.getAttribute("data-density") },
    inbox: { view: root.getAttribute("data-inbox-view"), density: "comfortable" },
  });
}

function applyLayoutAttrs() {
  const route = state.route === "inbox" ? "inbox" : "home";
  const prefs = state.layout[route] || state.layout.home;
  document.documentElement.setAttribute("data-density", (prefs && prefs.density) || "comfortable");
  document.documentElement.setAttribute("data-home-view", state.layout.home.view || "grid");
  document.documentElement.setAttribute("data-inbox-view", state.layout.inbox.view || "list");
  window.__DAP_LAYOUT__ = state.layout;
}

function commitLayout() {
  if (window.DAPLayout) state.layout = window.DAPLayout.normalize(state.layout);
  try { localStorage.setItem("dap.layout", JSON.stringify(state.layout)); } catch (e) { /* private mode */ }
  applyLayoutAttrs();
  render();
  api("/api/settings", { method: "PUT", body: { layout: state.layout } }).catch((err) => {
    state.error = err.message;
    render();
  });
}

function themeName(id) {
  const found = THEME_MODES.find((t) => t.id === id);
  return found ? found.label : "System";
}

function themeIcon(id) {
  if (id === "light") return ICONS.sun;
  if (id === "dark") return ICONS.moon;
  return ICONS.monitor;
}

function applyTheme(mode) {
  if (!["system", "light", "dark"].includes(mode)) return;
  state.theme = mode;
  document.documentElement.setAttribute("data-theme", mode);
  try { localStorage.setItem("dap.theme", mode); } catch (e) { /* private mode */ }
}

function themeControl() {
  const cur = state.theme || "system";
  const label = themeName(cur);
  const open = state.themeOpen;
  const items = THEME_MODES.map((t) => {
    const on = t.id === cur;
    return `<button type="button" role="menuitemradio" class="theme-option" aria-checked="${on ? "true" : "false"}" data-act="theme-set" data-theme="${t.id}">
      ${themeIcon(t.id)}<span>${t.label}</span>${on ? ICONS.check : ""}
    </button>`;
  }).join("");
  return `<div class="theme">
    <button type="button" class="theme-btn" data-act="theme-toggle" title="Theme: ${label}" aria-label="Theme: ${label}" aria-haspopup="menu" aria-expanded="${open ? "true" : "false"}" aria-controls="theme-menu">
      ${themeIcon(cur)}
    </button>
    ${open ? `<div id="theme-menu" class="theme-menu" role="menu" aria-label="Theme">${items}</div>` : ""}
  </div>`;
}

function withLayout(url) {
  try {
    const u = new URL(url, location.href);
    u.searchParams.set("density", (state.layout.home && state.layout.home.density) || "comfortable");
    u.searchParams.set("theme", state.theme || "system");
    return u.toString();
  } catch (e) {
    return url;
  }
}

function viewSwitcher(kind) {
  const current = state.layout[kind].view;
  const options = kind === "inbox"
    ? [["list", "List", ICONS.layoutList], ["grid", "Grid", ICONS.layoutGrid], ["board", "Board", ICONS.layoutBoard], ["split", "Split", ICONS.layoutSplit]]
    : [["grid", "Grid", ICONS.layoutGrid], ["list", "List", ICONS.layoutList], ["board", "Board", ICONS.layoutBoard]];
  return switcher(kind === "inbox" ? "Inbox layout" : "Home layout", kind, "layout-set", options, current);
}

function densitySwitcher(kind) {
  const current = state.layout[kind].density;
  const options = [["comfortable", "Comfortable", ICONS.densityComfy], ["compact", "Compact", ICONS.densityCompact]];
  return switcher(kind === "inbox" ? "Inbox density" : "Home density", kind, "density-set", options, current);
}

function switcher(label, kind, act, options, current) {
  const buttons = options.map(([id, name, icon]) => {
    const on = id === current;
    const focus = kind + "-" + act + "-" + id;
    return `<button type="button" role="radio" class="layout-btn" data-act="${act}" data-kind="${kind}" data-value="${id}" data-focus="${focus}" aria-checked="${on ? "true" : "false"}" aria-label="${esc(name)}" title="${esc(name)}" tabindex="${on ? "0" : "-1"}">${icon}</button>`;
  }).join("");
  return `<div class="layout-switch" role="radiogroup" aria-label="${esc(label)}">${buttons}</div>`;
}

function layoutBar(kind) {
  return `<div class="layout-bar">${viewSwitcher(kind)}${densitySwitcher(kind)}</div>`;
}

function hasId(list, id) {
  return (list || []).indexOf(id) >= 0;
}

function cardTools(kind, id, extra) {
  const prefs = state.layout[kind];
  const pinned = hasId(prefs.pinned, id);
  const collapsed = hasId(prefs.collapsed, id);
  return `<div class="card-tools">
    <button type="button" class="icon-btn drag-handle" draggable="true" data-kind="${esc(kind)}" data-id="${esc(id)}" data-focus="drag-${esc(kind)}-${esc(id)}" aria-label="Reorder. Arrow keys move this card." title="Drag to reorder. Arrow keys move it.">${ICONS.drag}</button>
    <button type="button" class="icon-btn" data-act="pin" data-kind="${esc(kind)}" data-id="${esc(id)}" aria-pressed="${pinned ? "true" : "false"}" aria-label="${pinned ? "Unpin" : "Pin"}" title="${pinned ? "Unpin" : "Pin"}">${ICONS.pin}</button>
    <button type="button" class="icon-btn" data-act="collapse" data-kind="${esc(kind)}" data-id="${esc(id)}" aria-pressed="${collapsed ? "true" : "false"}" aria-label="${collapsed ? "Expand card" : "Collapse card"}" title="${collapsed ? "Expand" : "Collapse"}">${ICONS.chevron}</button>
    ${extra || ""}
  </div>`;
}

function sizeSwitch(plugin, current, min) {
  const rank = { small: 1, medium: 2, large: 3 };
  const floor = rank[min] || 1;
  const buttons = ["small", "medium", "large"].map((size) => {
    const disabled = rank[size] < floor;
    const on = size === current;
    const label = size.charAt(0).toUpperCase() + size.slice(1);
    return `<button type="button" role="radio" class="size-btn" data-act="size" data-id="${esc(plugin)}" data-value="${size}" aria-checked="${on ? "true" : "false"}" aria-label="${label}" title="${label}" ${disabled ? "disabled" : ""}>${label.charAt(0)}</button>`;
  }).join("");
  return `<div class="size-switch" role="radiogroup" aria-label="Card size">${buttons}</div>`;
}

function arrangePlugins(plugins) {
  const list = plugins || [];
  if (!window.DAPLayout) return list;
  return window.DAPLayout.arrange(list, (p) => p.name, state.layout.home);
}

function homeBucket(p) {
  if ((p.pending || 0) > 0) return "needs";
  const status = (p.health && p.health.status) || "";
  if (status === "error") return "down";
  if (status === "degraded" || (p.missed && p.missed.text)) return "attention";
  return "healthy";
}

function pluginCard(p) {
  const health = p.health || {};
  const dot = health.status || "degraded";
  const missed = p.missed && p.missed.text ? `<div class="missed">${esc(p.missed.text)}</div>` : "";
  const lines = (p.tile && p.tile.lines || []).map((l) =>
    `<div class="tile-line"><span>${esc(l.label)}</span><strong>${esc(l.value)}</strong></div>`).join("");
  const tile = p.tile && (p.tile.title || lines)
    ? `<div class="tile"><strong>${esc(p.tile.title || p.name)}</strong>${lines}</div>` : "";
  const prefs = state.layout.home;
  const size = window.DAPLayout
    ? window.DAPLayout.resolveSize((prefs.sizes || {})[p.name], p.tile_size, p.tile_min)
    : (p.tile_size || "medium");
  const collapsed = hasId(prefs.collapsed, p.name);
  const pinned = hasId(prefs.pinned, p.name);
  const ui = p.ui_url
    ? `<a class="ghost" href="${esc(withLayout(p.ui_url))}" target="_blank" rel="noreferrer">Open ${esc(p.ui_title || "web UI")}</a>`
    : "";
  return `<article class="card plugin-card size-${esc(size)} ${collapsed ? "is-collapsed" : ""} ${pinned ? "is-pinned" : ""}" data-kind="home" data-card-id="${esc(p.name)}">
    <header>
      <span class="dot ${esc(dot)}"></span>
      <div>
        <h2>${esc(p.name)}</h2>
        <p class="sub">${esc(p.type)} · ${esc(p.schedule || "manual")}</p>
      </div>
      <div class="head-side">
        ${p.pending ? `<span class="count-pill">${p.pending} pending</span>` : ""}
        ${cardTools("home", p.name, sizeSwitch(p.name, size, p.tile_min || "small"))}
      </div>
    </header>
    <div class="card-details">
      <dl class="meta">
        <div><dt>Last run</dt><dd>${esc(fmtTime(p.last_run))}</dd></div>
        <div><dt>Next run</dt><dd>${esc(fmtTime(p.next_run))}</dd></div>
      </dl>
      ${tile}
      <p class="sub">${esc(health.message || "")}</p>
    </div>
    ${missed}
    <div class="actions">
      <button class="primary" data-act="run" data-name="${esc(p.name)}">Run now</button>
      ${ui}
    </div>
  </article>`;
}

function visibleInbox() {
  const cards = inboxCards();
  if (!window.DAPLayout) return cards;
  return window.DAPLayout.arrange(cards, (c) => c.id, state.layout.inbox);
}

function syncSelection(cards) {
  if (state.selectedId) {
    const found = cards.findIndex((c) => c.id === state.selectedId);
    if (found >= 0) state.selected = found;
  }
  if (!cards.length) {
    state.selected = 0;
    return cards;
  }
  if (state.selected >= cards.length) state.selected = cards.length - 1;
  if (state.selected < 0) state.selected = 0;
  state.selectedId = cards[state.selected].id;
  return cards;
}

function shell(inner, mainClass) {
  const pending = state.data.home ? state.data.home.pending : 0;
  const item = (route, label, icon) => {
    const active = state.route === route ? "active" : "";
    const badge = route === "inbox" && pending ? `<span class="badge">${pending}</span>` : "";
    const href = route === "home" ? "#/" : "#/" + route;
    return `<a class="${active}" href="${href}" ${active ? 'aria-current="page"' : ""}>${ICONS[icon]}<span>${label}</span>${badge}</a>`;
  };
  return `
    <div class="frame">
      <header class="topbar">
        <div class="brand"><strong>DAP</strong><span>Dashboard Agent Platform</span></div>
        ${themeControl()}
      </header>
      <div class="app">
      <aside class="sidebar">
        <nav class="nav">
          ${item("home", "Home", "home")}
          ${item("chat", "Chat", "chat")}
          ${item("inbox", "Decision inbox", "inbox")}
          ${item("activity", "Activity", "activity")}
          ${item("guardrails", "Guardrails", "guard")}
          ${item("plugins", "Plugins", "plugins")}
          ${item("connections", "Connections", "connections")}
          ${item("create", "Create plugin", "create")}
        </nav>
        ${quitControl()}
      </aside>
      <main class="main${mainClass ? " " + mainClass : ""}">
        ${state.error ? `<div class="banner">${esc(state.error)}</div>` : ""}
        ${state.notice ? `<div class="banner ok">${esc(state.notice)}</div>` : ""}
        ${inner}
      </main>
      </div>
    </div>`;
}

function quitNote(st) {
  const acting = Number(st.acting) || 0;
  const inflight = Number(st.inflight_actions) || 0;
  if (acting > 0 && inflight > 0) {
    return "An approved action is still executing, and a plugin action is in progress. DAP will wait a few seconds for it to finish.";
  }
  if (acting > 0) {
    return "An approved action is still executing. DAP will wait a few seconds for it to finish.";
  }
  if (Number(st.chat_turns) > 0) {
    return "A chat turn is still running. DAP will stop it.";
  }
  if (inflight > 0) {
    return "A plugin action is still in progress. DAP will wait a few seconds for it to finish.";
  }
  return "";
}

function quitControl() {
  const note = state.quitConfirm && state.quitConfirm.note
    ? `<p class="quit-note">${esc(state.quitConfirm.note)}</p>` : "";
  const dialog = state.quitConfirm ? `
    <div class="quit-dialog" role="dialog" aria-modal="true" aria-labelledby="quit-title" aria-describedby="quit-copy" tabindex="-1">
      <h2 id="quit-title">Quit DAP?</h2>
      <p id="quit-copy">Scheduled agents stop running until you open DAP again.</p>
      ${note}
      <div class="inline">
        <button type="button" class="primary" data-act="confirm-quit">Quit</button>
        <button type="button" class="quiet" data-act="cancel-quit">Cancel</button>
      </div>
    </div>` : "";
  return `
    <div class="sidebar-end">
      <div class="side-foot"><strong>Running locally</strong>All data stays on this machine.</div>
      <div class="quit-wrap">
        ${dialog}
        <button type="button" class="quit-btn" data-act="quit" aria-label="Quit DAP">${ICONS.power}<span>Quit</span></button>
      </div>
    </div>`;
}

function focusEl(selector) {
  const el = document.querySelector(selector);
  if (!el) return;
  try { el.focus({ focusVisible: true }); } catch (e) { el.focus(); }
}

async function beginQuit() {
  if (state.quitting || state.quitPending) return;
  state.quitPending = true;
  state.error = "";
  let st = {};
  try {
    st = await api("/api/quit-status");
  } catch (err) {
    state.quitPending = false;
    state.error = err.message;
    render();
    return;
  }
  const acting = Number(st.acting) || 0;
  const inflight = Number(st.inflight_actions) || 0;
  if (!st.running && acting === 0 && inflight === 0) {
    await doQuit();
    return;
  }
  state.quitConfirm = { note: quitNote(st) };
  state.quitPending = false;
  render();
  focusEl("[data-act=confirm-quit]");
}

async function doQuit() {
  state.quitting = true;
  state.quitPending = false;
  try {
    await api("/api/quit", { method: "POST", body: {}, headers: { "X-DAP-Request": "quit" } });
  } catch (err) {
    // The process may close the socket as it exits.
  }
  showQuitPage();
}

function showQuitPage() {
  state.quitting = true;
  document.title = "DAP has quit";
  document.body.innerHTML = `<main class="quit-page"><div><h1>DAP has quit.</h1><p>You can close this window.</p></div></main>`;
  const closeWin = () => { try { window.close(); } catch (e) {} };
  closeWin();
  setTimeout(closeWin, 200);
}

function homeView() {
  const home = state.data.home;
  if (!home) return shell(`<p class="sub">Loading…</p>`);
  const plugins = arrangePlugins(home.plugins);
  const cards = plugins.map(pluginCard).join("");
  const agents = (home.agents || []).map((a) => a.found ? a.name : null).filter(Boolean);
  const agentLine = agents.length
    ? `Coding agents on this machine: ${agents.join(", ")}.`
    : "No coding agent (Claude Code, Codex, OpenCode) was found on PATH. DAP still runs; the agent is optional.";
  const view = state.layout.home.view;
  let body = `<div class="card empty"><h2>No plugins yet</h2><p>Install hello-watch and workflow from the Plugins page.</p></div>`;
  if (cards && view === "board") {
    const labels = [["needs", "Needs you"], ["attention", "Attention"], ["down", "Down"], ["healthy", "Healthy"]];
    const cols = labels.map(([id, title]) => {
      const html = plugins.filter((p) => homeBucket(p) === id).map(pluginCard).join("");
      return html ? `<section class="board-col"><h2>${title}</h2><div class="stack">${html}</div></section>` : "";
    }).join("");
    body = `<div class="board">${cols}</div>`;
  } else if (cards) {
    body = `<div class="cards ${esc(view)}">${cards}</div>`;
  }
  return shell(`
    <header class="page-head">
      <div>
        <h1>Home</h1>
        <p>Every plugin reports here. Nothing sends, spends, or deletes without an approval.</p>
      </div>
      ${layoutBar("home")}
    </header>
    ${body}
    <p class="sub" style="margin-top:18px">${esc(agentLine)}</p>
  `);
}

function inboxCards() {
  return (state.data.inbox && state.data.inbox.cards) || [];
}

function decisionCard(c, i) {
  const selected = i === state.selected;
  const effects = (c.effects || []).map((e) => `<span class="pill ${esc(e)}">${esc(e)}</span>`).join(" ");
  const risk = `<span class="pill ${esc(c.risk)}">${esc(modeLabel(c.risk))}</span>`;
  const kind = c.kind === "change" ? `<span class="pill change">Change</span>` : "";
  const evidence = (c.evidence || []).map((e) => e.ref ? `${e.label}: ${e.ref}` : e.label).join(" · ");
  const editing = state.mode && state.mode.id === c.id;
  let extra = "";
  if (editing && state.mode.type === "edit") {
    extra = `<form class="editor" data-act="save-edit" data-id="${esc(c.id)}">
      <label class="field">Title<input name="title" type="text" value="${esc(c.title)}" /></label>
      <label class="field">Summary<input name="summary" type="text" value="${esc(c.summary || "")}" /></label>
      <label class="field">Body<textarea name="body">${esc(c.body || "")}</textarea></label>
      <div class="inline"><button class="primary" type="submit">Save edit</button><button class="quiet" type="button" data-act="cancel-mode">Cancel</button></div>
    </form>`;
  } else if (editing && state.mode.type === "reject") {
    extra = `<form class="reject-box" data-act="save-reject" data-id="${esc(c.id)}">
      <label class="field">Why not?<input name="reason" type="text" placeholder="A short reason" required /></label>
      <div class="inline"><button class="primary" type="submit">Reject</button><button class="quiet" type="button" data-act="cancel-mode">Cancel</button></div>
    </form>`;
  } else if (editing && state.mode.type === "snooze") {
    extra = `<div class="snooze-box inline">
      <button class="ghost" data-act="snooze-preset" data-id="${esc(c.id)}" data-preset="1h">1 hour</button>
      <button class="ghost" data-act="snooze-preset" data-id="${esc(c.id)}" data-preset="4h">4 hours</button>
      <button class="ghost" data-act="snooze-preset" data-id="${esc(c.id)}" data-preset="tomorrow">Tomorrow 8:00</button>
      <button class="quiet" data-act="cancel-mode">Cancel</button>
    </div>`;
  }
  const snoozed = c.status === "snoozed" ? `<span class="pill">Snoozed until ${esc(fmtTime(c.snooze_until))}</span>` : "";
  const collapsed = !editing && hasId(state.layout.inbox.collapsed, c.id);
  const pinned = hasId(state.layout.inbox.pinned, c.id);
  return `<article class="card decision ${selected ? "selected" : ""} ${collapsed ? "is-collapsed" : ""} ${pinned ? "is-pinned" : ""}" data-kind="inbox" data-card-id="${esc(c.id)}" data-index="${i}">
    <header class="row">
      <div style="flex:1">
        <h2>${esc(c.title)}</h2>
        <div class="who">${esc(c.plugin)} · ${esc(c.kind)}</div>
      </div>
      <div class="inline pills">${kind}${effects}${risk}${snoozed}</div>
      ${cardTools("inbox", c.id)}
    </header>
    <div class="card-details">
      ${c.summary ? `<p class="summary">${esc(c.summary)}</p>` : ""}
      ${c.body ? `<div class="body">${esc(c.body)}</div>` : ""}
      ${extra}
    </div>
    <footer>
      <div class="actions">
        <button class="primary" data-act="approve" data-id="${esc(c.id)}">Approve<kbd>A</kbd></button>
        <button class="ghost" data-act="edit" data-id="${esc(c.id)}">Edit<kbd>E</kbd></button>
        <button class="ghost" data-act="reject" data-id="${esc(c.id)}">Reject<kbd>R</kbd></button>
        <button class="ghost" data-act="snooze" data-id="${esc(c.id)}">Snooze<kbd>S</kbd></button>
      </div>
      <div class="evidence">${evidence ? "Evidence · " + esc(evidence) : ""}</div>
    </footer>
  </article>`;
}

function splitRow(c, i) {
  const selected = i === state.selected;
  return `<div class="split-item ${selected ? "selected" : ""}" data-kind="inbox" data-card-id="${esc(c.id)}">
    <button type="button" class="split-select" data-act="select-card" data-id="${esc(c.id)}" data-index="${i}">
      <strong>${esc(c.title)}</strong>
      <span class="sub">${esc(c.plugin)} · ${esc(modeLabel(c.risk))}</span>
    </button>
    <div class="actions">
      <button class="primary" data-act="approve" data-id="${esc(c.id)}">Approve</button>
      <button class="ghost" data-act="reject" data-id="${esc(c.id)}">Reject</button>
    </div>
  </div>`;
}

function inboxView() {
  const cards = syncSelection(visibleInbox());
  const list = cards.map(decisionCard).join("");
  const waiting = cards.filter((c) => c.status === "pending").length;
  const view = state.layout.inbox.view;
  let body = `<div class="card empty"><h2>Nothing waiting</h2><p>Plugins keep watching. Pending cards never block them.</p></div>`;
  if (list && view === "board") {
    const groups = [];
    const seen = {};
    cards.forEach((c, i) => {
      if (seen[c.plugin] == null) {
        seen[c.plugin] = groups.length;
        groups.push({ plugin: c.plugin, html: "" });
      }
      groups[seen[c.plugin]].html += decisionCard(c, i);
    });
    body = `<div class="board">${groups.map((g) => `<section class="board-col"><h2>${esc(g.plugin)}</h2><div class="stack">${g.html}</div></section>`).join("")}</div>`;
  } else if (list && view === "split") {
    const narrow = window.matchMedia("(max-width: 800px)").matches;
    const open = state.splitOpen && narrow ? " show-detail" : "";
    const current = cards[state.selected];
    body = `<div class="split${open}">
      <div class="split-list" role="listbox" aria-label="Waiting cards">${cards.map(splitRow).join("")}</div>
      <div class="split-detail">
        <button type="button" class="ghost split-back" data-act="split-back">Back</button>
        ${current ? decisionCard(current, state.selected) : ""}
      </div>
    </div>`;
  } else if (list) {
    body = `<div class="cards ${esc(view)}">${list}</div>`;
  }
  return shell(`
    <header class="page-head">
      <div>
        <h1>Decision inbox</h1>
        <p>Needs you · ${waiting}. <span class="keys">j/k move · a approve · e edit · r reject · s snooze</span></p>
      </div>
      ${layoutBar("inbox")}
    </header>
    ${body}
  `);
}

function activityView() {
  const rows = (state.data.activity && state.data.activity.entries) || [];
  const items = rows.map((e) => {
    const ev = (e.evidence || []).map((x) => x.ref || x.label).filter(Boolean).join(", ");
    return `<li>
      <time>${esc(fmtTime(e.ts))}</time>
      <span class="kind">${esc(e.kind)}</span>
      <div>${esc(e.message)}${e.plugin ? ` <span class="sub">${esc(e.plugin)}</span>` : ""}${ev ? `<div class="evidence">${esc(ev)}</div>` : ""}</div>
    </li>`;
  }).join("");
  return shell(`
    <header class="page-head"><div><h1>Activity</h1><p>Append-only. Rows are never rewritten.</p></div></header>
    ${items ? `<ul class="card activity">${items}</ul>` : `<div class="card empty"><h2>No activity yet</h2><p>Runs, approvals, and denials land here.</p></div>`}
  `);
}

function guardView() {
  const g = state.data.guard;
  if (!g) return shell(`<p class="sub">Loading…</p>`);
  const policy = (g.policy || []).map((p) => `<article class="card"><h2>${esc(p.title)}</h2><p>${esc(p.body)}</p></article>`).join("");
  const blocks = (g.plugins || []).map((p) => {
    const rows = (p.permissions || []).map((perm) => `
      <div class="perm">
        <div><strong>${esc(perm.name)}</strong><div class="sub">${esc(perm.plain)}</div></div>
        <div class="tighten">
          <span class="pill ${esc(perm.mode)}">${esc(modeLabel(perm.mode))}</span>
          <select data-act="rule" data-plugin="${esc(p.name)}" data-permission="${esc(perm.name)}">
            <option value="${esc(perm.mode)}" selected>${esc(modeLabel(perm.mode))}</option>
            ${perm.mode !== "never" ? `<option value="never">Never</option>` : ""}
            ${perm.mode === "allowed" ? `<option value="ask_first">Ask first</option>` : ""}
          </select>
        </div>
      </div>`).join("");
    return `<section class="card plugin-block"><h2 style="margin:0">${esc(p.name)}</h2><p class="sub">${esc(p.type)}</p>${rows || `<p class="sub">No permissions declared. Anything it didn't declare is denied.</p>`}</section>`;
  }).join("");
  const checks = (g.checks || []).slice(0, 12).map((c) =>
    `<li><span class="pill ${esc(c.decision)}">${esc(modeLabel(c.decision))}</span> ${esc(c.plugin)} ${esc(c.permission || "")} <span class="sub">${esc(c.reason)}</span></li>`).join("");
  return shell(`
    <header class="page-head"><div><h1>Guardrails</h1><p>Allowed, ask first, or never. Checked before every action.</p></div></header>
    <div class="policy">${policy}</div>
    ${blocks}
    <h2 class="section-title">Recent checks</h2>
    ${checks ? `<ul class="card plugin-block checks">${checks}</ul>` : `<p class="sub">No checks yet.</p>`}
  `);
}

function pluginsView() {
  const plugins = (state.data.plugins && state.data.plugins.plugins) || [];
  const cards = plugins.map((p) => {
    const perms = (p.permissions || []).map((perm) =>
      `<div class="perm"><div><strong>${esc(perm.name)}</strong><div class="sub">${esc(perm.plain)}</div></div><span class="pill ${esc(perm.mode)}">${esc(modeLabel(perm.mode))}</span></div>`).join("");
    const health = (p.health && p.health.message) || "";
    return `<article class="card plugin-block">
      <header class="row">
        <span class="dot ${esc((p.health || {}).status || "degraded")}"></span>
        <div style="flex:1">
          <h2 style="margin:0">${esc(p.name)} <span class="sub">${esc(p.version)}</span></h2>
          <p class="sub">${esc(p.type)} · ${esc(p.interface || "")} · ${p.running ? "running" : "stopped"}</p>
        </div>
      </header>
      <p>${esc(p.description || "")}</p>
      <p class="sub">${esc(health)}${p.error ? " · " + esc(p.error) : ""}</p>
      ${perms}
      <div class="actions" style="margin-top:12px">
        ${p.needs_approval ? `<button class="primary" data-act="approve-manifest" data-name="${esc(p.name)}">Approve permissions</button>` : ""}
        ${p.ui_url ? `<a class="ghost" href="${esc(withLayout(p.ui_url))}" target="_blank" rel="noreferrer">Open ${esc(p.ui_title || "web UI")}</a>` : ""}
        <button class="ghost" data-act="edit-agent" data-name="${esc(p.name)}">Edit with agent</button>
        <button class="ghost" data-act="remove" data-name="${esc(p.name)}">Remove</button>
      </div>
    </article>`;
  }).join("");
  const preview = state.preview;
  const previewHTML = preview ? `
    <div class="card plugin-block" style="margin-top:12px">
      <h2 style="margin:0">${esc(preview.name)} wants these permissions</h2>
      <p class="sub">${esc(preview.type)} · ${esc(preview.version)}. Anything not listed is denied.</p>
      ${(preview.permissions || []).map((perm) => `<div class="perm"><div><strong>${esc(perm.name)}</strong><div class="sub">${esc(perm.plain)}</div></div><span class="pill ${esc(perm.mode)}">${esc(modeLabel(perm.mode))}</span></div>`).join("")}
      <div class="actions" style="margin-top:12px">
        <button class="primary" data-act="confirm-install">Approve and install</button>
        <button class="quiet" data-act="cancel-install">Cancel</button>
      </div>
    </div>` : "";
  return shell(`
    <header class="page-head"><div><h1>Plugins</h1><p>Drop-in packages. Each one runs in its own process.</p></div></header>
    <form class="card plugin-block" data-act="preview-install" style="margin-bottom:14px">
      <h2 style="margin:0 0 8px">Install a plugin</h2>
      <div class="inline">
        <input name="path" type="text" placeholder="/path/to/plugin" value="${esc(state.installPath)}" style="flex:1" />
        <button class="primary" type="submit">Review permissions</button>
      </div>
    </form>
    ${previewHTML}
    <div class="stack">${cards || `<div class="card empty"><h2>Nothing installed</h2></div>`}</div>
  `);
}

function runtimeHint(lang) {
  const rows = (state.data.home && state.data.home.runtimes) || [];
  const node = rows.find((r) => r.name === "node");
  const bun = rows.find((r) => r.name === "bun");
  if (lang === "python") return "Python plugins run with python3. The other shipped plugins that are still Python keep working.";
  const nodeLine = node && node.ok ? "Node " + (node.version || "") + " is available." : "Node.js 20 or newer is not available. Install it from https://nodejs.org.";
  const bunLine = bun && bun.found ? " Bun can run TypeScript directly when a plugin sets runtime: bun." : "";
  return nodeLine + bunLine;
}

function suggestType(text) {
  const d = (text || "").toLowerCase();
  if (d.includes("workflow") || d.includes("step")) return "workflow";
  if (d.includes("capability") || d.includes("other plugin")) return "capability";
  return "watcher";
}

function createView() {
  const c = state.create;
  const edit = hashQuery().get("edit") || c.edit || "";
  const agents = (state.data.home && state.data.home.agents) || [];
  const found = agents.filter((a) => a.found);
  const suggested = suggestType(c.description);
  const type = c.type || suggested;
  if (c.phase === "run" || c.phase === "done") {
    const checks = (c.report && c.report.checks || []).map((row) =>
      `<li class="${row.ok ? "pass" : "fail"}"><strong>${esc(row.name)}</strong> ${esc(row.detail || "")}</li>`).join("");
    const failed = c.report && c.report.ok === false;
    const passed = c.report && c.report.ok === true;
    return shell(`
      <header class="page-head"><div><h1>${edit ? "Edit with agent" : "Create plugin"}</h1><p>${esc(c.name || edit || "Plugin")}</p></div></header>
      <section class="card plugin-block">
        <p class="sub">${esc(c.status || "Working")}</p>
        <pre class="create-log" aria-live="polite">${esc(c.log || "Starting…")}</pre>
        <div class="actions" style="margin-top:12px">
          ${c.status === "running" ? `<button class="ghost" data-act="create-cancel">Cancel</button>` : ""}
          ${failed ? `<button class="primary" data-act="create-fix">Ask the agent to fix</button>` : ""}
          ${passed ? `<a class="ghost" href="#/inbox">Review the change card</a>` : ""}
          <button class="quiet" data-act="create-reset">Start over</button>
        </div>
        ${checks ? `<h2 class="section-title">Validation</h2><ul class="checks">${checks}</ul>` : ""}
        ${passed ? `<p class="sub">Validation passed. A change card is in the decision inbox. The plugin is not installed until you approve it.</p>` : ""}
      </section>
    `);
  }
  const typeBtn = (id, label, hint) => `<button type="button" class="ghost type-choice ${type === id ? "selected" : ""}" data-act="create-type" data-type="${id}" aria-pressed="${type === id}">${label}<span class="sub">${hint}</span></button>`;
  const agentBtns = agents.map((a) => {
    const disabled = a.found ? "" : "disabled";
    const on = c.agent === a.name ? "selected" : "";
    return `<button type="button" class="ghost ${on}" data-act="create-agent" data-agent="${esc(a.name)}" ${disabled} aria-pressed="${c.agent === a.name}">${esc(a.name)}${a.found ? "" : " · not installed"}</button>`;
  }).join("");
  const hints = agents.filter((a) => !a.found).map((a) => `<li><strong>${esc(a.name)}</strong> is not on PATH. Install it and log in with your own account. DAP does not take an API key. ${esc(a.name === "claude" ? "Claude Code: https://docs.anthropic.com/en/docs/claude-code" : a.name === "codex" ? "Codex CLI, then codex login." : "OpenCode, then log in.")}</li>`).join("");
  return shell(`
    <header class="page-head">
      <div>
        <h1>${edit ? "Edit " + esc(edit) : "Create plugin"}</h1>
        <p>${edit ? "Describe the change. Approval still happens in the inbox." : "Describe what you want. Your coding agent writes the plugin. You approve before it is installed."}</p>
      </div>
    </header>
    <form class="card plugin-block stack" data-act="create-start">
      <label class="field">What should it do?
        <textarea name="description" required placeholder="Watch a folder and ask me before it deletes anything.">${esc(c.description)}</textarea>
      </label>
      ${edit ? "" : `<label class="field">Name
        <input name="name" type="text" placeholder="folder-watch" value="${esc(c.name)}" />
      </label>`}
      <div>
        <p class="sub">Type${type === suggested ? " · suggested" : ""}</p>
        <div class="type-row">
          ${typeBtn("watcher", "Watcher", "Notices something and proposes cards")}
          ${typeBtn("capability", "Capability", "A tool other plugins can use")}
          ${typeBtn("workflow", "Workflow", "Ordered steps, some may wait")}
        </div>
      </div>
      ${edit ? "" : `<label class="field inline"><input name="ui" type="checkbox" ${c.ui ? "checked" : ""} /> Include a small web page</label>
      <label class="field">Language
        <select name="lang">
          <option value="ts" ${c.lang === "ts" ? "selected" : ""}>TypeScript</option>
          <option value="python" ${c.lang === "python" ? "selected" : ""}>Python</option>
        </select>
      </label>
      <p class="sub">${runtimeHint(c.lang)}</p>`}
      <div>
        <p class="sub">Coding agent</p>
        <div class="type-row">${agentBtns || `<p class="sub">No agent list yet.</p>`}</div>
      </div>
      ${found.length ? "" : `<div class="banner">None of Claude Code, Codex, or OpenCode is on PATH. Install one and log in, then come back. DAP will not call a model for you.</div>`}
      ${hints ? `<ul class="sub">${hints}</ul>` : ""}
      <button class="primary" type="submit" ${found.length ? "" : "disabled"}> ${edit ? "Ask the agent to edit" : "Create draft and run agent"}</button>
    </form>
  `);
}

function connectionsView() {
  const data = state.data.connections;
  if (!data) return shell(`<p class="sub">Loading…</p>`);
  const cards = (data.connections || []).map((c) => connectionCard(c, !!data.key_saved)).join("");
  const agents = (data.agents || []).map((a) => `<li>${esc(a.name)} · ${a.found ? esc(a.path) : "not on PATH"}</li>`).join("");
  return shell(`
    <header class="page-head"><div>
      <h1>Connections</h1>
      <p>Sign in inside Google or Microsoft’s window. Composio holds those tokens. DAP stores the API key and each connection id in the keychain.</p>
    </div></header>
    <article class="card plugin-block">
      <h2 style="margin:0">Composio</h2>
      <p class="sub">One key covers Gmail, Outlook, and any toolkit you add later. Message contents pass through Composio.</p>
      <label class="field">API key
        <input data-composio-key type="password" autocomplete="off" spellcheck="false" placeholder="${data.key_saved ? "Saved — paste a new key to replace it" : "Paste the key from the Composio dashboard"}" />
      </label>
      <div class="actions" style="margin-top:12px">
        <button class="primary" data-act="save-key">Save key</button>
        ${data.key_saved ? `<span class="pill allowed">Key saved</span>` : `<span class="pill">No key yet</span>`}
      </div>
    </article>
    <div class="stack">${cards}</div>
    <h2 class="section-title">Coding agents</h2>
    <ul class="card plugin-block">${agents}</ul>
  `);
}

function connectionCard(c, keySaved) {
  const dot = c.connected ? "ok" : "degraded";
  const waiting = state.connecting === c.provider;
  const account = c.account ? " · " + esc(c.account) : "";
  const detail = c.refresh_detail && c.refresh_detail !== "not connected" ? `<p class="sub">${esc(c.refresh_detail)}</p>` : "";
  const throttle = c.throttle && c.throttle.active ? `<p class="missed">Throttled until ${esc(fmtTime(c.throttle.until))}.</p>` : "";
  if (c.provider === "fake") {
    return `<article class="card plugin-block">
      <header class="row">
        <span class="dot ${dot}"></span>
        <div style="flex:1">
          <h2 style="margin:0">${esc(c.label)}</h2>
          <p class="sub">${c.connected ? "Connected" : "Not connected"}${account}</p>
        </div>
        ${c.installed ? `<span class="pill allowed">Installed</span>` : `<span class="pill">Not installed</span>`}
      </header>
      <p class="sub">Local sample threads. No account and no token.</p>
      <div class="actions" style="margin-top:12px">
        ${c.connected
          ? `<button class="ghost" data-act="disconnect" data-provider="fake">Disconnect</button>`
          : `<button class="primary" data-act="connect" data-provider="fake">Connect</button>`}
      </div>
    </article>`;
  }
  const direct = c.mode === "direct";
  const connectLabel = waiting ? "Waiting for sign-in…" : "Connect " + (c.label || "account");
  let button = "";
  if (c.connected) {
    button = `<button class="ghost" data-act="disconnect" data-provider="${esc(c.provider)}">Disconnect</button>`;
  } else if (direct) {
    button = `<button class="primary" data-act="connect" data-provider="${esc(c.provider)}" ${waiting ? "disabled" : ""}>${esc(connectLabel)}</button>`;
  } else {
    button = `<button class="primary" data-act="connect" data-provider="${esc(c.provider)}" ${!keySaved || waiting ? "disabled" : ""}>${esc(connectLabel)}</button>`;
  }
  const needKey = !direct && !keySaved && !c.connected ? `<p class="sub">Save the Composio API key first.</p>` : "";
  return `<article class="card plugin-block">
    <header class="row">
      <span class="dot ${dot}"></span>
      <div style="flex:1">
        <h2 style="margin:0">${esc(c.label)}</h2>
        <p class="sub">${c.connected ? "Connected" : "Not connected"}${account}${direct ? " · direct" : " · Composio"}</p>
      </div>
      ${c.installed ? `<span class="pill allowed">Installed</span>` : ""}
    </header>
    ${c.summary ? `<p class="sub">${esc(c.summary)}</p>` : ""}
    ${detail}
    ${throttle}
    ${needKey}
    ${direct ? directFields(c) : ""}
    <div class="actions" style="margin-top:12px">
      ${button}
      ${direct ? `<button class="quiet" data-act="set-mode" data-provider="${esc(c.provider)}" data-mode="composio">Use Composio</button>` : ""}
    </div>
    ${c.direct_fallback && !direct ? graphFallback(c) : ""}
  </article>`;
}

function directFields(c) {
  const calendar = c.provider === "outlook" || c.provider === "graph" ? `
    <label class="sub"><input data-calendar type="checkbox" ${c.calendar_enabled ? "checked" : ""} /> Calendars.Read, off unless you need it</label>` : "";
  return `<label class="field">Client ID
      <input data-client="${esc(c.provider)}" type="text" value="${esc(c.client_id || "")}" placeholder="Public client id" autocomplete="off" />
    </label>
    ${calendar}
    <div class="actions" style="margin-top:8px">
      <button class="ghost" data-act="save-connection" data-provider="${esc(c.provider)}">Save client id</button>
    </div>`;
}

function graphFallback(c) {
  return `<p class="sub" style="margin-top:10px"><button class="quiet" data-act="set-mode" data-provider="${esc(c.provider)}" data-mode="direct">Microsoft Graph instead</button></p>`;
}

function chatProviderList() {
  if (state.chat.providers && state.chat.providers.length) return state.chat.providers;
  return [
    { id: "claude-code", label: "Claude Code" },
    { id: "codex", label: "Codex" },
  ];
}

function selectedChatProvider() {
  const id = (state.chat.session && state.chat.session.provider) || state.chat.provider || "claude-code";
  return chatProviderList().find((p) => p.id === id) || { id: id, label: id === "codex" ? "Codex" : "Claude Code" };
}

function chatReady(probe) {
  if (!probe || probe.installed == null) return true;
  if (!probe.installed) return false;
  if (probe.signed_in) return true;
  return !!probe.auth_unknown;
}

function chatView() {
  const providers = chatProviderList();
  const current = state.chat.session;
  const selected = (current && current.provider) || state.chat.provider || "claude-code";
  const probe = providers.find((p) => p.id === selected) || {};
  const locked = current && current.messages && current.messages.length;
  const picker = providers.map((p) => {
    const on = p.id === selected;
    const dot = !p.installed ? "bad" : p.signed_in ? "ok" : p.auth_unknown ? "warn" : "bad";
    const version = p.version ? ` <span class="chat-ver">${esc(p.version)}</span>` : "";
    return `<button type="button" class="chat-provider" role="radio" aria-checked="${on ? "true" : "false"}" data-act="chat-provider" data-provider="${esc(p.id)}" ${locked && !on ? "" : ""}>
      <span class="dot ${dot}"></span><span>${esc(p.label || p.id)}</span>${version}
    </button>`;
  }).join("");
  const hint = probe.hint
    ? `<div class="chat-hint" role="status"><strong>${esc(probe.login || "Setup")}</strong><p>${esc(probe.hint)}</p></div>`
    : "";
  const history = (state.chat.sessions || []).map((s) => {
    const on = s.id === state.chat.current;
    const who = s.provider === "codex" ? "Codex" : "Claude Code";
    return `<button type="button" class="chat-session${on ? " on" : ""}" data-act="chat-open" data-id="${esc(s.id)}" aria-current="${on ? "true" : "false"}">
      <strong>${esc(s.title || "New chat")}</strong><span>${esc(who)}</span>
    </button>`;
  }).join("");
  const ready = chatReady(probe);
  const streaming = state.chat.streaming;
  const sendLabel = streaming ? "Stop" : "Send";
  const sendAct = streaming ? "chat-stop" : "chat-send";
  return shell(`
    <div class="chat-shell">
      <header class="chat-head">
        <div>
          <h1>Chat</h1>
          <p>Read-only until you approve a card. <span class="keys">Ctrl+Shift+C</span></p>
        </div>
        <div class="chat-head-actions">
          <div class="chat-picker" role="radiogroup" aria-label="Coding agent">${picker}</div>
          <button type="button" class="ghost" data-act="chat-new">New chat</button>
        </div>
      </header>
      ${hint}
      <div class="chat-body">
        <aside class="chat-history" aria-label="Past chats">
          ${history || `<p class="chat-history-empty">No past chats</p>`}
        </aside>
        <section class="chat-stage">
          <div class="chat-thread" id="chat-thread">${chatThreadInner()}</div>
          <form class="chat-composer" data-act="chat-send">
            <label class="sr" for="chat-input">Message</label>
            <textarea id="chat-input" class="chat-input" rows="2" placeholder="${ready ? "Ask a question. A change or a send becomes a card." : "Set up the CLI to start chatting."}" ${ready && !streaming ? "" : "disabled"}>${esc(state.chat.draft || "")}</textarea>
            <button type="${streaming ? "button" : "submit"}" class="${streaming ? "ghost" : "primary"}" data-act="${sendAct}" ${!ready && !streaming ? "disabled" : ""}>${sendLabel}</button>
          </form>
        </section>
      </div>
    </div>
  `, "chat-main");
}

function chatThreadInner() {
  const sess = state.chat.session;
  const messages = (sess && sess.messages) || [];
  if (!messages.length) {
    if (state.chat.current && !sess) {
      return `<div class="chat-empty"><h2>Loading…</h2></div>`;
    }
    const who = selectedChatProvider().label;
    return `<div class="chat-empty">
      <h2>Ask ${esc(who)}</h2>
      <p>This chat can read its own workspace. Anything that would change data or send something becomes a card. Nothing happens until you approve it, and DAP never sees the CLI login.</p>
    </div>`;
  }
  return messages.map((m) => {
    if (m.role === "user") {
      return `<div class="chat-row user"><div class="chat-bubble">${esc(m.text)}</div></div>`;
    }
    const tools = (m.tools || []).map((t) => {
      const detail = t.detail ? ` · ${esc(t.detail)}` : "";
      const label = t.blocked ? "Not run" : esc(t.name || "Tool");
      return `<div class="chat-tool${t.blocked ? " blocked" : ""}"><span class="chat-tool-dot"></span>${t.blocked ? "Not run · " + esc(t.name || "Tool") : label}${detail}</div>`;
    }).join("");
    const cards = (m.cards || []).map((id) => chatCardHTML(state.chat.cards[id] || { id, title: "Approval card", status: "pending" })).join("");
    const body = m.text ? `<div class="chat-md">${renderMarkdown(m.text)}</div>` : (m.pending ? `<div class="chat-md chat-waiting">Thinking…</div>` : "");
    const err = m.error ? `<p class="chat-error">${esc(m.error)}</p>` : "";
    const cancelled = m.cancelled ? `<p class="chat-note">Stopped</p>` : "";
    return `<div class="chat-row assistant"><div class="chat-bubble">${tools}${body}${cards}${err}${cancelled}</div></div>`;
  }).join("");
}

function chatCardHTML(card) {
  if (!card) return "";
  const effects = (card.effects || []).map((e) => `<span class="pill ${esc(e)}">${esc(e)}</span>`).join(" ");
  const kind = card.kind === "change" ? `<span class="pill change">Change</span>` : "";
  const pending = !card.status || card.status === "pending" || card.status === "snoozed";
  const summary = card.result && card.result.summary ? card.result.summary : (card.summary || "");
  const actions = pending
    ? `<button type="button" class="primary" data-act="approve" data-id="${esc(card.id)}">Approve</button><a class="ghost chat-inbox" href="#/inbox">Open in inbox</a>`
    : `<span class="pill">${esc(card.status)}</span>`;
  const body = card.body ? `<div class="body chat-card-body">${esc(card.body)}</div>` : "";
  return `<article class="card chat-card">
    <header class="row"><div style="flex:1"><h2>${esc(card.title || "Approval")}</h2><div class="who">Chat · needs approval</div></div><div class="inline">${kind}${effects}</div></header>
    ${summary ? `<p class="summary">${esc(summary)}</p>` : ""}
    ${body}
    <footer><div class="actions">${actions}</div></footer>
  </article>`;
}

function renderMarkdown(src) {
  let text = esc(src || "");
  const blocks = [];
  text = text.replace(/```([^\n]*)\n([\s\S]*?)```/g, (_, lang, code) => {
    blocks.push(`<pre class="chat-code"><code>${code.replace(/\n$/, "")}</code></pre>`);
    return "\u0000" + (blocks.length - 1) + "\u0000";
  });
  text = text.replace(/`([^`]+)`/g, "<code>$1</code>");
  text = text.replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>');
  text = text.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  text = text.replace(/(^|[\s(])\*([^*\n]+)\*/g, "$1<em>$2</em>");
  const lines = text.split("\n");
  let html = "";
  let list = "";
  const flush = () => {
    if (!list) return;
    html += `<ul>${list}</ul>`;
    list = "";
  };
  lines.forEach((line) => {
    const holder = line.match(/^\u0000(\d+)\u0000$/);
    if (holder) {
      flush();
      html += blocks[Number(holder[1])] || "";
      return;
    }
    const item = line.match(/^\s*(?:[-*]|\d+\.)\s+(.*)$/);
    if (item) {
      list += `<li>${item[1]}</li>`;
      return;
    }
    flush();
    if (!line.trim()) {
      html += "";
      return;
    }
    html += `<p>${line}</p>`;
  });
  flush();
  return html;
}

function paintChatThread() {
  const thread = document.getElementById("chat-thread");
  if (!thread) {
    render();
    return;
  }
  thread.innerHTML = chatThreadInner();
  thread.scrollTop = thread.scrollHeight;
}

function applyChatEvent(ev) {
  const sess = state.chat.session;
  if (!sess) return;
  const msgs = sess.messages || (sess.messages = []);
  const last = msgs[msgs.length - 1];
  if (!last || last.role !== "assistant") return;
  if (ev.type === "text") {
    last.text = (last.text || "") + (ev.delta || "");
    last.pending = false;
  } else if (ev.type === "tool") {
    last.tools = last.tools || [];
    last.tools.push({ name: ev.name, detail: ev.detail, blocked: !!ev.blocked });
    last.pending = false;
  } else if (ev.type === "proposal" && ev.card) {
    state.chat.cards[ev.card.id] = ev.card;
    last.cards = last.cards || [];
    if (!last.cards.includes(ev.card.id)) last.cards.push(ev.card.id);
    last.pending = false;
  } else if (ev.type === "error") {
    last.error = ev.message || "The turn failed";
    last.pending = false;
  } else if (ev.type === "done") {
    if (ev.text) last.text = ev.text;
    last.pending = false;
  }
}

async function loadChat() {
  const [prov, list] = await Promise.all([
    api("/api/chat/providers"),
    api("/api/chat/sessions"),
  ]);
  state.chat.providers = prov.providers || [];
  if (!state.chat.provider) state.chat.provider = prov.provider || "claude-code";
  state.chat.sessions = list.sessions || [];
  if (state.chat.current && !state.chat.streaming) {
    const one = await api("/api/chat/sessions/" + state.chat.current);
    state.chat.session = one.session;
    state.chat.cards = one.cards || {};
  }
  state.chat.loaded = true;
}

async function sendChat() {
  const text = (state.chat.draft || "").trim();
  if (!text || state.chat.streaming) return;
  const provider = state.chat.provider || "claude-code";
  state.chat.draft = "";
  state.error = "";
  try {
    if (!state.chat.current) {
      const created = await api("/api/chat/sessions", { method: "POST", body: { provider } });
      state.chat.current = created.session.id;
      state.chat.session = created.session;
      state.chat.session.messages = [];
      state.chat.cards = {};
    }
    const sess = state.chat.session;
    sess.messages = sess.messages || [];
    sess.messages.push({ role: "user", text, at: new Date().toISOString() });
    sess.messages.push({ role: "assistant", text: "", tools: [], cards: [], pending: true });
    if (sess.title === "New chat" || !sess.title) sess.title = text.slice(0, 60);
    state.chat.streaming = true;
    render();
    const res = await fetch("/api/chat/sessions/" + encodeURIComponent(state.chat.current) + "/turn", {
      method: "POST",
      headers: {
        Authorization: "Bearer " + DAP.token,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ text }),
    });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      throw new Error(data.error || "Request failed");
    }
    const reader = res.body && res.body.getReader ? res.body.getReader() : null;
    if (!reader) throw new Error("This browser cannot stream the reply");
    const dec = new TextDecoder();
    let buf = "";
    while (true) {
      const step = await reader.read();
      if (step.done) break;
      buf += dec.decode(step.value, { stream: true });
      let idx;
      while ((idx = buf.indexOf("\n")) >= 0) {
        const line = buf.slice(0, idx).trim();
        buf = buf.slice(idx + 1);
        if (!line) continue;
        try { applyChatEvent(JSON.parse(line)); } catch (e) { continue; }
        paintChatThread();
      }
    }
  } catch (err) {
    state.error = err.message;
    const msgs = state.chat.session && state.chat.session.messages;
    if (msgs && msgs.length) {
      const last = msgs[msgs.length - 1];
      if (last.role === "assistant") {
        last.error = last.error || err.message;
        last.pending = false;
      }
    }
  } finally {
    state.chat.streaming = false;
    try { await loadChat(); } catch (e) { /* keep the local transcript */ }
    render();
  }
}

async function stopChat() {
  if (!state.chat.current) return;
  try {
    await api("/api/chat/sessions/" + encodeURIComponent(state.chat.current) + "/cancel", { method: "POST", body: {} });
  } catch (err) {
    state.error = err.message;
  }
}

function openChatSession(id) {
  if (state.chat.streaming) return;
  state.chat.current = id;
  state.chat.session = null;
  state.chat.focusComposer = true;
  render();
  loadChat().then(() => {
    const sess = state.chat.session;
    if (sess && sess.provider) state.chat.provider = sess.provider;
    render();
  }).catch((err) => {
    state.error = err.message;
    render();
  });
}

function newChat() {
  if (state.chat.streaming) return;
  state.chat.current = "";
  state.chat.session = null;
  state.chat.cards = {};
  state.chat.draft = "";
  state.chat.focusComposer = true;
  render();
}

function render() {
  if (state.quitting) return;
  const root = document.getElementById("app");
  if (!root) return;
  const html = {
    home: homeView,
    chat: chatView,
    inbox: inboxView,
    activity: activityView,
    guardrails: guardView,
    plugins: pluginsView,
    connections: connectionsView,
    create: createView,
  }[state.route]();
  const prevThread = document.getElementById("chat-thread");
  const threadTop = prevThread ? prevThread.scrollTop : 0;
  const stickThread = prevThread ? (prevThread.scrollHeight - prevThread.scrollTop - prevThread.clientHeight < 48) : true;
  const y = window.scrollY;
  root.innerHTML = html;
  if (state.scrollReset) {
    window.scrollTo(0, 0);
    state.scrollReset = false;
  } else if (state.forceScroll) {
    const selected = root.querySelector(".decision.selected, .split-item.selected");
    if (selected) selected.scrollIntoView({ block: "nearest" });
    state.forceScroll = false;
  } else {
    window.scrollTo(0, y);
  }
  const nextThread = document.getElementById("chat-thread");
  if (nextThread) nextThread.scrollTop = stickThread ? nextThread.scrollHeight : threadTop;
  if (state.chat.focusComposer) {
    state.chat.focusComposer = false;
    const box = root.querySelector(".chat-input");
    if (box && !box.disabled) box.focus({ preventScroll: true });
  }
  if (state.focusSelector) {
    const el = root.querySelector(state.focusSelector);
    if (el) el.focus({ preventScroll: true });
    state.focusSelector = "";
  }
}

async function refresh() {
  if (state.quitting || state.quitConfirm || state.quitPending) return;
  if (state.chat.streaming) return;
  try {
    if (state.route === "chat") await loadChat();
    if (state.route === "home" || !state.data.home) {
      state.data.home = await api("/api/home");
    }
    if (state.route === "inbox") state.data.inbox = await api("/api/inbox");
    if (state.route === "activity") state.data.activity = await api("/api/activity");
    if (state.route === "guardrails") state.data.guard = await api("/api/guardrails");
    if (state.route === "plugins" || state.route === "create") state.data.plugins = await api("/api/plugins");
    if (state.route === "connections") state.data.connections = await api("/api/connections");
    if (state.route !== "home") {
      state.data.home = await api("/api/home");
    }
    state.error = "";
  } catch (err) {
    state.error = err.message;
  }
  if (state.dragging || state.themeOpen) return;
  if (state.mode && (state.mode.type === "edit" || state.mode.type === "reject")) return;
  const ae = document.activeElement;
  if (ae && ae.closest && ae.closest(".layout-switch, .drag-handle, .size-switch")) return;
  render();
}

async function act(name, fn) {
  state.notice = "";
  try {
    await fn();
    state.error = "";
  } catch (err) {
    state.error = err.message;
    render();
    return;
  }
  state.mode = null;
  await refresh();
}

document.addEventListener("click", (ev) => {
  if (state.themeOpen && !ev.target.closest(".theme")) {
    state.themeOpen = false;
    if (!ev.target.closest("[data-act]") && !ev.target.closest(".decision")) {
      render();
      return;
    }
  }
  const el = ev.target.closest("[data-act]");
  if (!el) {
    const card = ev.target.closest(".decision");
    if (card && state.route === "inbox") {
      state.selected = Number(card.dataset.index);
      state.selectedId = card.dataset.cardId || "";
      render();
    }
    return;
  }
  const actName = el.dataset.act;
  const id = el.dataset.id;
  if (actName === "create-type") {
    state.create.type = el.dataset.type;
    render();
    return;
  }
  if (actName === "create-agent") {
    state.create.agent = el.dataset.agent;
    render();
    return;
  }
  if (actName === "edit-agent") {
    state.create = { description: "", name: el.dataset.name, type: "", lang: "ts", ui: false, agent: "", phase: "form", log: "", report: null, path: "", edit: el.dataset.name };
    location.hash = "#/create?edit=" + encodeURIComponent(el.dataset.name);
    state.route = "create";
    render();
    return;
  }
  if (actName === "create-cancel") {
    if (state.create.runId) api("/api/create/run/" + state.create.runId + "/cancel", { method: "POST", body: {} }).catch(() => {});
    state.create.status = "cancelled";
    render();
    return;
  }
  if (actName === "create-reset") {
    state.create = { description: "", name: "", type: "", lang: "ts", ui: false, agent: "", phase: "form", log: "", report: null, path: "", edit: "" };
    location.hash = "#/create";
    render();
    return;
  }
  if (actName === "create-fix") {
    const failed = (state.create.report.checks || []).filter((row) => !row.ok).map((row) => row.name + ": " + (row.detail || "")).join("\n");
    startAgent(state.create.description, failed);
    return;
  }
  if (actName === "theme-toggle") {
    state.themeOpen = !state.themeOpen;
    render();
    if (state.themeOpen) {
      const current = document.querySelector('.theme-option[aria-checked="true"]');
      if (current) current.focus();
    }
    return;
  }
  if (actName === "theme-set") {
    const mode = el.dataset.theme;
    state.themeOpen = false;
    applyTheme(mode);
    render();
    api("/api/settings", { method: "PUT", body: { theme: mode } }).catch((err) => {
      state.error = err.message;
      render();
    });
    return;
  }
  if (actName === "layout-set" || actName === "density-set") {
    const kind = el.dataset.kind === "inbox" ? "inbox" : "home";
    if (actName === "layout-set") state.layout[kind].view = el.dataset.value;
    else state.layout[kind].density = el.dataset.value;
    state.focusSelector = `[data-focus="${el.dataset.focus}"]`;
    commitLayout();
    return;
  }
  if (actName === "pin") {
    const kind = el.dataset.kind === "inbox" ? "inbox" : "home";
    const view = state.layout[kind];
    const pinned = new Set(view.pinned || []);
    if (pinned.has(id)) pinned.delete(id);
    else {
      pinned.add(id);
      view.order = [id].concat((view.order || []).filter((x) => x !== id));
    }
    view.pinned = [...pinned];
    commitLayout();
    return;
  }
  if (actName === "collapse") {
    const kind = el.dataset.kind === "inbox" ? "inbox" : "home";
    const view = state.layout[kind];
    const cur = new Set(view.collapsed || []);
    if (cur.has(id)) cur.delete(id);
    else cur.add(id);
    view.collapsed = [...cur];
    commitLayout();
    return;
  }
  if (actName === "size") {
    state.layout.home.sizes = state.layout.home.sizes || {};
    state.layout.home.sizes[id] = el.dataset.value;
    commitLayout();
    return;
  }
  if (actName === "select-card") {
    state.selected = Number(el.dataset.index);
    state.selectedId = id;
    if (window.matchMedia("(max-width: 800px)").matches) state.splitOpen = true;
    render();
    return;
  }
  if (actName === "split-back") {
    state.splitOpen = false;
    render();
    return;
  }
  if (actName === "approve") {
    act("approve", () => api("/api/inbox/" + id + "/approve", { method: "POST", body: {} }));
  } else if (actName === "edit" || actName === "reject" || actName === "snooze") {
    state.mode = { type: actName === "edit" ? "edit" : actName === "reject" ? "reject" : "snooze", id };
    const cards = inboxCards();
    state.selected = Math.max(0, cards.findIndex((c) => c.id === id));
    render();
  } else if (actName === "cancel-mode") {
    state.mode = null;
    render();
  } else if (actName === "snooze-preset") {
    act("snooze", () => api("/api/inbox/" + id + "/snooze", { method: "POST", body: { preset: el.dataset.preset } }));
  } else if (actName === "run") {
    act("run", () => api("/api/plugins/" + el.dataset.name + "/run", { method: "POST", body: {} }));
  } else if (actName === "remove") {
    state.mode = { type: "confirm-remove", name: el.dataset.name };
    if (window.confirm) {
      /* inline confirm would be calmer; a single local confirm keeps the path obvious */
    }
    const name = el.dataset.name;
    const row = el.closest("article");
    if (row && !row.querySelector(".confirm-remove")) {
      const bar = document.createElement("div");
      bar.className = "inline confirm-remove";
      bar.style.marginTop = "8px";
      bar.innerHTML = `<span class="sub">Remove ${esc(name)}?</span> <button class="primary" data-act="confirm-remove" data-name="${esc(name)}">Remove</button> <button class="quiet" data-act="cancel-remove">Cancel</button>`;
      row.appendChild(bar);
    }
  } else if (actName === "confirm-remove") {
    act("remove", () => api("/api/plugins/" + el.dataset.name + "/remove", { method: "POST", body: {} }));
  } else if (actName === "cancel-remove") {
    render();
  } else if (actName === "approve-manifest") {
    act("approve-manifest", () => api("/api/plugins/" + el.dataset.name + "/approve", { method: "POST", body: {} }));
  } else if (actName === "cancel-install") {
    state.preview = null;
    render();
  } else if (actName === "connect") {
    state.connecting = el.dataset.provider;
    render();
    act("connect", () => api("/api/connections/" + el.dataset.provider + "/connect", { method: "POST", body: {} }))
      .finally(() => { state.connecting = ""; render(); });
  } else if (actName === "disconnect") {
    act("disconnect", () => api("/api/connections/" + el.dataset.provider + "/disconnect", { method: "POST", body: {} }));
  } else if (actName === "save-key") {
    const input = document.querySelector("[data-composio-key]");
    const value = input ? input.value.trim() : "";
    act("save-key", async () => {
      if (!value) throw new Error("Paste the Composio API key first");
      await api("/api/connections/composio/setup", { method: "POST", body: { api_key: value } });
      if (input) input.value = "";
      state.notice = "API key saved in the keychain.";
    });
  } else if (actName === "set-mode") {
    act("mode", () => api("/api/connections/" + el.dataset.provider + "/setup", {
      method: "POST",
      body: { mode: el.dataset.mode },
    }));
  } else if (actName === "save-connection") {
    const provider = el.dataset.provider;
    const input = document.querySelector(`[data-client="${provider}"]`);
    const cal = document.querySelector("[data-calendar]");
    const body = { client_id: input ? input.value.trim() : "", calendar: !!(cal && cal.checked) };
    if (el.dataset.mode) body.mode = el.dataset.mode;
    act("setup", () => api("/api/connections/" + provider + "/setup", { method: "POST", body }));
  } else if (actName === "quit") {
    beginQuit();
  } else if (actName === "cancel-quit") {
    state.quitConfirm = null;
    render();
    focusEl("[data-act=quit]");
  } else if (actName === "confirm-quit") {
    doQuit();
  } else if (actName === "chat-new") {
    newChat();
  } else if (actName === "chat-open") {
    openChatSession(el.dataset.id);
  } else if (actName === "chat-provider") {
    const next = el.dataset.provider;
    const had = state.chat.session && state.chat.session.messages && state.chat.session.messages.length;
    state.chat.provider = next;
    api("/api/settings", { method: "PUT", body: { chat: { provider: next } } }).catch((err) => {
      state.error = err.message;
      render();
    });
    if (had && state.chat.session.provider !== next) newChat();
    else render();
  } else if (actName === "chat-stop") {
    stopChat();
  } else if (actName === "confirm-install") {
    act("install", async () => {
      await api("/api/plugins/install", { method: "POST", body: { path: state.installPath, approve: true } });
      state.preview = null;
      state.notice = "Installed.";
    });
  }
});

document.addEventListener("submit", (ev) => {
  const form = ev.target.closest("form");
  if (!form) return;
  ev.preventDefault();
  const kind = form.dataset.act;
  if (kind === "save-edit") {
    const body = {
      title: form.title.value,
      summary: form.summary.value,
      body: form.body.value,
    };
    act("edit", () => api("/api/inbox/" + form.dataset.id + "/edit", { method: "POST", body }));
  } else if (kind === "save-reject") {
    act("reject", () => api("/api/inbox/" + form.dataset.id + "/reject", { method: "POST", body: { reason: form.reason.value } }));
  } else if (kind === "chat-send") {
    const box = form.querySelector(".chat-input");
    if (box) state.chat.draft = box.value;
    sendChat();
  } else if (kind === "create-start") {
    state.create.description = form.description.value.trim();
    if (form.name) state.create.name = form.name.value.trim();
    if (form.lang) state.create.lang = form.lang.value;
    if (form.ui) state.create.ui = form.ui.checked;
    if (!state.create.type) state.create.type = suggestType(state.create.description);
    const agents = ((state.data.home && state.data.home.agents) || []).filter((a) => a.found);
    if (!state.create.agent && agents[0]) state.create.agent = agents[0].name;
    const edit = hashQuery().get("edit") || state.create.edit;
    if (edit) {
      state.create.edit = edit;
      startAgent(state.create.description, "");
      return;
    }
    act("scaffold", async () => {
      const draft = await api("/api/create/scaffold", {
        method: "POST",
        body: {
          name: state.create.name,
          description: state.create.description,
          type: state.create.type,
          lang: state.create.lang,
          ui: state.create.ui,
        },
      });
      state.create.path = draft.path;
      state.create.name = draft.name;
      state.create.type = draft.type;
      await startAgent(state.create.description, "");
    });
  } else if (kind === "preview-install") {
    state.installPath = form.path.value.trim();
    act("preview", async () => {
      state.preview = await api("/api/plugins/install", { method: "POST", body: { path: state.installPath, approve: false } });
    });
  }
});

document.addEventListener("change", (ev) => {
  const sel = ev.target.closest("select[data-act=rule]");
  if (!sel) return;
  const decision = sel.value;
  act("rule", () => api("/api/guardrails", {
    method: "POST",
    body: { plugin: sel.dataset.plugin, permission: sel.dataset.permission, decision },
  }));
});

function layoutKeys(ev) {
  const group = ev.target.closest(".layout-switch");
  if (!group) return;
  const buttons = [...group.querySelectorAll('button[role="radio"]:not(:disabled)')];
  const i = buttons.indexOf(ev.target.closest("button"));
  if (i < 0) return;
  let n = i;
  if (ev.key === "ArrowRight" || ev.key === "ArrowDown") n = (i + 1) % buttons.length;
  else if (ev.key === "ArrowLeft" || ev.key === "ArrowUp") n = (i - 1 + buttons.length) % buttons.length;
  else if (ev.key === "Home") n = 0;
  else if (ev.key === "End") n = buttons.length - 1;
  else return;
  ev.preventDefault();
  state.focusSelector = `[data-focus="${buttons[n].dataset.focus}"]`;
  buttons[n].click();
}

function reorderKeys(ev) {
  const handle = ev.target.closest(".drag-handle");
  if (!handle) return;
  if (ev.key !== "ArrowUp" && ev.key !== "ArrowDown") return;
  ev.preventDefault();
  const kind = handle.dataset.kind;
  const id = handle.dataset.id;
  const cards = kind === "inbox"
    ? visibleInbox()
    : arrangePlugins((state.data.home && state.data.home.plugins) || []);
  const ids = cards.map((item) => kind === "inbox" ? item.id : item.name);
  const index = ids.indexOf(id);
  const next = ev.key === "ArrowUp" ? index - 1 : index + 1;
  if (index < 0 || next < 0 || next >= ids.length) return;
  state.focusSelector = `[data-focus="drag-${kind}-${id}"]`;
  applyReorder(kind, id, ids[next]);
}

function applyReorder(kind, fromId, toId) {
  if (!fromId || !toId || fromId === toId || !window.DAPLayout) return;
  const prefs = state.layout[kind];
  const cards = kind === "inbox"
    ? visibleInbox()
    : arrangePlugins((state.data.home && state.data.home.plugins) || []);
  const ids = cards.map((item) => kind === "inbox" ? item.id : item.name);
  const next = window.DAPLayout.reorder(prefs, ids, fromId, toId);
  prefs.order = next.order;
  prefs.pinned = next.pinned;
  commitLayout();
}

document.addEventListener("dragstart", (ev) => {
  const handle = ev.target.closest && ev.target.closest(".drag-handle");
  if (!handle) return;
  state.dragging = true;
  ev.dataTransfer.setData("text/plain", JSON.stringify({ kind: handle.dataset.kind, id: handle.dataset.id }));
  ev.dataTransfer.effectAllowed = "move";
  const card = handle.closest("[data-card-id]");
  if (card) card.classList.add("is-dragging");
});

document.addEventListener("dragend", () => {
  state.dragging = false;
  document.querySelectorAll(".is-dragging, .drag-over").forEach((el) => el.classList.remove("is-dragging", "drag-over"));
});

document.addEventListener("dragover", (ev) => {
  if (!state.dragging) return;
  const card = ev.target.closest && ev.target.closest("[data-card-id]");
  if (!card) return;
  ev.preventDefault();
  document.querySelectorAll(".drag-over").forEach((el) => el.classList.remove("drag-over"));
  card.classList.add("drag-over");
});

document.addEventListener("drop", (ev) => {
  const card = ev.target.closest && ev.target.closest("[data-card-id]");
  if (!card || !state.dragging) return;
  ev.preventDefault();
  let payload = {};
  try { payload = JSON.parse(ev.dataTransfer.getData("text/plain") || "{}"); } catch (e) { payload = {}; }
  if (payload.kind && payload.kind === card.dataset.kind) applyReorder(payload.kind, payload.id, card.dataset.cardId);
  state.dragging = false;
});

document.addEventListener("input", (ev) => {
  if (ev.target.classList && ev.target.classList.contains("chat-input")) {
    state.chat.draft = ev.target.value;
  }
});

document.addEventListener("keydown", (ev) => {
  if ((ev.ctrlKey || ev.metaKey) && ev.shiftKey && (ev.key === "c" || ev.key === "C")) {
    if (state.quitting) return;
    ev.preventDefault();
    if (state.route !== "chat") {
      state.chat.focusComposer = true;
      location.hash = "#/chat";
    } else {
      const box = document.querySelector(".chat-input");
      if (box) box.focus();
    }
    return;
  }
  if (state.quitConfirm) {
    if (ev.key === "Escape") {
      state.quitConfirm = null;
      render();
      focusEl("[data-act=quit]");
    }
    return;
  }
  const inTheme = ev.target.closest && ev.target.closest(".theme");
  if (!state.themeOpen && inTheme && (ev.key === "ArrowDown" || ev.key === "ArrowUp")) {
    state.themeOpen = true;
    render();
    const current = document.querySelector('.theme-option[aria-checked="true"]');
    if (current) current.focus();
    ev.preventDefault();
    return;
  }
  if (state.themeOpen) {
    const items = [...document.querySelectorAll(".theme-option")];
    const index = items.findIndex((el) => el === document.activeElement);
    if (ev.key === "Escape") {
      state.themeOpen = false;
      render();
      const btn = document.querySelector(".theme-btn");
      if (btn) btn.focus();
      ev.preventDefault();
      return;
    }
    if (ev.key === "Tab") {
      state.themeOpen = false;
      const menu = document.getElementById("theme-menu");
      if (menu) menu.remove();
      const btn = document.querySelector(".theme-btn");
      if (btn) btn.setAttribute("aria-expanded", "false");
      return;
    }
    if (items.length && (ev.key === "ArrowDown" || ev.key === "ArrowUp" || ev.key === "Home" || ev.key === "End")) {
      let next = index;
      if (ev.key === "ArrowDown") next = index < 0 ? 0 : (index + 1) % items.length;
      if (ev.key === "ArrowUp") next = index < 0 ? items.length - 1 : (index - 1 + items.length) % items.length;
      if (ev.key === "Home") next = 0;
      if (ev.key === "End") next = items.length - 1;
      items[next].focus();
      ev.preventDefault();
    }
    if (ev.key === "Enter" || ev.key === " ") {
      const current = document.activeElement;
      if (current && current.dataset && current.dataset.theme) current.click();
      ev.preventDefault();
    }
    return;
  }
  if (inTheme) return;
  if (ev.target.closest && ev.target.closest(".layout-switch")) {
    layoutKeys(ev);
    return;
  }
  if (ev.target.closest && ev.target.closest(".drag-handle")) {
    reorderKeys(ev);
    return;
  }
  if (state.route === "chat") {
    if (ev.key === "Escape" && state.chat.streaming) {
      ev.preventDefault();
      stopChat();
    }
    if (ev.target.classList && ev.target.classList.contains("chat-input") && ev.key === "Enter" && !ev.shiftKey) {
      ev.preventDefault();
      state.chat.draft = ev.target.value;
      sendChat();
    }
    return;
  }
  if (state.route !== "inbox") return;
  const tag = (ev.target.tagName || "").toLowerCase();
  if (tag === "input" || tag === "textarea" || tag === "select" || tag === "button" || tag === "a") return;
  const cards = visibleInbox();
  if (!cards.length) return;
  const key = ev.key.toLowerCase();
  if (key === "j" || ev.key === "ArrowDown") {
    state.selected = Math.min(cards.length - 1, state.selected + 1);
    state.selectedId = cards[state.selected].id;
    state.forceScroll = true;
    render();
    ev.preventDefault();
  } else if (key === "k" || ev.key === "ArrowUp") {
    state.selected = Math.max(0, state.selected - 1);
    state.selectedId = cards[state.selected].id;
    state.forceScroll = true;
    render();
    ev.preventDefault();
  } else if (key === "a") {
    const c = cards[state.selected];
    if (c) act("approve", () => api("/api/inbox/" + c.id + "/approve", { method: "POST", body: {} }));
  } else if (key === "e") {
    const c = cards[state.selected];
    if (c) { state.mode = { type: "edit", id: c.id }; render(); }
  } else if (key === "r") {
    const c = cards[state.selected];
    if (c) { state.mode = { type: "reject", id: c.id }; render(); }
  } else if (key === "s") {
    const c = cards[state.selected];
    if (c) { state.mode = { type: "snooze", id: c.id }; render(); }
  }
});

async function startAgent(description, fix) {
  state.create.phase = "run";
  state.create.status = "running";
  state.create.log = "";
  state.create.report = null;
  render();
  const body = {
    path: state.create.path,
    plugin: state.create.edit || "",
    agent: state.create.agent,
    description,
    fix,
  };
  const run = await api("/api/create/run", { method: "POST", body });
  state.create.runId = run.id;
  state.create.log = run.log || "";
  render();
}

async function pollCreate() {
  if (state.route !== "create" || state.create.status !== "running" || !state.create.runId) return;
  try {
    const run = await api("/api/create/run/" + state.create.runId);
    state.create.log = run.log || state.create.log;
    state.create.status = run.status;
    if (run.error) state.create.log += "\n" + run.error;
    if (run.status === "running") {
      render();
      return;
    }
    state.create.status = "checking";
    const rep = await api("/api/create/validate", { method: "POST", body: { path: state.create.path, plugin: state.create.edit || "" } });
    state.create.report = rep;
    state.create.status = rep.ok ? "passed" : "needs a fix";
    if (rep.ok) {
      await api("/api/create/propose", { method: "POST", body: { path: state.create.path, plugin: state.create.edit || "" } });
    }
    state.create.phase = "done";
    render();
  } catch (err) {
    state.error = err.message;
    state.create.status = "failed";
    render();
  }
}

window.addEventListener("hashchange", () => {
  if (state.quitting) return;
  state.route = routeFromHash();
  if (state.route === "chat") state.chat.focusComposer = true;
  state.mode = null;
  state.quitConfirm = null;
  state.splitOpen = false;
  state.scrollReset = true;
  applyLayoutAttrs();
  render();
  refresh();
});

async function syncLayout() {
  let remote = null;
  try {
    const res = await api("/api/settings");
    remote = window.DAPLayout ? window.DAPLayout.normalize(res.layout) : res.layout;
  } catch (e) {
    return;
  }
  let hadLocal = false;
  try { hadLocal = !!localStorage.getItem("dap.layout"); } catch (e) { hadLocal = true; }
  if (!hadLocal && remote) {
    const next = JSON.stringify(remote);
    const cur = JSON.stringify(state.layout);
    state.layout = remote;
    try { localStorage.setItem("dap.layout", next); } catch (e) { /* private mode */ }
    applyLayoutAttrs();
    if (next !== cur) render();
    return;
  }
  if (remote && JSON.stringify(state.layout) !== JSON.stringify(remote)) {
    api("/api/settings", { method: "PUT", body: { layout: state.layout } }).catch(() => {});
  }
}

async function syncTheme() {
  try {
    const saved = await api("/api/settings");
    let local = "";
    try { local = localStorage.getItem("dap.theme") || ""; } catch (e) { /* ignore */ }
    if (!["system", "light", "dark"].includes(local)) local = "";
    if (!local && saved.theme) {
      applyTheme(saved.theme);
      render();
      return;
    }
    if (local && saved.theme && local !== saved.theme) {
      applyTheme(local);
      await api("/api/settings", { method: "PUT", body: { theme: local } });
    }
  } catch (e) { /* the page still follows localStorage */ }
}

applyLayoutAttrs();
refresh();
setInterval(() => { pollCreate(); }, 800);
syncLayout();
syncTheme();
setInterval(() => {
  if (document.hidden || state.dragging || state.themeOpen || state.chat.streaming) return;
  if (state.quitting || state.quitConfirm || state.quitPending) return;
  if (state.mode && (state.mode.type === "edit" || state.mode.type === "reject" || state.mode.type === "snooze")) return;
  const ae = document.activeElement;
  const tag = (ae && ae.tagName || "").toLowerCase();
  if (tag === "input" || tag === "textarea" || tag === "select") return;
  if (ae && ae.closest && ae.closest(".layout-switch, .drag-handle, .size-switch")) return;
  refresh();
}, 2000);
