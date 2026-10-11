#!/usr/bin/env node
/**
 * i18n 自检：把 web/js 里所有 `t("…")` 的键与两张文案表对一遍。
 *
 *   node tools/check-i18n.mjs
 *
 * - 漏配：源码里用到、表里没有 → 报错并列出所在文件
 * - 多配：表里有、源码里找不到 → 报错
 * - 缺栏：某条只有中文或只有英文 → 报错
 * - 键名不合规范（不是点号分层的小写符号名）→ 报错
 * - 同一个键在两张表里各定义一次 → 报错（跨页共用的键只在 i18n-app.js 里定义）
 *
 * 两张表（i18n-app.js / i18n-admin.js）一起读，整个 web/js 一起扫，
 * 这样主应用与登录/维护页各自改各自的，合并后才不会互相报假错。
 */
import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

import { app } from "../web/js/i18n-app.js";
import { admin } from "../web/js/i18n-admin.js";

const ROOT = join(dirname(fileURLToPath(import.meta.url)), "..");
const WEB_JS = join(ROOT, "web", "js");

/** i18n 骨架与两张表本身不参与扫描（它们里面的示例都写在注释里）。 */
const SKIP = new Set(["i18n.js", "i18n-app.js", "i18n-admin.js"]);

/** t("…") / t('…')：前面不能是标识符字符（避开 toast(、emit(、getContext( 之类）。 */
const CALL = /(?<![\w$.])t\(\s*(["'])((?:\\.|(?!\1)[^\\])*)\1/g;

/** 键名：点号分层的符号名，每段小写字母 / 数字 / 连字符，如 library.search-placeholder。 */
const KEY = /^[a-z][a-z0-9-]*(\.[a-z0-9-]+)+$/;

function walk(dir, out = []) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) walk(path, out);
    else if (entry.endsWith(".js")) out.push(path);
  }
  return out;
}

const sources = new Map(); // 键 -> [相对路径]
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

const tables = { "i18n-app.js": app, "i18n-admin.js": admin };
const catalog = {};
const broken = [];
for (const [file, table] of Object.entries(tables)) {
  for (const [key, entry] of Object.entries(table)) {
    if (key in catalog) broken.push(`${file}: 键 ${key} 在两张表里重复定义`);
    catalog[key] = entry;
    if (!KEY.test(key)) broken.push(`${file}: 键名不合规范 ${JSON.stringify(key)}`);
    if (!entry || typeof entry.zh !== "string" || entry.zh.trim() === "") broken.push(`${file}: ${key} 缺中文`);
    if (!entry || typeof entry.en !== "string" || entry.en.trim() === "") broken.push(`${file}: ${key} 缺英文`);
  }
}

const missing = [...sources.keys()].filter((key) => !(key in catalog)).sort();
const extra = Object.keys(catalog).filter((key) => !sources.has(key)).sort();

if (missing.length) {
  console.error(`漏配 ${missing.length} 条（源码里用了、表里没有）：`);
  for (const key of missing) console.error(`  ${JSON.stringify(key)}  ← ${sources.get(key).join(", ")}`);
}
if (extra.length) {
  console.error(`多配 ${extra.length} 条（表里有、源码里没找到）：`);
  for (const key of extra) console.error(`  ${JSON.stringify(key)}`);
}
if (broken.length) {
  console.error(`表本身有问题 ${broken.length} 条：`);
  for (const line of broken) console.error(`  ${line}`);
}
if (missing.length || extra.length || broken.length) process.exit(1);

console.log(`i18n ok: ${sources.size} 个键，${Object.keys(catalog).length} 条文案（中英两栏齐全）`);
