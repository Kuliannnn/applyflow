import { useEffect, useState, type ReactNode } from "react";
import { ApiError } from "../api/client";
import { errorText } from "../i18n/en";
export function ErrorNotice({ error }: { error: unknown }) {
  return error ? (
    <div className="notice error" role="alert">
      {errorText(error)}
      {error instanceof ApiError && error.status === 401 && (
        <p>
          <a href="/login" target="_blank" rel="noopener noreferrer">
            Sign in in another tab
          </a>
        </p>
      )}
    </div>
  ) : null;
}
export function Notice({ children }: { children: ReactNode }) {
  return (
    <div className="notice" role="status">
      {children}
    </div>
  );
}
export function useAction() {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>();
  return {
    busy,
    error,
    clear: () => setError(undefined),
    run: async (action: () => Promise<void>) => {
      if (busy) return;
      setBusy(true);
      setError(undefined);
      try {
        await action();
      } catch (e) {
        setError(e);
      } finally {
        setBusy(false);
      }
    },
  };
}
const unsavedGuards = new Set<(e: BeforeUnloadEvent) => void>();
export function discardUnsavedGuards() {
  for (const guard of unsavedGuards)
    window.removeEventListener("beforeunload", guard);
  unsavedGuards.clear();
}
export function useUnsaved(dirty: boolean) {
  useEffect(() => {
    if (!dirty) return;
    const guard = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    unsavedGuards.add(guard);
    window.addEventListener("beforeunload", guard);
    return () => {
      window.removeEventListener("beforeunload", guard);
      unsavedGuards.delete(guard);
    };
  }, [dirty]);
}
export function Arrow() {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
    >
      <path
        d="M4 12h15m-6-6 6 6-6 6"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
export function FileIcon() {
  return (
    <svg
      width="26"
      height="30"
      viewBox="0 0 24 28"
      fill="none"
      aria-hidden="true"
    >
      <path
        d="M5 2h9l6 6v18H5zM14 2v7h6M9 14h7m-7 5h7"
        stroke="currentColor"
        strokeWidth="1.4"
        strokeLinejoin="round"
      />
    </svg>
  );
}
