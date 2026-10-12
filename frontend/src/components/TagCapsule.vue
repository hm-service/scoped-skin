<script setup lang="ts">
/** 标签 capsule：配色由标签内容决定（同一个标签永远同色）。 */
import { computed } from "vue";

import { tagColor } from "@/tags";

const props = defineProps<{ tag: string }>();

const color = computed(() => tagColor(props.tag));
</script>

<template>
  <span class="tag" :class="color">{{ tag }}</span>
</template>

<style scoped>
.card .tags .tag { max-width: 48%; overflow: hidden; text-overflow: ellipsis; }

.tag {
  padding: 2px 9px;
  border-radius: 999px;
  background: var(--paper-sunk);
  color: var(--ink-soft);
  font-size: 10.5px;
  line-height: 1.6;
  white-space: nowrap;
}

/* 标签配色由标签内容决定（同一个标签永远同色） */
.tag.c0 { background: #e7f6ee; color: #1c7a4a; }

.tag.c1 { background: #eaeefb; color: #3b4e8f; }

.tag.c2 { background: #f5ebfa; color: #7a3b8f; }

.tag.c3 { background: #fdf1e3; color: #8a5a1c; }

.tag.c4 { background: #e6f4f8; color: #1d6d84; }
</style>
