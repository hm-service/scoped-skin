<script setup lang="ts">
/** 页面外框：主页面的顶栏（提示框见 components/toast-box.js）。 */
import LocaleSwitch from "@/components/LocaleSwitch.vue";
import { t } from "@/i18n";
import { store } from "@/store";
</script>

<template>
  <header class="topbar">
    <span class="brand-mark" aria-hidden="true"></span>
    <span class="brand-name">{{ t("brand.site") }}</span>
    <span class="brand-sub">{{ t("brand.sub") }}</span>
    <span class="chip">{{ store.statsText }}</span>
    <span v-if="store.state.player" class="chip chip-user"
          :title="'UUID ' + store.state.player.uuid">
      {{ store.state.player.player_name }} · {{ store.state.player.username }}
      <b class="role-flag" :class="store.role">{{ store.roleLabel }}</b>
    </span>
    <a v-if="store.state.player?.role === 'admin'" class="icon-btn" href="/maintain"
       :title="t('nav.accounts')">
      <svg class="icon"><use href="#i-users"/></svg>
    </a>
    <LocaleSwitch/>
    <button class="icon-btn" type="button" :title="t('nav.signout')" @click="store.logout()">
      <svg class="icon"><use href="#i-logout"/></svg>
    </button>
  </header>
</template>

<style scoped>
/* ── 维护页（/maintain） ────────────────────────────────────────── */

/* 顶栏里的身份组小标：admin 绿、normal 灰、guest 更淡 */
.role-flag {
  margin-left: 5px;
  padding: 1px 6px;
  border-radius: 999px;
  background: var(--paper-sunk);
  color: var(--ink-soft);
  font-size: 10.5px;
  font-weight: 600;
}

.role-flag.admin { background: var(--accent-soft); color: var(--accent-ink); }

.role-flag.guest { color: var(--ink-faint); }
</style>
