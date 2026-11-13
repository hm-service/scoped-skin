/**
 * 认证小工具：token 存 localStorage，所有 API 请求带 Bearer 头与 Accept-Language。
 *
 * 没有 token（或后端返回 401）就跳 /auth。
 */
import { getLocale } from "@/i18n";
import type { Player } from "@/types";

const TOKEN_KEY = "sms.token";
// 缓存的是接口原样的 player 快照：字段格式变了就换 key，旧值直接作废（不做迁移）。
const PLAYER_KEY = "sms.player.v1";

export function getToken(): string {
  try {
    return localStorage.getItem(TOKEN_KEY) || "";
  } catch {
    return "";
  }
}

export function setToken(token: string, player?: Player | null): void {
  try {
    localStorage.setItem(TOKEN_KEY, token);
    if (player) localStorage.setItem(PLAYER_KEY, JSON.stringify(player));
  } catch {
    /* 隐私模式下 localStorage 可能不可写，忽略 */
  }
}

export function getUser(): Player | null {
  try {
    return JSON.parse(localStorage.getItem(PLAYER_KEY) || "null");
  } catch {
    return null;
  }
}

function clearToken(): void {
  try {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(PLAYER_KEY);
  } catch {
    /* 同上 */
  }
}

/** 没登录就跳登录页；返回是否已登录。 */
export function requireAuth(): boolean {
  if (getToken()) return true;
  location.replace("/auth");
  return false;
}

/** 退出 / 掉登录态：清掉 token 回登录页。 */
export function toLogin(): void {
  clearToken();
  location.replace("/auth");
}

export function authHeaders(extra: Record<string, string> = {}): Record<string, string> {
  const token = getToken();
  // Accept-Language 就是当前界面语言：后端按它挑错误文案
  const headers: Record<string, string> = { ...extra, "Accept-Language": getLocale() };
  return token ? { ...headers, Authorization: `Bearer ${token}` } : headers;
}
