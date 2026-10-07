import { beforeEach, expect, it, vi } from "vitest";
import { prepareResumeSource } from "./prepare-source";
import { post, request } from "../../shared/api/client";
vi.mock("../../shared/api/client", () => ({
  ApiError: class extends Error {
    constructor(
      public status: number,
      public code: string,
    ) {
      super(code);
    }
  },
  post: vi.fn(),
  request: vi.fn(),
}));
const fact = (text: string) => ({
  id: text,
  category: "other",
  text,
  evidence: { source: "file", page: 1, excerpt: text },
});
beforeEach(() => vi.resetAllMocks());
function setup(text: string) {
  vi.mocked(request)
    .mockResolvedValueOnce({
      items: [{ id: "resume", current_revision_id: "old" }],
    })
    .mockResolvedValueOnce({
      id: "resume",
      current_revision_id: "old",
      version: 2,
    })
    .mockResolvedValueOnce({ facts: [fact(text)] });
  vi.mocked(post)
    .mockResolvedValueOnce({
      facts: [fact("Complete CV education, dates, achievements and skills.")],
    })
    .mockResolvedValueOnce({ revision: { id: "full" } });
}
it("repairs a legacy short revision using its original file before generation", async () => {
  setup("Previously entered summary");
  expect(await prepareResumeSource("old")).toBe("full");
  expect(vi.mocked(post).mock.calls[1]).toEqual([
    "/resumes/resume/revisions",
    {
      expected_version: 2,
      facts: [
        fact("Complete CV education, dates, achievements and skills."),
        fact("Previously entered summary"),
      ],
    },
  ]);
});
it("reuses a complete revision without creating another one", async () => {
  setup("Complete CV education, dates, achievements and skills.");
  expect(await prepareResumeSource("old")).toBe("old");
  expect(post).toHaveBeenCalledTimes(1);
});
it("does not fall back to short facts if source extraction fails", async () => {
  setup("Old summary");
  vi.mocked(post)
    .mockReset()
    .mockRejectedValueOnce(new Error("resume_extract_failed"));
  await expect(prepareResumeSource("old")).rejects.toThrow(
    "resume_extract_failed",
  );
  expect(post).toHaveBeenCalledTimes(1);
});
