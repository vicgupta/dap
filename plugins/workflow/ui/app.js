const DAP = readBoot();

function readBoot() {
  if (window.__DAP__ && window.__DAP__.token) return window.__DAP__;
  const meta = (name) => {
    const el = document.querySelector(`meta[name="${name}"]`);
    return el ? el.content : "";
  };
  return {
    token: meta("dap-token"),
    api: meta("dap-api") || "/api",
    inbox: meta("dap-inbox") || "/",
    plugin: meta("dap-plugin"),
  };
}

const state = { steps: [], every: "1h", dirty: false, phase: "", notice: "", error: "", pending: false };

function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}

async function api(path, opts = {}) {
  const res = await fetch((DAP.api || "/api") + path, {
    method: opts.method || "GET",
    headers: {
      Authorization: "Bearer " + DAP.token,
      "Content-Type": "application/json",
    },
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok || data.ok === false) throw new Error(data.error || data.reason || "Request failed");
  return data;
}

function blankStep() {
  return { id: "step-" + Math.random().toString(36).slice(2, 8), title: "", detail: "", effects: [] };
}

function render() {
  const steps = state.steps.map((step, i) => {
    const on = (effect) => (step.effects || []).includes(effect) ? "on" : "";
    return `<article class="card" data-index="${i}">
      <div class="step-head">
        <span class="n">${i + 1}</span>
        <div>
          <button class="quiet" data-act="up" data-index="${i}" type="button">Up</button>
          <button class="quiet" data-act="down" data-index="${i}" type="button">Down</button>
          <button class="quiet" data-act="remove" data-index="${i}" type="button">Remove</button>
        </div>
      </div>
      <input data-field="title" data-index="${i}" type="text" placeholder="Step title" value="${esc(step.title)}" />
      <textarea data-field="detail" data-index="${i}" placeholder="What this step does">${esc(step.detail || "")}</textarea>
      <div class="chips">
        ${["sends", "spends", "deletes"].map((effect) => `
          <label class="${on(effect)}"><input data-effect="${effect}" data-index="${i}" type="checkbox" ${(step.effects || []).includes(effect) ? "checked" : ""} /> ${effect}</label>
        `).join("")}
      </div>
    </article>`;
  }).join("");
  document.getElementById("app").innerHTML = `
    <div class="wrap">
      <div class="top">
        <div>
          <h1>Step editor</h1>
          <p class="sub">Steps run in order. Anything that sends, spends, or deletes pauses in the decision inbox.</p>
        </div>
        <a href="${esc(DAP.inbox || "/")}">Decision inbox</a>
      </div>
      ${state.error ? `<div class="banner">${esc(state.error)}</div>` : ""}
      ${state.notice ? `<div class="banner ok">${esc(state.notice)}</div>` : ""}
      ${state.pending ? `<div class="banner">A change is waiting in the decision inbox. It applies only after you approve it.</div>` : ""}
      <p class="sub">Phase: ${esc(state.phase || "idle")}. This page can only call the workflow API.</p>
      <div class="card schedule">
        <label for="every">Run every</label>
        <input id="every" data-field="every" type="text" value="${esc(state.every)}" />
        <span class="sub">Go duration, at least 5s. Example: 30m</span>
      </div>
      ${steps || `<div class="card"><p class="sub">No steps yet.</p></div>`}
      <div class="row">
        <button type="button" data-act="add">Add step</button>
        <button class="primary" type="button" data-act="save">Save changes</button>
      </div>
    </div>`;
}

function readForm() {
  document.querySelectorAll("[data-field=title]").forEach((el) => {
    state.steps[el.dataset.index].title = el.value;
  });
  document.querySelectorAll("[data-field=detail]").forEach((el) => {
    state.steps[el.dataset.index].detail = el.value;
  });
  const every = document.querySelector("[data-field=every]");
  if (every) state.every = every.value.trim();
}

async function load() {
  const data = await api("/context");
  if (state.dirty) {
    state.phase = (data.state && data.state.phase) || "";
    state.pending = (data.open_cards || []).some((c) => c.kind === "change" && (c.status === "pending" || c.status === "snoozed"));
    return;
  }
  const cfg = data.config || {};
  state.steps = Array.isArray(cfg.steps) ? cfg.steps.map((s) => ({
    id: s.id || blankStep().id,
    title: s.title || "",
    detail: s.detail || "",
    effects: Array.isArray(s.effects) ? s.effects.slice() : [],
  })) : [];
  state.every = cfg.every || (data.schedule && data.schedule.every) || "1h";
  state.phase = (data.state && data.state.phase) || "idle";
  state.pending = (data.open_cards || []).some((c) => c.kind === "change" && (c.status === "pending" || c.status === "snoozed"));
}

document.addEventListener("click", async (ev) => {
  const el = ev.target.closest("[data-act]");
  if (!el) return;
  readForm();
  const act = el.dataset.act;
  const index = Number(el.dataset.index);
  if (act === "add") {
    state.steps.push(blankStep());
    state.dirty = true;
  } else if (act === "remove") {
    state.steps.splice(index, 1);
    state.dirty = true;
  } else if (act === "up" && index > 0) {
    const [row] = state.steps.splice(index, 1);
    state.steps.splice(index - 1, 0, row);
    state.dirty = true;
  } else if (act === "down" && index < state.steps.length - 1) {
    const [row] = state.steps.splice(index, 1);
    state.steps.splice(index + 1, 0, row);
    state.dirty = true;
  } else if (act === "save") {
    state.error = "";
    if (!state.steps.length || state.steps.some((s) => !s.title.trim())) {
      state.error = "Every step needs a title.";
      render();
      return;
    }
    try {
      await api("/propose", {
        method: "POST",
        body: {
          title: "Update workflow steps",
          summary: "Apply an edited step list and schedule.",
          body: state.steps.map((s, i) => `${i + 1}. ${s.title}${s.effects.length ? " (" + s.effects.join(", ") + ")" : ""}`).join("\n"),
          kind: "change",
          permission: "workflow.rules",
          effects: [],
          editable: true,
          payload: {
            config: {
              every: state.every || "1h",
              steps: state.steps.map((s) => ({ id: s.id, title: s.title.trim(), detail: s.detail, effects: s.effects })),
            },
          },
        },
      });
      state.notice = "Saved as a change card. It applies after you approve it in the decision inbox.";
      state.dirty = false;
      state.pending = true;
    } catch (err) {
      state.error = err.message;
    }
  }
  render();
});

document.addEventListener("input", (ev) => {
  if (ev.target.matches("[data-field], [data-effect]")) state.dirty = true;
});

document.addEventListener("change", (ev) => {
  const box = ev.target.closest("[data-effect]");
  if (!box) return;
  readForm();
  const step = state.steps[box.dataset.index];
  const effect = box.dataset.effect;
  const set = new Set(step.effects || []);
  if (box.checked) set.add(effect);
  else set.delete(effect);
  step.effects = [...set];
  state.dirty = true;
  render();
});

async function boot() {
  try {
    await load();
  } catch (err) {
    state.error = err.message;
  }
  render();
}

boot();
setInterval(async () => {
  if (state.dirty) return;
  try { await load(); state.error = ""; } catch (err) { state.error = err.message; }
  render();
}, 3000);
