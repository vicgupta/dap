"""Thin DAP plugin helper. Stdout is JSON-RPC only. Logs go to stderr."""

import json
import sys
import urllib.request


class Host:
    def __init__(self, params):
        self.params = params or {}
        self.config = self.params.get("config") or {}
        self.data_dir = self.params.get("data_dir") or ""
        self.api_base = (self.params.get("api_base") or "").rstrip("/")
        self.token = self.params.get("token") or ""

    def _call(self, method, path, body=None):
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(
            self.api_base + path,
            data=data,
            method=method,
            headers={"Authorization": "Bearer " + self.token, "Content-Type": "application/json"},
        )
        try:
            with urllib.request.urlopen(req) as res:
                payload = json.loads(res.read().decode() or "{}")
        except urllib.error.HTTPError as exc:
            raw = exc.read().decode() or "{}"
            try:
                payload = json.loads(raw)
            except json.JSONDecodeError:
                payload = {"error": raw}
            raise RuntimeError(payload.get("error") or payload.get("reason") or "host call failed")
        if payload.get("ok") is False:
            raise RuntimeError(payload.get("error") or payload.get("reason") or "host call failed")
        return payload

    def state(self):
        return self._call("GET", "/state")

    def put_state(self, state):
        return self._call("PUT", "/state", state)

    def log(self, message, evidence=None):
        return self._call("POST", "/log", {"message": message, "evidence": evidence or []})

    def tile(self, tile):
        return self._call("POST", "/tile", tile)

    def propose(self, card):
        return self._call("POST", "/propose", card)

    def check(self, permission="", effects=None, card_id=""):
        return self._call("POST", "/check", {"permission": permission, "effects": effects or [], "card_id": card_id})

    def secret_get(self, account):
        return self._call("POST", "/secret/get", {"account": account})

    def secret_set(self, account, value):
        return self._call("POST", "/secret/set", {"account": account, "value": value})

    def secret_delete(self, account):
        return self._call("POST", "/secret/delete", {"account": account})

    def mail_threads(self, provider):
        return self._call("GET", "/mail/threads?provider=" + provider)

    def mail_send(self, provider, card_id, to, subject, body):
        return self._call("POST", "/mail/send", {
            "provider": provider, "card_id": card_id, "to": to, "subject": subject, "body": body,
        })


def serve(handler):
    host = {"api": None}

    def reply(rid, result=None, error=None):
        msg = {"jsonrpc": "2.0", "id": rid}
        if error is not None:
            msg["error"] = {"code": -32601, "message": str(error)}
        else:
            msg["result"] = result
        sys.stdout.write(json.dumps(msg) + "\n")
        sys.stdout.flush()

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except json.JSONDecodeError as exc:
            reply(None, error=str(exc))
            continue
        method = req.get("method")
        params = req.get("params") or {}
        try:
            if method == "initialize":
                host["api"] = Host(params)
                result = handler["initialize"](params, host["api"])
            elif method in ("watch", "propose", "act", "report"):
                result = handler[method](params, host["api"])
            else:
                raise RuntimeError("unknown method " + str(method))
            reply(req.get("id"), result=result)
        except Exception as exc:
            reply(req.get("id"), error=str(exc))
