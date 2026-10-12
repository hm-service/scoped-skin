<script setup lang="ts">
/** 右侧 3D 面板：持有 skinview3d 实例，把 store 里的状态映射成调用。 */
import { onBeforeUnmount, onMounted, ref, watch } from "vue";

import TagCapsule from "@/components/TagCapsule.vue";
import { t } from "@/i18n";
import { ANIMATIONS, store, viewerRef } from "@/store";
import { Preview3D } from "@/viewer";

const canvas = ref<HTMLCanvasElement | null>(null);
const animations = ANIMATIONS;

function loadSkin(): void {
  const skin = store.stageSkin;
  if (viewerRef.current && skin) viewerRef.current.setSkin(skin);
}

function loadCape(): void {
  viewerRef.current?.setCape(store.stageCapeUrl);
}

onMounted(() => {
  viewerRef.current = new Preview3D(canvas.value!, store.toastError);
  loadSkin();
  loadCape();
  const preview = viewerRef.current;
  preview.setAnimation(store.state.view.animation);
  preview.setPaused(store.state.view.paused);
  preview.setAutoRotate(store.state.view.rotate);
  preview.setSkinVisible(store.state.view.skinVisible);
  preview.setOuterLayerVisible(store.state.view.outerVisible);
  preview.setBackEquipment(store.stageBackEquipment);
});

onBeforeUnmount(() => {
  viewerRef.current?.dispose();
  viewerRef.current = null;
});

watch(() => [store.stageSkin?.texture_url, store.stageSkin?.kind], loadSkin);
watch(() => store.stageCapeUrl, loadCape);
watch(() => store.stageBackEquipment, (equipment) => viewerRef.current?.setBackEquipment(equipment));
watch(() => store.state.view.animation, (key) => viewerRef.current?.setAnimation(key));
watch(() => store.state.view.paused, (paused) => viewerRef.current?.setPaused(paused));
watch(() => store.state.view.rotate, (rotate) => viewerRef.current?.setAutoRotate(rotate));
watch(() => store.state.view.skinVisible, (visible) => viewerRef.current?.setSkinVisible(visible));
watch(() => store.state.view.outerVisible, (visible) => viewerRef.current?.setOuterLayerVisible(visible));

const resetPose = (): void => viewerRef.current?.resetPose();
</script>

<template>
  <section class="panel stage" :aria-label="t('preview.title')">
    <div class="stage-head">
      <div class="stage-title">
        <div class="tags">
          <TagCapsule v-for="tag in store.stage.tags" :key="tag" :tag="tag"/>
          <span v-if="!store.stage.tags.length" class="tag-empty">{{ store.stage.fallback }}</span>
        </div>
        <span v-if="store.stage.model" class="badge">{{ store.stage.model }}</span>
        <span v-if="store.stage.live" class="badge badge-live">{{ t("preview.pending") }}</span>
      </div>
      <div class="stage-tools">
        <button class="icon-btn" :class="{ on: store.state.view.rotate }" :title="t('preview.auto-rotate')"
                :aria-pressed="store.state.view.rotate" @click="store.toggleRotate()">
          <svg class="icon"><use href="#i-rotate"/></svg>
        </button>
        <button class="icon-btn" :title="t('preview.reset')" @click="resetPose()">
          <svg class="icon"><use href="#i-target"/></svg>
        </button>
      </div>
    </div>

    <div class="stage-view">
      <canvas id="viewer" ref="canvas"></canvas>
      <span class="stage-hint">{{ t("preview.hint") }}</span>
      <div v-if="store.stageEmpty" class="stage-empty">{{ t("preview.pick-skin") }}</div>
    </div>

    <div class="stage-foot">
      <div class="seg">
        <button v-for="animation in animations" :key="animation.key"
                :class="{ on: store.state.view.animation === animation.key }"
                @click="store.setAnimation(animation.key)">{{ animation.label }}</button>
      </div>
      <button class="ghost-btn" id="btnSkin" :class="{ on: store.state.view.skinVisible }"
              :title="t('preview.toggle-skin')" @click="store.toggleSkinVisible()">
        <svg class="icon"><use :href="store.state.view.skinVisible ? '#i-eye' : '#i-eye-off'"/></svg>{{ t("noun.skin") }}
      </button>
      <button class="ghost-btn" :class="{ on: store.state.view.outerVisible }"
              :title="t('preview.toggle-overlay')" @click="store.toggleOuterVisible()">
        <svg class="icon"><use href="#i-layers"/></svg>{{ t("noun.overlay") }}
      </button>
      <!-- 背面三档：披风 / 鞘翅 / 隐藏（游戏里鞘翅用的就是披风那张材质） -->
      <div class="seg seg-back" :aria-label="t('preview.back-equipment')">
        <button :class="{ on: store.stageBackEquipment === 'cape' }" :title="t('noun.cape')"
                :aria-label="t('noun.cape')" :aria-pressed="store.stageBackEquipment === 'cape'"
                @click="store.setBackEquipment('cape')">
          <svg class="icon"><use href="#i-cape"/></svg>
        </button>
        <button :class="{ on: store.stageBackEquipment === 'elytra' }" :title="t('preview.elytra')"
                :aria-label="t('preview.elytra')" :aria-pressed="store.stageBackEquipment === 'elytra'"
                @click="store.setBackEquipment('elytra')">
          <svg class="icon"><use href="#i-elytra"/></svg>
        </button>
        <button :class="{ on: !store.stageBackEquipment }" :title="t('preview.hide')"
                :aria-label="t('preview.hide')" :aria-pressed="!store.stageBackEquipment"
                @click="store.setBackEquipment(null)">
          <svg class="icon"><use href="#i-eye-off"/></svg>
        </button>
      </div>
      <button class="ghost-btn" :class="{ on: store.state.view.paused }"
              :title="t('preview.pause-toggle')" @click="store.togglePaused()">
        <svg class="icon"><use :href="store.state.view.paused ? '#i-play' : '#i-pause'"/></svg>{{ store.state.view.paused ? t('preview.resume') : t('preview.pause') }}
      </button>
    </div>
  </section>
</template>

<style scoped>
.badge {
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--paper-sunk);
  color: var(--ink-soft);
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
}

.badge-live { background: var(--accent-soft); color: var(--accent-ink); }

/* ── 右侧：3D 舞台 ─────────────────────────────────────────────── */

.stage-head {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--line);
  min-height: 52px;
}

.stage-title { display: flex; align-items: center; gap: 8px; min-width: 0; flex-wrap: wrap; }

.stage-title .tags { max-width: 100%; }

.stage-tools { display: flex; gap: 6px; flex: 0 0 auto; }

.stage-view {
  position: relative;
  flex: 1 1 auto;
  min-height: 0;
  background: radial-gradient(58% 52% at 50% 36%, #ffffff 0%, #eaeff9 58%, #dbe2f1 100%);
}

.stage-hint {
  position: absolute;
  right: 10px;
  bottom: 8px;
  z-index: 2;
  font-size: 11px;
  color: var(--ink-faint);
  background: rgba(255, 255, 255, .72);
  padding: 3px 8px;
  border-radius: 999px;
  pointer-events: none;
}

.stage-empty {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: grid;
  place-items: center;
  background: rgba(247, 249, 253, .72);
  color: var(--ink-faint);
  font-size: 13px;
  letter-spacing: .04em;
}

.stage-foot {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  padding: 9px 10px;
  border-top: 1px solid var(--line);
}

.seg {
  display: flex;
  padding: 3px;
  border: 1px solid var(--line);
  border-radius: var(--radius-md);
  background: var(--paper-soft);
}

.seg button {
  padding: 5px 9px;
  border: none;
  border-radius: 7px;
  background: transparent;
  color: var(--ink-soft);
  font-size: 12.5px;
  transition: all .15s;
}

.seg button:hover { color: var(--ink); }

.seg button.on { background: var(--paper); color: var(--accent-ink); font-weight: 600; box-shadow: var(--shadow-card); }

/* 背面三档：披风 / 鞘翅 / 隐藏（只有图标，靠 title 与高亮表示当前档） */
.seg-back button { display: grid; place-items: center; padding: 5px 8px; }

.seg-back .icon { width: 15px; height: 15px; }

.stage-foot #btnSkin { margin-left: auto; }

@media (max-width: 980px) {
  .stage { min-height: 520px; }
}

/* 这里本来有一条 ::after 画的「接地阴影」，是个钉死在 bottom: 9% / 宽 44% 的椭圆：
   不跟缩放、不跟旋转，也不跟中键平移，模型还没选中时它也在 —— 位置对不上的阴影比没有更假，删掉。 */
#viewer { display: block; width: 100%; height: 100%; position: relative; z-index: 1; }
</style>
