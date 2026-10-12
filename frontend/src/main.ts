/**
 * 库页（/）的入口：装配根组件，顺带把 <html lang> 与标题换成当前语言。
 */
import { createApp } from "vue";

import App from "@/App.vue";
import { requireAuth } from "@/auth";
import { applyDocumentLocale, t } from "@/i18n";
import { store, viewerRef } from "@/store";
import type { Preview3D } from "@/viewer";
import "@/style.css";

// 调试用句柄：浏览器控制台里可以直接检查 / 驱动界面状态
declare global {
  interface Window {
    __skin: {
      store: typeof store;
      select: typeof store.select;
      readonly viewer: Preview3D | null;
    };
  }
}

if (requireAuth()) {
  applyDocumentLocale(t("title.library"));
  createApp(App).mount("#app");
  window.__skin = {
    store,
    select: store.select,
    get viewer() {
      return viewerRef.current;
    },
  };
}
