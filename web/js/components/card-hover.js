/**
 * 卡片悬浮面板：卡片上只放得下四个 chip，其余标签在这里补全。
 *
 * 固定定位、挂在列表外面 —— 列表是滚动容器，面板放进去会被裁掉。
 * 尺寸要渲染出来才知道，所以在 post flush 里量一次再定位，不会闪。
 */
import { store } from "../store.js";
import { shortId } from "../tags.js";
import { computed, ref, watch } from "../vue.js";
import { TagCapsule } from "./tag-capsule.js";

export const CardHover = {
  name: "CardHover",
  components: { TagCapsule },
  setup() {
    const box = ref(null);
    const item = computed(() => store.state.hover.item);

    function place() {
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

    return { store, box, item, shortId };
  },
  template: `
    <div v-if="item" ref="box" class="card-hover">
      <div class="tags">
        <TagCapsule v-for="tag in item.tags" :key="tag" :tag="tag"/>
        <span v-if="!item.tags || !item.tags.length" class="tag-empty">{{ shortId(item) }}</span>
      </div>
      <p class="card-hover-meta">
        <span>{{ store.modelLabel(item) }} · {{ item.width }}×{{ item.height }}</span>
        <span class="card-hover-id">{{ shortId(item) }}</span>
      </p>
    </div>`,
};
