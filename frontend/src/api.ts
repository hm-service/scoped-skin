/**
 * 统一的 fetch 封装：自动带 Bearer 头，401 直接把人送回登录页。
 */
import { authHeaders, toLogin } from "@/auth";
import { t } from "@/i18n";
import type { ErrorResponse } from "@/types";

/** 接口报错：带 HTTP 状态码，调用处按它区分 403 / 404 之类。 */
export class ApiError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(path, { ...options, headers: authHeaders(options.headers as Record<string, string>) });
  const payload = (await response.json().catch(() => ({}))) as T & ErrorResponse;
  if (response.status === 401) {
    toLogin(); // token 失效：清掉回登录页
    throw new ApiError(payload.detail || t("toast.session-expired"), 401);
  }
  if (!response.ok) {
    throw new ApiError(payload.detail || t("error.request", { status: response.status }), response.status);
  }
  return payload;
}
