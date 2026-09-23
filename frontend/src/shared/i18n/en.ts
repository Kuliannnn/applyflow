import { ApiError } from "../api/client";
export const copy = {
  brand: "ApplyFlow",
  headline: "Your next role. Your best story.",
  intro:
    "Turn a job description into a resume and cover letter that sound like you.",
  jd: "Job description",
  jdPlaceholder: "Paste the job description here…",
  mock: "Preview mode · No AI provider connected",
  mockDetail:
    "This release creates clearly labelled sample drafts from your confirmed facts. No data is sent to an AI provider.",
  unsupported:
    "Screenshot and link reading are not available yet. Paste the job description as text to continue.",
  facts:
    "Automatic extraction is not available yet. Add the facts you want to use from your resume. Only confirm information that is true.",
  private: "Private to your account. Your original resume stays unchanged.",
  unsaved: "You have unsaved changes. Leave this page and discard them?",
};
const errors: Record<string, string> = {
  invalid_pdf:
    "The PDF could not be read. It may be damaged or password-protected. Export a fresh PDF without a password and upload it again.",
  pdf_restricted:
    "This PDF is encrypted or restricted. Export a copy without password protection, or upload a standard .docx file.",
  pdf_page_limit:
    "Your PDF exceeds the 20-page limit. Upload a shorter resume.",
  file_validator_unavailable:
    "The server could not run its PDF validation tool. Try again; for local development, check PDFINFO_BIN and restart the API.",
  unsupported_source_file:
    "This file could not pass validation. Upload a valid PDF (1–20 pages, not encrypted) or a standard .docx file without macros or embedded objects. Try exporting a fresh copy from your document editor.",
  unsupported_media_type:
    "The upload request has an unsupported format. Refresh the page and choose your PDF or .docx file again.",
  csrf_failed:
    "The security check failed. Refresh this page and try again. For local development, PUBLIC_ORIGIN must exactly match the browser address, including the port.",
  validation_error:
    "Check the required fields and their length, then try again.",
  conflict:
    "A newer version is available. Your edits are still here. Reload the saved version before trying again.",
  version_conflict: "A newer version is available. Your edits are still here.",
  invalid_credentials: "The email or password is incorrect.",
  email_in_use: "This email is already registered. Sign in instead.",
  rate_limited: "Too many requests. Wait a moment before trying again.",
  export_unsupported_character:
    "This export template supports English and Latin characters. Remove unsupported characters such as emoji before exporting.",
  export_render_timeout:
    "The export took too long. Try again with a new export task.",
  export_failed:
    "The export could not finish. Your saved document is safe. Try again.",
};
export function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 401)
      return "Your session has ended. Sign in again. Keep this page open to preserve unsaved edits.";
    if (error.status === 409) return errors.conflict;
    if (error.status === 404)
      return "This item could not be found in your account.";
    return (
      errors[error.code] ??
      "The request could not complete. Your input has been kept. Please try again."
    );
  }
  return "Unable to connect. Check your connection and try again. Your input has been kept.";
}
export function taskError(code: string | null | undefined) {
  return code
    ? (errors[code] ?? "The task could not finish. You can retry it.")
    : "";
}
