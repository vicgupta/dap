import { Host, serve, type InitParams, type PluginHandler } from "../vendor/plugin-sdk/dist/index.js";

type Thread = { id?: string; subject?: string; from?: string; snippet?: string };

let host: Host | undefined;
let digests = 0;

function text(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function providerOf(config: Record<string, unknown>): "gmail" | "outlook" | "graph" | "fake" {
  const raw = text(config.provider);
  if (raw === "outlook" || raw === "graph" || raw === "fake") return raw;
  return "gmail";
}

const plugin: PluginHandler = {
  async initialize(params: InitParams) {
    host = new Host(params);
    return { ready: true, interface: "dap.plugin.v1", health: { status: "ok", message: "Ready for the morning digest" } };
  },
  async watch() {
    const provider = providerOf(host?.config ?? {});
    let threads: Thread[] = [];
    let connected = true;
    try {
      const res = await host!.mailThreads(provider, { newer_than: "24h" });
      threads = Array.isArray(res.threads) ? res.threads as Thread[] : [];
    } catch {
      connected = false;
    }
    const summary = connected
      ? threads.length + " threads arrived in the last day"
      : "Mailbox is not connected";
    return {
      observations: [{
        id: "digest-" + new Date().toISOString().slice(0, 10),
        at: new Date().toISOString(),
        summary,
        data: { connected, count: threads.length, subjects: threads.slice(0, 8).map((t) => text(t.subject) || "(no subject)") },
      }],
      tile: {
        title: "Daily digest",
        status: connected ? "ok" : "degraded",
        lines: [{ label: "Yesterday", value: connected ? String(threads.length) : "No mailbox" }],
      },
    };
  },
  async propose(params) {
    digests += 1;
    const row = params.observations[0];
    const data = (row?.data ?? {}) as { connected?: boolean; count?: number; subjects?: string[] };
    const subjects = Array.isArray(data.subjects) ? data.subjects : [];
    const body = data.connected === false
      ? "Connect Gmail or Outlook under Connections, then the next morning card will list yesterday's threads."
      : (subjects.length ? subjects.map((s) => "- " + s).join("\n") : "No mail arrived yesterday.");
    const count = Number(data.count) || 0;
    return {
      cards: [{
        title: count === 1 ? "Yesterday: 1 thread" : "Yesterday: " + count + " threads",
        summary: row?.summary || "Morning digest",
        body,
        kind: "change",
        permission: "mail.read",
        effects: [],
        editable: false,
        payload: { config: host?.config ?? {} },
      }],
    };
  },
  async act() {
    return { status: "done", summary: "Noted the digest. Nothing was sent." };
  },
  async report() {
    return { metrics: [{ key: "digests", label: "Digests", value: digests }] };
  },
  async shutdown() {
    return { ok: true };
  },
};

serve(plugin);
