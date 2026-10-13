<script setup lang="ts">
/** 列表卡片：贴图缩略图（实时裁 2D 正面视图）+ 标签 + 收藏 / 编辑。 */
import { computed, onMounted, ref, watch } from "vue";

import { renderPreview } from "@/avatar";
import TagCapsule from "@/components/TagCapsule.vue";
import { t } from "@/i18n";
import { modelShort, store, thumbnailFor } from "@/store";
import { shortId } from "@/tags";
import type { Item } from "@/types";

const props = defineProps<{ item: Item; active: boolean; writable: boolean }>();
const emit = defineEmits<{
  (e: "select", item: Item): void;
  (e: "favorite", item: Item): void;
  (e: "edit", item: Item): void;
}>();

const canvas = ref<HTMLCanvasElement | null>(null);
const failed = ref(false);
const label = computed(() => modelShort(props.item));
const short = computed(() => shortId(props.item));

/**
 * 卡片上最多放四个 chip（两行），超出的进悬浮面板。
 * 四个以内全显示；四个以上只显示前三个 + 「+N」，这样无论多少标签都是两行 —— 卡片高度
 * 恒定（= .grid 的 --grid-row），虚拟列表按行高算滚动位置才准。
 */
const shownTags = computed(() => {
  const tags = props.item.tags ?? [];
  return tags.length > 4 ? tags.slice(0, 3) : tags;
});
const hiddenTags = computed(() => (props.item.tags?.length ?? 0) - shownTags.value.length);

async function draw(): Promise<void> {
  failed.value = false;
  try {
  const image = await thumbnailFor(props.item);
  const element = canvas.value;
  if (!element) return; // 卡片还没挂上（ref 为空）就没地方画
  // 模型已并进 kind（skin.steve / skin.alex），2D 预览按 kind 画
  renderPreview(element, image, props.item.kind);
  } catch {
  failed.value = true;
  }
}

// 挂上就画。列表本身已经虚拟化了（DOM 里只有视口附近那几十张），再套一层
// 「滚到跟前才画」既省不下多少，又会在窗口边缘漏画 —— 之前用 IntersectionObserver 时，
// 正好压在滚动视口底边上的那一行永远等不到回调，滚过去就是 5 张空白卡。
onMounted(draw);
watch(() => props.item.kind, draw);
</script>

<template>
  <!-- data-id 是给 CDP 验证脚本（shot.mjs 的 --eval）按卡片选元素的稳定钩子，别删 -->
  <article class="card"
           :class="{ cape: item.category === 'cape', on: active, builtin: item.readonly }"
           role="listitem" :data-id="item.id" @click="emit('select', item)"
           @mouseenter="store.hoverCard(item, $event)" @mouseleave="store.unhoverCard(item)">
    <button v-if="writable" class="fav-btn" :class="{ on: item.favorite }" type="button"
            :title="item.favorite ? t('action.unfavorite') : t('action.favorite')" @click.stop="emit('favorite', item)">
      <svg class="icon"><use href="#i-star"/></svg>
    </button>
    <button v-if="!item.readonly && writable" class="edit-btn" type="button" :title="t('action.edit')"
            @click.stop="emit('edit', item)">
      <svg class="icon"><use href="#i-pencil"/></svg>
    </button>
    <div class="thumb">
      <canvas v-show="!failed" ref="canvas"></canvas>
      <span v-if="failed" class="meta">{{ t("card.load-failed") }}</span>
    </div>
    <span v-if="item.readonly" class="builtin-flag">{{ t("card.builtin") }}</span>
    <div class="tags">
      <TagCapsule v-for="tag in shownTags" :key="tag" :tag="tag"/>
      <span v-if="hiddenTags" class="tag-more">+{{ hiddenTags }}</span>
      <span v-if="!item.tags || !item.tags.length" class="tag-empty">{{ short }}</span>
    </div>
    <span class="meta">{{ label }} · {{ item.width }}×{{ item.height }}</span>
  </article>
</template>

<style scoped>
.card {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  /* 上边留宽一点：正面视图是从贴图顶端开始画的，9px 会让脑袋快贴到卡片边 */
  padding: 16px 7px 8px;
  border: 1px solid var(--line);
  border-radius: var(--radius-md);
  background: var(--paper);
  transition: transform .15s, border-color .15s, box-shadow .15s;
  /* 高度必须恒定（缩略图 112 + 两行标签 46 + 一行信息 + 上下内边距 = 212，即 .grid 的 --grid-row）：
   虚拟化按 --grid-row 算位置，行高一旦随内容漂移，滚动位置就会和渲染窗口对不上。 */
  content-visibility: auto;
  contain-intrinsic-size: auto 212px;
}

.card:hover { transform: translateY(-2px); border-color: var(--line-strong); box-shadow: var(--shadow-pop); }

.card.on { border-color: var(--accent); box-shadow: 0 0 0 2px var(--accent-soft); }

.card .thumb {
  height: 112px;
  display: flex;
  align-items: flex-end;
  justify-content: center;
}

.card .thumb canvas {
  height: 100%;
  width: auto;
  image-rendering: pixelated;
  filter: drop-shadow(0 6px 6px rgba(22, 28, 45, .16));
}

/* 披风贴图矮（32 高），给它居中；高度仍与皮肤卡一致，行高才统一 */
.card.cape .thumb { align-items: center; }

/* 标签区固定两行：卡片上最多四个 chip（超过就换成 +N），剩下的进悬浮面板。
   width: 100% 是必须的：否则这个 wrap 容器会缩到内容宽度，百分比上限跟着缩，
   两个短标签也会被截成「官…」。 */
.card .tags {
  width: 100%;
  justify-content: center;
  align-content: flex-start;
  height: 46px;
  overflow: hidden;
}

.card .tag-more {
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--paper-soft);
  color: var(--ink-faint);
  font-size: 10.5px;
  line-height: 1.6;
  white-space: nowrap;
}

.card .meta {
  max-width: 100%;
  font-size: 11px;
  line-height: 1.45;
  color: var(--ink-faint);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.card .tags { justify-content: center; }

.fav-btn, .edit-btn {
  position: absolute;
  top: 5px;
  width: 24px;
  height: 24px;
  display: grid;
  place-items: center;
  padding: 0;
  border: 1px solid transparent;
  border-radius: 8px;
  background: transparent;
  color: var(--line-strong);
  transition: all .15s;
}

.fav-btn { right: 5px; }

.edit-btn { left: 5px; opacity: 0; color: var(--ink-faint); }

.card:hover .edit-btn { opacity: 1; }

.fav-btn .icon { width: 15px; height: 15px; }

.edit-btn .icon { width: 15px; height: 15px; }

.fav-btn:hover { background: #fff6e0; color: var(--warn); border-color: #f6e2b4; }

.fav-btn.on { color: var(--warn); }

.fav-btn.on .icon { fill: currentColor; }

.edit-btn:hover {
  background: var(--accent-soft);
  color: var(--accent-ink);
  border-color: color-mix(in srgb, var(--accent) 35%, transparent);
}

/* 内置资源角标（用户不能删） */
.builtin-flag {
  position: absolute;
  top: 6px;
  left: 6px;
  padding: 1px 7px;
  border-radius: 999px;
  background: #eef1f8;
  color: var(--ink-faint);
  font-size: 10px;
  letter-spacing: .04em;
}

.card.builtin:hover .builtin-flag { background: var(--accent-soft); color: var(--accent-ink); }
</style>
