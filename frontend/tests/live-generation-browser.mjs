// Synthetic UI/API boundary test. No provider request or real user data.
import { chromium } from "playwright";
import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
const fullResume = process.env.FULL_RESUME === "1";
const fullText =
  "Alex Example | alex@example.test\nEXPERIENCE: Backend Engineer, Northstar, 2022–2025. Built Go APIs and reduced batch processing time by 20%.\nEDUCATION: Bachelor of Computing, Example University, 2021.\nSKILLS: Go, PostgreSQL, TypeScript.";
let confirmedFacts;
let documentReads = 0;
let exportedRevision;
let downloadRequests = 0;
const relay = process.env.AI_TEST_PROVIDER === "aiwanwu";
const providerId = relay ? "aiwanwu" : "openai";
const providerName = relay ? "aiwanwu (third-party relay)" : "OpenAI";
const modelId = relay ? "gpt-6-sol" : "gpt-4.1-mini";
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
        provider_id: providerId,
        model_id: modelId,
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
        providers: [{ id: providerId, name: providerName, models: [modelId] }],
      });
    if (fullResume && p === "/api/files" && m === "POST")
      return reply(
        { id, original_name: "Full resume.pdf", media_type: "application/pdf" },
        201,
      );
    if (fullResume && p === "/api/resumes" && m === "POST")
      return reply(
        {
          resume: {
            id,
            name: "Full resume.pdf",
            source_file_id: id,
            version: 1,
            current_revision_id: null,
          },
          task_id: null,
        },
        201,
      );
    if (p === `/api/resumes/${id}/source-text`)
      return reply({
        page_count: 1,
        facts: [
          {
            id: revision,
            category: "other",
            text: fullText,
            evidence: { source: "file", page: 1, excerpt: fullText },
          },
        ],
      });
    if (p === `/api/resumes/${id}/revisions` && m === "POST") {
      confirmedFacts = r.postDataJSON().facts;
      assert.equal(confirmedFacts[0].text, fullText);
      return reply(
        {
          resume_version: 2,
          revision: { id: revision, resume_id: id, facts: confirmedFacts },
        },
        201,
      );
    }
    if (p === `/api/resumes/${id}`)
      return reply({ id, current_revision_id: revision, version: 2 });
    if (p === `/api/resumes/${id}/revisions/${revision}`)
      return reply({
        id: revision,
        resume_id: id,
        facts: confirmedFacts ?? [
          {
            id: sourceID,
            category: "experience",
            text: "Old short handwritten summary",
            evidence: { source: "user", page: null, excerpt: "" },
          },
        ],
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
        model_id: modelId,
        resume_task: {
          task_id: id,
          status: fullResume ? "completed" : "failed",
          error_code: fullResume ? null : "provider_timeout",
        },
        cover_letter_task: { task_id: revision, status: "completed" },
      });
    if (p === `/api/workspaces/${id}/documents`)
      return reply({
        items: fullResume
          ? [
              {
                id,
                kind: "resume",
                current_revision_id:
                  ++documentReads === 1 ? sourceID : revision,
                version: documentReads === 1 ? 1 : 2,
              },
            ]
          : [],
      });
    if (fullResume && p === `/api/documents/${id}/revisions/${sourceID}`)
      return reply({
        id: sourceID,
        document_id: id,
        content: { kind: "resume", title: "Old short draft", sections: [] },
      });
    if (fullResume && p === `/api/files/${revision}/download`) {
      if (++downloadRequests === 1) return reply({ code: "unavailable" }, 503);
      return route.fulfill({
        status: 200,
        contentType: "application/pdf",
        body: "%PDF-1.4 synthetic download test",
      });
    }
    if (fullResume && p === `/api/documents/${id}/exports`) {
      assert.equal(r.postDataJSON().template_version, "2");
      exportedRevision = r.postDataJSON().revision_id;
      assert.equal(exportedRevision, revision);
      return reply({
        revision_id: revision,
        task_id: runID,
        file_id: process.env.QUEUED_EXPORT ? null : revision,
        format: "pdf",
      });
    }
    if (fullResume && p === `/api/tasks/${runID}`)
      return reply({
        task_id: runID,
        status: "completed",
        result: { kind: "export", file_id: revision },
      });
    if (fullResume && p === `/api/documents/${id}/revisions/${revision}`)
      return reply({
        id: revision,
        document_id: id,
        change_summary: "Tailored from the complete confirmed source",
        content: {
          kind: "resume",
          title: "Alex Example",
          sections: [
            {
              heading: "Professional Summary",
              items: [
                {
                  text: "Backend engineer building reliable Go services and PostgreSQL applications.",
                  fact_ids: [revision],
                },
              ],
            },
            {
              heading: "Experience",
              items: [
                {
                  text: "Backend Engineer | Northstar | 2022–2025\nBuilt Go APIs and reduced batch processing time by 20%.",
                  fact_ids: [revision],
                },
              ],
            },
            {
              heading: "Education",
              items: [
                {
                  text: "Bachelor of Computing | Example University | 2021",
                  fact_ids: [revision],
                },
              ],
            },
          ],
        },
      });
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
    .fill(
      fullResume
        ? "Company: Northstar\nRole: Backend Engineer\nBuild Go services at Northstar."
        : "Build Go services at Northstar.",
    );
  if (fullResume) {
    await page
      .getByRole("button", { name: "Upload resume", exact: true })
      .click();
    await page.getByLabel("Upload base resume").setInputFiles({
      name: "Full resume.pdf",
      mimeType: "application/pdf",
      buffer: Buffer.from("synthetic-upload-only"),
    });
    await page.getByLabel("Resume text · part 1", { exact: true }).waitFor();
    assert.equal(
      await page
        .getByLabel("Resume text · part 1", { exact: true })
        .inputValue(),
      fullText,
    );
    await page.screenshot({
      path: artifacts + "full-resume-import.png",
      fullPage: true,
    });
    await page
      .getByRole("button", { name: "Confirm resume & use it", exact: true })
      .click();
  } else
    await page
      .getByLabel("Your base resume", { exact: true })
      .selectOption(revision);
  assert.equal(await page.getByLabel("Company", { exact: true }).count(), 0);
  assert.equal(await page.getByLabel("Role title", { exact: true }).count(), 0);
  assert.equal(
    await page
      .getByText("Is this the right opportunity?", { exact: true })
      .count(),
    0,
  );
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
  assert.equal(confirmedFacts[0].text, fullText);
  await page.getByText(`AI drafts · ${modelId}`, { exact: true }).waitFor();
  if (fullResume) {
    await page
      .getByRole("heading", { name: "Alex Example", exact: true })
      .waitFor();
    const downloaded = page.waitForEvent("download");
    await page
      .getByRole("button", { name: "Export PDF ↗", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Retry download", exact: true })
      .click();
    const file = await downloaded;
    assert.equal(file.suggestedFilename(), `resume-${revision}.pdf`);
    await page.getByText("Download started.", { exact: true }).waitFor();
    const again = page.waitForEvent("download");
    await page
      .getByRole("button", { name: "Export PDF ↗", exact: true })
      .click();
    await again;
    assert.equal(downloadRequests, 3);
    assert.equal(
      await page
        .getByRole("link", { name: "Download PDF", exact: true })
        .count(),
      0,
    );
    assert.equal(exportedRevision, revision);
    assert.equal(
      await page.getByLabel("Document title", { exact: true }).count(),
      0,
    );
    await page.setViewportSize({ width: 1440, height: 1080 });
    await page.screenshot({
      path: artifacts + "full-resume-preview.png",
      fullPage: true,
    });
    await page
      .getByRole("button", { name: "Edit document", exact: true })
      .click();
    await page
      .getByLabel("Document title", { exact: true })
      .fill("Alex Example — tailored resume");
    await page
      .getByRole("button", { name: "Preview document", exact: true })
      .click();
    await page
      .getByRole("heading", {
        name: "Alex Example — tailored resume",
        exact: true,
      })
      .waitFor();
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({
      path: artifacts + "full-resume-preview-mobile.png",
      fullPage: true,
    });
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth > innerWidth,
      ),
      false,
    );
  } else
    await page.getByText("The provider timed out.", { exact: false }).waitFor();
  assert.ok(generated);
  assert.deepEqual(crashes, []);
  console.log(
    "Personal-generation UI passed: explicit opt-in, data/cost notice, pinned revision request, mobile layout, truthful model and timeout display.",
  );
} catch (error) {
  await page.screenshot({
    path: artifacts + "live-test-failure.png",
    fullPage: true,
  });
  console.error(crashes, await page.locator("body").innerText());
  throw error;
} finally {
  await browser.close();
}
