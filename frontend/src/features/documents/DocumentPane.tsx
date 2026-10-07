import { downloadFile } from "./download-file";
import { useEffect, useRef, useState } from "react";
import type {
  Document,
  DocumentExport,
  DocumentRevision,
  DocumentRevisionList,
  TaskSnapshot,
} from "../../shared/api/generated";
import { Commands, post, request } from "../../shared/api/client";
import { useResource } from "../../shared/api/use-resource";
import {
  ErrorNotice,
  Notice,
  useAction,
  useUnsaved,
} from "../../shared/ui/common";
import { useDocumentDraft } from "./use-document-draft";
import { ContentEditor, ContentPreview } from "./ContentEditor";
import { terminal } from "../tasks/TaskStatus";
import { taskError } from "../../shared/i18n/en";
export function DocumentPane({
  document,
  run,
}: {
  document: Document;
  run: string;
}) {
  const d = useDocumentDraft(document);
  const action = useAction();
  const [commands, setCommands] = useState(() => new Commands());
  const [exported, setExported] = useState<DocumentExport>();
  const [exportDone, setExportDone] = useState(false);
  const [downloadAttempt, setDownloadAttempt] = useState(0);
  const delivered = useRef(0);
  const [downloading, setDownloading] = useState(false);
  const [downloadError, setDownloadError] = useState<unknown>();
  const [downloadStarted, setDownloadStarted] = useState(false);
  const [editing, setEditing] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const history = useResource<DocumentRevisionList>(
    historyOpen ? `/documents/${document.id}/revisions?limit=100` : null,
  );
  const [selected, setSelected] = useState<DocumentRevision>();
  const task = useResource<TaskSnapshot>(
    exported ? "/tasks/" + exported.task_id : null,
    exportDone ? 0 : 1400,
  );
  useUnsaved(d.dirty);
  useEffect(() => {
    if (
      task.data?.task_id === exported?.task_id &&
      task.data &&
      terminal(task.data.status)
    )
      setExportDone(true);
  }, [task.data, exported?.task_id]);
  const download =
    task.data?.task_id === exported?.task_id &&
    task.data?.result?.kind === "export"
      ? task.data.result.file_id
      : exported?.file_id;
  useEffect(() => {
    if (!download || !exported || delivered.current === downloadAttempt) return;
    delivered.current = downloadAttempt;
    setDownloading(true);
    setDownloadError(undefined);
    void downloadFile(
      download,
      `${document.kind}-${exported.revision_id}.${exported.format}`,
    )
      .then(() => setDownloadStarted(true))
      .catch(setDownloadError)
      .finally(() => setDownloading(false));
  }, [download, downloadAttempt, exported, document.kind]);
  async function exportFile(format: "pdf" | "docx") {
    const revision = await d.save();
    const result = await commands.send<DocumentExport>(
      "/documents/" + document.id + "/exports",
      { revision_id: revision.id, format, template_version: "2" },
    );
    setDownloadError(undefined);
    setDownloadStarted(false);
    setDownloadAttempt((n) => n + 1);
    setExported(result);
    setExportDone(!!result.file_id);
  }
  return (
    <div className="studio-columns">
      <section className="reading-surface">
        <div className="document-toolbar">
          <span className="small" role="status">
            {d.dirty
              ? "Unsaved changes"
              : d.base
                ? "Saved · version " + d.base.document.version
                : "Loading document…"}
          </span>
          <button
            className="text-button"
            disabled={action.busy || !d.dirty}
            onClick={() =>
              void action.run(async () => {
                await d.save();
                history.reload();
              })
            }
          >
            {action.busy ? "Please wait…" : "Save changes"}
          </button>
        </div>
        {d.draft && (
          <button className="secondary" onClick={() => setEditing(!editing)}>
            {editing ? "Preview document" : "Edit document"}
          </button>
        )}
        <ErrorNotice error={d.error} />
        {!!d.error && (
          <button onClick={() => void action.run(d.compare)}>
            Compare saved version
          </button>
        )}
        {d.loading && <p>Loading saved text…</p>}
        {d.draft && !editing && (
          <div className="resume-paper">
            <ContentPreview value={d.draft} />
          </div>
        )}
        {d.draft && editing && (
          <ContentEditor
            value={d.draft}
            onChange={d.setDraft}
            disabled={action.busy}
          />
        )}
        {d.remote && (
          <section className="comparison">
            <h3>Latest saved version</h3>
            <p>
              Your text above has not been replaced. Copy any changes you want
              to keep before loading this version.
            </p>
            <ContentPreview value={d.remote.content} />
            <button
              onClick={() => {
                if (
                  confirm(
                    "Discard your local edits and load the latest saved version?",
                  )
                )
                  void action.run(d.acceptRemote);
              }}
            >
              Use latest saved version
            </button>
          </section>
        )}
      </section>
      <aside className="document-aside">
        <p className="eyebrow">MAKE IT YOURS</p>
        <h2>A draft, with your final word.</h2>
        <p>
          Review the wording and edit directly. Keep only statements that
          reflect your experience.
        </p>
        <div className="aside-divider" />
        <h3>What changed</h3>
        <p className="small">
          {d.base?.revision.change_summary ?? "Waiting for the saved draft."}
        </p>
        <p className="small muted">
          Check all claims against your confirmed facts before sending.
        </p>
        <div className="aside-divider" />
        <h3>Take it with you</h3>
        <p className="small">
          Export saves your edits, prepares the file, and downloads it
          automatically.
        </p>
        <div className="export-buttons">
          <button
            className="primary"
            disabled={
              action.busy ||
              downloading ||
              !d.base ||
              (!!exported && !exportDone)
            }
            onClick={() => void action.run(() => exportFile("pdf"))}
          >
            Export PDF ↗
          </button>
          <button
            className="secondary"
            disabled={
              action.busy ||
              downloading ||
              !d.base ||
              (!!exported && !exportDone)
            }
            onClick={() => void action.run(() => exportFile("docx"))}
          >
            Word document ↗
          </button>
        </div>
        {exported && !exportDone && (
          <Notice>
            {task.data?.status === "running"
              ? "Rendering your document…"
              : "Export queued. Waiting for the worker…"}
          </Notice>
        )}
        {downloading && <p role="status">Downloading your document…</p>}
        {downloadStarted && !downloading && (
          <p className="small" role="status">
            Download started.
          </p>
        )}
        <ErrorNotice error={downloadError} />
        {!!downloadError && (
          <button
            type="button"
            onClick={() => setDownloadAttempt((n) => n + 1)}
          >
            Retry download
          </button>
        )}
        {task.data && ["failed", "cancelled"].includes(task.data.status) && (
          <Notice>
            {taskError(task.data.error_code) || "Export cancelled."}
            <button
              onClick={() => {
                setCommands(new Commands());
                setExported(undefined);
                setExportDone(false);
              }}
            >
              Start a new export
            </button>
          </Notice>
        )}
        <ErrorNotice error={task.error} />
        <ErrorNotice
          error={action.error === d.error ? undefined : action.error}
        />
        <div className="aside-divider" />
        <button
          className="text-button"
          onClick={() => setHistoryOpen(!historyOpen)}
        >
          Version history {historyOpen ? "−" : "+"}
        </button>
        {historyOpen && (
          <div className="history">
            <ErrorNotice error={history.error} />
            {history.data?.items.map((r) => (
              <button key={r.id} onClick={() => setSelected(r)}>
                {r.origin === "manual"
                  ? "Your edit"
                  : r.change_summary.startsWith("Mock draft")
                    ? "Sample draft"
                    : "AI draft"}{" "}
                · {new Date(r.created_at).toLocaleString("en-AU")}
              </button>
            ))}
            {history.data?.next_cursor && (
              <p className="small">Showing the latest 100 revisions.</p>
            )}
            {selected && (
              <>
                <ContentPreview value={selected.content} />
                {selected.run_id === run &&
                  selected.id !== d.base?.revision.id && (
                    <button
                      disabled={d.dirty || action.busy}
                      onClick={() =>
                        void action.run(async () => {
                          const latest = await request<Document>(
                            "/documents/" + document.id,
                          );
                          await post("/documents/" + document.id + "/apply", {
                            expected_version: latest.version,
                            revision_id: selected.id,
                          });
                          await d.acceptRemote();
                        })
                      }
                    >
                      Use this version
                    </button>
                  )}
                {d.dirty && (
                  <p className="small">
                    Save your edits before switching versions.
                  </p>
                )}
              </>
            )}
          </div>
        )}
        <p className="small muted">
          AI rewrite and application tracking are not connected yet.
        </p>
      </aside>
    </div>
  );
}
