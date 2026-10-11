/**
 * 3D 预览：对 skinview3d (global `skinview3d`, UMD bundle) 的一层薄封装。
 */
import { t } from "./i18n.js";

/** 库里的 kind → skinview3d 的模型参数：模型已并进 kind，披风不走 setSkin。 */
const MODEL_TYPES = {
  "skin.steve": "default",
  "skin.alex": "slim",
};

const ANIMATION_FACTORIES = {
  idle: () => new skinview3d.IdleAnimation(),
  walk: () => new skinview3d.WalkingAnimation(),
  run: () => new skinview3d.RunningAnimation(),
  fly: () => new skinview3d.FlyingAnimation(),
};

/** 渲染帧率上限。理由见 capFrameRate()。 */
const MAX_FPS = 60;

/**
 * 光照。这段数值和 vendor 默认值（ambient 3 / point 0.6）都不同，理由如下，改之前先读完。
 *
 * 一、系数是「显示空间」的系数。skinview3d 的皮肤贴图没有标 sRGB，而合成器最后一道 FXAA
 *    也没有做输出编码，所以整条链路实际等效于「贴图原始字节 × 光照系数」，跟 Minecraft
 *    自己一样是在显示空间里乘光。着色器里 Lambert 的 BRDF 带 1/π，于是
 *        亮度系数 = intensity / π
 *    正面系数取满 1，模型正脸就和贴图本身、和列表里的 2D 缩略图完全一致，不会过曝也不会发灰。
 *
 * 二、cameraLight 必须把 decay 归零。它是挂在相机上的点光源，离模型约 52 个世界单位，
 *    three 从 r155 起按物理方式衰减（intensity / d²），decay=2 时贡献约 1/2700 —— 等于没有，
 *    这正是原来预览「六个面一样亮、完全没有体积」的原因。decay=0 表示强度不随距离衰减。
 *
 * 三、把灯从相机轴上挪到「左上前方」，六个面才会分成几档亮度。系数取
 *    AMBIENT + KEY * cos ≈ 0.60 + 0.46 × 0.78 ≈ 0.96（正对光的面）、0.60（背光面），
 *    与游戏里实体预览的明暗差接近。灯留在相机坐标系里而不是世界坐标系：预览是自动旋转的，
 *    跟相机走才能保证任何角度下看得到的那一面都是亮的、暗面只出现在背光的一侧。
 *
 * 四、灯不能放太远。平行光下一个平面上的法线处处相同，正对着看时整张脸就是一个亮度
 *    （正面视图会平掉）；KEY_DISTANCE 取 60 左右，让方向在模型尺度上略有变化，正面也就有了
 *    一点上亮下暗的过渡。
 */
const LAMBERT_1_OVER_PI = Math.PI; // 见上面第一条：intensity = 系数 × π
const AMBIENT = 0.6;
const KEY = 0.46;
const KEY_DIR = [-0.36, 0.52, 0.78]; // 相机坐标系：左、上、前
const KEY_DISTANCE = 60; // 相机坐标系里的灯位（约等于默认视距），见上面第四条

export class Preview3D {
  constructor(canvas, onError) {
    this.canvas = canvas;
    this.onError = onError || (() => {});
    this.animationKey = "idle";
    this.paused = false;
    this.skinVisible = true;
    this.outerVisible = true;
    this.backEquipment = null; // cape | elytra | null

    this.viewer = new skinview3d.SkinViewer({
      canvas,
      width: Math.max(1, canvas.parentElement.clientWidth),
      height: Math.max(1, canvas.parentElement.clientHeight),
      model: "auto-detect",
      fov: 42,
      zoom: 0.9,
      enableControls: true,
    });
    this.viewer.controls.enablePan = false;
    this.viewer.controls.minDistance = 20;
    this.viewer.controls.maxDistance = 90;
    this.viewer.controls.minPolarAngle = Math.PI * 0.18;
    this.viewer.controls.maxPolarAngle = Math.PI * 0.82;
    this.viewer.autoRotate = true;
    this.viewer.autoRotateSpeed = 0.35;
    this.viewer.globalLight.intensity = AMBIENT * LAMBERT_1_OVER_PI;
    this.viewer.cameraLight.intensity = KEY * LAMBERT_1_OVER_PI;
    this.viewer.cameraLight.decay = 0;
    this.viewer.cameraLight.position.set(...KEY_DIR).normalize().multiplyScalar(KEY_DISTANCE);
    this.viewer.animation = ANIMATION_FACTORIES.idle();

    this.observer = new ResizeObserver(() => this.resize());
    this.observer.observe(canvas.parentElement);

    this.capFrameRate();
  }

  /**
   * 给 vendor 自己的渲染循环加一道帧率上限。
   *
   * skinview3d 的 SkinViewer 在构造时就挂上 rAF，每个显示帧都跑一遍 RenderPass + FXAA
   * 后处理，而且不看页面可见性 —— 对一张 64×64 的皮肤预览来说，120/144Hz 屏上是成倍白烧，
   * 丢在后台标签页里更是会一直烧下去。
   *
   * 动画与自动旋转在 vendor 里都是按 Δt 推进的（draw() 里的 clock.getDelta()），所以压低
   * 帧率只会更省电，不会让动作变慢。上限取 60：60Hz 屏上等同不节流，只在 120/144Hz 屏上
   * 才咬得住。
   *
   * 哪天 vendor 把 draw 改了名，这层会失效并退回「每个显示帧都画」—— 只是费电，不会坏。
   */
  capFrameRate() {
    const viewer = this.viewer;
    const originalDraw = viewer.draw.bind(viewer);
    const interval = 1000 / MAX_FPS;
    let last = 0;

    viewer.draw = () => {
      if (viewer.renderPaused) return; // 恢复交给 vendor 的 renderPaused setter
      const now = performance.now();
      if (now - last < interval) {
        viewer.animationID = requestAnimationFrame(() => viewer.draw());
        return;
      }
      // 按固定节拍推进，而不是 last = now：显示器帧间隔只要略小于 interval，后者就会
      // 「画一帧跳一帧」——60Hz 屏上直接掉到一半。落后超过一拍（例如刚从后台切回）才重新对齐。
      last += interval;
      if (now - last > interval) last = now;
      originalDraw(); // 原实现末尾会再挂上下一帧
    };

    const syncVisibility = () => {
      last = 0;
      viewer.renderPaused = document.hidden;
    };
    document.addEventListener("visibilitychange", syncVisibility);
    syncVisibility();
  }

  resize() {
    const box = this.canvas.parentElement;
    const width = Math.max(1, Math.round(box.clientWidth));
    const height = Math.max(1, Math.round(box.clientHeight));
    this.viewer.setSize(width, height);
  }

  /** @param {{texture_url: string, kind: string}} skin */
  setSkin(skin) {
    return this.viewer
      .loadSkin(skin.texture_url, { model: MODEL_TYPES[skin.kind] })
      .then(() => this.applyLayers()) // loadSkin 会把 visible 重置为 true，这里补回来
      .catch(() => {
        this.onError(t("皮肤贴图加载失败"));
      });
  }

  setSkinVisible(visible) {
    this.skinVisible = visible;
    this.viewer.playerObject.skin.visible = visible;
  }

  setOuterLayerVisible(visible) {
    this.outerVisible = visible;
    this.viewer.playerObject.skin.setOuterLayerVisible(visible);
  }

  /**
   * 载入背面贴图。披风与鞘翅共用这一张（游戏里鞘翅用的就是披风材质），
   * 到底显示哪一档交给 setBackEquipment —— 所以这里用 makeVisible: false 挡住 vendor 的默认切换。
   *
   * @param {string|null} url
   */
  setCape(url) {
    if (!url) {
      this.viewer.loadCape(null); // resetCape：连同 backEquipment 一起清掉
      return Promise.resolve();
    }
    return this.viewer
      .loadCape(url, { makeVisible: false })
      .then(() => this.setBackEquipment(this.backEquipment))
      .catch(() => {
        this.onError(t("披风贴图加载失败"));
      });
  }

  /** @param {"cape"|"elytra"|null} equipment */
  setBackEquipment(equipment) {
    this.backEquipment = equipment ?? null;
    this.viewer.playerObject.backEquipment = this.backEquipment;
  }

  applyLayers() {
    const skin = this.viewer.playerObject.skin;
    skin.visible = this.skinVisible;
    skin.setOuterLayerVisible(this.outerVisible);
    // loadSkin 只碰皮肤自己，但换装时重新对齐一次背面状态，省得依赖 vendor 的副作用
    this.viewer.playerObject.backEquipment = this.backEquipment;
  }

  setAnimation(key) {
    const factory = ANIMATION_FACTORIES[key];
    if (!factory) return;
    this.animationKey = key;
    const animation = factory();
    animation.paused = this.paused;
    this.viewer.animation = animation;
  }

  setPaused(paused) {
    this.paused = paused;
    if (this.viewer.animation) this.viewer.animation.paused = paused;
  }

  setAutoRotate(enabled) {
    this.viewer.autoRotate = enabled;
  }

  resetPose() {
    this.viewer.resetCameraPose();
  }
}
