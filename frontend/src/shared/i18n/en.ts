import { ApiError } from "../api/client";
export const copy = {
  brand: "ApplyFlow",
  headline: "Your next role. Your best story.",
  intro:
    "Turn a job description into a resume and cover letter that sound like you.",
  jd: "Job description",
  jdPlaceholder: "Paste the job description here…",
  mock: "Preview mode · Sample generation",
  mockDetail:
    "This release creates clearly labelled sample drafts from your confirmed facts. Generation does not send your documents to an AI provider.",
  unsupported:
    "Screenshot and link reading are not available yet. Paste the job description as text to continue.",
  facts:
    "Upload your full resume. We read PDF or Word text locally so you can check it before AI tailors a new document. Your original file stays unchanged.",
  private: "Private to your account. Your original resume stays unchanged.",
  unsaved: "You have unsaved changes. Leave this page and discard them?",
};
const errors: Record<string, string> = {
  resume_ocr_required:
    "This file has pages without readable text. Upload a text-based PDF or Word file, or paste the full resume text below. Scanned pages need OCR, which is not available yet.",
  resume_extract_failed:
    "We could not read this resume. Try a text-based PDF or Word file, or paste its full text below.",
  resume_extract_unavailable:
    "Resume reading is unavailable. Check the API installation or paste the full resume text below.",
  resume_extract_too_large:
    "The resume is too long to import without losing content. Use a shorter source file.",
  generation_failed:
    "The draft could not be completed. Check its status before explicitly retrying; any previous provider call may already have incurred a charge.",
  pending_generation:
    "A previous generation request has an uncertain outcome. Keep its original mode and AI configuration to retry the same request safely; do not start a different paid request yet.",
  ai_config_required:
    "This generation needs its saved, tested and enabled AI configuration. Check AI settings; if you changed the model or key, start a new generation from Job details.",
  ai_daily_limit:
    "Your generation request limit has been reached. Pending documents reserve calls too. Wait for the rolling 24-hour window or update your limit in AI settings.",
  ai_input_too_large:
    "The combined job description and confirmed facts are too long. Shorten them before generating again.",
  provider_outcome_unknown:
    "A provider call may already have happened. It will not be repeated automatically. An explicit retry may incur another charge.",
  provider_invalid_document:
    "The model returned invalid document structure or unsupported fact references. No draft was saved. You may explicitly retry.",
  provider_incomplete:
    "The provider stopped before completing the document. You may explicitly retry; the earlier call may have incurred a charge.",
  provider_refused:
    "The provider declined this request. Review your inputs before trying again.",
  provider_auth_failed:
    "The provider rejected the saved key. Check AI settings.",
  provider_model_unavailable:
    "The provider could not find the requested model or API endpoint. Check the provider connection details.",
  provider_rate_limited:
    "The provider reported a rate or quota limit. Check your provider account before retrying.",
  provider_timeout:
    "The provider timed out. The call may have incurred a charge; it will not be repeated automatically.",
  provider_unavailable:
    "The provider could not be reached. The call outcome is uncertain; it will not be repeated automatically.",
  credential_unavailable:
    "The worker could not decrypt this key. Check that API and worker use the same credential master key.",
  config_version_conflict:
    "Your settings changed in another tab. Review the latest settings before saving again. Your edits are still here.",
  config_test_required:
    "Test the saved connection successfully before enabling it.",
  config_changed:
    "The saved connection changed. Review the latest settings before testing again.",
  config_test_in_progress:
    "A connection test is already running. Refresh its status in a moment.",
  ai_credentials_unavailable:
    "Personal AI connections are unavailable on this server. Contact the person who manages your installation.",
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
  resume_select_current:
    "Select the current version of your saved resume before generating. Your input has been kept.",
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
    if (error.status === 409) return errors[error.code] ?? errors.conflict;
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

export const aiCopy = {
  eyebrow: "YOUR AI CONNECTION",
  title: "A little intelligence. On your terms.",
  intro: "Choose your model. Keep control of your key.",
  heading: "Personal connection",
  description:
    "Save your key, test the connection, then choose when to enable it.",
  preview:
    "Choose My AI connection on Create to generate real drafts. Sample mode stays available and never calls a provider.",
  unavailable:
    "Personal AI connections are not available on this installation yet. Your sample-draft workflow is still available.",
  relayNotice:
    "aiwanwu is a third-party service. Testing sends your key and a short prompt to 2api.aiwanwu.cc. Generation also sends your confirmed job description and resume facts through this service.",
  providerChange: "Switching providers requires a key for the new provider.",
  keyHelp:
    "Your key is encrypted on the server and is never shown again. Leave this empty to keep your saved key.",
  newKeyHelp:
    "Your key is encrypted on the server. It is used when you test the connection or generate with your enabled AI connection.",
  cost: "Testing sends a short, fixed prompt and may incur a small provider charge. Your resume and job description are not sent.",
  limitHelp:
    "Limits generation calls over a rolling 24 hours. Two documents reserve two calls. Failed or uncertain attempts count too. This is not a money cap; connection tests have a separate limit.",
  saved: "Settings saved. Your key has been cleared from this form.",
  deleted:
    "Connection removed. Saved keys have been deleted from this account.",
  deleteHelp:
    "Remove all saved keys for this account? This does not revoke your key at the provider or stop a request already in progress.",
  refreshed:
    "Latest settings loaded. Your unsaved edits are still here; check them before saving.",
  uncertain:
    "The result could not be confirmed. Check this same test again without starting another provider call.",
  statuses: {
    untested: "Not tested",
    testing: "Testing connection…",
    succeeded: "Connection verified",
    failed: "Test failed",
    inconclusive: "Result uncertain",
  },
  failures: {
    provider_auth_failed:
      "The provider rejected this key. Check its permissions or replace it.",
    provider_model_unavailable:
      "The provider returned 404. The model or API endpoint may be unavailable. Check the provider connection details.",
    provider_rate_limited:
      "The provider reported a rate or quota limit. Check your provider account before testing again.",
    provider_unavailable:
      "The provider could not be reached. You can explicitly test again later.",
    provider_invalid_response:
      "The provider did not return a usable response. The result is uncertain.",
    provider_timeout:
      "The provider did not respond in time. This attempt may still have incurred a charge.",
    credential_unavailable:
      "The server could not decrypt your saved key. Contact your installation administrator or replace the key.",
    test_interrupted:
      "The test was interrupted. Its result is unknown; another test may incur another charge.",
  } as Record<string, string>,
};
