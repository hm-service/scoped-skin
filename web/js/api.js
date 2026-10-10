/**
 * 统一的 fetch 封装：自动带 Bearer 头，401 直接把人送回登录页。
 */
import { authHeaders, toLogin } from "./auth.js";
import { t } from "./i18n.js";

export async function api(path, options = {}) {
  const response = await fetch(path, { ...options, headers: authHeaders(options.headers) });
  const payload = await response.json().catch(() => ({}));
  if (response.status === 401) {
    toLogin(); // token 失效：清掉回登录页
    throw new Error(payload.detail || t("登录状态已失效"));
  }
  if (!response.ok) {
    const error = new Error(payload.detail || t("请求失败 ({status})", { status: response.status }));
    error.status = response.status;
    throw error;
  }
  return payload;
}
