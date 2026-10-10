/**
 * /maintain 根组件：管理员维护页，一屏内完成用户管理。
 *
 * 页面本身不鉴权（谁都能打开），真正的闸门是后端的 /api/admin/*：非 admin 会
 * 拿到 403，这里就只剩一张「需要管理员身份」的提示卡。
 */
import { requireAuth } from "./auth.js";
import { LocaleSwitch } from "./components/locale-switch.js";
import { ToastBox } from "./components/toast-box.js";
import { UserEditor } from "./components/user-editor.js";
import { UserTable } from "./components/user-table.js";
import { installI18n, t } from "./i18n.js";
import { maintainStore } from "./maintain-store.js";
import { createApp, onMounted } from "./vue.js";

const MaintainPage = {
  name: "MaintainPage",
  components: { LocaleSwitch, ToastBox, UserEditor, UserTable },
  setup() {
    onMounted(() => maintainStore.load());
    return { store: maintainStore };
  },
  template: `
    <div class="app">
      <header class="topbar">
        <span class="brand-mark" aria-hidden="true"></span>
        <span class="brand-name">{{ t("皮肤站") }}</span>
        <span class="brand-sub">Maintain</span>
        <a class="ghost-btn" href="/">
          <svg class="icon"><use href="#i-back"/></svg>{{ t("回皮肤站") }}
        </a>
        <LocaleSwitch/>
        <span class="chip">{{ t("{count} 个账号", { count: store.state.users.length }) }}</span>
        <span v-if="store.me" class="chip chip-user" :title="'UUID ' + store.me.uuid">
          {{ store.me.player_name }} · {{ store.roleLabel(store.me.role) }}
        </span>
        <button class="icon-btn" type="button" :title="t('退出登录')" @click="store.logout()">
          <svg class="icon"><use href="#i-logout"/></svg>
        </button>
      </header>

      <main class="m-body">
        <div class="m-wrap">
          <section v-if="store.state.denied" class="m-card m-denied">
            <h2><svg class="icon"><use href="#i-shield"/></svg>{{ t("需要管理员身份") }}</h2>
            <p>{{ t("当前身份组没有用户管理权限。") }}</p>
            <a class="primary-btn" href="/">{{ t("回皮肤站") }}</a>
          </section>

          <!-- 身份由后端定：答复回来之前先别把表单画出来，否则非管理员会闪一下表单 -->
          <section v-else-if="store.state.loading" class="m-card">
            <span class="m-sub">{{ t("载入中…") }}</span>
          </section>

          <template v-else>
            <section class="m-card">
              <header class="m-card-head">
                <h2><svg class="icon"><use href="#i-users"/></svg>{{ t("新建用户") }}</h2>
                <span class="m-sub">{{ t("UUID 从游戏内名称推导，建好后可改") }}</span>
              </header>
              <form class="m-grid" @submit.prevent="store.create()">
                <label class="field">
                  <span>{{ t("游戏内名称") }}</span>
                  <input v-model.trim="store.state.draft.player_name" type="text" maxlength="16"
                         spellcheck="false" :placeholder="t('小写字母 / 数字 / 下划线')" required>
                </label>
                <label class="field">
                  <span>{{ t("账号") }}</span>
                  <input v-model.trim="store.state.draft.username" type="text" maxlength="32"
                         spellcheck="false" autocomplete="off" :placeholder="t('登录名')" required>
                </label>
                <label class="field">
                  <span>{{ t("密码") }}</span>
                  <input v-model="store.state.draft.password" type="password"
                         autocomplete="new-password" :placeholder="t('登录密码')" required>
                </label>
                <label class="field">
                  <span>{{ t("身份组") }}</span>
                  <select v-model="store.state.draft.role">
                    <option value="guest">{{ t("游客") }}</option>
                    <option value="normal">{{ t("普通") }}</option>
                    <option value="admin">{{ t("管理员") }}</option>
                  </select>
                </label>
                <div class="form-actions m-grid-actions">
                  <button type="submit" class="primary-btn" :disabled="store.state.creating">
                    <svg class="icon"><use href="#i-plus"/></svg>
                    {{ store.state.creating ? t('创建中…') : t('创建用户') }}
                  </button>
                </div>
              </form>
            </section>

            <section class="m-card">
              <header class="m-card-head">
                <h2><svg class="icon"><use href="#i-users"/></svg>{{ t("账号") }}</h2>
                <span class="m-sub" v-if="store.state.loading">{{ t("载入中…") }}</span>
                <span class="m-sub" v-else>{{ t("{count} 个", { count: store.state.users.length }) }}</span>
              </header>
              <UserTable/>
            </section>
          </template>
        </div>
      </main>

      <UserEditor/>
      <ToastBox :toast="store.state.toast"/>
    </div>`,
};

if (requireAuth()) {
  const app = createApp(MaintainPage);
  installI18n(app, t("用户管理 · 皮肤站"));
  app.mount("#app");
  window.__maintain = maintainStore; // 调试句柄，和主页面的 window.__skin 一个意思
}
