import { useState } from "react";
import { post, resetSession } from "../shared/api/client";
import { ErrorNotice, Arrow, useAction } from "../shared/ui/common";
export function LoginPage() {
  const register = location.pathname === "/register";
  const [email, setEmail] = useState(""),
    [password, setPassword] = useState("");
  const action = useAction();
  return (
    <main className="auth-page">
      <div className="intro">
        <p className="eyebrow">A LITTLE SPACE FOR YOUR NEXT CHAPTER</p>
        <h1>{register ? "Make room for what’s next." : "Welcome back."}</h1>
        <p>Your experience. A new possibility.</p>
      </div>
      <form
        className="glass auth-form"
        onSubmit={(e) => {
          e.preventDefault();
          void action.run(async () => {
            await post("/auth/" + (register ? "register" : "login"), {
              email,
              password,
            });
            resetSession();
            const next = /^\/(studio|create)\/[0-9a-f-]{36}$/i.test(
              location.pathname,
            )
              ? location.pathname + location.search
              : "/create";
            location.assign(next);
          });
        }}
      >
        <h2>{register ? "Create your account" : "Sign in to your studio"}</h2>
        <label>
          Email
          <input
            type="email"
            autoComplete="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </label>
        <label>
          Password
          <input
            type="password"
            autoComplete={register ? "new-password" : "current-password"}
            minLength={register ? 12 : 1}
            maxLength={72}
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </label>
        {register && <p className="small">Use at least 12 characters.</p>}
        <ErrorNotice error={action.error} />
        <button className="primary" disabled={action.busy}>
          {action.busy
            ? "Please wait…"
            : register
              ? "Create account"
              : "Sign in"}
          <Arrow />
        </button>
        <p className="small">
          {register ? "Already have an account?" : "New to ApplyFlow?"}{" "}
          <a href={register ? "/login" : "/register"}>
            {register ? "Sign in" : "Create an account"}
          </a>
        </p>
      </form>
    </main>
  );
}
