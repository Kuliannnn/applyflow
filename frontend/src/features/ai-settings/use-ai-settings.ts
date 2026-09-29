import { useEffect, useRef, useState } from "react";
import { ApiError } from "../../shared/api/client";
import type {
  AIConfig,
  AIConfigTest,
  AIProviders,
} from "../../shared/api/generated";
import { useUnsaved } from "../../shared/ui/common";
import { aiCopy } from "../../shared/i18n/en";
import { aiAPI } from "./api";

// Secrets live only in this mounted form. No query cache, Commands fingerprint,
// browser storage, URL, telemetry or persistent retry payload receives the key.
export function useAISettings() {
  const [config, setConfig] = useState<AIConfig>();
  const [catalogue, setCatalogue] = useState<AIProviders>();
  const [model, setModel] = useState<"gpt-4.1-mini" | "gpt-4.1">(
    "gpt-4.1-mini",
  );
  const [key, setKey] = useState("");
  const [limit, setLimit] = useState("20");
  const [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>(),
    [message, setMessage] = useState("");
  const [needsReview, setNeedsReview] = useState(false),
    [uncertainTest, setUncertainTest] = useState(false);
  const working = useRef(false),
    mounted = useRef(false);
  const attempt = useRef<{ body: AIConfigTest; key: string } | undefined>(
    undefined,
  );
  const dirty =
    !!config &&
    (key.length > 0 ||
      model !== (config.model_id ?? "gpt-4.1-mini") ||
      limit !== String(config.daily_request_limit));
  useUnsaved(dirty);
  const sync = (value: AIConfig) => {
    setConfig(value);
    setModel(value.model_id ?? "gpt-4.1-mini");
    setLimit(String(value.daily_request_limit));
  };
  useEffect(() => {
    mounted.current = true;
    const ctrl = new AbortController();
    Promise.all([aiAPI.read(ctrl.signal), aiAPI.providers(ctrl.signal)])
      .then(([value, providers]) => {
        if (!ctrl.signal.aborted) {
          sync(value);
          setCatalogue(providers);
        }
      })
      .catch((e) => {
        if (!ctrl.signal.aborted) setError(e);
      });
    return () => {
      mounted.current = false;
      ctrl.abort();
    };
  }, []);
  // Poll only safe metadata; stop on an error and offer an explicit refresh.
  useEffect(() => {
    if (config?.test_status !== "testing" || busy || error) return;
    const ctrl = new AbortController();
    const timer = setTimeout(() => {
      aiAPI
        .read(ctrl.signal)
        .then((value) => {
          if (!ctrl.signal.aborted) setConfig(value);
        })
        .catch((e) => {
          if (!ctrl.signal.aborted) setError(e);
        });
    }, 1200);
    return () => {
      clearTimeout(timer);
      ctrl.abort();
    };
  }, [config, busy, error]);
  async function run(action: () => Promise<void>, mutation = false) {
    if (working.current) return;
    working.current = true;
    setBusy(true);
    setError(undefined);
    setMessage("");
    try {
      await action();
    } catch (e) {
      if (mounted.current) {
        setError(e);
        if (mutation) setNeedsReview(true);
      }
    } finally {
      working.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  function refresh() {
    void run(async () => {
      const [value, providers] = await Promise.all([
        aiAPI.read(),
        aiAPI.providers(),
      ]);
      if (!mounted.current) return;
      setCatalogue(providers);
      if (dirty) {
        setConfig(value);
        setMessage(aiCopy.refreshed);
      } else sync(value);
      setNeedsReview(false);
    });
  }
  function save() {
    if (!config) return;
    void run(async () => {
      const value = await aiAPI.save({
        expected_version: config.version,
        provider_id: "openai",
        model_id: model,
        daily_request_limit: Number(limit),
        ...(key ? { api_key: key } : {}),
      });
      if (mounted.current) {
        sync(value);
        setKey("");
        setMessage(aiCopy.saved);
        setNeedsReview(false);
      }
    }, true);
  }
  function test() {
    if (!attempt.current && !config?.revision) return;
    void run(async () => {
      attempt.current ??= {
        body: {
          expected_version: config!.version,
          revision: config!.revision!,
        },
        key: crypto.randomUUID(),
      };
      try {
        await aiAPI.test(attempt.current.body, attempt.current.key);
        attempt.current = undefined;
        if (mounted.current) setUncertainTest(false);
        // A completed replay can be historical. Always read the current configuration.
        const value = await aiAPI.read();
        if (mounted.current) {
          setConfig(value);
          setNeedsReview(false);
        }
      } catch (e) {
        if (e instanceof ApiError && e.status < 500) {
          attempt.current = undefined;
          if (mounted.current) {
            setUncertainTest(false);
            setNeedsReview(true);
          }
        } else if (mounted.current) {
          setUncertainTest(!!attempt.current);
          if (!attempt.current) setNeedsReview(true);
        }
        throw e;
      }
    });
  }
  function enable() {
    if (!config) return;
    void run(async () => {
      const value = await aiAPI.enable(config.version, !config.enabled);
      if (mounted.current) {
        setConfig(value);
        setMessage(
          value.enabled
            ? "Connection enabled. Choose My AI connection on Create to use it."
            : "Connection disabled.",
        );
      }
    }, true);
  }
  function remove() {
    if (!config) return;
    void run(async () => {
      const value = await aiAPI.remove(config.version);
      if (mounted.current) {
        sync(value);
        setKey("");
        setMessage(aiCopy.deleted);
        setNeedsReview(false);
      }
    }, true);
  }
  return {
    config,
    catalogue,
    model,
    setModel,
    key,
    setKey,
    limit,
    setLimit,
    busy,
    error,
    message,
    dirty,
    needsReview,
    uncertainTest,
    refresh,
    save,
    test,
    enable,
    remove,
  };
}
