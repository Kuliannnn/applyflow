import { request } from "../../shared/api/client";
import type {
  AIConfig,
  AIConfigPut,
  AIConfigTest,
  AIProviders,
} from "../../shared/api/generated";
const path = "/me/ai-config";
export const aiAPI = {
  read: (signal?: AbortSignal) => request<AIConfig>(path, { signal }),
  providers: (signal?: AbortSignal) =>
    request<AIProviders>("/ai/providers", { signal }),
  save: (body: AIConfigPut) => request<AIConfig>(path, { method: "PUT", body }),
  test: (body: AIConfigTest, key: string) =>
    request<AIConfig>(path + "/test", { method: "POST", body, key }),
  enable: (version: number, enabled: boolean) =>
    request<AIConfig>(path, {
      method: "PATCH",
      body: { expected_version: version, enabled },
    }),
  remove: (version: number) =>
    request<AIConfig>(path, { method: "DELETE", ifMatch: `"ai-${version}"` }),
};
