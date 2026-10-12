/**
 * 维护页（/maintain）的入口。
 */
import { createApp } from "vue";

import MaintainPage from "@/MaintainPage.vue";
import { requireAuth } from "@/auth";
import { applyDocumentLocale, t } from "@/i18n";
import { maintainStore } from "@/maintain-store";
import "@/style.css";

declare global {
  interface Window {
    __maintain: typeof maintainStore;
  }
}

if (requireAuth()) {
  applyDocumentLocale(t("title.accounts"));
  createApp(MaintainPage).mount("#app");
  window.__maintain = maintainStore; // 调试句柄，和主页面的 window.__skin 一个意思
}
