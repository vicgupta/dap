#!/usr/bin/env python3
"""hello-watch — a DAP watcher. One JSON-RPC message per line on stdin/stdout."""

import json
import sys
from datetime import datetime, timezone

RT = {"ticks": 0, "acted": 0}


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


def handle(method, params):
    if method == "initialize":
        return {
            "ready": True,
            "interface": "dap.plugin.v1",
            "health": {"status": "ok", "message": "Watching"},
        }
    if method == "watch":
        RT["ticks"] += 1
        obs_id = "tick-%d" % RT["ticks"]
        return {
            "observations": [{"id": obs_id, "at": now(), "summary": "Watch ticked"}],
            "tile": {
                "title": "Hello watch",
                "status": "ok",
                "lines": [
                    {"label": "Ticks", "value": str(RT["ticks"])},
                    {"label": "Hellos", "value": str(RT["acted"])},
                ],
            },
        }
    if method == "propose":
        obs = params.get("observations") or []
        ref = obs[0]["id"] if obs else "tick"
        return {
            "cards": [
                {
                    "title": "Say hello",
                    "summary": "The watch ticked. Approving records a hello. Nothing is sent.",
                    "body": "Hello from hello-watch.\n\nThis card is the whole action. Approve it and the result lands in the activity ledger.",
                    "permission": "demo.greet",
                    "effects": [],
                    "kind": "action",
                    "editable": True,
                    "evidence": [{"label": "Watch", "ref": ref}],
                }
            ]
        }
    if method == "act":
        RT["acted"] += 1
        card = params.get("card") or {}
        title = card.get("title") or "Say hello"
        return {
            "status": "done",
            "summary": "Hello. Recorded “%s”." % title,
            "evidence": [{"label": "Hello", "ref": "hello-watch"}],
        }
    if method == "report":
        return {
            "metrics": [
                {"key": "ticks", "label": "Ticks", "value": RT["ticks"]},
                {"key": "hellos", "label": "Hellos", "value": RT["acted"]},
            ]
        }
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
