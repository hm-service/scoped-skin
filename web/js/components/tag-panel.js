/**
 * 标签面板：搜索框旁边的按钮，点开是库里所有标签（按热度排序，同数按名称）。
 * 点标签＝把它加进搜索串，再点一下移出去；面板本身不改标签，只是个搜索快捷键。
 */
import { t } from "../i18n.js";
import { store } from "../store.js";
import { byPopularity } from "../tags.js";
import { computed, onBeforeUnmount, onMounted, ref } from "../vue.js";

export const TagPanel = {
  name: "TagPanel",
  setup() {
    const root = ref(null);
    const open = ref(false);
    const tags = computed(() => [...store.state.library].sort(byPopularity));

    // 点面板外面 / 按 Esc 收起；面板内部的点击（包括按钮自己）不算「外面」
    function onPointerDown(event) {
      if (root.value && !root.value.contains(event.target)) open.value = false;
    }
    function onKeydown(event) {
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

    return { store, root, open, tags };
  },
  template: `
    <div ref="root" class="tag-panel">
      <button type="button" class="tag-panel-btn" :class="{ on: open }"
              :title="t('library.tags-all')" :aria-label="t('library.tags-all')" :aria-expanded="String(open)"
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
    </div>`,
};
