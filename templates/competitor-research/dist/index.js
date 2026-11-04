import { Host, serve } from "../vendor/plugin-sdk/dist/index.js";
let host;
let fetches = 0;
function urlsFrom(config) {
    const raw = config.urls;
    if (!Array.isArray(raw))
        return [];
    const out = [];
    for (const item of raw) {
        if (typeof item !== "string")
            continue;
        const url = item.trim();
        if (!url)
            continue;
        try {
            const parsed = new URL(url);
            if (parsed.protocol !== "http:" && parsed.protocol !== "https:")
                continue;
            out.push(parsed.toString());
        }
        catch {
            continue;
        }
    }
    return out.slice(0, 12);
}
async function fetchPage(url) {
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), 8000);
    try {
        const res = await fetch(url, { signal: ctrl.signal, redirect: "follow" });
        const html = await res.text();
        const match = html.match(/<title[^>]*>([^<]*)<\/title>/i);
        const title = (match?.[1] || url).replace(/\s+/g, " ").trim().slice(0, 140);
        return { url, title, status: res.status };
    }
    catch {
        return { url, title: "Could not fetch", status: 0 };
    }
    finally {
        clearTimeout(timer);
    }
}
const plugin = {
    async initialize(params) {
        host = new Host(params);
        return { ready: true, interface: "dap.plugin.v1", health: { status: "ok", message: "Watching competitor pages" } };
    },
    async watch() {
        const urls = urlsFrom(host?.config ?? {});
        if (urls.length === 0) {
            return {
                observations: [],
                tile: {
                    title: "Competitor research",
                    status: "ok",
                    lines: [{ label: "Pages", value: "Add URLs" }],
                },
            };
        }
        const pages = [];
        for (const url of urls) {
            pages.push(await fetchPage(url));
            fetches += 1;
        }
        return {
            observations: pages.map((page) => ({
                id: page.url,
                at: new Date().toISOString(),
                summary: page.title,
                data: { url: page.url, status: page.status, title: page.title },
            })),
            tile: {
                title: "Competitor research",
                status: pages.some((p) => p.status === 0) ? "degraded" : "ok",
                lines: [{ label: "Pages", value: String(pages.length) }],
            },
        };
    },
    async propose(params) {
        const cards = (params.observations ?? []).map((row) => {
            const data = (row.data ?? {});
            const title = data.title || row.summary;
            return {
                title: title === "Could not fetch" ? "Page unavailable" : title,
                summary: (data.url || "page") + (data.status ? " · " + String(data.status) : ""),
                body: "Public page: " + (data.url || "") + "\nTitle: " + title,
                kind: "action",
                permission: "web.fetch",
                effects: [],
                editable: false,
            };
        });
        return { cards };
    },
    async act() {
        return { status: "done", summary: "Recorded the page check. Nothing was sent." };
    },
    async report() {
        return { metrics: [{ key: "fetches", label: "Fetches", value: fetches }] };
    },
    async shutdown() {
        return { ok: true };
    },
};
serve(plugin);
