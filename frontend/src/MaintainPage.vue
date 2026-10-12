<script setup lang="ts">
/**
 * /maintain 根组件：管理员维护页，一屏内完成用户管理。
 *
 * 页面本身不鉴权（谁都能打开），真正的闸门是后端的 /api/admin/*：非 admin 会
 * 拿到 403，这里就只剩一张「需要管理员身份」的提示卡。
 */
import { onMounted } from "vue";

import LocaleSwitch from "@/components/LocaleSwitch.vue";
import ToastBox from "@/components/ToastBox.vue";
import UserCreate from "@/components/UserCreate.vue";
import UserEditor from "@/components/UserEditor.vue";
import UserTable from "@/components/UserTable.vue";
import { t } from "@/i18n";
import { maintainStore } from "@/maintain-store";

const store = maintainStore;

onMounted(() => maintainStore.load());
</script>

<template>
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
  </div>
</template>

<style scoped>
/* 维护页的返回链接：一条安静的入口，别跟品牌和右侧按钮抢注意力 */
.back-link {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-left: 2px;
  padding: 5px 9px;
  border-radius: 8px;
  color: var(--ink-soft);
  font-size: 12.5px;
  text-decoration: none;
  transition: background .15s, color .15s;
}

.back-link .icon { width: 14px; height: 14px; }

.back-link:hover { background: var(--paper-sunk); color: var(--accent-ink); }

.m-body { flex: 1 1 auto; min-height: 0; overflow: auto; }

.m-wrap {
  max-width: 1040px;
  margin: 0 auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.m-card {
  padding: 14px 16px 16px;
  border: 1px solid var(--line);
  border-radius: var(--radius-lg);
  background: var(--paper);
  box-shadow: var(--shadow-card);
}

.m-card-head { display: flex; align-items: baseline; gap: 10px; margin-bottom: 12px; }

.m-card-head h2 { display: flex; align-items: center; gap: 7px; margin: 0; font-size: 15px; }

.m-card-head .icon { color: var(--accent-ink); }

.m-sub { font-size: 12px; color: var(--ink-faint); }

.m-denied { padding: 40px 20px; text-align: center; }

.m-denied h2 { display: flex; align-items: center; justify-content: center; gap: 8px; margin: 0 0 8px; font-size: 17px; }

.m-denied p { margin: 0 0 16px; color: var(--ink-soft); font-size: 13px; }

.m-denied .primary-btn { display: inline-block; text-decoration: none; }
</style>
