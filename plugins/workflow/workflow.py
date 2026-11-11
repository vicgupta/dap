#!/usr/bin/env python3
"""workflow — run steps in order. Sensitive steps pause for an approved card.

Speaks DAP plugin interface dap.plugin.v1 over stdin/stdout.
Calls the core only through the scoped local API.
"""

import json
import sys
import urllib.error
import urllib.request
from datetime import datetime, timezone

RT = {"config": {}, "api": "", "token": ""}


def now():
    return datetime.now(timezone.utc).isoformat()


def reply(rid, result=None, error=None):
    msg = {"jsonrpc": "2.0", "id": rid}
    if error is not None:
        msg["error"] = {"code": -32000, "message": str(error)}
    else:
        msg["result"] = result
    sys.stdout.write(json.dumps(msg) + "\n")
    sys.stdout.flush()


def api(method, path, body=None):
    if not RT["api"]:
        raise RuntimeError("plugin is not initialized")
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(RT["api"] + path, data=data, method=method)
    req.add_header("Authorization", "Bearer " + RT["token"])
    req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            raw = resp.read().decode()
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode()
        try:
            parsed = json.loads(detail)
        except Exception:
            parsed = {"error": detail, "ok": False}
        return parsed


def steps():
    raw = (RT["config"] or {}).get("steps") or []
    out = []
    for step in raw:
        if not isinstance(step, dict):
            continue
        effects = step.get("effects") or []
        if isinstance(effects, str):
            effects = [effects]
        out.append(
            {
                "id": str(step.get("id") or step.get("title") or "step"),
                "title": str(step.get("title") or "Step"),
                "detail": str(step.get("detail") or ""),
                "effects": [str(e) for e in effects],
            }
        )
    return out


def sensitive(step):
    return any(effect in ("sends", "spends", "deletes") for effect in step["effects"])


def fresh_state(cycles=0, summary=""):
    return {
        "phase": "idle",
        "cursor": 0,
        "waiting_step": "",
        "completed": [],
        "cycles": cycles,
        "last_summary": summary,
    }


def load_state():
    data = api("GET", "/state")
    state = data.get("state")
    if not isinstance(state, dict) or not state:
        return fresh_state()
    base = fresh_state()
    base.update(state)
    if not isinstance(base.get("completed"), list):
        base["completed"] = []
    return base


def save_state(state):
    api("PUT", "/state", state)


def open_for(step_id):
    data = api("GET", "/cards")
    for card in data.get("cards") or []:
        if card.get("step_id") == step_id and card.get("status") in ("pending", "snoozed", "acting", "approved"):
            return True
    return False


def log(message, ref=""):
    body = {"message": message}
    if ref:
        body["evidence"] = [{"label": "Step", "ref": ref}]
    api("POST", "/log", body)


def write_tile(state):
    phase = state.get("phase") or "idle"
    done = len(state.get("completed") or [])
    total = len(steps())
    lines = [
        {"label": "Phase", "value": phase},
        {"label": "Steps", "value": "%d/%d" % (done, total)},
    ]
    if state.get("last_summary"):
        lines.append({"label": "Last", "value": state["last_summary"]})
    api(
        "POST",
        "/tile",
        {"title": "Workflow", "status": "ok" if phase != "error" else "error", "lines": lines},
    )


def card_for(step):
    effects = step["effects"]
    verb = ", ".join(effects) if effects else "needs a decision"
    return {
        "title": step["title"],
        "summary": "This step %s. It waits in the inbox until you approve it." % verb,
        "body": step["detail"],
        "kind": "action",
        "permission": "share.team",
        "effects": effects,
        "editable": True,
        "payload": {"step_id": step["id"]},
        "evidence": [{"label": "Step", "ref": step["id"]}],
    }


def complete(step, state):
    state["completed"] = list(state.get("completed") or []) + [step["id"]]
    state["last_summary"] = "Finished “%s”" % step["title"]
    log(state["last_summary"], step["id"])


def advance(state, start):
    """Run safe steps from start. Return a card when a step must pause."""
    all_steps = steps()
    index = start
    while index < len(all_steps):
        step = all_steps[index]
        if sensitive(step):
            state["phase"] = "paused"
            state["cursor"] = index
            state["waiting_step"] = step["id"]
            save_state(state)
            write_tile(state)
            return card_for(step)
        decision = api(
            "POST",
            "/check",
            {"permission": "workflow.run", "effects": step["effects"], "summary": step["title"]},
        )
        if decision.get("decision") != "allowed":
            state["phase"] = "paused"
            state["cursor"] = index
            state["waiting_step"] = step["id"]
            save_state(state)
            write_tile(state)
            card = card_for(step)
            card["permission"] = "workflow.run"
            return card
        complete(step, state)
        index += 1
        state["cursor"] = index
        save_state(state)
    state["phase"] = "complete"
    state["waiting_step"] = ""
    state["cursor"] = 0
    state["cycles"] = int(state.get("cycles") or 0) + 1
    state["last_summary"] = "Cycle complete"
    save_state(state)
    write_tile(state)
    return None


def handle(method, params):
    if method == "initialize":
        RT["config"] = params.get("config") or {}
        RT["api"] = (params.get("api_base") or "").rstrip("/")
        RT["token"] = params.get("token") or ""
        if not steps():
            return {
                "ready": False,
                "interface": "dap.plugin.v1",
                "health": {"status": "error", "message": "Add at least one step"},
            }
        return {
            "ready": True,
            "interface": "dap.plugin.v1",
            "health": {"status": "ok", "message": "%d steps" % len(steps())},
        }
    if method == "watch":
        state = load_state()
        all_steps = steps()
        if state.get("phase") == "paused" and state.get("waiting_step"):
            summary = "Paused before “%s”" % state["waiting_step"]
        elif all_steps:
            summary = "Ready to run %d steps" % len(all_steps)
        else:
            summary = "No steps yet"
        write_tile(state)
        return {
            "observations": [{"id": "wf-%s" % state.get("cycles", 0), "at": now(), "summary": summary}],
            "tile": {
                "title": "Workflow",
                "status": "ok",
                "lines": [{"label": "Phase", "value": state.get("phase") or "idle"}],
            },
        }
    if method == "propose":
        state = load_state()
        if state.get("phase") == "paused" and state.get("waiting_step") and open_for(state["waiting_step"]):
            write_tile(state)
            return {"cards": []}
        cycles = int(state.get("cycles") or 0)
        summary = state.get("last_summary") or ""
        state = fresh_state(cycles, summary)
        state["phase"] = "running"
        save_state(state)
        card = advance(state, 0)
        if card:
            return {"cards": [card]}
        return {"cards": []}
    if method == "act":
        card = params.get("card") or {}
        payload = card.get("payload") or {}
        if isinstance(payload, str):
            payload = json.loads(payload)
        step_id = payload.get("step_id")
        state = load_state()
        all_steps = steps()
        step = next((item for item in all_steps if item["id"] == step_id), None)
        if step is None:
            return {"status": "failed", "summary": "Unknown step."}
        check = api(
            "POST",
            "/call",
            {
                "permission": card.get("permission") or "share.team",
                "effects": step["effects"],
                "card_id": card.get("id"),
                "summary": step["title"],
            },
        )
        if check.get("decision") != "allowed":
            raise RuntimeError(check.get("reason") or "guardrail blocked this step")
        complete(step, state)
        nxt = 0
        for index, item in enumerate(all_steps):
            if item["id"] == step_id:
                nxt = index + 1
                break
        state["cursor"] = nxt
        state["waiting_step"] = ""
        state["phase"] = "running"
        save_state(state)
        follow = advance(state, nxt)
        if follow:
            api("POST", "/propose", follow)
            return {
                "status": "done",
                "summary": "Finished “%s”. The next step that sends, spends, or deletes is waiting." % step["title"],
                "evidence": [{"label": "Step", "ref": step["id"]}],
            }
        return {
            "status": "done",
            "summary": "Finished “%s”." % step["title"],
            "evidence": [{"label": "Step", "ref": step["id"]}],
        }
    if method == "report":
        state = load_state()
        return {
            "metrics": [
                {"key": "cycles", "label": "Cycles", "value": int(state.get("cycles") or 0)},
                {"key": "phase", "label": "Phase", "value": state.get("phase") or "idle"},
            ]
        }
    if method == "shutdown":
        return {"ok": True}
    raise RuntimeError("unknown method %s" % method)


def main():
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except Exception as exc:
            reply(None, error=str(exc))
            continue
        try:
            reply(req.get("id"), result=handle(req.get("method"), req.get("params") or {}))
        except Exception as exc:
            reply(req.get("id"), error=str(exc))


if __name__ == "__main__":
    main()
