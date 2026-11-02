export type HealthStatus = "ok" | "degraded" | "error";
export interface Health {
    status: HealthStatus;
    message?: string;
}
export interface Evidence {
    label: string;
    ref?: string;
}
export interface TileLine {
    label: string;
    value: string;
}
export interface Tile {
    title?: string;
    status?: string;
    lines?: TileLine[];
}
export interface Observation {
    id: string;
    at: string;
    summary: string;
    data?: Record<string, unknown>;
}
export interface Metric {
    key: string;
    label: string;
    value: unknown;
}
export type CardKind = "action" | "change";
export type Effect = "sends" | "spends" | "deletes";
export interface CardDraft {
    title: string;
    summary?: string;
    body?: string;
    kind?: CardKind;
    permission?: string;
    effects?: Effect[];
    editable?: boolean;
    payload?: Record<string, unknown>;
    evidence?: Evidence[];
}
export interface Card extends CardDraft {
    id: string;
    plugin?: string;
    status?: string;
}
export interface InitParams {
    interface: string;
    dap_version: string;
    config: Record<string, unknown>;
    plugin_dir: string;
    data_dir: string;
    api_base: string;
    token: string;
}
export interface InitResult {
    ready: boolean;
    interface: string;
    health: Health;
}
export interface WatchParams {
    since?: string;
}
export interface WatchResult {
    observations: Observation[];
    tile?: Tile;
}
export interface ProposeParams {
    observations: Observation[];
}
export interface ProposeResult {
    cards: CardDraft[];
}
export interface ActParams {
    card: Card;
}
export interface ActResult {
    status: "done" | "failed";
    summary: string;
    evidence?: Evidence[];
    output?: Record<string, unknown>;
}
export interface ReportParams {
    period: string;
    from: string;
    to: string;
}
export interface ReportResult {
    metrics: Metric[];
}
export interface ShutdownParams {
    reason?: string;
}
export interface ShutdownResult {
    ok: boolean;
}
export interface PluginHandler {
    initialize(params: InitParams): InitResult | Promise<InitResult>;
    watch(params: WatchParams): WatchResult | Promise<WatchResult>;
    propose(params: ProposeParams): ProposeResult | Promise<ProposeResult>;
    act(params: ActParams): ActResult | Promise<ActResult>;
    report(params: ReportParams): ReportResult | Promise<ReportResult>;
    shutdown?(params: ShutdownParams): ShutdownResult | Promise<ShutdownResult>;
}
export interface CheckRequest {
    permission?: string;
    effects?: Effect[];
    card_id?: string;
}
export interface CheckResult {
    ok: boolean;
    decision: "allowed" | "ask_first" | "never";
    reason?: string;
}
export interface SendRequest {
    provider: "gmail" | "graph" | "fake";
    card_id: string;
    to: string;
    subject: string;
    body: string;
}
