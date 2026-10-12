<script setup lang="ts">
/**
 * 卡片悬浮面板：卡片上只放得下四个 chip，其余标签在这里补全。
 *
 * 固定定位、挂在列表外面 —— 列表是滚动容器，面板放进去会被裁掉。
 * 尺寸要渲染出来才知道，所以在 post flush 里量一次再定位，不会闪。
 */
import { computed, ref, watch } from "vue";

import TagCapsule from "@/components/TagCapsule.vue";
import { store } from "@/store";
import { shortId } from "@/tags";

const box = ref<HTMLElement | null>(null);
const item = computed(() => store.state.hover.item);

function place(): void {
  const element = box.value;
  const anchor = store.state.hover.anchor;
  if (!element || !anchor) return;
  const width = element.offsetWidth;
  const height = element.offsetHeight;
  // 尽量居中挂在卡片下/上方，但不越出列表面板（免得盖住左边的分类栏）
  const bounds = document.querySelector("#grid")?.getBoundingClientRect();
  const minLeft = bounds ? bounds.left + 4 : 8;
  const maxLeft = (bounds ? bounds.right : window.innerWidth) - width - 4;
  const left = Math.min(Math.max(minLeft, anchor.left + anchor.width / 2 - width / 2), Math.max(minLeft, maxLeft));
  let top = anchor.bottom + 8;
  if (top + height > window.innerHeight - 8) top = Math.max(8, anchor.top - height - 8);
  element.style.left = `${Math.round(left)}px`;
  element.style.top = `${Math.round(top)}px`;
  element.style.visibility = "visible";
}

watch(
  item,
  () => {
  if (box.value) box.value.style.visibility = "hidden";
  if (item.value) place();
  },
  { flush: "post" },
);
</script>

<template>
  <div v-if="item" ref="box" class="card-hover">
    <div class="tags">
      <TagCapsule v-for="tag in item.tags" :key="tag" :tag="tag"/>
      <span v-if="!item.tags || !item.tags.length" class="tag-empty">{{ shortId(item) }}</span>
    </div>
    <p class="card-hover-meta">
      <span>{{ store.modelLabel(item) }} · {{ item.width }}×{{ item.height }}</span>
      <span class="card-hover-id">{{ shortId(item) }}</span>
    </p>
  </div>
</template>

<style scoped>
/* 卡片放不下的标签在悬浮面板里补全（固定定位，不会被列表的滚动容器裁掉） */
.card-hover {
  position: fixed;
  z-index: 45;
  width: 236px;
  max-height: 216px;
  overflow-y: auto;
  padding: 10px 12px;
  border: 1px solid var(--line);
  border-radius: var(--radius-md);
  background: var(--paper);
  box-shadow: var(--shadow-pop);
  visibility: hidden;   /* place() 量完尺寸再显示，免得先闪在左上角 */
  pointer-events: none; /* 别跟卡片自己的 hover 抢鼠标 */
}

.card-hover .tags { gap: 5px; }

.card-hover-meta {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  margin: 9px 0 0;
  font-size: 11px;
  color: var(--ink-faint);
}

.card-hover-id { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
</style>
