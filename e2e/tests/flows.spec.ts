import fs from "fs";
import path from "path";
import { test, expect, type Browser, type Page } from "@playwright/test";

const shotRoot = process.env.DAP_SHOT_DIR
  || (fs.existsSync("/opt/cursor/artifacts") ? "/opt/cursor/artifacts/screenshots" : path.join(__dirname, "..", "artifacts"));

test.beforeEach(async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Home" })).toBeVisible();
});

test("home tiles switch layout", async ({ page }) => {
  const tile = page.locator('[data-card-id="hello-watch"]');
  await expect(tile).toBeVisible();
  await expect(tile.getByRole("heading", { name: "hello-watch" })).toBeVisible();
  await page.locator('[data-act="layout-set"][data-value="list"]').click();
  await expect(page.locator(".cards.list")).toBeVisible();
  await page.locator('[data-act="layout-set"][data-value="grid"]').click();
  await expect(page.locator(".cards.grid")).toBeVisible();
  await page.locator('[data-act="density-set"][data-value="compact"]').click();
  await expect(page.locator("html")).toHaveAttribute("data-density", "compact");
});

test("decision inbox approve", async ({ page }) => {
  await page.evaluate(async () => {
    const token = document.querySelector('meta[name="dap-token"]')?.getAttribute("content") || "";
    const res = await fetch("/api/propose", {
      method: "POST",
      headers: { Authorization: "Bearer " + token, "Content-Type": "application/json" },
      body: JSON.stringify({
        plugin: "hello-watch",
        title: "Say hello",
        summary: "A greeting is waiting",
        permission: "demo.greet",
        effects: [],
        actor: "external",
      }),
    });
    if (!res.ok) throw new Error(await res.text());
  });
  await page.goto("/#/inbox");
  await expect(page.getByRole("heading", { name: "Decision inbox" })).toBeVisible();
  const card = page.locator("article.decision", { hasText: "Say hello" });
  await expect(card).toBeVisible();
  await card.getByRole("button", { name: "Approve" }).click();
  await expect(card).toHaveCount(0);
});

test("connections page", async ({ page }) => {
  await page.goto("/#/connections");
  await expect(page.getByRole("heading", { name: "Connections" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Composio" })).toBeVisible();
  await expect(page.getByText("Demo mailbox")).toBeVisible();
  await expect(page.locator("[data-composio-key]")).toBeVisible();
  await expect(page.getByRole("button", { name: "Save key" })).toBeVisible();
});

test("create plugin page", async ({ page }) => {
  await page.goto("/#/create");
  await expect(page.getByRole("heading", { name: "Create plugin" })).toBeVisible();
  await expect(page.locator('textarea[name="description"]')).toBeVisible();
  await expect(page.getByRole("button", { name: /Watcher/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /Capability/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /Workflow/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /Create draft and run agent/ })).toBeEnabled();
});

test("chat streams a fake provider reply", async ({ page }) => {
  await page.goto("/#/chat");
  await expect(page.getByRole("heading", { name: "Chat" })).toBeVisible();
  await expect(page.getByRole("radio", { name: /fake 9\.9\.9/ })).toBeVisible();
  await ask(page, "Say hello");
  await expect(page.locator("#chat-thread")).toContainText("Hello from the browser.", { timeout: 20_000 });
  await expect(page.locator(".chat-row.user")).toContainText("Say hello");
  await expect(page.locator("form.chat-composer button")).toHaveText("Send");
});

test("chat shortcut is Ctrl+Shift+Period", async ({ page }) => {
  await page.keyboard.press("Control+Shift+Period");
  await expect(page.getByRole("heading", { name: "Chat" })).toBeVisible();
  await expect(page.locator(".keys")).toContainText(/Ctrl\+Shift\+|⌘⇧\./);
});

test("home and chat screenshots", async ({ browser, browserName, baseURL }) => {
  test.skip(browserName === "chromium", "Firefox and WebKit are the requested artifacts");
  fs.mkdirSync(shotRoot, { recursive: true });
  for (const theme of ["light", "dark"] as const) {
    const home = await openThemed(browser, baseURL!, theme, "/");
    await expect(home.page.getByRole("heading", { name: "Home" })).toBeVisible();
    await expect(home.page.locator('[data-card-id="hello-watch"]')).toBeVisible();
    await home.page.screenshot({ path: path.join(shotRoot, `home_${browserName}_${theme}.png`) });
    await home.context.close();

    const chat = await openThemed(browser, baseURL!, theme, "/#/chat");
    await expect(chat.page.getByRole("radio", { name: /fake 9\.9\.9/ })).toBeVisible();
    await ask(chat.page, "Show me a reply");
    await expect(chat.page.locator("#chat-thread")).toContainText("Hello from the browser.", { timeout: 20_000 });
    await expect(chat.page.locator("form.chat-composer button")).toHaveText("Send");
    await chat.page.screenshot({ path: path.join(shotRoot, `chat_${browserName}_${theme}.png`) });
    await chat.context.close();
  }
});

async function ask(page: Page, text: string) {
  const input = page.locator("#chat-input");
  await expect(input).toBeEnabled();
  await input.click();
  await page.evaluate((message) => {
    const box = document.querySelector("#chat-input");
    if (!(box instanceof HTMLTextAreaElement)) throw new Error("chat input missing");
    box.focus();
    box.value = message;
    box.dispatchEvent(new Event("input", { bubbles: true }));
    box.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true }));
  }, text);
}

async function openThemed(browser: Browser, baseURL: string, theme: "light" | "dark", route: string) {
  const context = await browser.newContext({
    viewport: { width: 1280, height: 800 },
    colorScheme: theme,
    deviceScaleFactor: 1,
  });
  await context.addInitScript((mode) => {
    localStorage.setItem("dap.theme", mode);
  }, theme);
  const page = await context.newPage();
  await page.goto(baseURL + route);
  await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
  return { context, page };
}
