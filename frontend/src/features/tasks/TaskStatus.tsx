import type { TaskSnapshot } from "../../shared/api/generated";
import { post } from "../../shared/api/client";
import { ErrorNotice, useAction } from "../../shared/ui/common";
import { taskError } from "../../shared/i18n/en";
export const terminal = (status: string) =>
  ["completed", "failed", "cancelled"].includes(status);
export function TaskStatus({
  task,
  onRetry,
  onChange,
}: {
  task: TaskSnapshot;
  onRetry: () => Promise<void>;
  onChange: () => void;
}) {
  const action = useAction();
  return (
    <div className="task-status">
      <p role="status">
        {task.status === "completed"
          ? "Draft ready"
          : task.status === "running"
            ? "Preparing your sample draft…"
            : task.status === "queued"
              ? "Waiting for the worker…"
              : task.status === "retry_wait"
                ? "Waiting to retry…"
                : task.status === "cancelled"
                  ? "Draft cancelled"
                  : taskError(task.error_code)}
      </p>
      {!terminal(task.status) && (
        <button
          className="text-button"
          disabled={action.busy}
          onClick={() =>
            void action.run(async () => {
              await post("/tasks/" + task.task_id + "/cancel");
              onChange();
            })
          }
        >
          Cancel
        </button>
      )}
      {["failed", "cancelled"].includes(task.status) && (
        <button disabled={action.busy} onClick={() => void action.run(onRetry)}>
          Retry this document
        </button>
      )}
      <ErrorNotice error={action.error} />
    </div>
  );
}
