/** 浮动提示：与具体 store 解耦，调用方把 toast 状态对象传进来即可。 */
export const ToastBox = {
  name: "ToastBox",
  props: { toast: { type: Object, required: true } },
  template: `
    <div class="toast" :class="{ show: toast.visible, error: toast.kind === 'error' }">
      {{ toast.message }}
    </div>`,
};
