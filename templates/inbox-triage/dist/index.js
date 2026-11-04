import { Host, serve } from "../vendor/plugin-sdk/dist/index.js";
let host;
let runs = 0;
function text(value) {
    return typeof value === "string" ? value : "";
}
function labelFor(thread) {
    const subject = text(thread.subject).toLowerCase();
    if (/urgent|asap|invoice|payment|deadline|action required/.test(subject))
        return "Needs a reply";
    if (thread.unread)
        return "Review";
    return "FYI";
}
function topN(config) {
    const n = Number(config.top);
    if (!Number.isFinite(n) || n < 1)
        return 5;
    return Math.min(20, Math.floor(n));
}
async function loadThreads(provider) {
    if (!host)
        return [];
    try {
        const res = await host.mailThreads(provider === "outlook" || provider === "graph" || provider === "fake" ? provider : "gmail", { newer_than: "24h" });
        const rows = res.threads;
        return Array.isArray(rows) ? rows : [];
    }
    catch {
        return [];
    }
}
const plugin = {
    async initialize(params) {
        host = new Host(params);
        return { ready: true, interface: "dap.plugin.v1", health: { status: "ok", message: "Watching the inbox" } };
    },
    async watch() {
        runs += 1;
        const config = host?.config ?? {};
        const provider = text(config.provider) || "gmail";
        const threads = await loadThreads(provider);
        const lines = threads.slice(0, topN(config)).map((thread) => ({
            id: text(thread.id) || text(thread.subject) || "thread",
            at: new Date().toISOString(),
            summary: labelFor(thread) + ": " + (text(thread.subject) || "(no subject)"),
            data: { label: labelFor(thread), from: text(thread.from), snippet: text(thread.snippet) },
        }));
        const value = threads.length === 0 ? "Nothing new" : String(Math.min(threads.length, topN(config))) + " labeled";
        return {
            observations: lines,
            tile: {
                title: "Inbox triage",
                status: "ok",
                lines: [
                    { label: "Threads", value },
                    { label: "Runs", value: String(runs) },
                ],
            },
        };
    },
    async propose(params) {
        const rows = params.observations ?? [];
        if (rows.length === 0)
            return { cards: [] };
        const body = rows.map((row) => "- " + row.summary).join("\n");
        return {
            cards: [{
                    title: rows.length === 1 ? "1 thread to review" : rows.length + " threads to review",
                    summary: "Labeled from the last day. Nothing is sent.",
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
        return { status: "done", summary: "Recorded the triage. Nothing was sent." };
    },
    async report() {
        return { metrics: [{ key: "runs", label: "Runs", value: runs }] };
    },
    async shutdown() {
        return { ok: true };
    },
};
serve(plugin);
