const DAP = readBoot();

function readBoot() {
  const meta = (name) => {
    const el = document.querySelector(`meta[name="${name}"]`);
    return el ? el.content : "";
  };
  return {
    token: meta("dap-token"),
    api: meta("dap-api") || "/api",
    inbox: meta("dap-inbox") || "/",
  };
}

const core = (() => {
  try { return new URL(DAP.inbox).origin; } catch (e) { return ""; }
})();

const state = { cards: [], threads: [], context: {}, selected: 0, mode: null, filter: "", error: "" };

function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

async function api(path, opts = {}) {
  const res = await fetch(DAP.api + path, {
    method: opts.method || "GET",
    headers: { Authorization: "Bearer " + DAP.token, "Content-Type": "application/json" },
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok || data.ok === false) throw new Error(data.error || data.reason || "Request failed");
  return data;
}

function payload(card) {
  let p = card.payload || {};
  if (typeof p === "string") { try { p = JSON.parse(p); } catch (e) { p = {}; } }
  return p;
}

function age(iso) {
  if (!iso) return "";
  const h = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 36e5));
  if (h >= 24) return Math.max(1, Math.round(h / 24)) + "d";
  return h + "h";
}

function queue() {
  const q = state.filter.trim().toLowerCase();
  return state.cards.filter((c) => {
    if (c.kind === "change") return false;
    if (c.status !== "pending" && c.status !== "snoozed") return false;
    if (!q) return true;
    return (c.title + " " + c.summary + " " + c.body).toLowerCase().includes(q);
  });
}

function doneCards() {
  return state.cards.filter((c) => c.kind === "action" && c.status === "done").slice(-4).reverse();
}

function changeCard() {
  return state.cards.find((c) => c.kind === "change" && (c.status === "pending" || c.status === "snoozed"));
}

async function load() {
  const [ctx, inbox, status] = await Promise.all([
    api("/context"),
    api("/inbox"),
    api("/mail/status"),
  ]);
  state.context = ctx;
  state.cards = inbox.cards || [];
  const connected = (status.connections || []).filter((c) => c.connected).map((c) => c.provider);
  const lists = await Promise.all(connected.map((p) => api("/mail/threads?provider=" + encodeURIComponent(p)).catch(() => ({ threads: [] }))));
  state.threads = lists.flatMap((l) => l.threads || []);
}

function render() {
  const items = queue();
  if (state.selected >= items.length) state.selected = Math.max(0, items.length - 1);
  const ctx = state.context || {};
  const missed = ctx.missed ? `<span class="missed">${esc(ctx.missed)}</span>` : "";
  const cards = items.map((c, i) => {
    const p = payload(c);
    const batch = Array.isArray(p.batch);
    const who = p.to ? `To ${p.to}${p.subject ? " · Re: " + p.subject : ""}` : "";
    const ev = (c.evidence || []).map((e) => e.ref || e.label).filter(Boolean).join(" · ");
    let extra = "";
    if (state.mode && state.mode.id === c.id && state.mode.type === "edit") {
      extra = `<form data-act="save-edit" data-id="${esc(c.id)}">
        <input name="title" value="${esc(c.title)}" />
        <textarea name="body">${esc(c.body || "")}</textarea>
        <div class="actions"><button class="primary" type="submit">Save edit</button><button class="ghost" type="button" data-act="cancel">Cancel</button></div>
      </form>`;
    } else if (state.mode && state.mode.id === c.id && state.mode.type === "reject") {
      extra = `<form data-act="save-reject" data-id="${esc(c.id)}">
        <input name="reason" placeholder="Why not?" required />
        <div class="actions"><button class="primary" type="submit">Dismiss</button><button class="ghost" type="button" data-act="cancel">Cancel</button></div>
      </form>`;
    }
    return `<article class="card ${i === state.selected ? "selected" : ""}" data-index="${i}">
      <div class="row"><h2>${esc(c.title)}</h2><span class="pill">Sends as you · Ask first</span></div>
      <p class="sub">${esc(c.summary || "")}</p>
      ${who ? `<p class="meta">${esc(who)}</p>` : ""}
      ${c.body && !extra ? `<div class="draft">${esc(c.body)}</div>` : ""}
      ${extra}
      <div class="actions">
        <button class="primary" data-act="approve" data-id="${esc(c.id)}">${batch ? "Approve all" : "Approve"}<kbd>A</kbd></button>
        <button class="ghost" data-act="edit" data-id="${esc(c.id)}">Edit<kbd>E</kbd></button>
        <button class="ghost" data-act="reject" data-id="${esc(c.id)}">Reject<kbd>R</kbd></button>
        <button class="ghost" data-act="snooze" data-id="${esc(c.id)}">Snooze<kbd>S</kbd></button>
        <span class="evidence">${ev ? "Evidence: " + esc(ev) : ""}</span>
      </div>
    </article>`;
  }).join("");

  const owe = state.threads.filter((t) => !t.last_from_me).length;
  const promises = state.threads.filter((t) => t.promise).length;
  const old = state.threads.filter((t) => t.last_from_me && age(t.last_at).endsWith("d") && parseInt(age(t.last_at), 10) >= 3).length;
  const people = state.threads.filter((t) => t.title || t.subject).slice(0, 6).map((t) =>
    `<div class="person"><span>${esc(t.with_name)} · ${esc(t.subject)}</span><span class="age">${esc(age(t.last_at))}</span></div>`).join("");
  const rule = changeCard();
  const sent = doneCards().map((c) => {
    const p = payload(c);
    const id = ((c.result || {}).evidence || [])[0];
    const ref = id && id.ref;
    const undo = p.provider === "fake" && ref ? `<button class="ghost" data-act="undo" data-provider="fake" data-message="${esc(ref)}">Undo</button>` : "";
    return `<div class="person"><span>${esc((c.result && c.result.summary) || c.title)}</span>${undo}</div>`;
  }).join("");
  const bars = [8, 14, 10, 18, 12, 20, 16].map((n) => `<i style="height:${n}px"></i>`).join("");
  const closed = state.cards.filter((c) => c.status === "done" && c.kind === "action").length;

  document.getElementById("app").innerHTML = `
    <div class="app">
      <aside class="sidebar">
        <div class="brand"><strong>DAP</strong><span>Dashboard Agent Platform</span></div>
        <nav class="nav">
          <a href="${core}/#/">Home</a>
          <a href="${core}/#/inbox">Decision inbox</a>
          <a href="${core}/#/activity">Activity</a>
          <a href="${core}/#/guardrails">Guardrails</a>
          <a href="${core}/#/connections">Connections</a>
          <a href="${core}/#/plugins">Plugins</a>
        </nav>
        <div class="group">MY DAS</div>
        <div class="da current">Inbox follow-ups <span class="pill">${items.length}</span></div>
        <div class="side-foot"><strong>Running locally</strong>All data stays on this machine.</div>
      </aside>
      <main class="main">
        <div class="top">
          <div>
            <h1>Inbox follow-ups & commitments</h1>
            <p class="sub">Nothing sends without your approval. ${ctx.last_run ? "Last run " + esc(new Date(ctx.last_run).toLocaleString()) + " · " : ""}${ctx.next_run ? "Next " + esc(new Date(ctx.next_run).toLocaleString()) : ""} ${missed}</p>
          </div>
          <input class="filter" placeholder="Ask this DA…" value="${esc(state.filter)}" />
        </div>
        ${state.error ? `<p class="missed">${esc(state.error)}</p>` : ""}
        <div class="layout">
          <section>
            <p class="kicker">NEEDS YOU · ${items.length} <span class="sub">j/k move · a approve · e edit · r reject · s snooze</span></p>
            ${cards || `<div class="card empty">Nothing waiting. Connect a mailbox from Connections, then run this DA.</div>`}
            <p class="sub">Nothing sends without your approval.</p>
          </section>
          <aside class="rail">
            <article class="card">
              <h3>WATCH</h3>
              <div class="stats">
                <div><strong>${owe}</strong><span>awaiting your reply</span></div>
                <div><strong>${promises}</strong><span>promises due</span></div>
                <div><strong>${old}</strong><span>waiting on others &gt;3d</span></div>
              </div>
              ${people}
            </article>
            ${rule ? `<article class="card suggest"><h3>SUGGESTED CHANGE</h3><p>${esc(rule.summary || rule.title)}</p><div class="actions"><button class="primary" data-act="approve" data-id="${esc(rule.id)}">Approve change</button><button class="ghost" data-act="reject" data-id="${esc(rule.id)}">Dismiss</button></div></article>` : ""}
            <article class="card">
              <h3>PROVE · THIS WEEK</h3>
              <div class="stats">
                <div><strong>${closed}</strong><span>follow-ups closed</span></div>
                <div><strong>${Math.max(0, closed - 1)}</strong><span>threads touched</span></div>
                <div><strong>~${closed * 6}m</strong><span>time saved</span></div>
              </div>
              <div class="bars">${bars}</div>
              <p class="sub">Cost is not measured. DAP does not call a model.</p>
            </article>
            <article class="card"><h3>DONE</h3>${sent || `<p class="sub">Nothing sent yet.</p>`}</article>
          </aside>
        </div>
      </main>
    </div>`;
}

async function refresh() {
  try { await load(); state.error = ""; } catch (err) { state.error = err.message; }
  if (state.mode) return;
  render();
}

document.addEventListener("click", async (ev) => {
  const el = ev.target.closest("[data-act]");
  if (!el) {
    const card = ev.target.closest(".card[data-index]");
    if (card) { state.selected = Number(card.dataset.index); render(); }
    return;
  }
  const id = el.dataset.id;
  const act = el.dataset.act;
  try {
    if (act === "approve") {
      state.mode = null;
      await api("/inbox/" + id + "/approve", { method: "POST", body: {} });
    } else if (act === "edit") {
      state.mode = { type: "edit", id };
      render();
      return;
    } else if (act === "reject") {
      state.mode = { type: "reject", id };
      render();
      return;
    } else if (act === "snooze") {
      await api("/inbox/" + id + "/snooze", { method: "POST", body: { preset: "1h" } });
    } else if (act === "cancel") {
      state.mode = null;
    } else if (act === "undo") {
      await api("/mail/undo", { method: "POST", body: { provider: el.dataset.provider, message_id: el.dataset.message } });
    }
    state.error = "";
  } catch (err) {
    state.error = err.message;
  }
  await refresh();
  render();
});

document.addEventListener("submit", async (ev) => {
  const form = ev.target.closest("form");
  if (!form) return;
  ev.preventDefault();
  try {
    if (form.dataset.act === "save-edit") {
      await api("/inbox/" + form.dataset.id + "/edit", { method: "POST", body: { title: form.title.value, body: form.body.value } });
    } else if (form.dataset.act === "save-reject") {
      await api("/inbox/" + form.dataset.id + "/reject", { method: "POST", body: { reason: form.reason.value } });
    }
    state.mode = null;
    state.error = "";
  } catch (err) { state.error = err.message; }
  await refresh();
  render();
});

document.addEventListener("input", (ev) => {
  if (ev.target.classList.contains("filter")) {
    state.filter = ev.target.value;
    render();
    const again = document.querySelector(".filter");
    if (again) { again.focus(); again.setSelectionRange(again.value.length, again.value.length); }
  }
});

document.addEventListener("keydown", async (ev) => {
  const tag = (ev.target.tagName || "").toLowerCase();
  if (tag === "input" || tag === "textarea") return;
  const items = queue();
  if (!items.length) return;
  const key = ev.key.toLowerCase();
  if (key === "j" || ev.key === "ArrowDown") { state.selected = Math.min(items.length - 1, state.selected + 1); render(); ev.preventDefault(); }
  else if (key === "k" || ev.key === "ArrowUp") { state.selected = Math.max(0, state.selected - 1); render(); ev.preventDefault(); }
  else if ("aers".includes(key)) {
    const card = items[state.selected];
    if (!card) return;
    ev.preventDefault();
    if (key === "e") { state.mode = { type: "edit", id: card.id }; render(); return; }
    if (key === "r") { state.mode = { type: "reject", id: card.id }; render(); return; }
    try {
      if (key === "a") await api("/inbox/" + card.id + "/approve", { method: "POST", body: {} });
      if (key === "s") await api("/inbox/" + card.id + "/snooze", { method: "POST", body: { preset: "1h" } });
      state.error = "";
    } catch (err) { state.error = err.message; }
    await refresh();
    render();
  }
});

refresh();
setInterval(() => { if (!state.mode && document.activeElement?.tagName !== "INPUT") refresh(); }, 4000);
