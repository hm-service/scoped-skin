/**
 * 极简 i18n：与后端 i18n/i18n.go 同一套策略 —— **键是点号分层的符号名，中英两栏**。
 *
 * 用法：
 * - 模板里：`{{ t("upload.drop") }}`（由 installI18n 挂到 globalProperties 上）
 * - JS 里：`import { t } from "./i18n.js"`；带变量就 `t("toast.equipped", { name })`
 * - 文案表按区域拆两份（i18n-app.js / i18n-admin.js），每条形如
 *   `"upload.drop": { zh: "点击或拖入 PNG", en: "Click or drop a PNG" }`
 * - 键写错不静默：和后端 T() 一样原样返回键本身，`make i18n` 会在提交前拦住
 *
 * 语言选择存在 localStorage，不进账号；切换后直接 reload（文案散在模板里，重载最省事）。
 */
import { app as appCatalog } from "./i18n-app.js";
import { admin as adminCatalog } from "./i18n-admin.js";

const STORAGE_KEY = "sms.locale";
const SOURCE = "zh-Hans";

const LANGUAGES = [
  { id: "zh-Hans", label: "简体中文", short: "中" },
  { id: "en", label: "English", short: "EN" },
];

/** 两张表合并成一张 catalog（跨页共用的键只在 i18n-app.js 里定义一次）。 */
const catalog = { ...appCatalog, ...adminCatalog };

/** 把浏览器给的 / 存下来的语言串归一成我们支持的两种之一（认不出来返回空串）。 */
function normalizeLocale(value) {
  const text = String(value || "").toLowerCase();
  if (text.startsWith("en")) return "en";
  if (text.startsWith("zh")) return SOURCE;
  return "";
}

/** 当前界面语言：先是用户选的，其次跟浏览器，最后简体中文。 */
export function getLocale() {
  try {
    const saved = normalizeLocale(localStorage.getItem(STORAGE_KEY));
    if (saved) return saved;
  } catch {
    /* 隐私模式下 localStorage 可能不可写，忽略 */
  }
  return normalizeLocale(navigator.language) || SOURCE;
}

export function setLocale(id) {
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

/** 取文案：键不在表里就原样返回键（和后端 T() 一致，便于一眼看出漏配）。 */
export function t(key, vars) {
  const item = catalog[key];
  let out = item ? (getLocale() === "en" ? item.en : item.zh) : key;
  if (vars) {
    for (const [name, value] of Object.entries(vars)) {
      out = out.split(`{${name}}`).join(String(value));
    }
  }
  return out;
}

/**
 * 挂到 Vue 应用上：模板里就能直接用 t(...)，顺带把 <html lang> 与页面标题也换成当前语言。
 *
 * title 传**已经翻好**的标题（调用处写 `t("title.library")`，这样字典自检才扫得到那个键，
 * 而且只翻这一次）；不传就不动标题。
 */
export function installI18n(app, title) {
  document.documentElement.lang = getLocale();
  if (title) document.title = title;
  app.config.globalProperties.t = t;
  return app;
}
