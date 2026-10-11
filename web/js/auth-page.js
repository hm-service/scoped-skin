/**
 * 登录页：POST /api/auth/login 拿 token，存进 localStorage 后回首页。
 */
import { getToken, setToken } from "./auth.js";
import { LocaleSwitch } from "./components/locale-switch.js";
import { getLocale, installI18n, t } from "./i18n.js";
import { createApp, onMounted, reactive, ref } from "./vue.js";

const LoginPage = {
  name: "LoginPage",
  components: { LocaleSwitch },
  setup() {
    const form = reactive({ username: "", password: "" });
    const error = ref("");
    const busy = ref(false);
    const usernameInput = ref(null);

    async function submit() {
      const username = form.username.trim();
      if (!username || !form.password) {
        error.value = t("auth.empty");
        return;
      }
      busy.value = true;
      error.value = "";
      try {
        const response = await fetch("/api/auth/login", {
          method: "POST",
          headers: { "Content-Type": "application/json", "Accept-Language": getLocale() },
          body: JSON.stringify({ username, password: form.password }),
        });
        const payload = await response.json().catch(() => ({}));
        if (!response.ok) {
          throw new Error(payload.detail || t("auth.failed", { status: response.status }));
        }
        setToken(payload.token, payload.player);
        location.replace("/");
      } catch (failure) {
        error.value = failure.message;
        busy.value = false;
      }
    }

    onMounted(() => usernameInput.value?.focus());

    return { form, error, busy, usernameInput, submit };
  },
  template: `
    <main class="auth-page">
      <section class="auth-card">
        <header class="auth-head">
          <span class="brand-mark" aria-hidden="true"></span>
          <div>
            <h1>{{ t("brand.site") }}</h1>
            <p>{{ t("brand.sub") }}</p>
          </div>
          <LocaleSwitch/>
        </header>

        <form class="auth-form" novalidate @submit.prevent="submit">
          <label class="field">
            <span>{{ t("auth.account") }}</span>
            <input ref="usernameInput" v-model="form.username" type="text"
                   autocomplete="username" spellcheck="false" required>
          </label>
          <label class="field">
            <span>{{ t("auth.password") }}</span>
            <input v-model="form.password" type="password"
                   autocomplete="current-password" required>
          </label>
          <p v-if="error" class="auth-error">{{ error }}</p>
          <button type="submit" class="primary-btn auth-submit" :disabled="busy">
            {{ busy ? t('auth.submitting') : t('auth.submit') }}
          </button>
        </form>
      </section>
    </main>`,
};

// 已经有 token 就直接回首页（失效的话首页会再把人送回来）
if (getToken()) {
  location.replace("/");
} else {
  const app = createApp(LoginPage);
  installI18n(app, t("title.login"));
  app.mount("#app");
}
