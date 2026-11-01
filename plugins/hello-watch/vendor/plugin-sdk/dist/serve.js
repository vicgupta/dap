import { Host } from "./host.js";
function write(id, result, error) {
    const msg = { jsonrpc: "2.0", id };
    if (error !== undefined)
        msg.error = { code: -32601, message: error };
    else
        msg.result = result;
    process.stdout.write(JSON.stringify(msg) + "\n");
}
export function serve(handler) {
    let host;
    let buf = "";
    process.stdin.setEncoding("utf8");
    process.stdin.on("data", (chunk) => {
        buf += chunk;
        let nl = buf.indexOf("\n");
        while (nl >= 0) {
            const line = buf.slice(0, nl).trim();
            buf = buf.slice(nl + 1);
            nl = buf.indexOf("\n");
            if (!line)
                continue;
            void handleLine(line);
        }
    });
    async function handleLine(line) {
        let req;
        try {
            req = JSON.parse(line);
        }
        catch (err) {
            write(null, undefined, err instanceof Error ? err.message : String(err));
            return;
        }
        try {
            const params = (req.params ?? {});
            let result;
            switch (req.method) {
                case "initialize": {
                    const init = params;
                    host = new Host(init);
                    result = await handler.initialize(init);
                    break;
                }
                case "watch":
                    result = await handler.watch(params);
                    break;
                case "propose":
                    result = await handler.propose(params);
                    break;
                case "act":
                    result = await handler.act(params);
                    break;
                case "report":
                    result = await handler.report(params);
                    break;
                default:
                    throw new Error("unknown method " + String(req.method));
            }
            write(req.id, result);
        }
        catch (err) {
            write(req.id, undefined, err instanceof Error ? err.message : String(err));
        }
    }
}
export function currentHost(handlerHost) {
    if (!handlerHost)
        throw new Error("host calls are available after initialize");
    return handlerHost;
}
