import { useState } from "react";
import { ResumeSetup } from "../features/resumes/ResumeSetup";
export function AccountPage({ type }: { type: string }) {
  const [selected, setSelected] = useState("");
  return (
    <main className="account-page">
      <header className="intro">
        <p className="eyebrow">YOUR SPACE</p>
        <h1>
          {type === "resumes"
            ? "A foundation that stays yours."
            : type === "profile"
              ? "Your personal details."
              : type === "ai"
                ? "Choose how you create."
                : "Your next chapters."}
        </h1>
      </header>
      <section className="glass account-panel">
        {type === "resumes" ? (
          <>
            <h2>My resumes</h2>
            <ResumeSetup selected={selected} onSelect={setSelected} />
            {selected && (
              <p>
                Confirmed version selected. You can choose it when creating a
                new application.
              </p>
            )}
            <a href="/create">Back to Create ↗</a>
          </>
        ) : type === "ai" ? (
          <>
            <h2>AI connection is coming next</h2>
            <p>
              This release uses a local mock generator to exercise the workflow.
              No external model receives your data.
            </p>
            <p>
              Personal API keys cannot be saved or used yet. The provider
              connection form will appear when its backend is ready.
            </p>
            <a href="/create">Create sample drafts ↗</a>
          </>
        ) : type === "profile" ? (
          <>
            <h2>Your resume is enough to get started</h2>
            <p>
              Full profile editing is not connected yet. Add and confirm your
              real experience in My resumes to use it in drafts.
            </p>
            <a href="/resumes">Go to My resumes ↗</a>
          </>
        ) : (
          <>
            <h2>Application tracking is coming next</h2>
            <p>
              Your workspaces are saved separately. Exporting a document does
              not submit a job application.
            </p>
            <a href="/create">Find your recent workspaces ↗</a>
          </>
        )}
      </section>
    </main>
  );
}
