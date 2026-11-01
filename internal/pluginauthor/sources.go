package pluginauthor

func agentsMD(name, kind, lang string) string {
	return `# Build this DAP plugin

You are editing the plugin in this directory (` + name + `, type ` + kind + `, language ` + lang + `). DAP does not host a model. You are the user's coding agent. Follow this file. The long form is docs/plugin-authoring.md in the DAP repo, and the machine-readable contract is schemas/plugin.schema.json.

## Rules

- Speak JSON-RPC 2.0, one JSON object per line, on stdout. Logs go to stderr. Never print a prompt, a log line, or a stack trace on stdout.
- Implement initialize, watch, propose, act, and report. Unknown methods return a JSON-RPC error.
- Declare the smallest permission list that does the job. Modes are only allowed or ask_first. An undeclared permission is denied.
- Anything that sends, spends, or deletes must be a decision card with that effect. Do not perform it in watch or propose. act runs only after the person approves.
- A change to this plugin's own config or schedule is a change card (kind change). Do not write it yourself.
- Do not read or write raw OAuth tokens, API keys, or another plugin's secrets.
- Do not add network calls unless the user asked and a permission describes them. This core does not grant a general network permission.
- Do not weaken guardrails. You cannot set a permission to never in the manifest, and you cannot make an ask_first permission allowed in order to skip the inbox.
- Keep entrypoints inside this directory. No path may contain .. that escapes the folder.

## Self-check

From the DAP repo, or with da on PATH:

` + "```" + `
da plugin validate .
` + "```" + `

Fix every check that is not ok. The validator output is the bug list. Do not claim success until it passes.

## Protocol reminder

initialize result: {"ready": true, "interface": "dap.plugin.v1", "health": {"status": "ok", "message": "ready"}}

watch result: {"observations": [], "tile": {"title": "...", "status": "ok", "lines": []}}

propose result: {"cards": [{"title": "...", "summary": "...", "kind": "action", "permission": "...", "effects": []}]}

act result: {"status": "done", "summary": "..."}

report result: {"metrics": [{"key": "runs", "label": "Runs", "value": 1}]}

Effects may only be sends, spends, or deletes. kind is action or change.
`
}

func pythonSrc(name, kind, perm string) string {
	cards := pythonCards(name, kind, perm)
	return `#!/usr/bin/env python3
"""DAP plugin ` + name + `. Stdout is JSON-RPC only. Logs go to stderr."""
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
        return {"observations": [], "tile": {"title": "` + name + `", "status": "ok", "lines": [{"label": "Status", "value": "idle"}]}}
    if method == "propose":
        return {"cards": ` + cards + `}
    if method == "act":
        return {"status": "done", "summary": "Recorded."}
    if method == "report":
        return {"metrics": [{"key": "runs", "label": "Runs", "value": 1}]}
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
`
}

func pythonCards(name, kind, perm string) string {
	if kind == "capability" {
		return "[]"
	}
	return `[{
            "title": "Review ` + name + `",
            "summary": "Nothing is sent.",
            "kind": "action",
            "permission": "` + perm + `",
            "effects": [],
            "editable": True,
        }]`
}

func pythonTest() string {
	return `#!/usr/bin/env python3
"""Speaks one initialize to plugin.py. Exit 0 when the shape is right."""
import json
import subprocess
import sys

proc = subprocess.Popen(
    [sys.executable, "plugin.py"],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    text=True,
)
req = {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"interface": "dap.plugin.v1"}}
out, _ = proc.communicate(json.dumps(req) + "\n", timeout=10)
msg = json.loads(out.splitlines()[0])
result = msg["result"]
assert result["ready"] is True
assert result["interface"] == "dap.plugin.v1"
assert result["health"]["status"] == "ok"
print("ok")
`
}

func nodeSrc(name, kind, perm string) string {
	cards := "[]"
	if kind != "capability" {
		cards = `[{ title: "` + name + `", summary: "Nothing is sent.", kind: "action", permission: "` + perm + `", effects: [], editable: true }]`
	}
	return `#!/usr/bin/env node
// DAP plugin ` + name + `. Stdout is JSON-RPC only. Logs go to stderr.
const readline = require("readline");

function reply(id, result, error) {
  const msg = { jsonrpc: "2.0", id };
  if (error) msg.error = { code: -32601, message: String(error) };
  else msg.result = result;
  process.stdout.write(JSON.stringify(msg) + "\n");
}

function handle(method) {
  if (method === "initialize") {
    return { ready: true, interface: "dap.plugin.v1", health: { status: "ok", message: "ready" } };
  }
  if (method === "watch") {
    return { observations: [], tile: { title: "` + name + `", status: "ok", lines: [{ label: "Status", value: "idle" }] } };
  }
  if (method === "propose") return { cards: ` + cards + ` };
  if (method === "act") return { status: "done", summary: "Recorded." };
  if (method === "report") return { metrics: [{ key: "runs", label: "Runs", value: 1 }] };
  throw new Error("unknown method " + method);
}

const rl = readline.createInterface({ input: process.stdin });
rl.on("line", (line) => {
  line = line.trim();
  if (!line) return;
  let req;
  try { req = JSON.parse(line); } catch (err) { console.error("bad request", err.message); return; }
  try { reply(req.id, handle(req.method)); }
  catch (err) { reply(req.id, null, err.message); }
});
`
}

func nodeTest() string {
	return `import { spawn } from "node:child_process";

const proc = spawn(process.execPath, ["plugin.js"], { stdio: ["pipe", "pipe", "inherit"] });
let buf = "";
proc.stdout.on("data", (c) => { buf += c; });
proc.stdin.write(JSON.stringify({ jsonrpc: "2.0", id: 1, method: "initialize", params: {} }) + "\n");
proc.stdin.end();
await new Promise((resolve, reject) => {
  proc.on("exit", resolve);
  proc.on("error", reject);
});
const msg = JSON.parse(buf.trim().split("\n")[0]);
if (!msg.result || msg.result.ready !== true) {
  console.error(buf);
  process.exit(1);
}
console.log("ok");
`
}

func goSrc(name, kind, perm string) string {
	propose := `[]any{}`
	if kind != "capability" {
		propose = `[]any{map[string]any{"title": "` + name + `", "summary": "Nothing is sent.", "kind": "action", "permission": "` + perm + `", "effects": []any{}, "editable": true}}`
	}
	return `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req struct {
			ID     any    ` + "`json:\"id\"`" + `
			Method string ` + "`json:\"method\"`" + `
		}
		if err := json.Unmarshal(line, &req); err != nil {
			fmt.Fprintf(os.Stderr, "bad request: %v\n", err)
			continue
		}
		result, err := handle(req.Method)
		msg := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if err != nil {
			msg["error"] = map[string]any{"code": -32601, "message": err.Error()}
		} else {
			msg["result"] = result
		}
		b, _ := json.Marshal(msg)
		os.Stdout.Write(append(b, '\n'))
	}
}

func handle(method string) (any, error) {
	switch method {
	case "initialize":
		return map[string]any{"ready": true, "interface": "dap.plugin.v1", "health": map[string]any{"status": "ok", "message": "ready"}}, nil
	case "watch":
		return map[string]any{"observations": []any{}, "tile": map[string]any{"title": "` + name + `", "status": "ok", "lines": []any{map[string]any{"label": "Status", "value": "idle"}}}}, nil
	case "propose":
		return map[string]any{"cards": ` + propose + `}, nil
	case "act":
		return map[string]any{"status": "done", "summary": "Recorded."}, nil
	case "report":
		return map[string]any{"metrics": []any{map[string]any{"key": "runs", "label": "Runs", "value": 1}}}, nil
	default:
		return nil, fmt.Errorf("unknown method %s", method)
	}
}
`
}

func goTest() string {
	return `package main

import "testing"

func TestHandleInitialize(t *testing.T) {
	got, err := handle("initialize")
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	if m["ready"] != true {
		t.Fatalf("%v", m)
	}
	if _, err := handle("nope"); err == nil {
		t.Fatal("expected unknown method error")
	}
}
`
}

func uiHTML(name string) string {
	return `<!DOCTYPE html>
<html lang="en" data-theme="system">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>` + name + `</title>
  <script src="theme-boot.js"></script>
  <!--DAP_BOOT-->
  <link rel="stylesheet" href="app.css" />
</head>
<body>
  <main id="app"></main>
  <script src="app.js"></script>
</body>
</html>
`
}

func uiJS() string {
	return `const meta = (name) => {
  const el = document.querySelector('meta[name="' + name + '"]');
  return el ? el.content : "";
};
const token = meta("dap-token");
const api = meta("dap-api") || "/api";
document.getElementById("app").innerHTML = "<h1>Plugin</h1><p class=\"sub\">This page can call only this plugin's API.</p>";
if (token) {
  fetch(api + "/context", { headers: { Authorization: "Bearer " + token } })
    .then((r) => r.json())
    .then((data) => {
      const p = document.createElement("p");
      p.className = "sub";
      p.textContent = "Plugin " + ((data.context && data.context.plugin) || data.plugin || "");
      document.getElementById("app").appendChild(p);
    })
    .catch(() => {});
}
`
}

func uiCSS() string {
	return `:root, html[data-theme="light"] {
  color-scheme: light;
  --bg: #f4f5f7; --ink: #1b1c21; --muted: #4b5160; --surface: #fff; --line: #e6e7ee;
}
html[data-theme="dark"] { color-scheme: dark; --bg: #101218; --ink: #f2f4f8; --muted: #c5cbd8; --surface: #1a1d27; --line: #2c3142; }
@media (prefers-color-scheme: dark) {
  html[data-theme="system"] { color-scheme: dark; --bg: #101218; --ink: #f2f4f8; --muted: #c5cbd8; --surface: #1a1d27; --line: #2c3142; }
}
body { margin: 0; background: var(--bg); color: var(--ink); font: 14px/1.5 Inter, ui-sans-serif, system-ui, sans-serif; }
main { max-width: 640px; margin: 0 auto; padding: 32px 20px; }
h1 { font-size: 22px; margin: 0; }
.sub { color: var(--muted); }
`
}

func themeBoot() string {
	return `(function () {
  var allowed = { system: 1, light: 1, dark: 1 };
  var q = "";
  try { q = new URLSearchParams(location.search).get("theme") || ""; } catch (e) {}
  if (!allowed[q]) q = "";
  var theme = q || document.documentElement.getAttribute("data-theme") || "system";
  if (!allowed[theme]) theme = "system";
  document.documentElement.setAttribute("data-theme", theme);
})();
`
}
