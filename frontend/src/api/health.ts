import { apiOrigin } from "./client";

export interface HealthResponse {
  status: string;
  version: string;
}

/**
 * 死活確認。/healthz は /api/v1 の外側にあり、認証も不要なので
 * 通常の request() ではなくオリジン直下を叩く（docs/04-api-spec.md 2章）。
 */
export async function fetchHealth(signal?: AbortSignal): Promise<HealthResponse> {
  const response = await fetch(`${apiOrigin()}/healthz`, signal ? { signal } : {});
  if (!response.ok) {
    throw new Error(`ヘルスチェックに失敗しました (HTTP ${response.status})`);
  }
  return (await response.json()) as HealthResponse;
}
