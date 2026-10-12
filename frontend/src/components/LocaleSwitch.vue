<script setup lang="ts">
/** 语言切换按钮：显示当前语言，点一下换到另一种（换完重载）。 */
import { currentLanguage, otherLanguage, setLocale, t } from "@/i18n";

const current = currentLanguage();
const other = otherLanguage();
</script>

<template>
  <button class="locale-switch" type="button"
          :title="t('nav.switch-language', { language: other.label })"
          :aria-label="t('nav.switch-language', { language: other.label })"
          @click="setLocale(other.id)">{{ current.short }}</button>
</template>

<style scoped>
/* 语言切换：跟顶栏那排 icon-btn 同规格（同一行里不要两种方圆） */
.locale-switch {
  display: grid;
  place-items: center;
  min-width: 30px;
  height: 30px;
  padding: 0 6px;
  border: 1px solid var(--line);
  border-radius: 9px;
  background: var(--paper);
  color: var(--ink-soft);
  font: inherit;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: .04em;
  cursor: pointer;
  transition: all .15s;
}

.locale-switch:hover { border-color: var(--accent); color: var(--accent-ink); }
</style>
