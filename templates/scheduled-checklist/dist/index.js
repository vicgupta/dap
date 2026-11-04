// DAP_SCAFFOLD_STARTER
import { serve } from "../vendor/plugin-sdk/dist/index.js";
let runs = 0;
let steps = [];
function asSteps(value) {
    if (!Array.isArray(value))
        return [];
    return value.filter((item) => item && typeof item === "object");
}
function effectsOf(step) {
    const out = [];
    for (const raw of step.effects ?? []) {
        if (raw === "sends" || raw === "spends" || raw === "deletes")
            out.push(raw);
    }
    return out;
}
const plugin = {
    async initialize(params) {
        steps = asSteps(params.config?.steps);
        return { ready: true, interface: "dap.plugin.v1", health: { status: "ok", message: steps.length + " steps" } };
    },
    async watch() {
        runs += 1;
        return {
            observations: steps.map((step, i) => ({
                id: step.id || "step-" + String(i),
                at: new Date().toISOString(),
                summary: step.title || "Step",
                data: { detail: step.detail || "", effects: effectsOf(step) },
            })),
            tile: {
                title: "Scheduled checklist",
                status: "ok",
                lines: [{ label: "Steps", value: String(steps.length) }],
            },
        };
    },
    async propose(params) {
        const cards = (params.observations ?? []).map((row) => {
            const data = (row.data ?? {});
            const effects = Array.isArray(data.effects) ? data.effects : [];
            const pauses = effects.length > 0;
            return {
                title: row.summary,
                summary: pauses ? "This step waits for approval." : "This step does not send, spend, or delete.",
                body: data.detail || row.summary,
                kind: "action",
                permission: "workflow.run",
                effects,
                editable: pauses,
            };
        });
        return { cards };
    },
    async act(params) {
        const title = params.card.title || "Step";
        return { status: "done", summary: "Finished “" + title + "”." };
    },
    async report() {
        return { metrics: [{ key: "runs", label: "Runs", value: runs }] };
    },
    async shutdown() {
        return { ok: true };
    },
};
serve(plugin);
