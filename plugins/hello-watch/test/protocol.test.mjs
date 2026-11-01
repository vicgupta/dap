import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import test from "node:test";

test("initialize answers the DAP contract", async () => {
  const proc = spawn(process.execPath, ["dist/index.js"], { stdio: ["pipe", "pipe", "inherit"] });
  let buf = "";
  proc.stdout.on("data", (chunk) => { buf += chunk; });
  proc.stdin.write(JSON.stringify({ jsonrpc: "2.0", id: 1, method: "initialize", params: { interface: "dap.plugin.v1", config: {} } }) + "\n");
  proc.stdin.end();
  const code = await new Promise((resolve, reject) => {
    proc.on("exit", resolve);
    proc.on("error", reject);
  });
  assert.equal(code, 0);
  const msg = JSON.parse(buf.trim().split("\n")[0]);
  assert.equal(msg.result.ready, true);
  assert.equal(msg.result.health.status, "ok");
});
