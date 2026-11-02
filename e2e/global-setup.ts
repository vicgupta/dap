import { spawn, spawnSync, type ChildProcess } from "child_process";
import fs from "fs";
import os from "os";
import path from "path";

const e2eDir = __dirname;
const root = path.resolve(e2eDir, "..");
const binDir = path.join(e2eDir, "bin");
const stateFile = path.join(e2eDir, ".servers.json");

const servers = [
  { name: "chromium", port: 17391 },
  { name: "firefox", port: 17392 },
  { name: "webkit", port: 17393 },
];

type Running = { name: string; pid: number; dir: string; url: string };

export default async function globalSetup() {
  build();
  writeFakeStream();
  const base = fs.mkdtempSync(path.join(os.tmpdir(), "dap-e2e-"));
  const running: Running[] = [];
  const children: ChildProcess[] = [];
  try {
    for (const spec of servers) {
      const dir = path.join(base, spec.name);
      fs.mkdirSync(dir, { recursive: true });
      da(dir, ["init"]);
      da(dir, ["plugin", "install", "--approve", path.join(root, "plugins", "hello-watch")]);
      const child = start(dir, spec.port);
      children.push(child);
      const url = await waitForURL(child);
      running.push({ name: spec.name, pid: child.pid!, dir, url });
    }
  } catch (err) {
    for (const child of children) stop(child.pid);
    throw err;
  }
  fs.writeFileSync(stateFile, JSON.stringify(running, null, 2));
}

function build() {
  fs.mkdirSync(binDir, { recursive: true });
  const daBin = path.join(root, "da");
  run("go", ["build", "-o", daBin, "./cmd/da"], root);
  run("go", ["build", "-o", path.join(binDir, "claude"), "./internal/chat/fakecli"], root);
}

function writeFakeStream() {
  const events = [
    `{"type":"system","subtype":"init","session_id":"11111111-1111-4111-8111-111111111111"}`,
    `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello "}}}`,
    `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"from the browser."}}}`,
    `{"type":"result","subtype":"success","result":"Hello from the browser.","session_id":"11111111-1111-4111-8111-111111111111"}`,
    "",
  ].join("\n");
  fs.writeFileSync(path.join(binDir, "events.ndjson"), events);
  fs.writeFileSync(path.join(binDir, "pace"), "30ms\n");
}

function da(dir: string, args: string[]) {
  const result = spawnSync(path.join(root, "da"), ["--data", dir, ...args], {
    cwd: root,
    encoding: "utf8",
    env: env(),
  });
  if (result.status !== 0) {
    throw new Error(`da ${args.join(" ")} failed\n${result.stdout}\n${result.stderr}`);
  }
}

function start(dir: string, port: number) {
  const log = path.join(e2eDir, "artifacts");
  fs.mkdirSync(log, { recursive: true });
  const out = fs.openSync(path.join(log, `server-${port}.log`), "a");
  const child = spawn(path.join(root, "da"), ["--data", dir, "serve", "--no-browser", "--addr", `127.0.0.1:${port}`], {
    cwd: root,
    env: env(),
    detached: true,
    stdio: ["ignore", "pipe", "pipe"],
  });
  child.stdout?.on("data", (chunk) => fs.writeSync(out, chunk));
  child.stderr?.on("data", (chunk) => fs.writeSync(out, chunk));
  child.on("exit", () => {
    try { fs.closeSync(out); } catch { /* already closed */ }
  });
  return child;
}

function env() {
  return {
    ...process.env,
    PATH: `${binDir}${path.delimiter}${process.env.PATH || ""}`,
    DAP_SECRETS: "file",
  };
}

function waitForURL(child: ChildProcess) {
  return new Promise<string>((resolve, reject) => {
    let buf = "";
    const timer = setTimeout(() => {
      reject(new Error(`da serve did not print a URL\n${buf}`));
    }, 20_000);
    const onData = (chunk: Buffer) => {
      buf += chunk.toString();
      const match = buf.match(/"url":\s*"([^"]+)"/);
      if (match) {
        clearTimeout(timer);
        child.stdout?.off("data", onData);
        resolve(match[1]);
      }
    };
    child.stdout?.on("data", onData);
    child.on("exit", (code) => {
      clearTimeout(timer);
      reject(new Error(`da serve exited ${code} before printing a URL\n${buf}`));
    });
  });
}

function run(cmd: string, args: string[], cwd: string) {
  const result = spawnSync(cmd, args, { cwd, encoding: "utf8" });
  if (result.status !== 0) {
    throw new Error(`${cmd} ${args.join(" ")} failed\n${result.stdout}\n${result.stderr}`);
  }
}

function stop(pid: number | undefined) {
  if (!pid) return;
  try { process.kill(-pid, "SIGTERM"); } catch { try { process.kill(pid, "SIGTERM"); } catch { /* gone */ } }
}
