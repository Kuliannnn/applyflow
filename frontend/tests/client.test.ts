import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  Commands,
  request,
  resetSession,
} from "../src/shared/api/client";
const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
afterEach(() => {
  resetSession();
  vi.unstubAllGlobals();
});
describe("session and mutation safety", () => {
  it("obtains a CSRF token once, uses cookies, and reuses an idempotency key after a lost response", async () => {
    const calls: RequestInit[] = [];
    let mutation = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, opts: RequestInit) => {
        if (url.endsWith("/csrf")) return json({ csrf_token: "signed-token" });
        calls.push(opts);
        if (mutation++ === 0) throw new TypeError("lost response");
        return json({ id: "saved" });
      }),
    );
    const commands = new Commands();
    await expect(commands.send("/workspaces", {})).rejects.toThrow(
      "lost response",
    );
    expect(await commands.retry("/workspaces")).toEqual({ id: "saved" });
    expect(calls[0].headers).toEqual(calls[1].headers);
    expect((calls[0].headers as Record<string, string>)["X-CSRF-Token"]).toBe(
      "signed-token",
    );
    expect(calls[0].credentials).toBe("same-origin");
    expect(commands.retry("/workspaces")).toBeUndefined();
  });
  it("never silently retries a conflicting edit", async () => {
    let writes = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) => {
        if (url.endsWith("/csrf")) return json({ csrf_token: "token" });
        writes++;
        return json({ code: "version_conflict" }, 409);
      }),
    );
    await expect(
      request("/documents/id/revisions", {
        method: "POST",
        body: { expected_version: 1 },
      }),
    ).rejects.toBeInstanceOf(ApiError);
    expect(writes).toBe(1);
  });
  it("does not set a JSON content type on private multipart uploads", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, opts: RequestInit) => {
        if (url.endsWith("/csrf")) return json({ csrf_token: "token" });
        expect(
          (opts.headers as Record<string, string>)["Content-Type"],
        ).toBeUndefined();
        expect(opts.body).toBeInstanceOf(FormData);
        return json({ id: "file" });
      }),
    );
    await request("/files", { method: "POST", body: new FormData() });
  });
});
