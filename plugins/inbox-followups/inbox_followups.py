#!/usr/bin/env python3
"""inbox-followups — watcher that proposes follow-up drafts. Sends wait for approval."""

import json
import sys
import urllib.error
import urllib.request
from datetime import datetime, timezone

RT = {"config": {}, "api": "", "token": ""}


def reply(rid, result=None, error=None):
    msg = {"jsonrpc": "2.0", "id": rid}
    if error is not None:
        msg["error"] = {"code": -32000, "message": str(error)}
    else:
        msg["result"] = result
    sys.stdout.write(json.dumps(msg) + "\n")
    sys.stdout.flush()


def api(method, path, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(RT["api"] + path, data=data, method=method)
    req.add_header("Authorization", "Bearer " + RT["token"])
    req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            raw = resp.read().decode()
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode()
        try:
            return json.loads(detail)
        except Exception:
            return {"ok": False, "error": detail, "http_status": exc.code}


def hours(value, default=24):
    try:
        return float(value)
    except (TypeError, ValueError):
        return float(default)


def age_hours(iso):
    if not iso:
        return 0
    text = iso.replace("Z", "+00:00")
    try:
        then = datetime.fromisoformat(text)
    except ValueError:
        return 0
    if then.tzinfo is None:
        then = then.replace(tzinfo=timezone.utc)
    return (datetime.now(timezone.utc) - then).total_seconds() / 3600


def domain(email):
    if not email or "@" not in email:
        return ""
    return email.split("@", 1)[1].lower()


def skipped(thread, domains):
    host = domain(thread.get("with_email") or "")
    return host and host in domains


def bucket(thread, wait):
    if thread.get("short") and not thread.get("last_from_me") and age_hours(thread.get("last_at")) >= 1:
        return "ack"
    if thread.get("last_from_me") and age_hours(thread.get("last_at")) >= wait:
        return "followup"
    if not thread.get("last_from_me") and age_hours(thread.get("last_at")) >= wait:
        return "owe"
    return ""


def first_name(name):
    name = (name or "").strip()
    return name.split()[0] if name else "there"


def draft_for(thread, signature):
    if thread.get("draft"):
        return thread["draft"]
    who = first_name(thread.get("with_name"))
    subject = thread.get("subject") or "this"
    if thread.get("last_from_me"):
        return "Hi %s — circling back on %s. Happy to help if anything is stuck.\nBest, %s" % (who, subject, signature)
    return "Hi %s — thanks for the note on %s. I'll follow up shortly.\nBest, %s" % (who, subject, signature)


def title_for(thread, kind):
    if thread.get("title"):
        return thread["title"]
    name = thread.get("with_name") or thread.get("with_email") or "them"
    subject = thread.get("subject") or "a thread"
    if kind == "followup":
        return "Send follow-up to %s re: %s" % (name, subject)
    if kind == "owe":
        return "Reply to %s: %s" % (name, subject)
    return "Quick acknowledgement to %s" % name


def why_for(thread, kind):
    if thread.get("why"):
        return thread["why"]
    if kind == "followup":
        return "You wrote last and there has been no reply."
    if kind == "ack":
        return "A short note you can close in one line."
    return "They wrote last. This is waiting on you."


def providers():
    data = api("GET", "/mail/status")
    found = []
    for item in data.get("connections") or []:
        if item.get("connected"):
            found.append(item.get("provider"))
    return found


def threads_for(provider):
    data = api("GET", "/mail/threads?provider=" + provider)
    if data.get("ok") is False:
        return []
    rows = data.get("threads") or []
    for row in rows:
        row["provider"] = provider
    return rows


def open_threads(cards):
    taken = set()
    change_open = False
    for card in cards:
        status = card.get("status")
        if card.get("kind") == "change" and status in ("pending", "snoozed", "acting"):
            change_open = True
        payload = card.get("payload") or {}
        if isinstance(payload, str):
            try:
                payload = json.loads(payload)
            except Exception:
                payload = {}
        ids = []
        if payload.get("thread_id"):
            ids.append(payload["thread_id"])
        for item in payload.get("batch") or []:
            if item.get("thread_id"):
                ids.append(item["thread_id"])
        if status in ("pending", "snoozed", "acting", "approved", "done"):
            taken.update(ids)
    return taken, change_open


def card_for(thread, kind, signature):
    return {
        "title": title_for(thread, kind),
        "summary": why_for(thread, kind),
        "body": draft_for(thread, signature),
        "kind": "action",
        "permission": "mail.send",
        "effects": ["sends"],
        "editable": True,
        "payload": {
            "provider": thread.get("provider"),
            "thread_id": thread.get("id"),
            "message_id": thread.get("message_id") or "",
            "to": thread.get("with_email"),
            "subject": thread.get("subject"),
            "name": thread.get("with_name"),
            "bucket": kind,
            "promise": bool(thread.get("promise")),
            "last_at": thread.get("last_at"),
        },
        "evidence": [{"label": "Evidence", "ref": thread.get("evidence") or "thread"}],
    }


def handle(method, params):
    if method == "initialize":
        RT["config"] = params.get("config") or {}
        RT["api"] = (params.get("api_base") or "").rstrip("/")
        RT["token"] = params.get("token") or ""
        return {
            "ready": True,
            "interface": "dap.plugin.v1",
            "health": {"status": "ok", "message": "Watching for follow-ups"},
        }
    if method == "watch":
        connected = providers() if RT["api"] else []
        summary = "No mailbox connected" if not connected else "Watching " + ", ".join(connected)
        return {
            "observations": [{"id": "mailboxes", "at": datetime.now(timezone.utc).isoformat(), "summary": summary}],
            "tile": {
                "title": "Inbox follow-ups",
                "status": "ok" if connected else "degraded",
                "lines": [{"label": "Mailboxes", "value": ", ".join(connected) or "none"}],
            },
        }
    if method == "propose":
        cfg = RT["config"] or {}
        wait = hours(cfg.get("wait_hours"), 24)
        signature = cfg.get("signature") or "Vic"
        domains = [d.strip().lower() for d in str(cfg.get("skip_domains") or "").split(",") if d.strip()]
        inbox = api("GET", "/inbox")
        taken, change_open = open_threads(inbox.get("cards") or [])
        acks = []
        cards = []
        promises = 0
        owe = 0
        waiting_on_them = 0
        for provider in providers():
            for thread in threads_for(provider):
                if skipped(thread, domains):
                    continue
                kind = bucket(thread, wait)
                if not kind:
                    continue
                if kind == "followup":
                    waiting_on_them += 1
                    if thread.get("promise"):
                        promises += 1
                elif kind == "owe":
                    owe += 1
                if thread.get("id") in taken:
                    continue
                if kind == "ack":
                    acks.append(thread)
                    continue
                cards.append(card_for(thread, kind, signature))
        if acks:
            names = [t.get("with_name") or t.get("with_email") for t in acks]
            cards.append({
                "title": "%d quick acknowledgements" % len(acks),
                "summary": "“Thanks, got it” replies to %s" % ", ".join(names),
                "body": "Thanks, got it",
                "kind": "action",
                "permission": "mail.send",
                "effects": ["sends"],
                "editable": True,
                "payload": {
                    "batch": [
                        {
                            "provider": t.get("provider"),
                            "thread_id": t.get("id"),
                            "message_id": t.get("message_id") or "",
                            "to": t.get("with_email"),
                            "subject": t.get("subject") or "Thanks",
                            "body": t.get("draft") or "Thanks, got it",
                        }
                        for t in acks
                    ],
                    "bucket": "ack",
                },
                "evidence": [{"label": "Threads", "ref": str(len(acks))}],
            })
        if cfg.get("preview_rule") and not change_open and not domains:
            cards.append({
                "title": "Skip follow-ups to your company domain",
                "summary": "Add a rule: never auto-draft to @yourcompany.com.",
                "body": "Approving this only changes the rule. It does not send anything.",
                "kind": "change",
                "permission": "mail.draft",
                "effects": [],
                "editable": True,
                "payload": {
                    "config": {
                        "wait_hours": wait,
                        "signature": signature,
                        "skip_domains": "yourcompany.com",
                        "preview_rule": False,
                    }
                },
            })
        return {"cards": cards}
    if method == "act":
        card = params.get("card") or {}
        payload = card.get("payload") or {}
        if isinstance(payload, str):
            payload = json.loads(payload)
        body = card.get("body") or ""
        sent = []
        batch = payload.get("batch") or []
        if batch:
            for item in batch:
                result = api("POST", "/mail/send", {
                    "provider": item.get("provider"),
                    "thread_id": item.get("thread_id"),
                    "message_id": item.get("message_id") or "",
                    "to": item.get("to"),
                    "subject": item.get("subject"),
                    "body": item.get("body") or body,
                    "card_id": card.get("id"),
                })
                if result.get("ok") is False:
                    raise RuntimeError(result.get("error") or result.get("reason") or "send was refused")
                sent.append(result.get("result") or {})
            return {
                "status": "done",
                "summary": "Sent %d acknowledgements." % len(sent),
                "evidence": [{"label": "Sent", "ref": (sent[0].get("message_id") if sent else "")}],
            }
        result = api("POST", "/mail/send", {
            "provider": payload.get("provider"),
            "thread_id": payload.get("thread_id"),
            "message_id": payload.get("message_id") or "",
            "to": payload.get("to"),
            "subject": payload.get("subject"),
            "body": body,
            "card_id": card.get("id"),
        })
        if result.get("ok") is False:
            raise RuntimeError(result.get("error") or result.get("reason") or "send was refused")
        sent_to = payload.get("to") or "them"
        return {
            "status": "done",
            "summary": "Sent to %s." % sent_to,
            "evidence": [{"label": "Sent", "ref": (result.get("result") or {}).get("message_id", "")}],
        }
    if method == "report":
        inbox = api("GET", "/inbox") if RT["api"] else {}
        done = [c for c in (inbox.get("cards") or []) if c.get("status") == "done" and c.get("kind") == "action"]
        return {"metrics": [
            {"key": "sent", "label": "Follow-ups sent", "value": len(done)},
            {"key": "waiting", "label": "Still waiting", "value": sum(1 for c in (inbox.get("cards") or []) if c.get("status") == "pending")},
        ]}
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
