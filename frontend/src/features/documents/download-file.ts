import { ApiError } from "../../shared/api/client";

// Fetch before navigating so an unavailable private file stays an inline error,
// rather than replacing the editor with a JSON error page.
export async function downloadFile(fileId: string, filename: string) {
  const response = await fetch(
    `/api/files/${encodeURIComponent(fileId)}/download`,
    {
      credentials: "same-origin",
      cache: "no-store",
    },
  );
  if (!response.ok) {
    const problem = await response.json().catch(() => ({}));
    throw new ApiError(
      response.status,
      problem.code ?? "unavailable",
      problem.request_id,
    );
  }
  const url = URL.createObjectURL(await response.blob());
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.append(link);
  link.click();
  link.remove();
  // Allow the browser to consume the blob before releasing private bytes.
  setTimeout(() => URL.revokeObjectURL(url), 30_000);
}
