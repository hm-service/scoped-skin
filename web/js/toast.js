/**
 * 浮动提示：store.js 与 maintain-store.js 的 toast 状态形状一致，
 * 「显示 2.2 秒后自动隐藏」的逻辑只写这一份。
 *
 * state.toast 必须是 { message, kind, visible } 形状的响应式对象。
 */
export function createToast(state) {
  let timer = null;

  function toast(message, kind = "info") {
    state.toast.message = message;
    state.toast.kind = kind;
    state.toast.visible = true;
    clearTimeout(timer);
    timer = setTimeout(() => {
      state.toast.visible = false;
    }, 2200);
  }

  const toastError = (message) => toast(message, "error");

  return { toast, toastError };
}
