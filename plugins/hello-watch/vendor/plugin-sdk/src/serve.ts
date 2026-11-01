import { Host } from "./host.js";
import type { ActParams, InitParams, PluginHandler, ProposeParams, ReportParams, WatchParams } from "./types.js";

interface RpcRequest {
  id?: unknown;
  method?: string;
  params?: unknown;
}

function write(id: unknown, result?: unknown, error?: string): void {
  const msg: Record<string, unknown> = { jsonrpc: "2.0", id };
  if (error !== undefined) msg.error = { code: -32601, message: error };
  else msg.result = result;
  process.stdout.write(JSON.stringify(msg) + "\n");
}

export function serve(handler: PluginHandler): void {
  let host: Host | undefined;
  let buf = "";
  process.stdin.setEncoding("utf8");
  process.stdin.on("data", (chunk: string) => {
    buf += chunk;
    let nl = buf.indexOf("\n");
    while (nl >= 0) {
      const line = buf.slice(0, nl).trim();
      buf = buf.slice(nl + 1);
      nl = buf.indexOf("\n");
      if (!line) continue;
      void handleLine(line);
    }
  });

  async function handleLine(line: string): Promise<void> {
    let req: RpcRequest;
    try {
      req = JSON.parse(line) as RpcRequest;
    } catch (err) {
      write(null, undefined, err instanceof Error ? err.message : String(err));
      return;
    }
    try {
      const params = (req.params ?? {}) as Record<string, unknown>;
      let result: unknown;
      switch (req.method) {
        case "initialize": {
          const init = params as unknown as InitParams;
          host = new Host(init);
          result = await handler.initialize(init);
          break;
        }
        case "watch":
          result = await handler.watch(params as unknown as WatchParams);
          break;
        case "propose":
          result = await handler.propose(params as unknown as ProposeParams);
          break;
        case "act":
          result = await handler.act(params as unknown as ActParams);
          break;
        case "report":
          result = await handler.report(params as unknown as ReportParams);
          break;
        default:
          throw new Error("unknown method " + String(req.method));
      }
      write(req.id, result);
    } catch (err) {
      write(req.id, undefined, err instanceof Error ? err.message : String(err));
    }
  }
}

export function currentHost(handlerHost: Host | undefined): Host {
  if (!handlerHost) throw new Error("host calls are available after initialize");
  return handlerHost;
}

export type { Host };
