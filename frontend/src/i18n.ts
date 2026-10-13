/**
 * 极简 i18n：与后端 i18n/i18n.go 同一套策略 —— **键是点号分层的符号名，中英两栏**。
 *
 * 用法：
 * - 模板 / 代码里都 `import { t } from "@/i18n"`：`t("upload.drop")`、
 *   带变量就 `t("toast.equipped", { name })`
 * - 文案表按区域拆两份（locales/app.ts / locales/admin.ts），每条形如
 *   `"upload.drop": { zh: "点击或拖入 PNG", en: "Click or drop a PNG" }`
 * - 键写错不静默：t() 的参数类型就是合并后 catalog 的键，vue-tsc 直接报错
 *
 * 语言选择存在 localStorage，不进账号；切换后直接 reload（文案散在模板里，重载最省事）。
 */
import { admin as adminCatalog } from "@/locales/admin";
import { app as appCatalog } from "@/locales/app";

const STORAGE_KEY = "sms.locale";
const SOURCE = "zh-Hans";

export const LANGUAGES = [
  { id: "zh-Hans", label: "简体中文", short: "中" },
  { id: "en", label: "English", short: "EN" },
] as const;

export type Locale = (typeof LANGUAGES)[number]["id"];

/** 两张表合并成一张 catalog（跨页共用的键只在 locales/app.ts 里定义一次）。 */
const catalog = { ...appCatalog, ...adminCatalog };

/** t() 认得的键名。 */
export type MessageKey = keyof typeof catalog;

/** t() 的变量：文案里写成 {name}，这里给什么就换成什么。 */
export type MessageVars = Record<string, string | number>;

/** 把浏览器给的 / 存下来的语言串归一成我们支持的两种之一（认不出来返回空串）。 */
function normalizeLocale(value: string | null | undefined): Locale | "" {
  const text = String(value || "").toLowerCase();
  if (text.startsWith("en")) return "en";
  if (text.startsWith("zh")) return SOURCE;
  return "";
}

/** 当前界面语言：先是用户选的，其次跟浏览器，最后简体中文。 */
export function getLocale(): Locale {
  try {
    const saved = normalizeLocale(localStorage.getItem(STORAGE_KEY));
    if (saved) return saved;
  } catch {
    /* 隐私模式下 localStorage 可能不可写，忽略 */
  }
  return normalizeLocale(navigator.language) || SOURCE;
}

export function setLocale(id: string): void {
  const next = normalizeLocale(id) || SOURCE;
  try {
    localStorage.setItem(STORAGE_KEY, next);
  } catch {
    /* 同上 */
  }
  location.reload();
}

export function currentLanguage() {
  const current = getLocale();
  return LANGUAGES.find((language) => language.id === current) ?? LANGUAGES[0];
}

export function otherLanguage() {
  const current = getLocale();
  return LANGUAGES.find((language) => language.id !== current) ?? LANGUAGES[0];
}

/** 取文案：变量按 {name} 逐个替换；运行时查不到就原样回键，坏键不会被静默吞掉。 */
export function t(key: MessageKey, vars?: MessageVars): string {
  const item = catalog[key];
  if (!item) return key;
  let out = getLocale() === "en" ? item.en : item.zh;
  if (vars) {
    for (const [name, value] of Object.entries(vars)) {
      out = out.split(`{${name}}`).join(String(value));
    }
  }
  return out;
}

/**
 * 每个页面在挂载前调一次：把 <html lang> 与页面标题换成当前语言。
 *
 * title 传**已经翻好**的标题（调用处写 `t("title.library")`，字典自检才扫得到那个键，
 * 而且只翻这一次）。
 */
export function applyDocumentLocale(title: string): void {
  document.documentElement.lang = getLocale();
  document.title = title;
}
