import { serve, type ActParams, type PluginHandler, type ProposeParams } from "../vendor/plugin-sdk/dist/index.js";

let ticks = 0;
let acted = 0;

const plugin: PluginHandler = {
  async initialize() {
    return { ready: true, interface: "dap.plugin.v1", health: { status: "ok", message: "Watching" } };
  },
  async watch() {
    ticks += 1;
    const id = "tick-" + String(ticks);
    return {
      observations: [{ id, at: new Date().toISOString(), summary: "Watch ticked" }],
      tile: {
        title: "Hello watch",
        status: "ok",
        lines: [
          { label: "Ticks", value: String(ticks) },
          { label: "Hellos", value: String(acted) },
        ],
      },
    };
  },
  async propose(params: ProposeParams) {
    const ref = params.observations[0]?.id ?? "tick";
    return {
      cards: [{
        title: "Say hello",
        summary: "The watch ticked. Approving records a hello. Nothing is sent.",
        body: "Hello from hello-watch.\n\nThis card is the whole action. Approve it and the result lands in the activity ledger.",
        permission: "demo.greet",
        effects: [],
        kind: "action",
        editable: true,
        evidence: [{ label: "Watch", ref }],
      }],
    };
  },
  async act(params: ActParams) {
    acted += 1;
    const title = params.card.title || "Say hello";
    return {
      status: "done",
      summary: "Hello. Recorded “" + title + "”.",
      evidence: [{ label: "Hello", ref: "hello-watch" }],
    };
  },
  async report() {
    return {
      metrics: [
        { key: "ticks", label: "Ticks", value: ticks },
        { key: "hellos", label: "Hellos", value: acted },
      ],
    };
  },
};

serve(plugin);
