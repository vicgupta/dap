#!/usr/bin/env python3
"""mail-fake — demo mailbox capability. Connect it from the Connections page."""

import json
import sys


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
            "health": {"status": "ok", "message": "Demo mailbox installed. Connect it from Connections."},
        }
    if method == "watch":
        return {
            "observations": [],
            "tile": {"title": "Demo mailbox", "status": "ok", "lines": [{"label": "Role", "value": "Example data"}]},
        }
    if method == "propose":
        return {"cards": []}
    if method == "act":
        raise RuntimeError("The demo mailbox does not send on its own.")
    if method == "report":
        return {"metrics": [{"key": "provider", "label": "Provider", "value": "fake"}]}
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
