import { ConnectionForm } from "../features/ai-settings/ConnectionForm";
import { aiCopy } from "../shared/i18n/en";
export function AISettingsPage() {
  return (
    <main className="account-page ai-page">
      <header className="intro">
        <p className="eyebrow">{aiCopy.eyebrow}</p>
        <h1>{aiCopy.title}</h1>
        <p>{aiCopy.intro}</p>
      </header>
      <ConnectionForm />
      <p className="footnote">
        <a href="/create">Back to Create ↗</a>
      </p>
    </main>
  );
}
