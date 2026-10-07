import { GenerationUsage } from "./GenerationUsage";
import { useEffect, useState } from "react";
import { aiCopy as text } from "../../shared/i18n/en";
import { ErrorNotice } from "../../shared/ui/common";
import { useAISettings } from "./use-ai-settings";

export function ConnectionForm() {
  const s = useAISettings();
  const [showKey, setShowKey] = useState(false),
    [confirmDelete, setConfirmDelete] = useState(false);
  useEffect(() => {
    if (!s.key) setShowKey(false);
  }, [s.key]);
  const c = s.config;
  if (!c || !s.catalogue)
    return (
      <section className="glass account-panel">
        <p role="status">
          {s.error
            ? "Your connection settings could not be loaded."
            : "Loading your connection…"}
        </p>
        <ErrorNotice error={s.error} />
        {!!s.error && (
          <button onClick={s.refresh} disabled={s.busy}>
            Try again
          </button>
        )}
      </section>
    );
  const selectedProvider = s.catalogue.providers.find(
    (p) => p.id === s.provider,
  );
  const canKeepKey = c.has_key && c.provider_id === s.provider;
  const testing = c.test_status === "testing";
  const blocked = s.busy || s.needsReview || s.uncertainTest || testing;
  const editable =
    !s.busy && !testing && !s.uncertainTest && s.catalogue.available;
  const validLimit =
    /^\d+$/.test(s.limit) &&
    Number(s.limit) >= 1 &&
    Number(s.limit) <= s.catalogue.max_daily_request_limit;
  const canSave =
    !blocked &&
    s.catalogue.available &&
    validLimit &&
    (canKeepKey || s.key.length >= 8) &&
    (s.dirty || !c.has_key);
  const status = !c.has_key
    ? "No key saved"
    : c.enabled
      ? "Enabled"
      : text.statuses[c.test_status];
  const step =
    !c.has_key || s.dirty ? 1 : c.test_status !== "succeeded" ? 2 : 3;
  return (
    <section className="glass account-panel ai-panel" aria-busy={s.busy}>
      <header className="ai-panel-heading">
        <div>
          <h2>{text.heading}</h2>
          <p>{text.description}</p>
        </div>
        <span className="mode-pill" role="status">
          {status}
        </span>
      </header>
      <ol className="ai-steps" aria-label="Connection setup">
        <li aria-current={step === 1 ? "step" : undefined}>
          01 <span>Save</span>
        </li>
        <li aria-current={step === 2 ? "step" : undefined}>
          02 <span>Test</span>
        </li>
        <li aria-current={step === 3 ? "step" : undefined}>
          03 <span>Enable</span>
        </li>
      </ol>
      {!s.catalogue.available && (
        <p className="notice" role="status">
          {text.unavailable}
        </p>
      )}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (canSave) s.save();
        }}
        autoComplete="off"
      >
        <fieldset className="workflow-fields" disabled={!editable}>
          <div className="two-fields ai-fields">
            <div>
              <label htmlFor="ai-provider">Provider</label>
              <select
                id="ai-provider"
                value={s.provider}
                onChange={(e) =>
                  s.changeProvider(e.target.value as typeof s.provider)
                }
              >
                {s.catalogue.providers.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label htmlFor="ai-model">Model</label>
              <select
                id="ai-model"
                value={s.model}
                onChange={(e) => s.setModel(e.target.value as typeof s.model)}
              >
                {selectedProvider?.models.map((model) => (
                  <option key={model}>{model}</option>
                ))}
              </select>
            </div>
          </div>
          <p className="small">{selectedProvider?.endpoint}</p>
          {s.provider === "aiwanwu" && (
            <p className="notice">{text.relayNotice}</p>
          )}
          <div className="ai-key-label">
            <label htmlFor="ai-key">
              {c.has_key ? "Replace API key" : "API key"}
            </label>
            {c.has_key && (
              <span className="small">Key saved · never displayed</span>
            )}
          </div>
          <div className="ai-key-control">
            <input
              id="ai-key"
              name="provider-secret"
              type={showKey ? "text" : "password"}
              autoComplete="new-password"
              autoCapitalize="none"
              spellCheck={false}
              maxLength={4096}
              minLength={8}
              required={!canKeepKey}
              placeholder={
                canKeepKey
                  ? "Leave empty to keep your saved key"
                  : "Enter the key for this provider"
              }
              value={s.key}
              onChange={(e) => s.setKey(e.target.value)}
              aria-describedby="ai-key-help"
            />
            <button
              type="button"
              onClick={() => setShowKey(!showKey)}
              aria-pressed={showKey}
              aria-label={showKey ? "Hide API key" : "Show API key"}
            >
              {showKey ? "Hide" : "Show"}
            </button>
          </div>
          <p id="ai-key-help" className="small">
            {canKeepKey ? text.keyHelp : text.newKeyHelp}
            {c.has_key && !canKeepKey && ` ${text.providerChange}`}
          </p>
          <details className="ai-options">
            <summary>Usage preferences</summary>
            <label htmlFor="ai-limit">
              Daily request limit
              <input
                id="ai-limit"
                type="number"
                min="1"
                max={s.catalogue.max_daily_request_limit}
                step="1"
                value={s.limit}
                onChange={(e) => s.setLimit(e.target.value)}
                aria-describedby="ai-limit-help"
              />
            </label>
            <p id="ai-limit-help" className="small">
              {text.limitHelp}
            </p>
          </details>
        </fieldset>
        {(s.dirty || !c.has_key) && (
          <div className="ai-actions">
            <span className="small">
              {s.dirty
                ? "Unsaved changes"
                : "Your key stays private to your account."}
            </span>
            <button className="primary" type="submit" disabled={!canSave}>
              {s.busy ? "Working…" : "Save connection"}
            </button>
          </div>
        )}
      </form>
      {!!s.error && <ErrorNotice error={s.error} />}
      {s.needsReview && (
        <button onClick={s.refresh} disabled={s.busy}>
          Review latest settings
        </button>
      )}
      {s.uncertainTest && (
        <div className="notice" role="status">
          <p>{text.uncertain}</p>
          <button onClick={s.test} disabled={s.busy}>
            Check same test
          </button>
        </div>
      )}
      {!!s.message && (
        <p className="ai-feedback" role="status">
          {s.message}
        </p>
      )}
      {c.has_key && (
        <section className="ai-verification" aria-labelledby="ai-test-heading">
          <h3 id="ai-test-heading">
            {testing ? text.statuses.testing : "Verify your connection"}
          </h3>
          <p className="small" id="ai-cost">
            {text.cost}
          </p>
          {c.error_code && (
            <p className="notice" role="status">
              {text.failures[c.error_code]}
            </p>
          )}
          {c.last_tested_at && (
            <p className="small">
              Last tested: {new Date(c.last_tested_at).toLocaleString("en")}
            </p>
          )}
          {s.dirty && (
            <p className="small">
              Save your changes before testing or enabling this connection.
            </p>
          )}
          <div className="ai-actions">
            <button
              type="button"
              className={
                c.test_status !== "succeeded" && !s.dirty
                  ? "primary"
                  : "secondary"
              }
              onClick={s.test}
              disabled={blocked || s.dirty || !s.catalogue.available}
              aria-describedby="ai-cost"
            >
              {testing
                ? "Testing…"
                : c.test_status === "untested"
                  ? "Test connection"
                  : "Test again"}
            </button>
            {c.test_status === "succeeded" && (
              <button
                type="button"
                className={c.enabled ? "secondary" : "primary"}
                onClick={s.enable}
                disabled={
                  blocked || s.dirty || (!s.catalogue.available && !c.enabled)
                }
              >
                {c.enabled ? "Disable connection" : "Enable connection"}
              </button>
            )}
            {(testing || !!s.error) && (
              <button
                type="button"
                className="text-button"
                onClick={s.refresh}
                disabled={s.busy}
              >
                Refresh status
              </button>
            )}
          </div>
        </section>
      )}
      <GenerationUsage />
      <p className="ai-preview small">{text.preview}</p>
      {c.has_key && (
        <div className="ai-remove">
          {confirmDelete ? (
            <>
              <p>{text.deleteHelp}</p>
              <div className="ai-actions">
                <button
                  type="button"
                  className="danger-button"
                  disabled={blocked}
                  onClick={() => {
                    s.remove();
                    setConfirmDelete(false);
                  }}
                >
                  Remove saved connection
                </button>
                <button type="button" onClick={() => setConfirmDelete(false)}>
                  Keep connection
                </button>
              </div>
            </>
          ) : (
            <button
              type="button"
              className="text-button"
              disabled={blocked}
              onClick={() => setConfirmDelete(true)}
            >
              Remove connection
            </button>
          )}
        </div>
      )}
    </section>
  );
}
