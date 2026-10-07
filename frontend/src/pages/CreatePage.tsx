import { GenerationMode } from "../features/studio/GenerationMode";
import { copy } from "../shared/i18n/en";
import { Arrow, ErrorNotice, Notice } from "../shared/ui/common";
import { ResumeSetup } from "../features/resumes/ResumeSetup";
import { useCreateWorkflow } from "../features/studio/use-create-workflow";
export function CreatePage({ id }: { id?: string }) {
  const {
    choice,
    setChoice,
    text,
    setText,
    selected,
    setSelected,
    unsupported,
    setUnsupported,
    ready,
    setDirty,
    action,
    recent,
    url,
    submit,
  } = useCreateWorkflow(id);
  return (
    <main className="create-page">
      <header className="intro">
        <p className="eyebrow">THE APPLICATION STUDIO</p>
        <h1>{copy.headline}</h1>
        <p>{copy.intro}</p>
      </header>
      <form
        className="glass composer"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <fieldset disabled={action.busy || !ready} className="workflow-fields">
          <div className="section-heading">
            <label htmlFor="job-text">{copy.jd}</label>
            <span className="small">Start with the opportunity</span>
          </div>
          <div
            className="job-input"
            onDragOver={(e) => e.preventDefault()}
            onDrop={(e) => {
              e.preventDefault();
              if (e.dataTransfer.files.length) setUnsupported(true);
            }}
          >
            <textarea
              id="job-text"
              aria-describedby="input-support"
              placeholder={copy.jdPlaceholder}
              value={text}
              required
              maxLength={65536}
              disabled={!ready || action.busy}
              onPaste={(e) => {
                if ([...e.clipboardData.items].some((i) => i.kind === "file")) {
                  e.preventDefault();
                  setUnsupported(true);
                }
              }}
              onChange={(e) => {
                setText(e.target.value);
                setDirty(true);
              }}
            />
            <div className="input-foot">
              <span>Start with the words. We’ll help shape the story.</span>
              <span className="small">Text JD</span>
            </div>
          </div>
          <p id="input-support" className="small muted">
            Screenshot & link support is coming next.
          </p>
          {(unsupported || url) && <Notice>{copy.unsupported}</Notice>}
          <ResumeSetup
            selected={selected}
            onSelect={(value) => {
              setSelected(value);
              setDirty(true);
            }}
          />
          <GenerationMode choice={choice} onChange={setChoice} />
          <ErrorNotice error={action.error} />
          <div className="composer-bottom">
            <div>
              <p className="mode-label">
                <span className="status-dot" />
                {choice.mode === "personal"
                  ? "Personal AI · Two documents"
                  : copy.mock}
              </p>
              <p className="small muted">
                {choice.mode === "personal"
                  ? "Generate sends this JD and your confirmed resume to your selected AI provider."
                  : copy.mockDetail}
              </p>
            </div>
            <button
              className="primary"
              disabled={
                action.busy || !ready || !text.trim() || !selected || url
              }
            >
              {action.busy
                ? "Preparing your drafts…"
                : choice.mode === "personal"
                  ? "Generate with AI"
                  : "Generate sample drafts"}
              <Arrow />
            </button>
          </div>
        </fieldset>
      </form>
      <p className="footnote">{copy.private}</p>
      {!id && !!recent.data?.items.length && (
        <section className="recent">
          <h2>Pick up where you left off</h2>
          {recent.data.items.map((item) => (
            <a
              className="recent-link"
              key={item.id}
              href={(item.current_run_id ? "/studio/" : "/create/") + item.id}
            >
              <span>{item.title}</span>
              <span className="small">
                {new Date(item.updated_at).toLocaleDateString("en-AU")} ↗
              </span>
            </a>
          ))}
        </section>
      )}
    </main>
  );
}
