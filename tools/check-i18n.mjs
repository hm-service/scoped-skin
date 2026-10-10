#!/usr/bin/env node
/**
 * i18n 自检：把源码里所有 `t("…")` / `t('…')` 的字面量和两个英文字典对一遍。
 *
 *   node tools/check-i18n.mjs
 *
 * - 漏翻：源码里有、字典里没有 → 报错并列出所在文件
 * - 多余：字典里有、源码里找不到 → 报错
 *
 * 两本字典（i18n-en-app.js / i18n-en-admin.js）一起读，整个 web/js 一起扫，
 * 这样主应用与登录/维护页各自改各自的，合并后才不会互相报假错。
 */
import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

import { app } from "../web/js/i18n-en-app.js";
import { admin } from "../web/js/i18n-en-admin.js";

const ROOT = join(dirname(fileURLToPath(import.meta.url)), "..");
const WEB_JS = join(ROOT, "web", "js");

/** i18n 骨架与字典本身不参与扫描（它们里面的示例都写在注释里）。 */
const SKIP = new Set(["i18n.js", "i18n-en-app.js", "i18n-en-admin.js"]);

/** t("…") / t('…')：前面不能是标识符字符（避开 toast(、emit(、getContext( 之类）。 */
const CALL = /(?<![\w$.])t\(\s*(["'])((?:\\.|(?!\1)[^\\])*)\1/g;

function walk(dir, out = []) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) walk(path, out);
    else if (entry.endsWith(".js")) out.push(path);
  }
  return out;
}

const sources = new Map(); // 原文 -> [相对路径]
for (const path of walk(WEB_JS)) {
  const base = path.slice(path.lastIndexOf("/") + 1);
  if (SKIP.has(base)) continue;
  const text = readFileSync(path, "utf8");
  const file = relative(ROOT, path);
  for (const match of text.matchAll(CALL)) {
    const key = match[2];
    if (!sources.has(key)) sources.set(key, []);
    if (!sources.get(key).includes(file)) sources.get(key).push(file);
  }
}

const dictionary = { ...app, ...admin };

const missing = [...sources.keys()].filter((key) => !(key in dictionary)).sort();
const extra = Object.keys(dictionary).filter((key) => !sources.has(key)).sort();

if (missing.length) {
  console.error(`漏翻 ${missing.length} 条：`);
  for (const key of missing) console.error(`  ${JSON.stringify(key)}  ← ${sources.get(key).join(", ")}`);
}
if (extra.length) {
  console.error(`多余 ${extra.length} 条（字典里有、源码里没找到）：`);
  for (const key of extra) console.error(`  ${JSON.stringify(key)}`);
}
if (missing.length || extra.length) process.exit(1);

console.log(`i18n ok: ${sources.size} 条`);
