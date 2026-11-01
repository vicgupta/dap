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
  quitConfirm: null,
  quitPending: false,
  quitting: false,
};

const ICONS = {
  home: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M2.5 7.2 8 2.8l5.5 4.4V13a1 1 0 0 1-1 1h-3.2V9.6H6.7V14H3.5a1 1 0 0 1-1-1V7.2Z"/></svg>',
  inbox: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M2.5 8.5 4 3.5h8l1.5 5M2.5 8.5h3l.8 1.6h3.4l.8-1.6h3V12a1 1 0 0 1-1 1h-9a1 1 0 0 1-1-1V8.5Z"/></svg>',
  activity: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M2 8h2.2l1.4-3.2L8 12l1.6-4H14"/></svg>',
  guard: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M8 2.2 13 4.2v3.6c0 3-2.1 4.8-5 6-2.9-1.2-5-3-5-6V4.2L8 2.2Z"/></svg>',
  plugins: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M6 2.5h4V5H6zM3 6.5h4V9H3zM9 6.5h4V9H9zM6 10h4v2.5H6z"/><path d="M8 5v1.5M5 9v1.5h6V9"/></svg>',
  connections: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="4" cy="8" r="1.6"/><circle cx="12" cy="5" r="1.6"/><circle cx="12" cy="11" r="1.6"/><path d="M5.5 7.4 10.4 5.6M5.5 8.6l4.9 2"/></svg>',
  power: '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="M8 2.2v5.2"/><path d="M5.05 3.7a5 5 0 1 0 5.9 0"/></svg>',
};

function routeFromHash() {
  const h = (location.hash || "#/").replace(/^#\/?/, "");
  if (["inbox", "activity", "guardrails", "plugins", "connections"].includes(h)) return h;
  return "home";
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

function shell(inner) {
  const pending = state.data.home ? state.data.home.pending : 0;
  const item = (route, label, icon) => {
    const active = state.route === route ? "active" : "";
    const badge = route === "inbox" && pending ? `<span class="badge">${pending}</span>` : "";
    const href = route === "home" ? "#/" : "#/" + route;
    return `<a class="${active}" href="${href}" ${active ? 'aria-current="page"' : ""}>${ICONS[icon]}<span>${label}</span>${badge}</a>`;
  };
  return `
    <div class="app">
      <aside class="sidebar">
        <div class="brand"><strong>DAP</strong><span>Dashboard Agent Platform</span></div>
        <nav class="nav">
          ${item("home", "Home", "home")}
          ${item("inbox", "Decision inbox", "inbox")}
          ${item("activity", "Activity", "activity")}
          ${item("guardrails", "Guardrails", "guard")}
          ${item("plugins", "Plugins", "plugins")}
          ${item("connections", "Connections", "connections")}
        </nav>
        ${quitControl()}
      </aside>
      <main class="main">
        ${state.error ? `<div class="banner">${esc(state.error)}</div>` : ""}
        ${state.notice ? `<div class="banner ok">${esc(state.notice)}</div>` : ""}
        ${inner}
      </main>
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
        ${p.ui_url ? `<a class="ghost" style="text-decoration:none;padding:7px 12px;border-radius:10px;border:1px solid var(--line-strong)" href="${esc(p.ui_url)}" target="_blank" rel="noreferrer">Open ${esc(p.ui_title || "web UI")}</a>` : ""}
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
        ${p.ui_url ? `<a class="ghost" style="text-decoration:none;padding:7px 12px;border-radius:10px;border:1px solid var(--line-strong)" href="${esc(p.ui_url)}" target="_blank" rel="noreferrer">Open ${esc(p.ui_title || "web UI")}</a>` : ""}
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

function connectionsView() {
  const data = state.data.connections;
  if (!data) return shell(`<p class="sub">Loading…</p>`);
  const cards = (data.connections || []).map((c) => {
    const dot = c.connected ? "ok" : "degraded";
    const scopes = (c.scopes || []).join(", ") || "—";
    const throttle = c.throttle && c.throttle.active ? `Throttled until ${esc(fmtTime(c.throttle.until))}.` : "";
    const client = c.provider === "fake" ? "" : `
      <label class="field">Client ID
        <input data-client="${esc(c.provider)}" type="text" value="${esc(c.client_id || "")}" placeholder="Public client id" />
      </label>`;
    const calendar = c.provider === "graph" ? `
      <label class="sub"><input data-calendar type="checkbox" ${c.calendar_enabled ? "checked" : ""} /> Calendars.Read (off unless you need it)</label>` : "";
    return `<article class="card plugin-block">
      <header class="row">
        <span class="dot ${dot}"></span>
        <div style="flex:1">
          <h2 style="margin:0">${esc(c.label)}</h2>
          <p class="sub">${c.connected ? "Connected" : "Not connected"}${c.account ? " · " + esc(c.account) : ""} · ${esc(c.broker || "")}</p>
      </div>
        ${c.installed ? `<span class="pill allowed">Installed</span>` : `<span class="pill">Not installed</span>`}
      </header>
      ${c.refresh_detail && c.refresh_detail !== "not connected" ? `<p class="sub">${esc(c.refresh_detail)}</p>` : ""}
      <p class="sub">Scopes: ${esc(scopes)}</p>
      ${throttle ? `<p class="missed">${throttle}</p>` : ""}
      ${client}
      ${calendar}
      <div class="actions" style="margin-top:12px">
        ${c.provider !== "fake" ? `<button class="ghost" data-act="save-connection" data-provider="${esc(c.provider)}">Save client id</button>` : ""}
        ${c.connected
          ? `<button class="ghost" data-act="disconnect" data-provider="${esc(c.provider)}">Disconnect</button>`
          : `<button class="primary" data-act="connect" data-provider="${esc(c.provider)}">Connect</button>`}
      </div>
    </article>`;
  }).join("");
  const agents = (data.agents || []).map((a) => `<li>${esc(a.name)} · ${a.found ? esc(a.path) : "not on PATH"}</li>`).join("");
  return shell(`
    <header class="page-head"><div>
      <h1>Connections</h1>
      <p>Mail tokens stay in the OS keychain. Plugins never see them. You sign in inside Google or Microsoft’s window.</p>
    </div></header>
    <div class="stack">${cards}</div>
    <h2 class="section-title">Coding agents</h2>
    <ul class="card plugin-block">${agents}</ul>
  `);
}

function render() {
  if (state.quitting) return;
  const root = document.getElementById("app");
  if (!root) return;
  const html = {
    home: homeView,
    inbox: inboxView,
    activity: activityView,
    guardrails: guardView,
    plugins: pluginsView,
    connections: connectionsView,
  }[state.route]();
  root.innerHTML = html;
  const selected = root.querySelector(".decision.selected");
  if (selected) selected.scrollIntoView({ block: "nearest" });
}

async function refresh() {
  if (state.quitting || state.quitConfirm || state.quitPending) return;
  try {
    if (state.route === "home" || !state.data.home) {
      state.data.home = await api("/api/home");
    }
    if (state.route === "inbox") state.data.inbox = await api("/api/inbox");
    if (state.route === "activity") state.data.activity = await api("/api/activity");
    if (state.route === "guardrails") state.data.guard = await api("/api/guardrails");
    if (state.route === "plugins") state.data.plugins = await api("/api/plugins");
    if (state.route === "connections") state.data.connections = await api("/api/connections");
    if (state.route !== "home") {
      state.data.home = await api("/api/home");
    }
    state.error = "";
  } catch (err) {
    state.error = err.message;
  }
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
    act("connect", () => api("/api/connections/" + el.dataset.provider + "/connect", { method: "POST", body: {} }));
  } else if (actName === "disconnect") {
    act("disconnect", () => api("/api/connections/" + el.dataset.provider + "/disconnect", { method: "POST", body: {} }));
  } else if (actName === "save-connection") {
    const provider = el.dataset.provider;
    const input = document.querySelector(`[data-client="${provider}"]`);
    const cal = document.querySelector("[data-calendar]");
    act("setup", () => api("/api/connections/" + provider + "/setup", {
      method: "POST",
      body: { client_id: input ? input.value.trim() : "", calendar: !!(cal && cal.checked && provider === "graph") },
    }));
  } else if (actName === "quit") {
    beginQuit();
  } else if (actName === "cancel-quit") {
    state.quitConfirm = null;
    render();
    focusEl("[data-act=quit]");
  } else if (actName === "confirm-quit") {
    doQuit();
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
  if (state.quitConfirm) {
    if (ev.key === "Escape") {
      state.quitConfirm = null;
      render();
      focusEl("[data-act=quit]");
    }
    return;
  }
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
  if (state.quitting) return;
  state.route = routeFromHash();
  state.mode = null;
  state.quitConfirm = null;
  refresh();
});

refresh();
setInterval(() => {
  if (document.hidden) return;
  if (state.quitting || state.quitConfirm || state.quitPending) return;
  if (state.mode && (state.mode.type === "edit" || state.mode.type === "reject" || state.mode.type === "snooze")) return;
  const tag = (document.activeElement && document.activeElement.tagName || "").toLowerCase();
  if (tag === "input" || tag === "textarea" || tag === "select") return;
  refresh();
}, 2000);
