<script setup lang="ts">
/**
 * 主页面：根组件只负责骨架与装配，逻辑全在 store，视图全在组件。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";

import CardHover from "@/components/CardHover.vue";
import SkinCard from "@/components/SkinCard.vue";
import StagePane from "@/components/StagePane.vue";
import TagPanel from "@/components/TagPanel.vue";
import ToastBox from "@/components/ToastBox.vue";
import TopBar from "@/components/TopBar.vue";
import UploadPanel from "@/components/UploadPanel.vue";
import { t } from "@/i18n";
import { store } from "@/store";
import type { Item } from "@/types";

/** 渲染窗口前后各多留几行：滚动时不会看到空白，也不至于把几百张卡片都塞进 DOM。 */
const GRID_OVERSCAN = 3;

const isActive = (item: Item): boolean =>
  item.category === "skin" ? store.state.skin?.id === item.id : store.state.cape?.id === item.id;

// 拖到按钮的子元素（图标、文字）上也会触发 dragleave，认 relatedTarget 才不闪
function leaveUploadTab(event: DragEvent): void {
  const current = event.currentTarget as HTMLElement;
  if (event.relatedTarget instanceof Node && current.contains(event.relatedTarget)) return;
  store.state.uploadOver = false;
}

/* ── 列表虚拟化 ────────────────────────────────────────────────
   卡片高度恒定（CSS 的 --grid-row），所以「第几行」与「滚到哪」之间只是一次乘除法：
   spacer 撑出全量高度，window 是绝对定位的渲染窗口，里面只放视口附近那几行。
   行高与列数都从 DOM 现成的答案里读（--grid-row / 浏览器算好的 grid-template-columns），
   不在 JS 里复刻一遍布局规则。 */
const gridRef = ref<HTMLElement | null>(null);
const gridScroll = ref(0); // 滚动位置
const gridViewport = ref(0); // 视口高度
const gridColumns = ref(1); // 列数
const gridRowHeight = ref(212); // 一行占的高度（卡片 + 行距）
const gridGap = ref(9); // 行距：spacer 的最后一行的间距不该算进去

const gridWindow = computed(() => {
  const columns = gridColumns.value;
  const total = store.state.items.length;
  const rows = Math.ceil(total / columns);
  const height = gridRowHeight.value;
  const first = Math.floor(gridScroll.value / height);
  const visible = Math.ceil(gridViewport.value / height) + 1;
  const startRow = Math.max(0, first - GRID_OVERSCAN);
  const endRow = Math.min(rows, first + visible + GRID_OVERSCAN);
  return {
  start: startRow * columns,
  end: Math.min(total, endRow * columns),
  offset: startRow * height,
  height: rows ? rows * height - gridGap.value : 0,
  };
});

const gridItems = computed(() => store.state.items.slice(gridWindow.value.start, gridWindow.value.end));

function measureGrid(): void {
  const element = gridRef.value;
  if (!element) return;
  const css = getComputedStyle(element);
  gridRowHeight.value =
  (parseFloat(css.getPropertyValue("--grid-row")) || 212) +
  (parseFloat(css.getPropertyValue("--grid-gap")) || 9);
  gridGap.value = parseFloat(css.getPropertyValue("--grid-gap")) || 9;
  gridViewport.value = element.clientHeight;
  const layer = element.querySelector(".grid-window");
  if (!layer) return;
  // 浏览器按 auto-fill 算出来的轨迹列表，条目数就是列数
  const tracks = getComputedStyle(layer).gridTemplateColumns;
  const columns = tracks && tracks !== "none" ? tracks.split(/\s+/).length : 0;
  if (columns) gridColumns.value = columns;
}

let gridObserver: ResizeObserver | null = null;
watch(gridRef, (element) => {
  gridObserver?.disconnect();
  gridObserver = null;
  if (!element) return;
  gridObserver = new ResizeObserver(measureGrid);
  gridObserver.observe(element);
  measureGrid();
});
// 列表换了（切换 / 搜索 / 拉取）窗口要重新量：window 是跟着 v-if 出来的，晚一拍才在
watch(
  () => store.state.items,
  () => nextTick(measureGrid),
);
onBeforeUnmount(() => gridObserver?.disconnect());

function onGridScroll(event: Event): void {
  gridScroll.value = (event.target as HTMLElement).scrollTop;
  store.hideHover(); // 卡片可能被滚出窗口，悬浮面板不能留在原地
}

// 换筛选条件后停在原来的滚动位置，会让人以为「没结果」
watch([() => store.state.tab, () => store.state.category, () => store.state.query], () => {
  if (gridRef.value) gridRef.value.scrollTop = 0;
  gridScroll.value = 0;
});

/** 把某张卡片滚进视野（上传成功后用）。block: nearest 的等价实现。 */
function revealCard(id: string): void {
  const element = gridRef.value;
  const index = store.state.items.findIndex((item) => item.id === id);
  if (!element || index < 0) return;
  const top = Math.floor(index / gridColumns.value) * gridRowHeight.value;
  const bottom = top + gridRowHeight.value;
  if (top < element.scrollTop) element.scrollTo({ top, behavior: "smooth" });
  else if (bottom > element.scrollTop + element.clientHeight) {
  element.scrollTo({ top: bottom - element.clientHeight, behavior: "smooth" });
  }
}

watch(
  () => store.state.reveal?.nonce,
  (nonce) => {
  if (nonce) nextTick(() => revealCard(store.state.reveal?.id ?? ""));
  },
);

// 页面别处的漏网拖拽：吞掉事件，免得浏览器直接打开/接管这张图。
// 具名注册、卸载时摘掉 —— 根组件虽然不常卸载，但监听挂在 document 上，留着会跨实例叠加。
const swallowDrag = (event: Event) => event.preventDefault();
onMounted(async () => {
  document.addEventListener("dragover", swallowDrag);
  document.addEventListener("drop", swallowDrag);
  try {
  await store.loadTags();
  await store.loadMe();
  await store.refresh();
  } catch (error) {
  store.toastError(error instanceof Error ? error.message : String(error));
  }
});
onBeforeUnmount(() => {
  document.removeEventListener("dragover", swallowDrag);
  document.removeEventListener("drop", swallowDrag);
});
</script>

<template>
  <div class="app">
    <TopBar/>

    <main class="workspace">
      <!-- ── 左：贴图库 ─────────────────────────────────────── -->
      <section class="panel" :aria-label="t('library.textures')">
        <div class="panel-head">
          <div class="tabs" role="tablist">
            <!-- guest 是只读身份组：收藏不了任何东西，这个 tab 只会是空的，直接不给它看 -->
            <button v-if="store.canWrite" class="tab" role="tab" :class="{ on: store.state.tab === 'favorites' }"
                    @click="store.setTab('favorites')">
              <svg class="icon"><use href="#i-star"/></svg>{{ t("library.favorites") }}
            </button>
            <button class="tab" role="tab" :class="{ on: store.state.tab === 'all' }"
                    @click="store.setTab('all')">
              <svg class="icon"><use href="#i-grid"/></svg>{{ t("library.all") }}
            </button>
          </div>
          <button v-if="store.canUpload" class="tab tab-upload" role="tab"
                  :class="{ on: store.uploading, over: store.state.uploadOver }"
                  @click="store.setTab('upload')"
                  @dragenter.prevent="store.state.uploadOver = true"
                  @dragover.prevent="store.state.uploadOver = true"
                  @dragleave="leaveUploadTab"
                  @drop.prevent="store.dropOnUploadTab($event.dataTransfer?.files?.[0])">
            <svg class="icon"><use href="#i-upload"/></svg>{{ t("library.upload-skin") }}
          </button>
        </div>

        <div class="library-body">
          <nav class="rail" :aria-label="t('library.category')">
            <button class="rail-btn" :class="{ on: store.state.category === 'skin' }"
                    @click="store.setCategory('skin')">
              <svg class="icon"><use href="#i-skin"/></svg>{{ t("noun.skin") }}
            </button>
            <button class="rail-btn" :class="{ on: store.state.category === 'cape' }"
                    @click="store.setCategory('cape')">
              <svg class="icon"><use href="#i-cape"/></svg>{{ t("noun.cape") }}
            </button>
          </nav>

          <div class="pane">
            <div class="pane-head">
              <h2>{{ store.paneTitle }}</h2>
              <span class="pane-count">{{ store.paneCount }}</span>
              <label class="search">
                <svg class="icon"><use href="#i-search"/></svg>
                <input type="search" autocomplete="off" spellcheck="false"
                       :placeholder="t('library.search-placeholder')" :value="store.state.query"
                       @input="store.setQuery(($event.target as HTMLInputElement).value)"
                       @keydown.enter.prevent="store.submitSearch()"
                       @keydown.esc="store.clearQuery()">
                <button v-if="store.state.query" type="button" class="search-clear" :title="t('library.clear')"
                        @click="store.clearQuery()">
                  <svg class="icon"><use href="#i-x"/></svg>
                </button>
              </label>
              <TagPanel/>
            </div>

            <div v-if="!store.uploading && store.state.items.length" class="grid" id="grid" ref="gridRef"
                 @scroll.passive="onGridScroll">
              <div class="grid-spacer" role="presentation" :style="{ height: gridWindow.height + 'px' }">
                <div class="grid-window" role="list" :aria-label="t('library.textures')"
                     :style="{ transform: 'translateY(' + gridWindow.offset + 'px)' }">
                  <SkinCard v-for="item in gridItems" :key="item.id" :item="item"
                            :active="isActive(item)" :writable="store.canWrite"
                            @select="store.select(item)"
                            @favorite="store.toggleFavorite(item)" @edit="store.editItem(item)"/>
                </div>
              </div>
            </div>

            <p v-if="!store.uploading && !store.state.items.length" class="placeholder">
              <span v-if="store.emptyKind === 'search'">{{ t("empty.no-match", { noun: store.emptyNoun }) }}<b
                v-for="token in store.emptyTokens" :key="token">{{ t("empty.tag", { tag: token }) }}</b>{{ t("empty.no-match-tail", { noun: store.emptyNoun }) }}</span>
              <span v-else-if="store.emptyKind === 'favorites'">{{ t("empty.no-favorites", { noun: store.emptyNoun }) }}<br>{{ t("empty.favorite-hint") }}</span>
              <span v-else>{{ t("empty.library", { noun: store.emptyNoun }) }}{{ store.canUpload ? t("empty.upload-hint") : t("common.period") }}</span>
            </p>

            <UploadPanel v-if="store.uploading"/>
          </div>
        </div>
      </section>

      <!-- ── 右：3D 预览 ────────────────────────────────────── -->
      <StagePane/>
    </main>

    <ToastBox :toast="store.state.toast"/>
    <CardHover/>
  </div>
</template>

<style scoped>
.workspace {
  flex: 1 1 auto;
  min-height: 0;
  display: grid;
  grid-template-columns: minmax(0, 1.22fr) minmax(340px, 1fr);
  gap: 12px;
  padding: 12px;
}

/* ── 左侧：tab + 竖排类别 + 内容 ───────────────────────────────── */

.panel-head {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--line);
}

.tabs { display: flex; gap: 6px; }

.tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 7px 14px;
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--ink-soft);
  font-size: 13.5px;
  transition: background .15s, color .15s, border-color .15s;
}

.tab .icon { width: 15px; height: 15px; }

.tab:hover { background: var(--paper-soft); color: var(--ink); }

.tab.on {
  background: var(--accent-soft);
  border-color: color-mix(in srgb, var(--accent) 35%, transparent);
  color: var(--accent-ink);
  font-weight: 600;
}

.tab-upload { margin-left: auto; border: 1px dashed var(--line-strong); }

.tab-upload:hover { border-color: var(--accent); color: var(--accent-ink); }

.tab-upload.on { border-style: solid; }

/* 文件正拖在按钮上：给个明确的落点反馈 */
.tab-upload.over {
  border-style: solid;
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent-ink);
}

.library-body {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  gap: 12px;
  padding: 12px;
}

.rail {
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.rail-btn {
  width: 56px;
  height: 54px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 3px;
  border: 1px solid var(--line);
  border-radius: var(--radius-md);
  background: var(--paper-soft);
  color: var(--ink-soft);
  font-size: 12px;
  transition: all .15s;
}

.rail-btn .icon { width: 17px; height: 17px; }

.rail-btn:hover { border-color: var(--line-strong); color: var(--ink); }

.rail-btn.on {
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent-ink);
  font-weight: 600;
}

/* 内容区不再套第二层卡片：直接铺在面板里 */
.pane {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.pane-head {
  flex: 0 0 auto;
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 2px 2px 10px;
}

.pane-head h2 { margin: 0; font-size: 13.5px; font-weight: 650; letter-spacing: .02em; }

.pane-count { font-size: 12px; color: var(--ink-faint); font-variant-numeric: tabular-nums; }

/* 标签搜索：空格分隔，取交集；宽度把右边那个标签面板按钮一起算进去 */
.search {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 244px;
  padding: 5px 8px;
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-md);
  background: var(--paper);
  color: var(--ink-faint);
  transition: border-color .15s, box-shadow .15s;
}

.search:focus-within { border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-soft); }

.search .icon { width: 14px; height: 14px; }

.search input {
  flex: 1 1 auto;
  min-width: 0;
  border: none;
  background: transparent;
  font-size: 12.5px;
  color: var(--ink);
  outline: none;
  -webkit-appearance: none;
  appearance: none;
}

.search input::-webkit-search-cancel-button { display: none; }

.search-clear {
  display: grid;
  place-items: center;
  width: 18px;
  height: 18px;
  padding: 0;
  border: none;
  border-radius: 50%;
  background: var(--paper-sunk);
  color: var(--ink-soft);
}

.search-clear:hover { background: var(--accent-soft); color: var(--accent-ink); }

.search-clear .icon { width: 11px; height: 11px; stroke-width: 2.2; }

.grid {
  /* 标准卡片宽 123px（就是截图里那个视觉尺寸）作为轨迹下限：网格宽度富余时列先「适度增宽」，
   宽到能再放下一张卡就加一列 —— 列数不设上限。用下限而不是固定值，
   是为了卡片永远不会掉到标准宽以下（固定 minmax(118px,1fr) 时，网格越宽列越多、
   每列反而越贴下限）。 */
  --grid-card: 123px;
  --grid-gap: 9px;
  /* 行高（= 卡片高度）恒定，虚拟化就拿它做「第几行 ↔ 滚到哪」的换算，不用测量、不用前缀和 */
  --grid-row: 212px;

  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 2px;
  scrollbar-width: thin;
}

/* 虚拟滚动：spacer 撑出全量行数的高度（决定滚动条长度），window 是绝对定位的渲染窗口，
   用 translateY 挪到该出现的位置。窗口本身才是那个 grid（auto-fill 的列数由它的宽度算），
   所以窗口的宽度必须跟着 spacer 走 —— 用 left/right: 0 而不是 width: 100%。 */
.grid-spacer { position: relative; }

.grid-window {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(var(--grid-card), 1fr));
  grid-auto-rows: var(--grid-row);
  gap: var(--grid-gap);
  align-content: start;
}

.placeholder {
  flex: 1 1 auto;
  display: grid;
  place-items: center;
  margin: 0;
  padding: 40px 24px;
  color: var(--ink-faint);
  font-size: 13px;
  text-align: center;
  line-height: 1.9;
}

@media (max-width: 980px) {
  .workspace { grid-template-columns: 1fr; }
}

.placeholder b { color: var(--ink-soft); font-weight: 600; }

/* 英文下 rail 按钮显示 Skin / Cape（字典里是小写，句子里要用小写名词） */
.rail-btn { text-transform: capitalize; }
</style>
