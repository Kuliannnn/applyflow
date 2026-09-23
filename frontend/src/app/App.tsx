import type { ReactNode } from "react";
import { useResource } from "../shared/api/use-resource";
import { ApiError } from "../shared/api/client";
import type { Session } from "../shared/api/generated";
import { SessionContext } from "../features/auth/Session";
import { LoginPage } from "../pages/LoginPage";
import { CreatePage } from "../pages/CreatePage";
import { StudioPage } from "../pages/StudioPage";
import { AccountPage } from "../pages/AccountPage";
import { ErrorNotice } from "../shared/ui/common";
import { Shell } from "./AppShell";
export function App() {
  const session = useResource<Session>("/auth/me");
  const path = location.pathname;
  const auth = ["/login", "/register"].includes(path);
  if (auth)
    return (
      <Shell>
        <LoginPage />
      </Shell>
    );
  if (session.error instanceof ApiError && session.error.status === 401)
    return (
      <Shell>
        <LoginPage />
      </Shell>
    );
  if (!session.data)
    return (
      <Shell>
        <main className="account-page">
          <section className="glass account-panel">
            <p role="status">Opening your studio…</p>
            <ErrorNotice error={session.error} />
            {!!session.error && (
              <button onClick={session.reload}>Try again</button>
            )}
          </section>
        </main>
      </Shell>
    );
  const parts = path.split("/").filter(Boolean);
  const validID = /^[0-9a-f-]{36}$/i;
  let page: ReactNode;
  if (
    path === "/" ||
    (parts[0] === "create" && (!parts[1] || validID.test(parts[1])))
  )
    page = <CreatePage id={parts[1]} />;
  else if (parts[0] === "studio" && validID.test(parts[1] ?? ""))
    page = <StudioPage id={parts[1]} />;
  else if (
    ["resumes", "profile", "applications"].includes(parts[0]) ||
    path === "/settings/ai"
  )
    page = <AccountPage type={path === "/settings/ai" ? "ai" : parts[0]} />;
  else
    page = (
      <main className="account-page">
        <h1>That page isn’t here.</h1>
        <a href="/create">Return to Create</a>
      </main>
    );
  return (
    <SessionContext.Provider value={session.data.user}>
      <Shell email={session.data.user.email}>{page}</Shell>
    </SessionContext.Provider>
  );
}
