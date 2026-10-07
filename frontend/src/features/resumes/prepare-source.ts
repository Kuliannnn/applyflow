import { ApiError, post, request } from "../../shared/api/client";
import type {
  Resume,
  ResumeConfirmed,
  ResumeList,
  ResumeRevision,
  ResumeSourceText,
} from "../../shared/api/generated";

// Generation always includes the saved original, including resumes imported
// before automatic extraction existed. Never mutate an old revision or run.
export async function prepareResumeSource(selected: string): Promise<string> {
  const list = await request<ResumeList>("/resumes?limit=100");
  const entry = list.items.find((r) => r.current_revision_id === selected);
  if (!entry) throw new ApiError(409, "resume_select_current");
  const resume = await request<Resume>(`/resumes/${entry.id}`);
  if (resume.current_revision_id !== selected)
    throw new ApiError(409, "resume_select_current");
  const confirmed = await request<ResumeRevision>(
    `/resumes/${resume.id}/revisions/${selected}`,
  );
  const source = await post<ResumeSourceText>(
    `/resumes/${resume.id}/source-text`,
    {},
  );
  const normalize = (value: string) => value.replace(/\s+/g, " ").trim();
  const confirmedText = normalize(confirmed.facts.map((f) => f.text).join(" "));
  if (source.facts.every((f) => confirmedText.includes(normalize(f.text))))
    return selected;
  // Retain user additions/corrections alongside the complete original source.
  const sourceText = normalize(source.facts.map((f) => f.text).join(" "));
  const additions = confirmed.facts.filter(
    (f) => !sourceText.includes(normalize(f.text)),
  );
  const result = await post<ResumeConfirmed>(
    `/resumes/${resume.id}/revisions`,
    {
      expected_version: resume.version,
      facts: [...source.facts, ...additions],
    },
  );
  return result.revision.id;
}
