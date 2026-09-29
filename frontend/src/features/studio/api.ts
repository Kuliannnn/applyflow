import {
  ApiError,
  Commands,
  patch,
  post,
  request,
} from "../../shared/api/client";
import type {
  Workspace,
  SourceAccepted,
  JobConfirmed,
  GenerationAccepted,
  JobSource,
  JobRevision,
} from "../../shared/api/generated";
import type { GenerationChoice } from "./GenerationMode";
export const workspace = (id: string) =>
  request<Workspace>("/workspaces/" + id);
export async function loadInput(id: string) {
  const w = await workspace(id);
  const [source, job] = await Promise.all([
    w.current_source_id
      ? request<JobSource>(`/workspaces/${id}/sources/${w.current_source_id}`)
      : null,
    w.job_revision_id
      ? request<JobRevision>(
          `/workspaces/${id}/job-revisions/${w.job_revision_id}`,
        )
      : null,
  ]);
  return { workspace: w, source, job };
}
export async function saveInput(
  commands: Commands,
  w: Workspace | undefined,
  text: string,
  resume: string,
  onProgress: (w: Workspace) => void = () => {},
) {
  const previouslyLoaded = w;
  if (!w) w = await commands.send<Workspace>("/workspaces", {});
  const fresh = await workspace(w.id);
  // Do not turn a fresh GET into permission to overwrite another tab's input.
  if (previouslyLoaded && fresh.version !== previouslyLoaded.version) {
    const source = fresh.current_source_id
      ? await request<JobSource>(
          `/workspaces/${fresh.id}/sources/${fresh.current_source_id}`,
        )
      : null;
    const sameInput =
      fresh.resume_revision_id === resume &&
      source?.source.kind === "text" &&
      source.source.text === text;
    const recoveredSelection =
      fresh.version === previouslyLoaded.version + 1 &&
      fresh.resume_revision_id === resume &&
      fresh.current_source_id === previouslyLoaded.current_source_id &&
      fresh.job_revision_id === previouslyLoaded.job_revision_id &&
      fresh.current_run_id === previouslyLoaded.current_run_id;
    if (!sameInput && !recoveredSelection)
      throw new ApiError(409, "version_conflict");
  }
  w = fresh;
  if (w.resume_revision_id !== resume)
    w = await patch<Workspace>("/workspaces/" + w.id, {
      expected_version: w.version,
      resume_revision_id: resume,
    });
  onProgress(w);
  let same = false;
  if (w.current_source_id) {
    const source = await request<JobSource>(
      `/workspaces/${w.id}/sources/${w.current_source_id}`,
    );
    same = source.source.kind === "text" && source.source.text === text;
  }
  if (!same) {
    await commands.send<SourceAccepted>(`/workspaces/${w.id}/sources`, {
      expected_version: w.version,
      source: { kind: "text", text },
    });
    w = await workspace(w.id);
  }
  onProgress(w);
  return w;
}
export async function generate(
  commands: Commands,
  w: Workspace,
  company: string,
  role: string,
  text: string,
  profile: number,
  choice: GenerationChoice = { mode: "mock" },
) {
  const replay = commands.retry<GenerationAccepted>(
    `/workspaces/${w.id}/generations`,
    (body) => {
      const previous = body as { execution_mode: string; ai_revision?: number };
      return (
        previous.execution_mode === choice.mode &&
        previous.ai_revision === choice.revision
      );
    },
  );
  if (replay) return replay;
  w = await workspace(w.id);
  let confirmed: JobRevision | undefined;
  if (w.job_revision_id) {
    confirmed = await request<JobRevision>(
      `/workspaces/${w.id}/job-revisions/${w.job_revision_id}`,
    );
  }
  if (
    !confirmed ||
    confirmed.company !== company ||
    confirmed.role_title !== role ||
    confirmed.job_description !== text
  ) {
    const source = await request<JobSource>(
      `/workspaces/${w.id}/sources/${w.current_source_id}`,
    );
    const result = await post<JobConfirmed>(
      `/workspaces/${w.id}/job-revisions`,
      {
        expected_version: w.version,
        source_id: source.id,
        expected_source_version: source.source_version,
        company,
        role_title: role,
        job_description: text,
      },
    );
    w = {
      ...w,
      version: result.workspace_version,
      job_revision_id: result.revision.id,
    };
  }
  const title = Array.from(role + " · " + company)
    .slice(0, 160)
    .join("");
  if (w.title !== title)
    w = await patch<Workspace>("/workspaces/" + w.id, {
      expected_version: w.version,
      title,
    });
  return commands.send<GenerationAccepted>(`/workspaces/${w.id}/generations`, {
    expected_version: w.version,
    job_revision_id: w.job_revision_id,
    resume_revision_id: w.resume_revision_id,
    expected_profile_version: profile,
    execution_mode: choice.mode,
    ...(choice.mode === "personal" ? { ai_revision: choice.revision } : {}),
    locale: "en",
  });
}
