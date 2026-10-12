<script setup lang="ts">
/**
 * 登录页（/auth）根组件：POST /api/auth/login 拿 token，存进 localStorage 后回首页。
 */
import { onMounted, reactive, ref } from "vue";

import { setToken } from "@/auth";
import LocaleSwitch from "@/components/LocaleSwitch.vue";
import { getLocale, t } from "@/i18n";
import type { ErrorResponse, LoginResponse } from "@/types";

const form = reactive({ username: "", password: "" });
const error = ref("");
const busy = ref(false);
const usernameInput = ref<HTMLInputElement | null>(null);

async function submit(): Promise<void> {
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
  const payload = (await response.json().catch(() => ({}))) as LoginResponse & ErrorResponse;
  if (!response.ok) {
    throw new Error(payload.detail || t("auth.failed", { status: response.status }));
  }
  setToken(payload.token, payload.player);
  location.replace("/");
  } catch (failure) {
  error.value = failure instanceof Error ? failure.message : String(failure);
  busy.value = false;
  }
}

onMounted(() => usernameInput.value?.focus());
</script>

<template>
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
  </main>
</template>

<style scoped>
/* ── 登录页 ─────────────────────────────────────────────────────── */

.auth-page {
  width: min(430px, calc(100vw - 32px));
  padding: 16px;
}

.auth-card {
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 24px;
  border: 1px solid var(--line);
  border-radius: var(--radius-lg);
  background: var(--paper);
  box-shadow: var(--shadow-pop);
}

.auth-head { display: flex; align-items: center; gap: 12px; }

/* 语言切换贴到卡片右上角：跟着标题中线摆，会被看成挂在标题上的小徽章 */
.auth-head .locale-switch { margin-left: auto; align-self: flex-start; }

.auth-head h1 { margin: 0; font-size: 19px; letter-spacing: .04em; }

.auth-head p { margin: 2px 0 0; font-size: 12px; color: var(--ink-faint); letter-spacing: .06em; }

.auth-head .brand-mark { width: 34px; height: 34px; border-radius: 10px; }

.auth-head .brand-mark::after { inset: 8px 8px auto 8px; height: 5px; }

.auth-form { display: flex; flex-direction: column; gap: 12px; }

.auth-form .field { flex-direction: column; align-items: stretch; gap: 6px; }

.auth-form .field input {
  padding: 10px 12px;
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-md);
  font-size: 14px;
  outline: none;
  transition: border-color .15s, box-shadow .15s;
}

.auth-form .field input:focus { border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-soft); }

.auth-error {
  margin: 0;
  padding: 8px 12px;
  border-radius: var(--radius-md);
  background: #fdecec;
  color: var(--danger);
  font-size: 12.5px;
}

.auth-submit { width: 100%; padding: 11px 22px; }
</style>
