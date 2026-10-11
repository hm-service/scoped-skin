/** 语言切换按钮：显示当前语言，点一下换到另一种（换完重载）。 */
import { currentLanguage, otherLanguage, setLocale } from "../i18n.js";

export const LocaleSwitch = {
  name: "LocaleSwitch",
  setup() {
    return { current: currentLanguage(), other: otherLanguage(), setLocale };
  },
  template: `
    <button class="locale-switch" type="button"
            :title="t('nav.switch-language', { language: other.label })"
            :aria-label="t('nav.switch-language', { language: other.label })"
            @click="setLocale(other.id)">{{ current.short }}</button>`,
};
