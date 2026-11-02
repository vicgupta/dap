export type ThemeMode = "system" | "light" | "dark";

export interface PluginBoot {
  token: string;
  api: string;
  plugin: string;
  inbox: string;
  theme: ThemeMode;
}

function meta(name: string): string {
  if (typeof document === "undefined") return "";
  const el = document.querySelector(`meta[name="${name}"]`);
  return el ? el.getAttribute("content") || "" : "";
}

export function readBoot(): PluginBoot {
  let theme: ThemeMode = "system";
  try {
    const raw = new URLSearchParams(location.search).get("theme") || document.documentElement.getAttribute("data-theme") || "system";
    if (raw === "light" || raw === "dark" || raw === "system") theme = raw;
  } catch {
    theme = "system";
  }
  return {
    token: meta("dap-token"),
    api: meta("dap-api") || "/api",
    plugin: meta("dap-plugin"),
    inbox: meta("dap-inbox"),
    theme,
  };
}

export function applyTheme(theme: ThemeMode): void {
  document.documentElement.setAttribute("data-theme", theme);
}

export async function pluginFetch(boot: PluginBoot, path: string, body?: unknown): Promise<unknown> {
  const res = await fetch(boot.api.replace(/\/$/, "") + path, {
    method: body === undefined ? "GET" : "POST",
    headers: {
      Authorization: "Bearer " + boot.token,
      "Content-Type": "application/json",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = (await res.json().catch(() => ({}))) as { ok?: boolean; error?: string; reason?: string };
  if (!res.ok || data.ok === false) throw new Error(data.error || data.reason || "request failed");
  return data;
}
