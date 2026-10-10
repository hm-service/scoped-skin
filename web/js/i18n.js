/**
 * 极简 i18n：**简体中文是源语言** —— 代码/模板里写的就是中文，英文走字典。
 *
 * 用法：
 * - 模板里：`{{ t("点击或拖入 PNG") }}`（由 installI18n 挂到 globalProperties 上）
 * - JS 里：`import { t } from "./i18n.js"`；带变量就 `t("已穿上「{name}」", { name })`
 * - 字典按区域拆文件（i18n-en-app.js / i18n-en-admin.js），拼起来就是全量英文
 *
 * 语言选择存在 localStorage，不进账号；切换后直接 reload（文案散在模板里，重载最省事）。
 */
import { app as appDictionary } from "./i18n-en-app.js";
import { admin as adminDictionary } from "./i18n-en-admin.js";

const STORAGE_KEY = "sms.locale";
const SOURCE = "zh-Hans";

const LANGUAGES = [
  { id: "zh-Hans", label: "简体中文", short: "中" },
  { id: "en", label: "English", short: "EN" },
];

const dictionaries = {
  en: { ...appDictionary, ...adminDictionary },
};

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

/** 取文案：字典里没有就原样返回中文（也方便一眼看出漏翻）。 */
export function t(text, vars) {
  const table = dictionaries[getLocale()];
  let out = (table && table[text]) || text;
  if (vars) {
    for (const [key, value] of Object.entries(vars)) {
      out = out.split(`{${key}}`).join(String(value));
    }
  }
  return out;
}

/**
 * 挂到 Vue 应用上：模板里就能直接用 t(...)，顺带把 <html lang> 与页面标题也换成当前语言。
 *
 * title 传**已经翻好**的标题（调用处写 `t("…")`，这样字典自检才扫得到）；不传就不动。
 */
export function installI18n(app, title) {
  document.documentElement.lang = getLocale();
  if (title) document.title = t(title);
  app.config.globalProperties.t = t;
  return app;
}
