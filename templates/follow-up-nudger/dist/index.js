// DAP_SCAFFOLD_STARTER
import { Host, serve } from "../vendor/plugin-sdk/dist/index.js";
let host;
let nudges = 0;
function text(value) {
    return typeof value === "string" ? value : "";
}
function hours(value) {
    const n = Number(value);
    if (!Number.isFinite(n) || n < 1)
        return 24;
    return n;
}
function waiting(thread, waitHours) {
    const at = Date.parse(text(thread.last_at));
    if (!Number.isFinite(at))
        return true;
    return Date.now() - at >= waitHours * 60 * 60 * 1000;
}
const plugin = {
    async initialize(params) {
        host = new Host(params);
        return { ready: true, interface: "dap.plugin.v1", health: { status: "ok", message: "Watching for threads to nudge" } };
    },
    async watch() {
        const config = host?.config ?? {};
        const waitHours = hours(config.wait_hours);
        const provider = "gmail";
        let threads = [];
        try {
            const res = await host.mailThreads(provider, { newer_than: "7d" });
            threads = Array.isArray(res.threads) ? res.threads : [];
        }
        catch {
            threads = [];
        }
        const due = threads.filter((thread) => waiting(thread, waitHours)).slice(0, 5);
        return {
            observations: due.map((thread, i) => ({
                id: text(thread.id) || "nudge-" + String(i),
                at: new Date().toISOString(),
                summary: text(thread.subject) || "(no subject)",
                data: {
                    to: text(thread.with_email) || text(thread.from),
                    subject: text(thread.subject) || "(no subject)",
                },
            })),
            tile: {
                title: "Follow-up nudger",
                status: "ok",
                lines: [{ label: "Due", value: String(due.length) }],
            },
        };
    },
    async propose(params) {
        const signature = text(host?.config.signature) || "You";
        const cards = [];
        for (const row of params.observations ?? []) {
            const data = (row.data ?? {});
            const body = "Checking in on " + (text(data.subject) || "this") + ".\n\n" + signature + "\n";
            if (host && data.to) {
                try {
                    await host.mailDraft({ provider: "gmail", to: data.to, subject: "Re: " + text(data.subject), body });
                    nudges += 1;
                }
                catch {
                    // The card still holds the draft. Nothing is sent.
                }
            }
            cards.push({
                title: "Nudge: " + (text(data.subject) || row.summary),
                summary: "A follow-up draft is ready. Nothing is sent.",
                body,
                kind: "change",
                permission: "mail.draft",
                effects: [],
                editable: true,
                payload: { config: host?.config ?? {} },
            });
        }
        return { cards };
    },
    async act() {
        return { status: "done", summary: "Draft saved. Nothing was sent." };
    },
    async report() {
        return { metrics: [{ key: "nudges", label: "Nudges", value: nudges }] };
    },
    async shutdown() {
        return { ok: true };
    },
};
serve(plugin);
