<script setup lang="ts">
/**
 * 标签输入：已选 capsule + 自由输入 + 「前缀 → 中缀 → 后缀」排序的下拉候选。
 * 组件本身不持有标签，只把 add / remove 抛给上层。
 */
import { computed, nextTick, ref, watch } from "vue";

import { t } from "@/i18n";
import { hasTag, matchTags } from "@/tags";
import type { TagCandidate } from "@/tags";
import type { TagCount } from "@/types";

const props = withDefaults(
  defineProps<{
  selected?: readonly string[];
  library?: readonly TagCount[];
  }>(),
  { selected: () => [], library: () => [] },
);
const emit = defineEmits<{
  (e: "add", tag: string): void;
  (e: "remove", tag: string): void;
}>();

const input = ref<HTMLInputElement | null>(null);
const text = ref("");
const open = ref(false);
const index = ref(-1);
const needle = computed(() => text.value.trim());
const suggestions = computed(() => matchTags(text.value, props.library, props.selected));

// 候选变化后把高亮项拉回有效范围
watch(suggestions, () => {
  index.value = suggestions.value.length ? 0 : -1;
});

function show(): void {
  open.value = true;
  index.value = suggestions.value.length ? 0 : -1;
}

function close(): void {
  open.value = false;
  index.value = -1;
}

function add(tag: string): void {
  const clean = tag.trim();
  if (!clean) return close();
  if (!hasTag(props.selected, clean)) emit("add", clean);
  text.value = "";
  close();
  nextTick(() => input.value?.focus());
}

function apply(entry: TagCandidate): void {
  if (entry.action === "create") add(entry.tag);
  else if (hasTag(props.selected, entry.tag)) emit("remove", entry.tag);
  else add(entry.tag);
}

function move(step: number): void {
  if (!suggestions.value.length) return;
  const size = suggestions.value.length;
  index.value = (index.value + step + size) % size;
}

function onInput(): void {
  show();
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === "ArrowDown") {
  event.preventDefault();
  if (!open.value) show();
  else move(1);
  } else if (event.key === "ArrowUp") {
  event.preventDefault();
  move(-1);
  } else if (event.key === "Enter") {
  event.preventDefault();
  if (index.value >= 0) apply(suggestions.value[index.value]);
  else add(text.value);
  } else if (event.key === "Escape") {
  close();
  } else if (event.key === "," || event.key === "，") {
  event.preventDefault();
  add(text.value);
  } else if (event.key === "Backspace" && !text.value && props.selected.length) {
  emit("remove", props.selected.at(-1)!);
  }
}

// mousedown 而不是 click：避免 input 先失焦
function onBlur(): void {
  setTimeout(close, 120);
}

function focusAndShow(): void {
  input.value?.focus();
  show();
}
</script>

<template>
  <div class="tag-combo" @click="focusAndShow">
    <span class="tag-selected">
      <span v-for="tag in selected" :key="tag" class="tag-chip">
        <span>{{ tag }}</span>
        <button type="button" class="remove" :title="t('tag.remove')" @click.stop="emit('remove', tag)">
          <svg class="icon"><use href="#i-x"/></svg>
        </button>
      </span>
    </span>
    <span class="tag-input-wrap">
      <input ref="input" v-model="text" type="text" autocomplete="off" spellcheck="false"
             role="combobox" :aria-expanded="open" aria-controls="tagSuggest"
             aria-autocomplete="list"
             :placeholder="selected.length ? t('tag.add-more') : t('tag.placeholder')"
             @input="onInput" @focus="show" @blur="onBlur" @keydown="onKeydown">
      <ul class="tag-suggest" id="tagSuggest" role="listbox" v-show="open">
        <li v-if="!suggestions.length" class="sug-empty">{{ t("tag.no-match") }}</li>
        <li v-for="(entry, i) in suggestions" :key="entry.action + entry.tag" role="option"
            class="sug-item" :class="{ active: i === index, create: entry.action === 'create' }"
            :title="entry.action === 'create'
              ? t('tag.create', { tag: entry.tag })
              : t('tag.usage', { count: entry.count })"
            @mousedown.prevent="apply(entry)">
          <template v-if="entry.action === 'create'">
            <svg class="icon"><use href="#i-plus"/></svg>
            <span>{{ t("tag.create-quoted", { tag: entry.tag }) }}</span>
          </template>
          <template v-else>
            <span v-if="entry.from >= 0">
              {{ entry.tag.slice(0, entry.from) }}<mark>{{ entry.tag.slice(entry.from, entry.from + needle.length) }}</mark>{{ entry.tag.slice(entry.from + needle.length) }}
            </span>
            <span v-else>{{ entry.tag }}</span>
            <span class="sug-count">{{ t("tag.usage", { count: entry.count }) }}</span>
          </template>
        </li>
      </ul>
    </span>
  </div>
</template>

<style scoped>
/* 已选 + 输入框合成一个多选输入框 */
.tag-combo {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  min-height: 42px;
  padding: 5px 8px;
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-md);
  background: var(--paper);
  transition: border-color .15s, box-shadow .15s;
}

.tag-combo:focus-within { border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-soft); }

.tag-selected { display: contents; }

.tag-chip .remove {
  display: grid;
  place-items: center;
  width: 16px;
  height: 16px;
  padding: 0;
  border: none;
  border-radius: 50%;
  background: transparent;
  color: inherit;
  opacity: .6;
}

.tag-chip .remove:hover { opacity: 1; background: rgba(18, 112, 63, .14); }

.tag-chip .remove .icon { width: 11px; height: 11px; stroke-width: 2.2; }

.tag-input-wrap { position: relative; flex: 1 1 170px; display: flex; min-width: 150px; }

.tag-input-wrap input {
  flex: 1 1 auto;
  min-width: 0;
  padding: 4px 2px;
  border: none;
  background: transparent;
  font-size: 13px;
  outline: none;
}

.tag-suggest {
  position: absolute;
  top: calc(100% + 8px);
  left: -4px;
  z-index: 20;
  width: max(240px, 100%);
  max-height: 220px;
  overflow-y: auto;
  margin: 0;
  padding: 4px;
  list-style: none;
  border: 1px solid var(--line);
  border-radius: var(--radius-md);
  background: var(--paper);
  box-shadow: var(--shadow-pop);
}

.sug-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 7px;
  font-size: 13px;
  cursor: pointer;
}

.sug-item.active { background: var(--accent-soft); color: var(--accent-ink); }

.sug-item mark { background: transparent; color: inherit; font-weight: 700; }

.sug-item .sug-count { margin-left: auto; color: var(--ink-faint); font-size: 11px; }

.sug-item.create { color: var(--accent-ink); }

.sug-item.create .icon { width: 13px; height: 13px; }

.sug-empty { padding: 8px; color: var(--ink-faint); font-size: 12.5px; }
</style>
