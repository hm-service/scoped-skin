/** 列表卡片：贴图缩略图（实时裁 2D 正面视图）+ 标签 + 收藏 / 删除。 */
import { detectModel, renderPreview } from "../avatar.js";
import { t } from "../i18n.js";
import { modelLabel, thumbnailFor } from "../store.js";
import { shortId } from "../tags.js";
import { computed, onMounted, ref, watch } from "../vue.js";
import { TagCapsule } from "./tag-capsule.js";

export const SkinCard = {
  name: "SkinCard",
  components: { TagCapsule },
  props: {
    item: { type: Object, required: true },
    active: Boolean,
    deletable: Boolean, // 删除资源只给 admin（DESIGN.md 5.1），其余身份组不显示删除按钮
  },
  emits: ["select", "favorite", "remove"],
  setup(props) {
    const canvas = ref(null);
    const failed = ref(false);
    const label = computed(() => modelLabel(props.item));
    const short = computed(() => shortId(props.item));

    async function draw() {
      failed.value = false;
      try {
        const image = await thumbnailFor(props.item);
        // share（通用）由渲染端判断模型：2D 预览这里就地探测一次
        const model =
          props.item.kind === "skin" && props.item.model === "share"
            ? detectModel(image)
            : props.item.model;
        renderPreview(canvas.value, image, props.item.kind, model);
      } catch {
        failed.value = true;
      }
    }

    onMounted(draw);
    watch(() => [props.item.kind, props.item.model], draw);

    return { canvas, failed, label, short };
  },
  template: `
    <article class="card"
             :class="{ cape: item.kind === 'cape', on: active, builtin: item.is_builtin }"
             role="listitem" :data-id="item.id" @click="$emit('select', item)">
      <button class="fav-btn" :class="{ on: item.favorite }" type="button"
              :title="item.favorite ? t('取消收藏') : t('收藏')" @click.stop="$emit('favorite', item)">
        <svg class="icon"><use href="#i-star"/></svg>
      </button>
      <button v-if="!item.is_builtin && deletable" class="del-btn" type="button" :title="t('删除')"
              @click.stop="$emit('remove', item)">
        <svg class="icon"><use href="#i-trash"/></svg>
      </button>
      <div class="thumb">
        <canvas v-show="!failed" ref="canvas"></canvas>
        <span v-if="failed" class="meta">{{ t("无法加载") }}</span>
      </div>
      <span v-if="item.is_builtin" class="builtin-flag">{{ t("内置") }}</span>
      <div class="tags">
        <TagCapsule v-for="tag in item.tags" :key="tag" :tag="tag"/>
        <span v-if="!item.tags || !item.tags.length" class="tag-empty">{{ short }}</span>
      </div>
      <span class="meta">{{ label }} · {{ item.width }}×{{ item.height }}</span>
    </article>`,
};
