#!/usr/bin/env python3
"""DAP plugin note-watch. Stdout is JSON-RPC only. Logs go to stderr."""
import json
import sys

def reply(i, result=None, error=None):
    msg = {"jsonrpc": "2.0", "id": i}
    if error is not None:
        msg["error"] = {"code": -32601, "message": str(error)}
    else:
        msg["result"] = result
    sys.stdout.write(json.dumps(msg) + "\n")
    sys.stdout.flush()

def handle(method, params):
    if method == "initialize":
        return {"ready": True, "interface": "dap.plugin.v1", "health": {"status": "ok", "message": "ready"}}
    if method == "watch":
        return {"observations": [], "tile": {"title": "note-watch", "status": "ok", "lines": [{"label": "Status", "value": "idle"}]}}
    if method == "propose":
        return {"cards": [{
            "title": "Save a note",
            "summary": "Nothing is sent.",
            "kind": "action",
            "permission": "note.watch",
            "effects": [],
            "editable": True,
        }]}
    if method == "act":
        return {"status": "done", "summary": "Recorded."}
    if method == "report":
        return {"metrics": [{"key": "runs", "label": "Runs", "value": 1}]}
    if method == "shutdown":
        return {"ok": True}
    raise RuntimeError("unknown method " + str(method))

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        req = json.loads(line)
    except json.JSONDecodeError as exc:
        sys.stderr.write("bad request: %s\n" % exc)
        continue
    try:
        reply(req.get("id"), result=handle(req.get("method"), req.get("params") or {}))
    except Exception as exc:
        reply(req.get("id"), error=str(exc))
