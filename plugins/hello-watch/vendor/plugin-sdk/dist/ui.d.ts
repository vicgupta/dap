export type ThemeMode = "system" | "light" | "dark";
export interface PluginBoot {
    token: string;
    api: string;
    plugin: string;
    inbox: string;
    theme: ThemeMode;
}
export declare function readBoot(): PluginBoot;
export declare function applyTheme(theme: ThemeMode): void;
export declare function pluginFetch(boot: PluginBoot, path: string, body?: unknown): Promise<unknown>;
