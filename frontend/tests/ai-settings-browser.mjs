// UI contract tests with synthetic responses only. Never sends a provider request.
import { chromium } from "playwright";
import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
const artifacts = fileURLToPath(new URL("../test-results/", import.meta.url));
const browser = await chromium.launch({
  headless: true,
  ...(process.env.CHROMIUM_PATH
    ? { executablePath: process.env.CHROMIUM_PATH }
    : {}),
});
const context = await browser.newContext({
  viewport: { width: 1440, height: 1080 },
});
const page = await context.newPage();
const failures = [];
page.on("pageerror", (e) => failures.push(e.message));
const empty = {
  version: 0,
  mode: "personal",
  enabled: false,
  revision: null,
  provider_id: null,
  model_id: null,
  has_key: false,
  test_status: "untested",
  last_tested_at: null,
  error_code: null,
  daily_request_limit: 20,
  platform_available: false,
};
let config = { ...empty },
  available = true,
  loseTest = true,
  conflict = false,
  puts = 0,
  testKeys = [];
const receipts = new Map();
await page.route(
  (url) => url.pathname.startsWith("/api/"),
  async (route) => {
    const r = route.request(),
      path = new URL(r.url()).pathname,
      method = r.method();
    const reply = (body, status = 200) =>
      route.fulfill({
        status,
        contentType: "application/json",
        body: JSON.stringify(body),
      });
    if (path === "/api/auth/me")
      return reply({
        user: {
          id: "20000000-0000-4000-8000-000000000001",
          email: "ui-test@example.test",
        },
      });
    if (path === "/api/auth/csrf")
      return reply({ csrf_token: "synthetic-csrf-context" });
    if (path === "/api/ai/providers")
      return reply({
        providers: [
          { id: "openai", name: "OpenAI", models: ["gpt-4.1-mini", "gpt-4.1"] },
        ],
        available,
        platform_available: false,
        generation_available: false,
        test_notice: "Synthetic notice",
        test_limits: { per_minute: 3, per_day: 20 },
        max_daily_request_limit: 100,
      });
    if (path === "/api/me/ai-config" && method === "GET") return reply(config);
    if (method !== "GET")
      assert.equal(r.headers()["x-csrf-token"], "synthetic-csrf-context");
    if (path === "/api/me/ai-config" && method === "PUT") {
      puts++;
      const b = r.postDataJSON();
      assert.ok(!("owner_id" in b));
      if (conflict) {
        conflict = false;
        config.version++;
        return reply({ code: "config_version_conflict" }, 409);
      }
      assert.equal(b.expected_version, config.version);
      const changed = !!b.api_key || b.model_id !== config.model_id;
      config = {
        ...config,
        version: config.version + 1,
        provider_id: b.provider_id,
        model_id: b.model_id,
        has_key: true,
        daily_request_limit: b.daily_request_limit,
        ...(changed
          ? {
              revision: (config.revision ?? 0) + 1,
              enabled: false,
              test_status: "untested",
            }
          : {}),
      };
      return reply(config);
    }
    if (path === "/api/me/ai-config/test") {
      const key = r.headers()["idempotency-key"];
      testKeys.push(key);
      if (receipts.has(key)) return reply(receipts.get(key));
      assert.equal(r.postDataJSON().expected_version, config.version);
      config = {
        ...config,
        version: config.version + 2,
        test_status: "succeeded",
        last_tested_at: new Date().toISOString(),
      };
      receipts.set(key, { ...config });
      if (loseTest) {
        loseTest = false;
        return route.abort("connectionfailed");
      }
      return reply(config);
    }
    if (path === "/api/me/ai-config" && method === "PATCH") {
      const b = r.postDataJSON();
      assert.equal(b.expected_version, config.version);
      config = { ...config, version: config.version + 1, enabled: b.enabled };
      return reply(config);
    }
    if (path === "/api/me/ai-config" && method === "DELETE") {
      assert.equal(r.headers()["if-match"], `"ai-${config.version}"`);
      config = { ...empty, version: config.version + 1 };
      return reply(config);
    }
    throw new Error(`Unexpected API call: ${method} ${path}`);
  },
);
await mkdir(artifacts, { recursive: true });
try {
  await page.goto(
    (process.env.E2E_URL ?? "http://127.0.0.1:5194") + "/settings/ai",
  );
  await page
    .getByRole("heading", { name: "Personal connection", exact: true })
    .waitFor();
  assert.equal(
    await page
      .getByRole("button", { name: "Save connection", exact: true })
      .isDisabled(),
    true,
  );
  await page.screenshot({
    path: artifacts + "ai-desktop.png",
    fullPage: true,
  });
  const key = page.getByLabel("API key", { exact: true });
  await key.fill("synthetic-only-not-a-real-key");
  await page.getByRole("button", { name: "Show API key", exact: true }).click();
  assert.equal(await key.getAttribute("type"), "text");
  await page
    .getByRole("button", { name: "Save connection", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Test connection", exact: true })
    .waitFor();
  assert.equal(
    await page.getByLabel("Replace API key", { exact: true }).inputValue(),
    "",
  );
  await page
    .getByRole("button", { name: "Test connection", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Check same test", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Enable connection", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Disable connection", exact: true })
    .waitFor();
  assert.equal(testKeys.length, 2);
  assert.equal(testKeys[0], testKeys[1]);
  assert.equal(receipts.size, 1);
  await page.getByLabel("Model", { exact: true }).selectOption("gpt-4.1");
  assert.equal(
    await page
      .getByRole("button", { name: "Test again", exact: true })
      .isDisabled(),
    true,
  );
  conflict = true;
  await page
    .getByRole("button", { name: "Save connection", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Review latest settings", exact: true })
    .click();
  assert.equal(
    await page.getByLabel("Model", { exact: true }).inputValue(),
    "gpt-4.1",
  );
  await page
    .getByRole("button", { name: "Save connection", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Test connection", exact: true })
    .waitFor();
  assert.equal(puts, 3);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: artifacts + "ai-mobile.png", fullPage: true });
  assert.equal(
    await page.evaluate(
      () => document.documentElement.scrollWidth > innerWidth,
    ),
    false,
  );
  await page
    .getByRole("button", { name: "Remove connection", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Keep connection", exact: true })
    .click();
  assert.equal(config.has_key, true);
  await page
    .getByRole("button", { name: "Remove connection", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Remove saved connection", exact: true })
    .click();
  await page.getByLabel("API key", { exact: true }).waitFor();
  assert.equal(config.has_key, false);
  assert.equal(
    await page.evaluate(() => localStorage.length + sessionStorage.length),
    0,
  );
  available = false;
  await page.reload();
  await page
    .getByText(
      "Personal AI connections are not available on this installation yet.",
      { exact: false },
    )
    .waitFor();
  assert.equal(
    await page.getByLabel("API key", { exact: true }).isDisabled(),
    true,
  );
  assert.deepEqual(failures, []);
  console.log(
    "AI settings UI passed: save, secret clearing, safe test replay, explicit enable, conflict review, model replacement, delete confirmation, unavailable state, mobile overflow.",
  );
} finally {
  await browser.close();
}
