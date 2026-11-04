import { Host, serve } from "../vendor/plugin-sdk/dist/index.js";
let host;
let drafted = 0;
function text(value) {
    return typeof value === "string" ? value : "";
}
function providerOf(config) {
    const raw = text(config.provider);
    if (raw === "outlook" || raw === "graph" || raw === "fake")
        return raw;
    return "gmail";
}
function flagged(thread) {
    if (thread.flagged || thread.starred)
        return true;
    return /flag/i.test(text(thread.subject));
}
function draftBody(thread) {
    const subject = text(thread.subject) || "your note";
    const snippet = text(thread.snippet);
    const heard = snippet ? " I saw: " + snippet.slice(0, 180) : "";
    return "Thanks for the note about " + subject + "." + heard + "\n\nI will follow up shortly.\n";
}
const plugin = {
    async initialize(params) {
        host = new Host(params);
        return { ready: true, interface: "dap.plugin.v1", health: { status: "ok", message: "Watching for flagged threads" } };
    },
    async watch() {
        const provider = providerOf(host?.config ?? {});
        let threads = [];
        try {
            const res = await host.mailThreads(provider, { newer_than: "7d" });
            threads = Array.isArray(res.threads) ? res.threads : [];
        }
        catch {
            threads = [];
        }
        const flaggedThreads = threads.filter(flagged).slice(0, 5);
        return {
            observations: flaggedThreads.map((thread, i) => ({
                id: text(thread.id) || "flag-" + String(i),
                at: new Date().toISOString(),
                summary: text(thread.subject) || "(no subject)",
                data: {
                    provider,
                    to: text(thread.with_email) || text(thread.from),
                    subject: text(thread.subject) || "(no subject)",
                    body: draftBody(thread),
                },
            })),
            tile: {
                title: "Reply drafter",
                status: "ok",
                lines: [{ label: "Flagged", value: String(flaggedThreads.length) }],
            },
        };
    },
    async propose(params) {
        const cards = [];
        for (const row of params.observations ?? []) {
            const data = (row.data ?? {});
            const body = text(data.body) || "Thanks for the note.\n";
            if (host && data.to) {
                try {
                    await host.mailDraft({
                        provider: (data.provider === "outlook" || data.provider === "graph" || data.provider === "fake" ? data.provider : "gmail"),
                        to: data.to,
                        subject: data.subject ? "Re: " + data.subject : "Re: your note",
                        body,
                    });
                    drafted += 1;
                }
                catch {
                    // The card still shows the text. Nothing is sent.
                }
            }
            cards.push({
                title: "Draft: " + (text(data.subject) || row.summary),
                summary: "A reply is drafted. Nothing is sent.",
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
        return { status: "done", summary: "The draft is saved. Nothing was sent." };
    },
    async report() {
        return { metrics: [{ key: "drafts", label: "Drafts", value: drafted }] };
    },
    async shutdown() {
        return { ok: true };
    },
};
serve(plugin);
