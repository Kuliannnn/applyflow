import { useState } from "react";
import { Commands, post, request } from "../../shared/api/client";
import type {
  PrivateFile,
  Resume,
  ResumeConfirmed,
  ResumeFact,
  ResumeImported,
  ResumeList,
  ResumeSourceText,
} from "../../shared/api/generated";
import {
  ErrorNotice,
  FileIcon,
  useAction,
  useUnsaved,
} from "../../shared/ui/common";
import { useResource } from "../../shared/api/use-resource";
import { copy } from "../../shared/i18n/en";
const blank = (): ResumeFact => ({
  id: crypto.randomUUID(),
  category: "experience",
  text: "",
  evidence: { source: "user", page: null, excerpt: "" },
});
export function ResumeSetup({
  onSelect,
  selected,
}: {
  onSelect: (id: string, name: string) => void;
  selected?: string;
}) {
  const list = useResource<ResumeList>("/resumes?limit=100");
  const [commands] = useState(() => new Commands());
  const [resume, setResume] = useState<Resume>(),
    [facts, setFacts] = useState<ResumeFact[]>([blank()]);
  const [show, setShow] = useState(false);
  const [extracted, setExtracted] = useState(false);
  const [uploaded, setUploaded] = useState<PrivateFile>();
  const action = useAction();
  useUnsaved(show && facts.some((f) => !!f.text));
  async function upload(file: File) {
    if (file.size > 10 * 1024 * 1024) throw new Error("file too large");
    const body = new FormData();
    body.set("purpose", "base_resume");
    body.set("file", file);
    const saved = await request<PrivateFile>("/files", {
      method: "POST",
      body,
    });
    setUploaded(saved);
    await importFile(saved);
  }
  async function importFile(file: PrivateFile) {
    const result = await commands.send<ResumeImported>("/resumes", {
      file_id: file.id,
      name: file.original_name,
      import_mode: "manual",
    });
    setResume(result.resume);
    setFacts([blank()]);
    setExtracted(false);
    setShow(true);
    list.reload();
    await extractSource(result.resume);
  }
  async function extractSource(value: Resume) {
    const source = await post<ResumeSourceText>(
      `/resumes/${value.id}/source-text`,
      {},
    );
    setFacts(source.facts);
    setExtracted(true);
  }
  function update(index: number, patch: Partial<ResumeFact>) {
    setFacts((previous) =>
      previous.map((f, i) => (i === index ? { ...f, ...patch } : f)),
    );
  }
  return (
    <section className="resume-setup">
      <div className="resume-row">
        <FileIcon />
        <div className="grow">
          <label htmlFor="resume-select">Your base resume</label>
          <select
            id="resume-select"
            value={selected ?? ""}
            onChange={(e) => {
              const r = list.data?.items.find(
                (r) => r.current_revision_id === e.target.value,
              );
              if (r) onSelect(r.current_revision_id!, r.name);
            }}
          >
            <option value="">Choose a confirmed resume</option>
            {selected &&
              !list.data?.items.some(
                (r) => r.current_revision_id === selected,
              ) && (
                <option value={selected}>Previously selected version</option>
              )}
            {list.data?.items
              .filter((r) => r.current_revision_id)
              .map((r) => (
                <option key={r.id} value={r.current_revision_id!}>
                  {r.name}
                </option>
              ))}
          </select>
        </div>
        <button
          type="button"
          className="secondary"
          onClick={() => setShow(!show)}
        >
          {show ? "Close" : "Upload resume"}
        </button>
      </div>
      <ErrorNotice error={list.error} />
      {show && (
        <div className="resume-expanded">
          <p className="small">{copy.facts}</p>
          {!resume ? (
            <>
              <label className="upload-control">
                PDF or Word · up to 10 MB
                <input
                  aria-label="Upload base resume"
                  disabled={action.busy}
                  type="file"
                  accept=".pdf,.docx"
                  onChange={(e) => {
                    const file = e.target.files?.[0];
                    if (file) void action.run(() => upload(file));
                  }}
                />
              </label>
              {uploaded && (
                <button
                  type="button"
                  onClick={() => void action.run(() => importFile(uploaded))}
                >
                  Retry import of {uploaded.original_name}
                </button>
              )}
              {list.data?.items
                .filter((r) => !r.current_revision_id)
                .map((r) => (
                  <button
                    key={r.id}
                    type="button"
                    onClick={() =>
                      void action.run(async () => {
                        setResume(r);
                        setFacts([blank()]);
                        setExtracted(false);
                        await extractSource(r);
                      })
                    }
                  >
                    Continue confirming {r.name}
                  </button>
                ))}
            </>
          ) : (
            <>
              <p className="small">
                Original saved:{" "}
                <a href={"/api/files/" + resume.source_file_id + "/download"}>
                  {resume.name}
                </a>
              </p>
              <button
                type="button"
                disabled={action.busy}
                onClick={() => {
                  if (
                    !facts.some((f) => f.text.trim()) ||
                    confirm(
                      "Replace the text below with text read from the original file?",
                    )
                  )
                    void action.run(() => extractSource(resume));
                }}
              >
                {action.busy
                  ? "Reading resume…"
                  : "Read text from uploaded resume"}
              </button>
              {extracted && (
                <p className="notice">
                  Full resume text extracted. Check that names, dates, education
                  and experience are complete, then confirm. This text will be
                  used to tailor your new resume.
                </p>
              )}
              {facts.map((fact, i) => (
                <div className="fact" key={fact.id}>
                  <div className="row">
                    {!extracted && (
                      <label className="grow">
                        Fact type
                        <select
                          disabled={action.busy}
                          value={fact.category}
                          onChange={(e) =>
                            update(i, {
                              category: e.target
                                .value as ResumeFact["category"],
                            })
                          }
                        >
                          {[
                            "summary",
                            "experience",
                            "education",
                            "skill",
                            "project",
                            "other",
                          ].map((c) => (
                            <option key={c}>{c}</option>
                          ))}
                        </select>
                      </label>
                    )}
                    <button
                      type="button"
                      className="text-button"
                      disabled={facts.length === 1 || action.busy}
                      onClick={() => setFacts(facts.filter((_, n) => n !== i))}
                    >
                      Remove
                    </button>
                  </div>
                  <label>
                    {extracted
                      ? `Resume text · part ${i + 1}`
                      : "Confirmed fact"}
                    <textarea
                      aria-label={
                        extracted
                          ? `Resume text · part ${i + 1}`
                          : "Confirmed fact"
                      }
                      required
                      disabled={action.busy}
                      maxLength={4000}
                      rows={extracted ? 12 : 3}
                      value={fact.text}
                      onChange={(e) => update(i, { text: e.target.value })}
                      placeholder="What you did, where, and when. Use your own facts."
                    />
                  </label>
                </div>
              ))}
              <div className="row">
                <button
                  type="button"
                  disabled={facts.length >= 200 || action.busy}
                  onClick={() => setFacts([...facts, blank()])}
                >
                  Add a fact
                </button>
                <button
                  type="button"
                  className="primary"
                  disabled={action.busy || facts.some((f) => !f.text.trim())}
                  onClick={() =>
                    void action.run(async () => {
                      const result = await post<ResumeConfirmed>(
                        "/resumes/" + resume.id + "/revisions",
                        { expected_version: resume.version, facts },
                      );
                      onSelect(result.revision.id, resume.name);
                      setShow(false);
                      setFacts([blank()]);
                      setExtracted(false);
                      setResume(undefined);
                      setUploaded(undefined);
                      list.reload();
                    })
                  }
                >
                  {action.busy ? "Saving…" : "Confirm resume & use it"}
                </button>
              </div>
            </>
          )}
        </div>
      )}
      <ErrorNotice error={action.error} />
    </section>
  );
}
