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
  error: "",
  notice: "",
  mode: null,
  installPath: "",
  preview: null,
  data: {},
  theme: document.documentElement.getAttribute("data-theme") || "system",
  themeOpen: false,
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
  sun: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="8" cy="8" r="2.3" fill="currentColor" stroke="none"/><path d="M8 1.5v1.8M8 12.7v1.8M1.5 8h1.8M12.7 8h1.8M3.3 3.3l1.3 1.3M11.4 11.4l1.3 1.3M12.7 3.3l-1.3 1.3M4.6 11.4l-1.3 1.3"/></svg>',
  moon: '<svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true"><path fill="currentColor" d="M9.4 1.7a5.5 5.5 0 1 0 4.6 8.9A4.6 4.6 0 0 1 9.4 1.7Z"/></svg>',
  monitor: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="2" y="2.4" width="12" height="8.2" rx="1.3"/><path d="M6 13.6h4M8 10.6v3"/></svg>',
  check: '<svg class="check" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M3.2 8.2 6.4 11.4 12.8 4.6"/></svg>',
};

function routeFromHash() {
  const h = (location.hash || "#/").replace(/^#\/?/, "");
  if (["inbox", "activity", "guardrails", "plugins"].includes(h)) return h;
  return "home";
}

function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}

async function api(path, opts = {}) {
  const res = await fetch(path, {
    method: opts.method || "GET",
    headers: {
      Authorization: "Bearer " + DAP.token,
      "Content-Type": "application/json",
    },
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

function withTheme(url) {
  if (!url) return "";
  try {
    const u = new URL(url, location.origin);
    u.searchParams.set("theme", state.theme || "system");
    return u.toString();
  } catch (e) {
    return url;
  }
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

function shell(inner) {
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
            ${item("inbox", "Decision inbox", "inbox")}
            ${item("activity", "Activity", "activity")}
            ${item("guardrails", "Guardrails", "guard")}
            ${item("plugins", "Plugins", "plugins")}
          </nav>
          <div class="side-foot"><strong>Running locally</strong>All data stays on this machine.</div>
        </aside>
        <main class="main">
          ${state.error ? `<div class="banner">${esc(state.error)}</div>` : ""}
          ${state.notice ? `<div class="banner ok">${esc(state.notice)}</div>` : ""}
          ${inner}
        </main>
      </div>
    </div>`;
}

function homeView() {
  const home = state.data.home;
  if (!home) return shell(`<p class="sub">Loading…</p>`);
  const cards = (home.plugins || []).map((p) => {
    const health = p.health || {};
    const dot = health.status || "degraded";
    const missed = p.missed && p.missed.text ? `<div class="missed">${esc(p.missed.text)}</div>` : "";
    const lines = (p.tile && p.tile.lines || []).map((l) =>
      `<div class="tile-line"><span>${esc(l.label)}</span><strong>${esc(l.value)}</strong></div>`).join("");
    const tile = p.tile && (p.tile.title || lines)
      ? `<div class="tile"><strong>${esc(p.tile.title || p.name)}</strong>${lines}</div>` : "";
    return `<article class="card plugin-card">
      <header>
        <span class="dot ${esc(dot)}"></span>
        <div>
          <h2>${esc(p.name)}</h2>
          <p class="sub">${esc(p.type)} · ${esc(p.schedule || "manual")}</p>
        </div>
        ${p.pending ? `<span class="count-pill">${p.pending} pending</span>` : ""}
      </header>
      <dl class="meta">
        <div><dt>Last run</dt><dd>${esc(fmtTime(p.last_run))}</dd></div>
        <div><dt>Next run</dt><dd>${esc(fmtTime(p.next_run))}</dd></div>
      </dl>
      ${missed}
      ${tile}
      <p class="sub">${esc(health.message || "")}</p>
      <div class="actions">
        <button class="primary" data-act="run" data-name="${esc(p.name)}">Run now</button>
        ${p.ui_url ? `<a class="ghost" href="${esc(withTheme(p.ui_url))}" target="_blank" rel="noreferrer">Open ${esc(p.ui_title || "web UI")}</a>` : ""}
      </div>
    </article>`;
  }).join("");
  const agents = (home.agents || []).map((a) => a.found ? a.name : null).filter(Boolean);
  const agentLine = agents.length
    ? `Coding agents on this machine: ${agents.join(", ")}.`
    : "No coding agent (Claude Code, Codex, OpenCode) was found on PATH. DAP still runs; the agent is optional.";
  return shell(`
    <header class="page-head">
      <div>
        <h1>Home</h1>
        <p>Every plugin reports here. Nothing sends, spends, or deletes without an approval.</p>
      </div>
    </header>
    ${cards ? `<div class="grid">${cards}</div>` : `<div class="card empty"><h2>No plugins yet</h2><p>Install hello-watch and workflow from the Plugins page.</p></div>`}
    <p class="sub" style="margin-top:18px">${esc(agentLine)}</p>
  `);
}

function inboxCards() {
  return (state.data.inbox && state.data.inbox.cards) || [];
}

function inboxView() {
  const cards = inboxCards();
  if (state.selected >= cards.length) state.selected = Math.max(0, cards.length - 1);
  const list = cards.map((c, i) => {
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
    return `<article class="card decision ${selected ? "selected" : ""}" data-index="${i}">
      <header class="row">
        <div style="flex:1">
          <h2>${esc(c.title)}</h2>
          <div class="who">${esc(c.plugin)} · ${esc(c.kind)}</div>
        </div>
        <div class="inline">${kind}${effects}${risk}${snoozed}</div>
      </header>
      ${c.summary ? `<p class="summary">${esc(c.summary)}</p>` : ""}
      ${c.body ? `<div class="body">${esc(c.body)}</div>` : ""}
      ${extra}
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
  }).join("");
  const waiting = cards.filter((c) => c.status === "pending").length;
  return shell(`
    <header class="page-head">
      <div>
        <h1>Decision inbox</h1>
        <p>Needs you · ${waiting}. <span class="keys">j/k move · a approve · e edit · r reject · s snooze</span></p>
      </div>
    </header>
    ${list ? `<div class="stack">${list}</div>` : `<div class="card empty"><h2>Nothing waiting</h2><p>Plugins keep watching. Pending cards never block them.</p></div>`}
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
        ${p.ui_url ? `<a class="ghost" href="${esc(withTheme(p.ui_url))}" target="_blank" rel="noreferrer">Open ${esc(p.ui_title || "web UI")}</a>` : ""}
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

function render() {
  const root = document.getElementById("app");
  const html = {
    home: homeView,
    inbox: inboxView,
    activity: activityView,
    guardrails: guardView,
    plugins: pluginsView,
  }[state.route]();
  root.innerHTML = html;
  const selected = root.querySelector(".decision.selected");
  if (selected) selected.scrollIntoView({ block: "nearest" });
}

async function refresh() {
  try {
    if (state.route === "home" || !state.data.home) {
      state.data.home = await api("/api/home");
    }
    if (state.route === "inbox") state.data.inbox = await api("/api/inbox");
    if (state.route === "activity") state.data.activity = await api("/api/activity");
    if (state.route === "guardrails") state.data.guard = await api("/api/guardrails");
    if (state.route === "plugins") state.data.plugins = await api("/api/plugins");
    if (state.route !== "home") {
      state.data.home = await api("/api/home");
    }
    state.error = "";
  } catch (err) {
    state.error = err.message;
  }
  if (state.themeOpen) return;
  if (state.mode && (state.mode.type === "edit" || state.mode.type === "reject")) return;
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
    if (card) {
      state.selected = Number(card.dataset.index);
      render();
    }
    return;
  }
  const actName = el.dataset.act;
  const id = el.dataset.id;
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

document.addEventListener("keydown", (ev) => {
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
    return;
  }
  if (inTheme) return;
  if (state.route !== "inbox") return;
  const tag = (ev.target.tagName || "").toLowerCase();
  if (tag === "input" || tag === "textarea" || tag === "select") return;
  const cards = inboxCards();
  if (!cards.length) return;
  const key = ev.key.toLowerCase();
  if (key === "j" || ev.key === "ArrowDown") {
    state.selected = Math.min(cards.length - 1, state.selected + 1);
    render();
    ev.preventDefault();
  } else if (key === "k" || ev.key === "ArrowUp") {
    state.selected = Math.max(0, state.selected - 1);
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

window.addEventListener("hashchange", () => {
  state.route = routeFromHash();
  state.mode = null;
  refresh();
});

refresh();
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

syncTheme();

setInterval(() => {
  if (document.hidden) return;
  if (state.themeOpen) return;
  if (state.mode && (state.mode.type === "edit" || state.mode.type === "reject" || state.mode.type === "snooze")) return;
  const tag = (document.activeElement && document.activeElement.tagName || "").toLowerCase();
  if (tag === "input" || tag === "textarea" || tag === "select") return;
  refresh();
}, 2000);
