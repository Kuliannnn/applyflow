import { afterEach, expect, it, vi } from "vitest";
import { saveInput } from "../src/features/studio/api";
import { Commands, resetSession } from "../src/shared/api/client";
import type { Workspace } from "../src/shared/api/generated";
const original: Workspace = {
  id: "workspace",
  title: "Draft",
  version: 2,
  current_source_id: "source",
  job_revision_id: null,
  resume_revision_id: "resume",
  current_run_id: null,
  application_id: null,
  archived_at: null,
  created_at: "2026-09-23",
  updated_at: "2026-09-23",
};
afterEach(() => {
  vi.unstubAllGlobals();
  resetSession();
});
it("does not refresh a stale version merely to overwrite another tab’s JD", async () => {
  const fetcher = vi.fn(async (url: string, init: RequestInit) => {
    expect(init.method).toBe("GET");
    return new Response(
      JSON.stringify(
        url.endsWith("/workspace")
          ? { ...original, version: 3, current_source_id: "other-source" }
          : { source: { kind: "text", text: "Other tab description" } },
      ),
    );
  });
  vi.stubGlobal("fetch", fetcher);
  await expect(
    saveInput(new Commands(), original, "My unsaved description", "resume"),
  ).rejects.toMatchObject({ status: 409 });
  expect(fetcher).toHaveBeenCalledTimes(2);
});
it("recovers an accepted source after a lost response without creating it twice", async () => {
  const fetcher = vi.fn(async (url: string, init: RequestInit) => {
    expect(init.method).toBe("GET");
    return new Response(
      JSON.stringify(
        url.endsWith("/workspace")
          ? { ...original, version: 3, current_source_id: "accepted-source" }
          : { source: { kind: "text", text: "My description" } },
      ),
    );
  });
  vi.stubGlobal("fetch", fetcher);
  const saved = await saveInput(
    new Commands(),
    original,
    "My description",
    "resume",
  );
  expect(saved.version).toBe(3);
});
