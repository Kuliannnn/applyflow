// Synthetic UI/API boundary test. No provider request or real user data.
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
const page = await browser.newPage({ viewport: { width: 1440, height: 1080 } });
const id = "20000000-0000-4000-8000-000000000001",
  revision = "20000000-0000-4000-8000-000000000002",
  sourceID = "20000000-0000-4000-8000-000000000003",
  runID = "20000000-0000-4000-8000-000000000004";
let w = {
  id,
  version: 1,
  title: "Untitled application",
  resume_revision_id: null,
  job_revision_id: null,
  current_source_id: null,
  current_run_id: null,
};
let source, job, generated;
const crashes = [];
page.on("pageerror", (e) => crashes.push(e.message));
await page.route(
  (url) => url.pathname.startsWith("/api/"),
  async (route) => {
    const r = route.request(),
      p = new URL(r.url()).pathname,
      m = r.method();
    const reply = (body, status = 200) =>
      route.fulfill({
        status,
        contentType: "application/json",
        body: JSON.stringify(body),
      });
    if (p === "/api/auth/me")
      return reply({
        user: { id, email: "synthetic@example.test", profile_version: 1 },
      });
    if (p === "/api/auth/csrf") return reply({ csrf_token: "test-token" });
    if (p === "/api/me/ai-config")
      return reply({
        version: 4,
        mode: "personal",
        enabled: true,
        revision: 1,
        provider_id: "openai",
        model_id: "gpt-4.1-mini",
        has_key: true,
        test_status: "succeeded",
        last_tested_at: null,
        error_code: null,
        daily_request_limit: 20,
        platform_available: false,
      });
    if (p === "/api/ai/providers")
      return reply({
        generation_available: true,
        available: true,
        providers: [{ id: "openai", name: "OpenAI", models: ["gpt-4.1-mini"] }],
      });
    if (p === "/api/resumes")
      return reply({
        items: [
          { id, name: "Confirmed resume", current_revision_id: revision },
        ],
        next_cursor: null,
      });
    if (p === "/api/workspaces")
      return reply(
        m === "GET" ? { items: [], next_cursor: null } : w,
        m === "GET" ? 200 : 201,
      );
    if (p === `/api/workspaces/${id}`) {
      if (m === "PATCH") {
        const b = r.postDataJSON();
        assert.equal(b.expected_version, w.version);
        w = { ...w, ...b, version: w.version + 1 };
      }
      return reply(w);
    }
    if (p === `/api/workspaces/${id}/sources`) {
      source = {
        id: sourceID,
        source_version: 1,
        source: r.postDataJSON().source,
      };
      w = { ...w, version: w.version + 1, current_source_id: sourceID };
      return reply(
        { source, task_id: sourceID, workspace_version: w.version },
        202,
      );
    }
    if (p === `/api/workspaces/${id}/sources/${sourceID}`) return reply(source);
    if (p === `/api/workspaces/${id}/job-revisions`) {
      job = { ...r.postDataJSON(), id: revision };
      w = { ...w, version: w.version + 1, job_revision_id: revision };
      return reply({ revision: job, workspace_version: w.version }, 201);
    }
    if (p === `/api/workspaces/${id}/job-revisions/${revision}`)
      return reply(job);
    if (p === `/api/workspaces/${id}/generations`) {
      generated = r.postDataJSON();
      assert.equal(generated.execution_mode, "personal");
      assert.equal(generated.ai_revision, 1);
      assert.equal(generated.expected_version, w.version);
      assert.ok(r.headers()["idempotency-key"]);
      w = { ...w, current_run_id: runID, version: w.version + 1 };
      return reply(
        {
          run_id: runID,
          workspace_id: id,
          workspace_version: w.version,
          resume_task_id: id,
          cover_letter_task_id: revision,
        },
        202,
      );
    }
    if (p === `/api/workspaces/${id}/generations/${runID}`)
      return reply({
        id: runID,
        execution_mode: "personal",
        model_id: "gpt-4.1-mini",
        resume_task: {
          task_id: id,
          status: "failed",
          error_code: "provider_timeout",
        },
        cover_letter_task: { task_id: revision, status: "completed" },
      });
    if (p === `/api/workspaces/${id}/documents`) return reply({ items: [] });
    throw new Error("Unexpected synthetic request: " + m + " " + p);
  },
);
await mkdir(artifacts, { recursive: true });
try {
  await page.goto((process.env.E2E_URL ?? "http://127.0.0.1:5194") + "/create");
  const mode = page.getByLabel("Generation mode", { exact: true });
  assert.equal(await mode.inputValue(), "mock");
  await page.waitForFunction(
    () =>
      !document.querySelector('#generation-mode option[value="personal"]')
        .disabled,
  );
  await mode.selectOption("personal");
  await page
    .getByText("for two separate paid calls.", { exact: false })
    .waitFor();
  await page
    .getByLabel("Job description", { exact: true })
    .fill("Build Go services at Northstar.");
  await page
    .getByLabel("Your base resume", { exact: true })
    .selectOption(revision);
  await page
    .getByRole("button", { name: "Create my drafts", exact: true })
    .click();
  await page.getByLabel("Company", { exact: true }).fill("Northstar");
  await page.getByLabel("Role title", { exact: true }).fill("Backend Engineer");
  await page.screenshot({
    path: artifacts + "live-create-desktop.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: artifacts + "live-create-mobile.png",
    fullPage: true,
  });
  assert.equal(
    await page.evaluate(
      () => document.documentElement.scrollWidth > innerWidth,
    ),
    false,
  );
  await page
    .getByRole("button", { name: "Generate with AI", exact: true })
    .click();
  await page.waitForURL("**/studio/" + id);
  await page.getByText("AI drafts · gpt-4.1-mini", { exact: true }).waitFor();
  await page.getByText("The provider timed out.", { exact: false }).waitFor();
  assert.ok(generated);
  assert.deepEqual(crashes, []);
  console.log(
    "Personal-generation UI passed: explicit opt-in, data/cost notice, pinned revision request, mobile layout, truthful model and timeout display.",
  );
} finally {
  await browser.close();
}
