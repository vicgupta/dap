#!/usr/bin/env python3
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
