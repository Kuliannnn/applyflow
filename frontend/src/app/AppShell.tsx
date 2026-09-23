import { useState, type ReactNode } from "react";
import { post, resetSession } from "../shared/api/client";
import {
  ErrorNotice,
  useAction,
  discardUnsavedGuards,
} from "../shared/ui/common";
export function Shell({
  children,
  email,
}: {
  children: ReactNode;
  email?: string;
}) {
  const [open, setOpen] = useState(false);
  const action = useAction();
  return (
    <>
      <a className="skip-link" href="#content">
        Skip to content
      </a>
      <header className="topbar">
        <a className="brand" href="/create">
          <svg
            width="26"
            height="30"
            viewBox="0 0 28 32"
            fill="none"
            aria-hidden="true"
          >
            <path
              d="M5 28C1 17 5 6 23 3c2 17-5 26-18 25Z"
              fill="currentColor"
            />
            <path
              d="m5 28 15-20M11 21l-2-9m7 3 7-1"
              stroke="#edf0dd"
              strokeWidth="1.2"
            />
          </svg>
          ApplyFlow
        </a>
        {email && (
          <nav aria-label="Main navigation">
            <a
              aria-current={
                location.pathname.startsWith("/create") ||
                location.pathname.startsWith("/studio")
                  ? "page"
                  : undefined
              }
              href="/create"
            >
              Create
            </a>
            <a
              aria-current={
                location.pathname === "/applications" ? "page" : undefined
              }
              href="/applications"
            >
              My applications
            </a>
          </nav>
        )}
        <div className="account">
          {email ? (
            <>
              <button
                className="account-button"
                aria-expanded={open}
                aria-controls="account-menu"
                onClick={() => setOpen(!open)}
              >
                {email.split("@")[0]} <span aria-hidden="true">⌄</span>
              </button>
              {open && (
                <div
                  id="account-menu"
                  className="glass menu"
                  onKeyDown={(e) => {
                    if (e.key === "Escape") {
                      setOpen(false);
                      (
                        document.querySelector(".account-button") as HTMLElement
                      )?.focus();
                    }
                  }}
                >
                  <p className="small account-email">{email}</p>
                  <a href="/profile">Profile</a>
                  <a href="/resumes">My resumes</a>
                  <a href="/settings/ai">AI settings</a>
                  <button
                    disabled={action.busy}
                    onClick={() =>
                      void action.run(async () => {
                        if (
                          !confirm(
                            "Sign out? Unsaved edits on this page will be discarded.",
                          )
                        )
                          return;
                        await post("/auth/logout");
                        resetSession();
                        discardUnsavedGuards();
                        location.assign("/login");
                      })
                    }
                  >
                    Sign out
                  </button>
                  <ErrorNotice error={action.error} />
                </div>
              )}
            </>
          ) : (
            <a href="/register">Get started ↗</a>
          )}
        </div>
      </header>
      <div id="content">{children}</div>
      <footer className="site-footer">
        <span>Made for your next chapter.</span>
        <span>ApplyFlow / Studio preview</span>
      </footer>
    </>
  );
}
