/** 页面外框：主页面的顶栏（提示框见 components/toast-box.js）。 */
import { t } from "../i18n.js";
import { store } from "../store.js";
import { LocaleSwitch } from "./locale-switch.js";

export const TopBar = {
  name: "TopBar",
  components: { LocaleSwitch },
  setup() {
    return { store };
  },
  template: `
    <header class="topbar">
      <span class="brand-mark" aria-hidden="true"></span>
      <span class="brand-name">{{ t("皮肤站") }}</span>
      <span class="brand-sub">{{ t("Minecraft Skin Library") }}</span>
      <span class="chip">{{ store.statsText }}</span>
      <span v-if="store.state.player" class="chip chip-user"
            :title="'UUID ' + store.state.player.uuid">
        {{ store.state.player.player_name }} · {{ store.state.player.username }}
        <b class="role-flag" :class="store.role">{{ store.roleLabel }}</b>
      </span>
      <a v-if="store.state.player?.role === 'admin'" class="icon-btn" href="/maintain"
         :title="t('用户管理')">
        <svg class="icon"><use href="#i-users"/></svg>
      </a>
      <LocaleSwitch/>
      <button class="icon-btn" type="button" :title="t('退出登录')" @click="store.logout()">
        <svg class="icon"><use href="#i-logout"/></svg>
      </button>
    </header>`,
};
