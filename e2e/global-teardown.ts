import fs from "fs";
import path from "path";

const stateFile = path.join(__dirname, ".servers.json");

export default async function globalTeardown() {
  if (!fs.existsSync(stateFile)) return;
  const rows = JSON.parse(fs.readFileSync(stateFile, "utf8")) as { pid: number }[];
  for (const row of rows) {
    try {
      process.kill(-row.pid, "SIGTERM");
    } catch {
      try { process.kill(row.pid, "SIGTERM"); } catch { /* already stopped */ }
    }
  }
  await new Promise((resolve) => setTimeout(resolve, 400));
  for (const row of rows) {
    try { process.kill(-row.pid, 0); } catch { continue; }
    try { process.kill(-row.pid, "SIGKILL"); } catch { /* gone */ }
  }
}
