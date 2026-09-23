import { StrictMode, Component, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app/App";
import "./app/theme/tokens.css";
import "./app/theme/layout.css";
class ErrorBoundary extends Component<
  { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    return this.state.failed ? (
      <main className="account-page">
        <h1>Something interrupted your studio.</h1>
        <p>Reload to recover your saved work. Unsaved edits may be lost.</p>
        <button onClick={() => location.reload()}>Reload saved work</button>
      </main>
    ) : (
      this.props.children
    );
  }
}
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </StrictMode>,
);
