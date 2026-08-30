/**
 * APIクライアントの共通部分。
 * エラーレスポンスの形式は docs/05-architecture.md 5章で全エンドポイント共通に決まっているので、
 * 各画面が個別にエラー解釈を書かなくて済むよう、ここで1回だけ変換する。
 */

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/api/v1";

/** 監視系エンドポイント（/healthz など）は /api/v1 の外にあるため、オリジンだけを取り出す。 */
export function apiOrigin(): string {
  return new URL(BASE_URL).origin;
}

/** サーバーが返すエラーコード。docs/05-architecture.md 5章の表と対応する。 */
export type ApiErrorCode =
  | "VALIDATION_ERROR"
  | "NOT_FOUND"
  | "FORBIDDEN"
  | "INSUFFICIENT_STOCK"
  | "INVALID_STATUS_TRANSITION"
  | "INTERNAL_ERROR";

export interface ApiErrorBody {
  error: {
    code: ApiErrorCode;
    message: string;
    details?: unknown[];
  };
}

/** 在庫不足(409)のときに details へ入る内訳。 */
export interface StockShortage {
  skuCode: string;
  requested: number;
  available: number;
}

export class ApiError extends Error {
  readonly code: ApiErrorCode;
  readonly status: number;
  readonly details: unknown[];
  /** 障害調査でサーバーログと突き合わせるためのID。 */
  readonly requestId: string | null;

  constructor(
    status: number,
    code: ApiErrorCode,
    message: string,
    details: unknown[],
    requestId: string | null,
  ) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
    this.requestId = requestId;
  }

  /** 在庫不足なら不足SKUの一覧を返す。それ以外は空配列。 */
  stockShortages(): StockShortage[] {
    if (this.code !== "INSUFFICIENT_STOCK") return [];
    return this.details as StockShortage[];
  }
}

interface RequestOptions {
  method?: string;
  body?: unknown;
  /** 注文登録など、再送で二重実行されては困る操作に付ける(FR-302)。 */
  idempotencyKey?: string;
  signal?: AbortSignal;
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = "GET", body, idempotencyKey, signal } = options;

  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (idempotencyKey) headers["Idempotency-Key"] = idempotencyKey;

  const token = getAccessToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const response = await fetch(`${BASE_URL}${path}`, {
    method,
    headers,
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
    ...(signal ? { signal } : {}),
  });

  if (!response.ok) {
    throw await toApiError(response);
  }

  // 204 など本文を持たないレスポンス。
  if (response.status === 204 || response.headers.get("Content-Length") === "0") {
    return undefined as T;
  }

  return (await response.json()) as T;
}

async function toApiError(response: Response): Promise<ApiError> {
  const requestId = response.headers.get("X-Request-ID");

  // 502/504 などプロキシが返すエラーは、想定のJSON形式になっていない。
  // パースに失敗しても ApiError として扱えるようフォールバックする。
  try {
    const body = (await response.json()) as ApiErrorBody;
    return new ApiError(
      response.status,
      body.error.code,
      body.error.message,
      body.error.details ?? [],
      requestId,
    );
  } catch {
    return new ApiError(
      response.status,
      "INTERNAL_ERROR",
      `サーバーとの通信に失敗しました (HTTP ${response.status})`,
      [],
      requestId,
    );
  }
}

const ACCESS_TOKEN_KEY = "minato.accessToken";

export function getAccessToken(): string | null {
  return localStorage.getItem(ACCESS_TOKEN_KEY);
}

export function setAccessToken(token: string): void {
  localStorage.setItem(ACCESS_TOKEN_KEY, token);
}

export function clearAccessToken(): void {
  localStorage.removeItem(ACCESS_TOKEN_KEY);
}
