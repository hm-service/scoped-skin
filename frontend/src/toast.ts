/**
 * 浮动提示：store.ts 与 maintain-store.ts 的 toast 状态形状一致，
 * 「显示 2.2 秒后自动隐藏」的逻辑只写这一份。
 */

export type ToastKind = "info" | "error";

/** 两个 store 的状态里都有这一格，形状一致。 */
export interface HasToast {
  toast: { message: string; kind: ToastKind; visible: boolean };
}

export function createToast(state: HasToast) {
  let timer: ReturnType<typeof setTimeout> | null = null;

  function toast(message: string, kind: ToastKind = "info"): void {
    state.toast.message = message;
    state.toast.kind = kind;
    state.toast.visible = true;
    clearTimeout(timer ?? undefined);
    timer = setTimeout(() => {
      state.toast.visible = false;
    }, 2200);
  }

  const toastError = (message: string) => toast(message, "error");

  return { toast, toastError };
}
