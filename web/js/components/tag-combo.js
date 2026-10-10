/**
 * 标签输入：已选 capsule + 自由输入 + 「前缀 → 中缀 → 后缀」排序的下拉候选。
 * 组件本身不持有标签，只把 add / remove 抛给上层。
 */
import { t } from "../i18n.js";
import { MATCH_LABELS, hasTag, matchTags } from "../tags.js";
import { computed, nextTick, ref, watch } from "../vue.js";

export const TagCombo = {
  name: "TagCombo",
  props: {
    selected: { type: Array, default: () => [] },
    library: { type: Array, default: () => [] },
  },
  emits: ["add", "remove"],
  setup(props, { emit }) {
    const input = ref(null);
    const text = ref("");
    const open = ref(false);
    const index = ref(-1);
    const needle = computed(() => text.value.trim());
    const suggestions = computed(() => matchTags(text.value, props.library, props.selected));

    // 候选变化后把高亮项拉回有效范围
    watch(suggestions, () => {
      index.value = suggestions.value.length ? 0 : -1;
    });

    function show() {
      open.value = true;
      index.value = suggestions.value.length ? 0 : -1;
    }

    function close() {
      open.value = false;
      index.value = -1;
    }

    function add(tag) {
      const clean = tag.trim();
      if (!clean) return close();
      if (!hasTag(props.selected, clean)) emit("add", clean);
      text.value = "";
      close();
      nextTick(() => input.value?.focus());
    }

    function apply(entry) {
      if (entry.action === "create") add(entry.tag);
      else if (hasTag(props.selected, entry.tag)) emit("remove", entry.tag);
      else add(entry.tag);
    }

    function move(step) {
      if (!suggestions.value.length) return;
      const size = suggestions.value.length;
      index.value = (index.value + step + size) % size;
    }

    function onInput() {
      show();
    }

    function onKeydown(event) {
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
        emit("remove", props.selected.at(-1));
      }
    }

    // mousedown 而不是 click：避免 input 先失焦
    function onBlur() {
      setTimeout(close, 120);
    }

    function focusAndShow() {
      input.value?.focus();
      show();
    }

    return {
      input, text, open, index, needle, suggestions,
      MATCH_LABELS,
      add, apply, move, onInput, onKeydown, onBlur, focusAndShow,
    };
  },
  template: `
    <div class="tag-combo" @click="focusAndShow">
      <span class="tag-selected">
        <span v-for="tag in selected" :key="tag" class="tag-chip">
          <span>{{ tag }}</span>
          <button type="button" class="remove" :title="t('移除')" @click.stop="$emit('remove', tag)">
            <svg class="icon"><use href="#i-x"/></svg>
          </button>
        </span>
      </span>
      <span class="tag-input-wrap">
        <input ref="input" v-model="text" type="text" autocomplete="off" spellcheck="false"
               role="combobox" :aria-expanded="String(open)" aria-controls="tagSuggest"
               aria-autocomplete="list"
               :placeholder="selected.length ? t('继续添加…') : t('输入标签后回车，或从下拉选择')"
               @input="onInput" @focus="show" @blur="onBlur" @keydown="onKeydown">
        <ul class="tag-suggest" id="tagSuggest" role="listbox" v-show="open">
          <li v-if="!suggestions.length" class="sug-empty">{{ t("没有匹配，回车新建") }}</li>
          <li v-for="(entry, i) in suggestions" :key="entry.action + entry.tag" role="option"
              class="sug-item" :class="{ active: i === index, create: entry.action === 'create' }"
              :title="entry.action === 'create'
                ? t('新建标签 {tag}', { tag: entry.tag })
                : t('{match}匹配 · 用过 {count} 次', { match: MATCH_LABELS[entry.rank], count: entry.count })"
              @mousedown.prevent="apply(entry)">
            <template v-if="entry.action === 'create'">
              <svg class="icon"><use href="#i-plus"/></svg>
              <span>{{ t("新建「{tag}」", { tag: entry.tag }) }}</span>
            </template>
            <template v-else>
              <span v-if="entry.from >= 0">
                {{ entry.tag.slice(0, entry.from) }}<mark>{{ entry.tag.slice(entry.from, entry.from + needle.length) }}</mark>{{ entry.tag.slice(entry.from + needle.length) }}
              </span>
              <span v-else>{{ entry.tag }}</span>
              <span v-if="entry.from >= 0" class="sug-kind">{{ MATCH_LABELS[entry.rank] }}</span>
              <span class="sug-count">{{ t("用过 {count} 次", { count: entry.count }) }}</span>
            </template>
          </li>
        </ul>
      </span>
    </div>`,
};
