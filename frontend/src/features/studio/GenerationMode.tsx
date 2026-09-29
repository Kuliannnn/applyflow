import type { AIConfig, AIProviders } from "../../shared/api/generated";
import { useResource } from "../../shared/api/use-resource";
import { ErrorNotice } from "../../shared/ui/common";
export type GenerationChoice = { mode: "mock" | "personal"; revision?: number };
export function GenerationMode({
  choice,
  onChange,
}: {
  choice: GenerationChoice;
  onChange: (value: GenerationChoice) => void;
}) {
  const config = useResource<AIConfig>("/me/ai-config");
  const providers = useResource<AIProviders>("/ai/providers");
  const ai = config.data;
  const ready = !!(
    ai?.has_key &&
    ai.enabled &&
    ai.test_status === "succeeded" &&
    ai.revision &&
    providers.data?.generation_available
  );
  return (
    <div className="generation-mode">
      <label htmlFor="generation-mode">Generation mode</label>
      <select
        id="generation-mode"
        value={choice.mode}
        onChange={(e) =>
          onChange(
            e.target.value === "personal"
              ? { mode: "personal", revision: ai?.revision ?? undefined }
              : { mode: "mock" },
          )
        }
      >
        <option value="mock">Sample drafts · no AI call</option>
        <option value="personal" disabled={!ready}>
          My AI connection
          {ready ? ` · ${ai?.model_id}` : " · enable in AI settings"}
        </option>
      </select>
      {choice.mode === "personal" ? (
        <p className="small">
          Generate sends this confirmed job description and all selected resume
          facts to OpenAI ({ai?.model_id}) for two separate paid calls. Files
          and unrelated profile fields are not sent. Include only facts you want
          to share. Review every generated claim before using it.
        </p>
      ) : (
        <p className="small">
          Sample drafts use your confirmed facts without an external AI call.
        </p>
      )}
      {!ready && (
        <p className="small">
          <a href="/settings/ai" target="_blank" rel="noopener noreferrer">
            Set up AI connection ↗
          </a>{" "}
          ·{" "}
          <button
            type="button"
            className="text-button"
            onClick={() => {
              config.reload();
              providers.reload();
            }}
          >
            Refresh connection
          </button>
        </p>
      )}
      <ErrorNotice error={config.error || providers.error} />
    </div>
  );
}
