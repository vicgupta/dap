import { Host } from "./host.js";
import type { PluginHandler } from "./types.js";
export declare function serve(handler: PluginHandler): void;
export declare function currentHost(handlerHost: Host | undefined): Host;
export type { Host };
