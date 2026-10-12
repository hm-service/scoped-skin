/**
 * 登录页（/auth）的入口。
 */
import { createApp } from "vue";

import LoginPage from "@/LoginPage.vue";
import { getToken } from "@/auth";
import { applyDocumentLocale, t } from "@/i18n";
import "@/style.css";

// 已经有 token 就直接回首页（失效的话首页会再把人送回来）
if (getToken()) {
  location.replace("/");
} else {
  applyDocumentLocale(t("title.login"));
  createApp(LoginPage).mount("#app");
}
