/**
 * /maintain 根组件：管理员维护页，一屏内完成用户管理。
 *
 * 页面本身不鉴权（谁都能打开），真正的闸门是后端的 /api/admin/*：非 admin 会
 * 拿到 403，这里就只剩一张「需要管理员身份」的提示卡。
 */
import { requireAuth } from "./auth.js";
import { LocaleSwitch } from "./components/locale-switch.js";
import { ToastBox } from "./components/toast-box.js";
import { UserCreate } from "./components/user-create.js";
import { UserEditor } from "./components/user-editor.js";
import { UserTable } from "./components/user-table.js";
import { installI18n, t } from "./i18n.js";
import { maintainStore } from "./maintain-store.js";
import { createApp, onMounted } from "./vue.js";

const MaintainPage = {
  name: "MaintainPage",
  components: { LocaleSwitch, ToastBox, UserCreate, UserEditor, UserTable },
  setup() {
    onMounted(() => maintainStore.load());
    return { store: maintainStore };
  },
  template: `
    <div class="app">
      <header class="topbar">
        <span class="brand-mark" aria-hidden="true"></span>
        <span class="brand-name">{{ t("brand.site") }}</span>
        <span class="brand-sub">{{ t("brand.sub-maintain") }}</span>
        <a class="back-link" href="/">
          <svg class="icon"><use href="#i-back"/></svg>{{ t("nav.back") }}
        </a>
        <span class="chip">{{ t("maintain.count", { count: store.state.users.length }) }}</span>
        <span v-if="store.me" class="chip chip-user" :title="'UUID ' + store.me.uuid">
          {{ store.me.player_name }} · {{ store.roleLabel(store.me.role) }}
        </span>
        <LocaleSwitch/>
        <button class="icon-btn" type="button" :title="t('nav.signout')" @click="store.logout()">
          <svg class="icon"><use href="#i-logout"/></svg>
        </button>
      </header>

      <main class="m-body">
        <div class="m-wrap">
          <section v-if="store.state.denied" class="m-card m-denied">
            <h2><svg class="icon"><use href="#i-shield"/></svg>{{ t("maintain.admin-required") }}</h2>
            <p>{{ t("maintain.no-permission") }}</p>
            <a class="primary-btn" href="/">{{ t("nav.back") }}</a>
          </section>

          <!-- 身份由后端定：答复回来之前先别把表单画出来，否则非管理员会闪一下表单 -->
          <section v-else-if="store.state.loading" class="m-card">
            <span class="m-sub">{{ t("common.loading") }}</span>
          </section>

          <template v-else>
            <section class="m-card">
              <header class="m-card-head">
                <h2><svg class="icon"><use href="#i-users"/></svg>{{ t("auth.account") }}</h2>
                <span class="m-sub" v-if="store.state.loading">{{ t("common.loading") }}</span>
                <span class="m-sub" v-else>{{ t("common.count", { count: store.state.users.length }) }}</span>
              </header>
              <UserTable/>
            </section>
          </template>
        </div>
      </main>

      <UserCreate/>
      <UserEditor/>
      <ToastBox :toast="store.state.toast"/>
    </div>`,
};

if (requireAuth()) {
  const app = createApp(MaintainPage);
  installI18n(app, t("title.accounts"));
  app.mount("#app");
  window.__maintain = maintainStore; // 调试句柄，和主页面的 window.__skin 一个意思
}
