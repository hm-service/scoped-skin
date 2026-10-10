/**
 * Vue 3 全局构建：/static/vendor/vue.global.prod.js 暴露 `window.Vue`。
 * 其它模块统一从这里取 API，不各自去读全局对象。
 */
const Vue = globalThis.Vue;

if (!Vue) {
  throw new Error("未找到 Vue：页面需要在模块脚本之前引入 /static/vendor/vue.global.prod.js");
}

export const { createApp, computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } =
  Vue;
