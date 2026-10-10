/** 标签相关的小工具：配色、排序、模糊匹配、代称（纯函数，不碰 DOM）。 */
import { t } from "./i18n.js";

/** 匹配级别：前缀 → 中缀 → 后缀。 */
export const MATCH_LABELS = [t("前缀"), t("中缀"), t("后缀")];

/** 同一个标签永远落在同一个配色上。 */
export function tagColor(tag) {
  let hash = 0;
  for (const char of tag) hash = (hash * 31 + char.codePointAt(0)) % 100003;
  return `c${hash % 5}`;
}

/** 没有标签时用来指代一个贴图：内容 sha256 的前 8 位。 */
export function shortId(item) {
  return `#${(item.id || "").slice(0, 8)}`;
}

export function hasTag(selected, tag) {
  const needle = tag.toLowerCase();
  return [...selected].some((item) => item.toLowerCase() === needle);
}

/** 现有标签的统一排序：用得多的在前，其次按名称（下拉候选与「已有」列表共用）。 */
export const byPopularity = (a, b) =>
  b.count - a.count || a.tag.localeCompare(b.tag, "zh");

/**
 * 模糊匹配：前缀(0) → 中缀(1) → 后缀(2)，同级按使用次数、名称排序；
 * 末尾在没有同名标签、也没被选中时补一条「新建」。
 */
export function matchTags(query, library, selected = [], limit = 8) {
  const text = query.trim();
  const needle = text.toLowerCase();

  if (!text) {
    return [...library]
      .sort(byPopularity)
      .slice(0, limit)
      .map(({ tag, count }) => ({ action: "add", tag, count, rank: 0, from: -1 }));
  }

  const found = [];
  for (const { tag, count } of library) {
    const at = tag.toLowerCase().indexOf(needle);
    if (at < 0) continue;
    const rank = at === 0 ? 0 : at + needle.length === tag.length ? 2 : 1;
    found.push({ action: "add", tag, count, rank, from: at });
  }
  found.sort((a, b) => a.rank - b.rank || b.count - a.count || a.tag.localeCompare(b.tag, "zh"));

  const result = found.slice(0, limit);
  const exists = library.some(({ tag }) => tag.toLowerCase() === needle);
  if (!exists && !hasTag(selected, text)) {
    result.push({ action: "create", tag: text, count: 0, rank: 3, from: -1 });
  }
  return result;
}
