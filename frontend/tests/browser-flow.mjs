// Disposable stack only: creates synthetic account. BASE_RESUME must be a synthetic PDF/DOCX.
import { chromium } from "playwright";
import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
const base = process.env.E2E_URL ?? "http://127.0.0.1:5173";
if (!process.env.BASE_RESUME)
  throw new Error("Set BASE_RESUME to a synthetic PDF/DOCX");
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
const crashes = [];
page.on("pageerror", (e) => crashes.push(e.message));
await mkdir("test-results", { recursive: true });
try {
  await page.goto(base + "/register");
  await page
    .getByLabel("Email", { exact: true })
    .fill(`studio-${Date.now()}@example.test`);
  await page
    .getByLabel("Password", { exact: true })
    .fill("Synthetic-test-password-2026");
  await page
    .getByRole("button", { name: "Create account", exact: true })
    .click();
  await page.waitForURL("**/create");
  await page
    .getByRole("heading", { name: "Your next role. Your best story." })
    .waitFor();
  await page.screenshot({
    path: "test-results/create-desktop.png",
    fullPage: true,
  });
  await page
    .getByLabel("Job description", { exact: true })
    .fill(
      "Northstar is hiring a Backend Engineer. Build Go APIs and PostgreSQL services. Collaborate with product teams and maintain reliable systems.",
    );
  await page
    .getByRole("button", { name: "Upload resume", exact: true })
    .click();
  await page
    .getByLabel("Upload base resume")
    .setInputFiles(process.env.BASE_RESUME);
  await page
    .getByLabel("Resume text · part 1", { exact: true })
    .fill(
      "Built and maintained Go APIs and PostgreSQL services at Northstar from 2022 to 2025.",
    );
  await page
    .getByRole("button", { name: "Confirm resume & use it", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Generate sample drafts", exact: true })
    .click();
  await page.waitForURL("**/studio/**");
  await page
    .getByRole("button", { name: "Edit document", exact: true })
    .click();
  await page
    .getByLabel("Document title", { exact: true })
    .waitFor({ timeout: 20000 });
  const title = page.getByLabel("Document title", { exact: true });
  await title.fill("Alex Morgan — Backend Engineer");
  await page.getByRole("tab", { name: "Cover letter", exact: true }).click();
  if (
    await page
      .getByRole("button", { name: "Edit document", exact: true })
      .count()
  )
    await page
      .getByRole("button", { name: "Edit document", exact: true })
      .click();
  await page
    .getByLabel("Salutation", { exact: true })
    .waitFor({ timeout: 20000 });
  await page
    .getByLabel("Salutation", { exact: true })
    .fill("Dear Northstar Team,");
  await page.getByRole("tab", { name: "Resume", exact: true }).click();
  assert.equal(await title.inputValue(), "Alex Morgan — Backend Engineer");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await page
    .getByText(/^Saved · version /)
    .filter({ visible: true })
    .waitFor();
  await page.screenshot({
    path: "test-results/studio-desktop.png",
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Export PDF ↗", exact: true })
    .click();
  const pdf = page.getByRole("link", { name: "Download PDF", exact: true });
  await pdf.waitFor({ timeout: 20000 });
  const [download] = await Promise.all([
    page.waitForEvent("download"),
    pdf.click(),
  ]);
  await download.saveAs("test-results/resume.pdf");
  assert.match(download.suggestedFilename(), /\.pdf$/);
  await page.getByRole("tab", { name: "Cover letter", exact: true }).click();
  assert.equal(
    await page.getByLabel("Salutation", { exact: true }).inputValue(),
    "Dear Northstar Team,",
  );
  await page
    .getByRole("button", { name: "Word document ↗", exact: true })
    .click();
  const word = page.getByRole("link", { name: "Download DOCX", exact: true });
  await word.waitFor({ timeout: 20000 });
  const [docx] = await Promise.all([
    page.waitForEvent("download"),
    word.click(),
  ]);
  await docx.saveAs("test-results/letter.docx");
  await page.reload();
  await page.getByLabel("Salutation", { exact: true }).waitFor();
  assert.equal(
    await page.getByLabel("Salutation", { exact: true }).inputValue(),
    "Dear Northstar Team,",
  );
  // Two real tabs edit the same immutable base; a conflict must preserve local text.
  await page.getByRole("tab", { name: "Resume", exact: true }).click();
  const other = await context.newPage();
  await other.goto(page.url());
  await other
    .getByRole("button", { name: "Edit document", exact: true })
    .click();
  await other.getByLabel("Document title", { exact: true }).waitFor();
  await page
    .getByLabel("Document title", { exact: true })
    .fill("My unsaved local revision");
  await other
    .getByLabel("Document title", { exact: true })
    .fill("Saved in the other tab");
  await other
    .getByRole("button", { name: "Save changes", exact: true })
    .click();
  await other
    .getByText(/^Saved · version /)
    .filter({ visible: true })
    .waitFor();
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await page.getByRole("alert").first().waitFor();
  assert.equal(
    await page.getByLabel("Document title", { exact: true }).inputValue(),
    "My unsaved local revision",
  );
  await page
    .getByRole("button", { name: "Compare saved version", exact: true })
    .click();
  await page
    .getByRole("heading", { name: "Saved in the other tab", exact: true })
    .waitFor();
  assert.equal(
    await page.getByLabel("Document title", { exact: true }).inputValue(),
    "My unsaved local revision",
  );
  page.once("dialog", (dialog) => dialog.accept());
  await page
    .getByRole("button", { name: "Use latest saved version", exact: true })
    .click();
  await page.waitForFunction(
    () =>
      document.querySelector(".document-title")?.value ===
      "Saved in the other tab",
  );
  await other.close();
  await page.getByRole("tab", { name: "Cover letter", exact: true }).click();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.evaluate(async () => {
    window.scrollTo(0, 0);
    if (document.activeElement instanceof HTMLElement)
      document.activeElement.blur();
    await new Promise((resolve) =>
      requestAnimationFrame(() => requestAnimationFrame(resolve)),
    );
  });
  await page.screenshot({
    path: "test-results/studio-mobile.png",
    fullPage: true,
  });
  assert.ok(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
    "Mobile overflow",
  );
  await page.goto(base + "/create");
  await page
    .getByRole("heading", { name: "Your next role. Your best story." })
    .waitFor();
  await page.screenshot({
    path: "test-results/create-mobile.png",
    fullPage: true,
  });
  assert.ok(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
    "Create mobile overflow",
  );
  assert.deepEqual(crashes, []);
  console.log(
    "PASS: registration, upload, facts, JD, generation, tab buffers, save, PDF/DOCX download, reload, two-tab conflict preservation, 390px layout.",
  );
} catch (e) {
  await page.screenshot({ path: "test-results/failure.png", fullPage: true });
  console.error(await page.locator("body").innerText());
  throw e;
} finally {
  await browser.close();
}
