import type { CardDraft, CheckRequest, CheckResult, Evidence, InitParams, SendRequest, Tile } from "./types.js";

export class Host {
  constructor(private readonly init: InitParams) {}

  get config(): Record<string, unknown> {
    return this.init.config;
  }

  get dataDir(): string {
    return this.init.data_dir;
  }

  private async call<T>(method: string, path: string, body?: unknown): Promise<T> {
    const res = await fetch(this.init.api_base.replace(/\/$/, "") + path, {
      method,
      headers: {
        Authorization: "Bearer " + this.init.token,
        "Content-Type": "application/json",
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const data = (await res.json().catch(() => ({}))) as T & { error?: string; reason?: string; ok?: boolean };
    if (!res.ok || data.ok === false) {
      throw new Error(data.error || data.reason || "host call failed");
    }
    return data;
  }

  context(): Promise<unknown> {
    return this.call("GET", "/context");
  }

  state(): Promise<{ state: unknown }> {
    return this.call("GET", "/state");
  }

  putState(state: unknown): Promise<unknown> {
    return this.call("PUT", "/state", state);
  }

  log(message: string, evidence?: Evidence[]): Promise<unknown> {
    return this.call("POST", "/log", { message, evidence });
  }

  tile(tile: Tile): Promise<unknown> {
    return this.call("POST", "/tile", tile);
  }

  propose(card: CardDraft): Promise<unknown> {
    return this.call("POST", "/propose", card);
  }

  check(req: CheckRequest): Promise<CheckResult> {
    return this.call("POST", "/check", req);
  }

  secretGet(account: string): Promise<{ value?: string }> {
    return this.call("POST", "/secret/get", { account });
  }

  secretSet(account: string, value: string): Promise<unknown> {
    return this.call("POST", "/secret/set", { account, value });
  }

  secretDelete(account: string): Promise<unknown> {
    return this.call("POST", "/secret/delete", { account });
  }

  mailThreads(provider: "gmail" | "graph" | "fake"): Promise<{ threads: unknown[] }> {
    return this.call("GET", "/mail/threads?provider=" + encodeURIComponent(provider));
  }

  mailSend(req: SendRequest): Promise<unknown> {
    return this.call("POST", "/mail/send", req);
  }
}
