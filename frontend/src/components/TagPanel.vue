<script setup lang="ts">
/**
 * 标签面板：搜索框旁边的按钮，点开是库里所有标签（按热度排序，同数按名称）。
 * 点标签＝把它加进搜索串，再点一下移出去；面板本身不改标签，只是个搜索快捷键。
 */
import { computed, onBeforeUnmount, onMounted, ref } from "vue";

import { t } from "@/i18n";
import { store } from "@/store";
import { byPopularity } from "@/tags";

const root = ref<HTMLElement | null>(null);
const open = ref(false);
const tags = computed(() => [...store.state.library].sort(byPopularity));

// 点面板外面 / 按 Esc 收起；面板内部的点击（包括按钮自己）不算「外面」
function onPointerDown(event: PointerEvent): void {
  if (root.value && !root.value.contains(event.target as Node)) open.value = false;
}
function onKeydown(event: KeyboardEvent): void {
  if (event.key === "Escape") open.value = false;
}

onMounted(() => {
  document.addEventListener("pointerdown", onPointerDown);
  document.addEventListener("keydown", onKeydown);
});
onBeforeUnmount(() => {
  document.removeEventListener("pointerdown", onPointerDown);
  document.removeEventListener("keydown", onKeydown);
});
</script>

<template>
  <div ref="root" class="tag-panel">
    <button type="button" class="tag-panel-btn" :class="{ on: open }"
            :title="t('library.tags-all')" :aria-label="t('library.tags-all')" :aria-expanded="open"
            @click="open = !open">
      <svg class="icon"><use href="#i-tag"/></svg>
    </button>
    <div class="tag-panel-pop" v-show="open">
      <p class="tag-panel-hint">{{ t("library.tags-hint") }}</p>
      <div class="tag-panel-list">
        <button v-for="entry in tags" :key="entry.tag" type="button"
                class="tag-chip ghost" :class="{ on: store.queryHasTag(entry.tag) }"
                @click="store.toggleQueryTag(entry.tag)">
          <span>{{ entry.tag }}</span><span class="count">{{ entry.count }}</span>
        </button>
        <span v-if="!tags.length" class="tag-empty">{{ t("library.tags-empty") }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 标签面板：搜索框右边的按钮 + 悬浮出来的全部标签 */
.tag-panel { position: relative; align-self: stretch; display: flex; }

.tag-panel-btn {
  display: grid;
  place-items: center;
  padding: 0 10px;
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-md);
  background: var(--paper);
  color: var(--ink-soft);
  transition: border-color .15s, background .15s, color .15s;
}

.tag-panel-btn .icon { width: 15px; height: 15px; }

.tag-panel-btn:hover { border-color: var(--accent); color: var(--accent-ink); }

.tag-panel-btn.on { border-color: var(--accent); background: var(--accent-soft); color: var(--accent-ink); }

.tag-panel-pop {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  z-index: 30;
  width: 272px;
  max-height: 288px;
  overflow-y: auto;
  padding: 10px;
  border: 1px solid var(--line);
  border-radius: var(--radius-md);
  background: var(--paper);
  box-shadow: var(--shadow-pop);
}

.tag-panel-hint { margin: 0 0 8px; font-size: 11.5px; color: var(--ink-faint); }

.tag-panel-list { display: flex; flex-wrap: wrap; gap: 6px; }
</style>
