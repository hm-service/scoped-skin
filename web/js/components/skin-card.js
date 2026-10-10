/** 列表卡片：贴图缩略图（实时裁 2D 正面视图）+ 标签 + 收藏 / 编辑。 */
import { renderPreview } from "../avatar.js";
import { t } from "../i18n.js";
import { modelShort, store, thumbnailFor } from "../store.js";
import { shortId } from "../tags.js";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "../vue.js";
import { TagCapsule } from "./tag-capsule.js";

/**
 * 全列表共用一份 IntersectionObserver：卡片滚到视口附近才画缩略图。
 * 每张卡片各建一个 observer 是几百个回调对象，没必要；这里用一张表按元素派发。
 */
const pendingDraws = new Map();
let drawObserver = null;

function drawWhenNear(element, draw) {
  if (!element) return () => {};
  if (!drawObserver) {
    drawObserver = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) continue;
          const run = pendingDraws.get(entry.target);
          pendingDraws.delete(entry.target);
          drawObserver.unobserve(entry.target);
          run?.();
        }
      },
      // 提前一屏开画：滚下来时图上已经有了，不会看到空白卡片
      { rootMargin: "300px 0px" },
    );
  }
  pendingDraws.set(element, draw);
  drawObserver.observe(element);
  return () => {
    pendingDraws.delete(element);
    drawObserver.unobserve(element);
  };
}

export const SkinCard = {
  name: "SkinCard",
  components: { TagCapsule },
  props: {
    item: { type: Object, required: true },
    active: Boolean,
    writable: Boolean, // 收藏与编辑都要写库，guest 是只读身份组，这两个入口都不显示（DESIGN.md 5.1）
  },
  emits: ["select", "favorite", "edit"],
  setup(props) {
    const canvas = ref(null);
    const failed = ref(false);
    const label = computed(() => modelShort(props.item));
    const short = computed(() => shortId(props.item));

    /**
     * 卡片上最多放四个 chip（两行），超出的进悬浮面板。
     * 四个以内全显示；四个以上只显示前三个 + 「+N」，这样无论多少标签都是两行 —— 高度恒定，
     * 列表的 content-visibility 估算才稳，将来真要换成窗口化列表也是现成的。
     */
    const shownTags = computed(() => {
      const tags = props.item.tags ?? [];
      return tags.length > 4 ? tags.slice(0, 3) : tags;
    });
    const hiddenTags = computed(() => (props.item.tags?.length ?? 0) - shownTags.value.length);
    let stopDraw = () => {};

    async function draw() {
      failed.value = false;
      try {
        const image = await thumbnailFor(props.item);
        // 模型已并进 kind（skin.steve / skin.alex），2D 预览按 kind 画
        renderPreview(canvas.value, image, props.item.kind);
      } catch {
        failed.value = true;
      }
    }

    onMounted(() => {
      stopDraw = drawWhenNear(canvas.value, draw);
    });
    onBeforeUnmount(() => stopDraw());
    // 模型换了要重画：这时卡片已经在视口里，不必再等观察器
    watch(() => props.item.kind, draw);

    return { store, canvas, failed, label, short, shownTags, hiddenTags };
  },
  template: `
    <article class="card"
             :class="{ cape: item.category === 'cape', on: active, builtin: item.readonly }"
             role="listitem" :data-id="item.id" @click="$emit('select', item)"
             @mouseenter="store.hoverCard(item, $event)" @mouseleave="store.unhoverCard(item)">
      <button v-if="writable" class="fav-btn" :class="{ on: item.favorite }" type="button"
              :title="item.favorite ? t('取消收藏') : t('收藏')" @click.stop="$emit('favorite', item)">
        <svg class="icon"><use href="#i-star"/></svg>
      </button>
      <button v-if="!item.readonly && writable" class="edit-btn" type="button" :title="t('编辑')"
              @click.stop="$emit('edit', item)">
        <svg class="icon"><use href="#i-pencil"/></svg>
      </button>
      <div class="thumb">
        <canvas v-show="!failed" ref="canvas"></canvas>
        <span v-if="failed" class="meta">{{ t("无法加载") }}</span>
      </div>
      <span v-if="item.readonly" class="builtin-flag">{{ t("内置") }}</span>
      <div class="tags">
        <TagCapsule v-for="tag in shownTags" :key="tag" :tag="tag"/>
        <span v-if="hiddenTags" class="tag-more">+{{ hiddenTags }}</span>
        <span v-if="!item.tags || !item.tags.length" class="tag-empty">{{ short }}</span>
      </div>
      <span class="meta">{{ label }} · {{ item.width }}×{{ item.height }}</span>
    </article>`,
};
