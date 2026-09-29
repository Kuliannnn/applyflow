import type { AIGenerationUsage } from "../../shared/api/generated";
import { useResource } from "../../shared/api/use-resource";
import { ErrorNotice } from "../../shared/ui/common";
export function GenerationUsage() {
  const usage = useResource<AIGenerationUsage>("/me/ai-usage");
  const u = usage.data;
  return (
    <section className="ai-verification">
      <h3>Generation usage · last 24 hours</h3>
      {u ? (
        <>
          <p className="small">
            {u.attempted_requests} calls attempted · {u.reserved_requests}{" "}
            reserved · limit {u.daily_request_limit}
          </p>
          <p className="small">
            Known tokens: {u.known_input_tokens.toLocaleString("en")} input /{" "}
            {u.known_output_tokens.toLocaleString("en")} output.
            {u.unknown_usage_requests > 0
              ? ` Usage is unknown for ${u.unknown_usage_requests} calls; these are not counted as zero.`
              : ""}
          </p>
        </>
      ) : (
        <p className="small">Loading usage…</p>
      )}
      <ErrorNotice error={usage.error} />
      <button type="button" className="text-button" onClick={usage.reload}>
        Refresh usage
      </button>
    </section>
  );
}
