<script setup lang="ts">
/** 浮动提示：与具体 store 解耦，调用方把 toast 状态对象传进来即可。 */
import type { ToastKind } from "@/toast";

defineProps<{ toast: { message: string; kind: ToastKind; visible: boolean } }>();
</script>

<template>
  <div class="toast" :class="{ show: toast.visible, error: toast.kind === 'error' }">
    {{ toast.message }}
  </div>
</template>

<style scoped>
/* ── 提示条 ────────────────────────────────────────────────────── */

.toast {
  position: fixed;
  left: 50%;
  bottom: 26px;
  transform: translate(-50%, 12px);
  z-index: 50;
  padding: 9px 18px;
  border-radius: var(--radius-md);
  background: rgba(22, 28, 45, .92);
  color: #fff;
  font-size: 13px;
  box-shadow: var(--shadow-pop);
  pointer-events: none;
  opacity: 0;
  transition: opacity .2s, transform .2s;
}

.toast.show { opacity: 1; transform: translate(-50%, 0); }

.toast.error { background: rgba(190, 40, 45, .94); }
</style>
