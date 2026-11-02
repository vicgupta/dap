import type { CardDraft, CheckRequest, CheckResult, DraftRequest, Evidence, InitParams, MailProvider, MailQuery, SendRequest, Tile } from "./types.js";
export declare class Host {
    private readonly init;
    constructor(init: InitParams);
    get config(): Record<string, unknown>;
    get dataDir(): string;
    private call;
    context(): Promise<unknown>;
    state(): Promise<{
        state: unknown;
    }>;
    putState(state: unknown): Promise<unknown>;
    log(message: string, evidence?: Evidence[]): Promise<unknown>;
    tile(tile: Tile): Promise<unknown>;
    propose(card: CardDraft): Promise<unknown>;
    check(req: CheckRequest): Promise<CheckResult>;
    secretGet(account: string): Promise<{
        value?: string;
    }>;
    secretSet(account: string, value: string): Promise<unknown>;
    secretDelete(account: string): Promise<unknown>;
    mailThreads(provider: MailProvider, query?: MailQuery): Promise<{
        threads: unknown[];
    }>;
    mailDraft(req: DraftRequest): Promise<unknown>;
    mailSend(req: SendRequest): Promise<unknown>;
}
