/**
 * 主页面：根组件只负责骨架与装配，逻辑全在 store，视图全在组件。
 */
import { requireAuth } from "./auth.js";
import { TopBar } from "./components/chrome.js";
import { SkinCard } from "./components/skin-card.js";
import { StagePane } from "./components/stage-pane.js";
import { ToastBox } from "./components/toast-box.js";
import { UploadPanel } from "./components/upload-panel.js";
import { installI18n, t } from "./i18n.js";
import { store, viewerRef } from "./store.js";
import { createApp, onMounted } from "./vue.js";

const App = {
  name: "App",
  components: { TopBar, ToastBox, SkinCard, StagePane, UploadPanel },
  setup() {
    const isActive = (item) =>
      item.kind === "skin"
        ? store.state.skin?.id === item.id
        : store.state.cape?.id === item.id;

    onMounted(async () => {
      try {
        await store.loadTags();
        await store.loadMe();
        await store.refresh();
      } catch (error) {
        store.toastError(error.message);
      }
    });

    return { store, isActive };
  },
  template: `
    <div class="app">
      <TopBar/>

      <main class="workspace">
        <!-- ── 左：贴图库 ─────────────────────────────────────── -->
        <section class="panel" :aria-label="t('贴图库')">
          <div class="panel-head">
            <div class="tabs" role="tablist">
              <button class="tab" role="tab" :class="{ on: store.state.tab === 'favorites' }"
                      @click="store.setTab('favorites')">
                <svg class="icon"><use href="#i-star"/></svg>{{ t("我的收藏") }}
              </button>
              <button class="tab" role="tab" :class="{ on: store.state.tab === 'all' }"
                      @click="store.setTab('all')">
                <svg class="icon"><use href="#i-grid"/></svg>{{ t("全部") }}
              </button>
            </div>
            <button v-if="store.canUpload" class="tab tab-upload" role="tab"
                    :class="{ on: store.uploading }" @click="store.setTab('upload')">
              <svg class="icon"><use href="#i-upload"/></svg>{{ t("上传新皮肤") }}
            </button>
          </div>

          <div class="library-body">
            <nav class="rail" :aria-label="t('类别')">
              <button class="rail-btn" :class="{ on: store.state.kind === 'skin' }"
                      @click="store.setKind('skin')">
                <svg class="icon"><use href="#i-skin"/></svg>{{ t("皮肤") }}
              </button>
              <button class="rail-btn" :class="{ on: store.state.kind === 'cape' }"
                      @click="store.setKind('cape')">
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
              </div>

              <div class="grid" id="grid" role="list"
                   v-if="!store.uploading && store.state.items.length">
                <SkinCard v-for="item in store.state.items" :key="item.id" :item="item"
                          :active="isActive(item)" :deletable="store.canDelete"
                          @select="store.select(item)"
                          @favorite="store.toggleFavorite(item)" @remove="store.remove(item)"/>
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
