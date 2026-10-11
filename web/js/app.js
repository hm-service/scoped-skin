/**
 * 主页面：根组件只负责骨架与装配，逻辑全在 store，视图全在组件。
 */
import { requireAuth } from "./auth.js";
import { CardHover } from "./components/card-hover.js";
import { TopBar } from "./components/chrome.js";
import { SkinCard } from "./components/skin-card.js";
import { StagePane } from "./components/stage-pane.js";
import { TagPanel } from "./components/tag-panel.js";
import { ToastBox } from "./components/toast-box.js";
import { UploadPanel } from "./components/upload-panel.js";
import { installI18n, t } from "./i18n.js";
import { store, viewerRef } from "./store.js";
import { computed, createApp, nextTick, onBeforeUnmount, onMounted, ref, watch } from "./vue.js";

/** 渲染窗口前后各多留几行：滚动时不会看到空白，也不至于把几百张卡片都塞进 DOM。 */
const GRID_OVERSCAN = 3;

const App = {
  name: "App",
  components: { TopBar, ToastBox, SkinCard, StagePane, UploadPanel, TagPanel, CardHover },
  setup() {
    const isActive = (item) =>
      item.category === "skin"
        ? store.state.skin?.id === item.id
        : store.state.cape?.id === item.id;

    // 拖到按钮的子元素（图标、文字）上也会触发 dragleave，认 relatedTarget 才不闪
    function leaveUploadTab(event) {
      if (event.currentTarget.contains(event.relatedTarget)) return;
      store.state.uploadOver = false;
    }

    /* ── 列表虚拟化 ────────────────────────────────────────────────
       卡片高度恒定（CSS 的 --grid-row），所以「第几行」与「滚到哪」之间只是一次乘除法：
       spacer 撑出全量高度，window 是绝对定位的渲染窗口，里面只放视口附近那几行。
       行高与列数都从 DOM 现成的答案里读（--grid-row / 浏览器算好的 grid-template-columns），
       不在 JS 里复刻一遍布局规则。 */
    const gridRef = ref(null);
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

    const gridItems = computed(() =>
      store.state.items.slice(gridWindow.value.start, gridWindow.value.end),
    );

    function measureGrid() {
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

    let gridObserver = null;
    watch(gridRef, (element) => {
      gridObserver?.disconnect();
      gridObserver = null;
      if (!element) return;
      gridObserver = new ResizeObserver(measureGrid);
      gridObserver.observe(element);
      measureGrid();
    });
    // 列表换了（切换 / 搜索 / 拉取）窗口要重新量：window 是跟着 v-if 出来的，晚一拍才在
    watch(() => store.state.items, () => nextTick(measureGrid));
    onBeforeUnmount(() => gridObserver?.disconnect());

    function onGridScroll(event) {
      gridScroll.value = event.target.scrollTop;
      store.hideHover(); // 卡片可能被滚出窗口，悬浮面板不能留在原地
    }

    // 换筛选条件后停在原来的滚动位置，会让人以为「没结果」
    watch(
      [() => store.state.tab, () => store.state.category, () => store.state.query],
      () => {
        if (gridRef.value) gridRef.value.scrollTop = 0;
        gridScroll.value = 0;
      },
    );

    /** 把某张卡片滚进视野（上传成功后用）。block: nearest 的等价实现。 */
    function revealCard(id) {
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
        if (nonce) nextTick(() => revealCard(store.state.reveal.id));
      },
    );

    onMounted(async () => {
      // 页面别处的漏网拖拽：吞掉事件，免得浏览器直接打开/接管这张图
      document.addEventListener("dragover", (event) => event.preventDefault());
      document.addEventListener("drop", (event) => event.preventDefault());
      try {
        await store.loadTags();
        await store.loadMe();
        await store.refresh();
      } catch (error) {
        store.toastError(error.message);
      }
    });

    return { store, isActive, leaveUploadTab, gridRef, gridWindow, gridItems, onGridScroll };
  },
  template: `
    <div class="app">
      <TopBar/>

      <main class="workspace">
        <!-- ── 左：贴图库 ─────────────────────────────────────── -->
        <section class="panel" :aria-label="t('贴图库')">
          <div class="panel-head">
            <div class="tabs" role="tablist">
              <!-- guest 是只读身份组：收藏不了任何东西，这个 tab 只会是空的，直接不给它看 -->
              <button v-if="store.canWrite" class="tab" role="tab" :class="{ on: store.state.tab === 'favorites' }"
                      @click="store.setTab('favorites')">
                <svg class="icon"><use href="#i-star"/></svg>{{ t("我的收藏") }}
              </button>
              <button class="tab" role="tab" :class="{ on: store.state.tab === 'all' }"
                      @click="store.setTab('all')">
                <svg class="icon"><use href="#i-grid"/></svg>{{ t("全部") }}
              </button>
            </div>
            <button v-if="store.canUpload" class="tab tab-upload" role="tab"
                    :class="{ on: store.uploading, over: store.state.uploadOver }"
                    @click="store.setTab('upload')"
                    @dragenter.prevent="store.state.uploadOver = true"
                    @dragover.prevent="store.state.uploadOver = true"
                    @dragleave="leaveUploadTab"
                    @drop.prevent="store.dropOnUploadTab($event.dataTransfer?.files?.[0])">
              <svg class="icon"><use href="#i-upload"/></svg>{{ t("上传新皮肤") }}
            </button>
          </div>

          <div class="library-body">
            <nav class="rail" :aria-label="t('类别')">
              <button class="rail-btn" :class="{ on: store.state.category === 'skin' }"
                      @click="store.setCategory('skin')">
                <svg class="icon"><use href="#i-skin"/></svg>{{ t("皮肤") }}
              </button>
              <button class="rail-btn" :class="{ on: store.state.category === 'cape' }"
                      @click="store.setCategory('cape')">
                <svg class="icon"><use href="#i-cape"/></svg>{{ t("披风") }}
              </button>
            </nav>

            <div class="pane">
              <div class="pane-head">
                <h2>{{ store.paneTitle }}</h2>
                <span class="pane-count">{{ store.paneCount }}</span>
                <label class="search">
                  <svg class="icon"><use href="#i-search"/></svg>
                  <input type="search" autocomplete="off" spellcheck="false"
                         :placeholder="t('搜索标签 · 空格分隔取交集')" :value="store.state.query"
                         @input="store.setQuery($event.target.value)"
                         @keydown.enter.prevent="store.submitSearch()"
                         @keydown.esc="store.clearQuery()">
                  <button v-if="store.state.query" type="button" class="search-clear" :title="t('清除')"
                          @click="store.clearQuery()">
                    <svg class="icon"><use href="#i-x"/></svg>
                  </button>
                </label>
                <TagPanel/>
              </div>

              <div class="grid" id="grid" ref="gridRef"
                   v-if="!store.uploading && store.state.items.length"
                   @scroll.passive="onGridScroll">
                <div class="grid-spacer" role="presentation" :style="{ height: gridWindow.height + 'px' }">
                  <div class="grid-window" role="list" :aria-label="t('贴图库')"
                       :style="{ transform: 'translateY(' + gridWindow.offset + 'px)' }">
                    <SkinCard v-for="item in gridItems" :key="item.id" :item="item"
                              :active="isActive(item)" :writable="store.canWrite"
                              @select="store.select(item)"
                              @favorite="store.toggleFavorite(item)" @edit="store.editItem(item)"/>
                  </div>
                </div>
              </div>

              <p class="placeholder" v-if="!store.uploading && !store.state.items.length">
                <span v-if="store.emptyKind === 'search'">{{ t("没有同时带上", { noun: store.emptyNoun }) }}<b
                  v-for="token in store.emptyTokens" :key="token">{{ t("「{tag}」", { tag: token }) }}</b>{{ t("的{noun}。", { noun: store.emptyNoun }) }}</span>
                <span v-else-if="store.emptyKind === 'favorites'">{{ t("还没有收藏{noun}。", { noun: store.emptyNoun }) }}<br>{{ t("点卡片右上角的星标收藏。") }}</span>
                <span v-else>{{ t("{noun}库是空的", { noun: store.emptyNoun }) }}{{ store.canUpload ? t("，去「上传新皮肤」传一张。") : t("。") }}</span>
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
    </div>`,
};

if (requireAuth()) {
  const app = createApp(App);
  installI18n(app, t("皮肤站 · Minecraft Skin Library"));
  app.mount("#app");
  // 调试用句柄：浏览器控制台里可以直接检查 / 驱动界面状态
  window.__skin = {
    store,
    select: store.select,
    get viewer() {
      return viewerRef.current;
    },
  };
}
