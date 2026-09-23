import { useState } from "react";
import { Commands } from "../shared/api/client";
import type {
  DocumentKind,
  DocumentList,
  Generation,
  Workspace,
} from "../shared/api/generated";
import { useResource } from "../shared/api/use-resource";
import { DocumentPane } from "../features/documents/DocumentPane";
import { TaskStatus, terminal } from "../features/tasks/TaskStatus";
import { ErrorNotice } from "../shared/ui/common";
import { workspace } from "../features/studio/api";
export function StudioPage({ id }: { id: string }) {
  const w = useResource<Workspace>("/workspaces/" + id);
  const [done, setDone] = useState(false);
  const run = useResource<Generation>(
    w.data?.current_run_id
      ? `/workspaces/${id}/generations/${w.data.current_run_id}`
      : null,
    done ? 0 : 1600,
  );
  const docs = useResource<DocumentList>(
    "/workspaces/" + id + "/documents",
    done ? 0 : 1700,
  );
  const [tab, setTab] = useState<DocumentKind>(
    new URLSearchParams(location.search).get("document") === "cover_letter"
      ? "cover_letter"
      : "resume",
  );
  const [commands] = useState(() => new Commands());
  const allDone =
    !!run.data &&
    terminal(run.data.resume_task.status) &&
    terminal(run.data.cover_letter_task.status);
  if (allDone && !done) setDone(true);
  return (
    <main className="studio-page">
      <header className="studio-heading">
        <div>
          <a className="small back-link" href={"/create/" + id}>
            ← Job details
          </a>
          <h1>
            {w.data?.title && w.data.title !== "Untitled application"
              ? w.data.title
              : "Your story, shaped for the role."}
          </h1>
          <p>Two documents. One next chapter.</p>
        </div>
        <span className="mode-pill">Sample drafts</span>
      </header>
      <ErrorNotice error={w.error || run.error || docs.error} />
      <section className="glass studio">
        <div className="tabs" role="tablist" aria-label="Application documents">
          {(["resume", "cover_letter"] as const).map((kind) => (
            <button
              id={"tab-" + kind}
              aria-controls={"panel-" + kind}
              key={kind}
              role="tab"
              aria-selected={tab === kind}
              tabIndex={tab === kind ? 0 : -1}
              onKeyDown={(e) => {
                if (
                  ["ArrowLeft", "ArrowRight", "Home", "End"].includes(e.key)
                ) {
                  e.preventDefault();
                  const next = kind === "resume" ? "cover_letter" : "resume";
                  setTab(next);
                  document.getElementById("tab-" + next)?.focus();
                }
              }}
              onClick={() => {
                setTab(kind);
                history.replaceState(
                  null,
                  "",
                  `/studio/${id}?document=${kind}`,
                );
              }}
            >
              {kind === "resume" ? "Resume" : "Cover letter"}
            </button>
          ))}
        </div>
        {(["resume", "cover_letter"] as const).map((kind) => {
          const doc = docs.data?.items.find((d) => d.kind === kind);
          const task =
            kind === "resume"
              ? run.data?.resume_task
              : run.data?.cover_letter_task;
          return (
            <div
              id={"panel-" + kind}
              role="tabpanel"
              aria-labelledby={"tab-" + kind}
              key={kind}
              hidden={tab !== kind}
            >
              {task && (
                <TaskStatus
                  task={task}
                  onChange={() => {
                    run.reload();
                    docs.reload();
                  }}
                  onRetry={async () => {
                    const fresh = await workspace(id);
                    await commands.send(
                      `/workspaces/${id}/generations/${fresh.current_run_id}/retry`,
                      { expected_version: fresh.version, kind },
                    );
                    setDone(false);
                    run.reload();
                    docs.reload();
                  }}
                />
              )}
              {doc?.current_revision_id ? (
                <DocumentPane
                  key={doc.id}
                  document={doc}
                  run={w.data?.current_run_id ?? ""}
                />
              ) : (
                <div className="empty-document">
                  <h2>
                    {task && !terminal(task.status)
                      ? "A little space for your next draft."
                      : "Your document will appear here."}
                  </h2>
                  <p>
                    {task && !terminal(task.status)
                      ? "You can leave this page and return while the worker prepares both documents."
                      : "Return to job details to confirm your inputs and generate drafts."}
                  </p>
                </div>
              )}
            </div>
          );
        })}
      </section>
    </main>
  );
}
