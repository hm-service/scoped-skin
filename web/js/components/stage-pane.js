/** 右侧 3D 面板：持有 skinview3d 实例，把 store 里的状态映射成调用。 */
import { ANIMATIONS, store, viewerRef } from "../store.js";
import { t } from "../i18n.js";
import { onBeforeUnmount, onMounted, ref, watch } from "../vue.js";
import { Preview3D } from "../viewer.js";
import { TagCapsule } from "./tag-capsule.js";

export const StagePane = {
  name: "StagePane",
  components: { TagCapsule },
  setup() {
    const canvas = ref(null);
    const animations = ANIMATIONS;

    function loadSkin() {
      const skin = store.stageSkin;
      if (viewerRef.current && skin) viewerRef.current.setSkin(skin);
    }

    function loadCape() {
      viewerRef.current?.setCape(store.stageCapeUrl);
    }

    onMounted(() => {
      viewerRef.current = new Preview3D(canvas.value, store.toastError);
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

    const resetPose = () => viewerRef.current?.resetPose();

    return { store, canvas, animations, resetPose };
  },
  template: `
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
                  :aria-pressed="String(store.state.view.rotate)" @click="store.toggleRotate()">
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
                  :aria-label="t('noun.cape')" :aria-pressed="String(store.stageBackEquipment === 'cape')"
                  @click="store.setBackEquipment('cape')">
            <svg class="icon"><use href="#i-cape"/></svg>
          </button>
          <button :class="{ on: store.stageBackEquipment === 'elytra' }" :title="t('preview.elytra')"
                  :aria-label="t('preview.elytra')" :aria-pressed="String(store.stageBackEquipment === 'elytra')"
                  @click="store.setBackEquipment('elytra')">
            <svg class="icon"><use href="#i-elytra"/></svg>
          </button>
          <button :class="{ on: !store.stageBackEquipment }" :title="t('preview.hide')"
                  :aria-label="t('preview.hide')" :aria-pressed="String(!store.stageBackEquipment)"
                  @click="store.setBackEquipment(null)">
            <svg class="icon"><use href="#i-eye-off"/></svg>
          </button>
        </div>
        <button class="ghost-btn" :class="{ on: store.state.view.paused }"
                :title="t('preview.pause-toggle')" @click="store.togglePaused()">
          <svg class="icon"><use :href="store.state.view.paused ? '#i-play' : '#i-pause'"/></svg>{{ store.state.view.paused ? t('preview.resume') : t('preview.pause') }}
        </button>
      </div>
    </section>`,
};
