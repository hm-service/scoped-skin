/**
 * 2D 预览：直接从皮肤 / 披风贴图裁出正面视图，画到 canvas 上。
 * 坐标与 skinview3d 的模型一致（64×64 现代布局为基准，HD 贴图按倍数缩放）。
 */
import { t } from "./i18n.js";

/** 每个部件的贴图区域 [x, y, w, h]（以 64×64 布局为单位）。 */
const UV = {
  head: [8, 8, 8, 8],
  hat: [40, 8, 8, 8],
  body: [20, 20, 8, 12],
  jacket: [20, 36, 8, 12],
  rightLeg: [4, 20, 4, 12],
  rightLegOverlay: [4, 36, 4, 12],
  leftLeg: [20, 52, 4, 12],
  leftLegOverlay: [4, 52, 4, 12],
  rightArm: [44, 20, 4, 12],
  rightArmOverlay: [44, 36, 4, 12],
  leftArm: [36, 52, 4, 12],
  leftArmOverlay: [52, 52, 4, 12],
};

/** 单位尺寸（皮肤坐标格）→ canvas 像素 */
const SCALE = 8;

/** 正面视图的绘制顺序：腿 → 身体 → 手臂 → 头，外层（overlay）紧随其后。 */
function bodyParts(model) {
  const slim = model === "alex";
  const armWidth = slim ? 3 : 4;
  const rightArmX = slim ? 1 : 0;
  const leftArmX = 12; // 经典与纤细模型的左臂都靠右贴边（纤细时只是窄 1 格）
  return [
    [UV.rightLeg, 4, 20],
    [UV.leftLeg, 8, 20],
    [UV.rightLegOverlay, 4, 20],
    [UV.leftLegOverlay, 8, 20],
    [UV.body, 4, 8],
    [UV.jacket, 4, 8],
    [[...UV.rightArm.slice(0, 2), armWidth, 12], rightArmX, 8],
    [[...UV.leftArm.slice(0, 2), armWidth, 12], leftArmX, 8],
    [[...UV.rightArmOverlay.slice(0, 2), armWidth, 12], rightArmX, 8],
    [[...UV.leftArmOverlay.slice(0, 2), armWidth, 12], leftArmX, 8],
    [UV.head, 4, 0],
    [UV.hat, 4, 0],
  ];
}

/** 加载一张图片，返回 HTMLImageElement。 */
export function loadImage(url) {
  return new Promise((resolve, reject) => {
    const image = new Image();
    image.crossOrigin = "anonymous";
    image.onload = () => resolve(image);
    image.onerror = () => reject(new Error(t("图片加载失败：{url}", { url })));
    image.src = url;
  });
}

/** 把皮肤贴图渲染成 16×32 的正面小人。 */
function drawSkinAvatar(canvas, image, model = "steve") {
  const factor = image.naturalWidth / 64;
  canvas.width = 16 * SCALE;
  canvas.height = 32 * SCALE;
  const ctx = canvas.getContext("2d");
  ctx.imageSmoothingEnabled = false;
  ctx.clearRect(0, 0, canvas.width, canvas.height);

  for (const [rect, dx, dy] of bodyParts(model)) {
    const [x, y, w, h] = rect;
    if ((y + h) * factor > image.naturalHeight + 0.5) {
      continue; // 64×32 老格式没有下半张图，跳过缺失部件
    }
    ctx.drawImage(
      image,
      x * factor,
      y * factor,
      w * factor,
      h * factor,
      dx * SCALE,
      dy * SCALE,
      w * SCALE,
      h * SCALE,
    );
  }
}

/** 把披风贴图渲染成 10×16 的正面披风（与 skinview3d 的 UV 一致）。 */
function drawCapeAvatar(canvas, image) {
  const factor = image.naturalWidth / 64;
  canvas.width = 10 * SCALE;
  canvas.height = 16 * SCALE;
  const ctx = canvas.getContext("2d");
  ctx.imageSmoothingEnabled = false;
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  ctx.drawImage(
    image,
    0,
    0,
    10 * factor,
    16 * factor,
    0,
    0,
    canvas.width,
    canvas.height,
  );
}

/**
 * 模型探测，算法与 blessing-skin-server 一致：
 * 左臂贴图块最右侧 2 列（x 46..47）整条（y 52..63）都是纯黑/空，就是纤细（alex）。
 * 传入的贴图必须是正方形（非正方形一律按经典处理）。
 *
 * @returns {"alex"|"steve"}
 */
export function detectModel(image) {
  if (image.naturalWidth !== image.naturalHeight) return "steve";

  const probe = document.createElement("canvas");
  probe.width = image.naturalWidth;
  probe.height = image.naturalHeight;
  const ctx = probe.getContext("2d", { willReadFrequently: true });
  ctx.drawImage(image, 0, 0);

  const ratio = image.naturalWidth / 64;
  const x = Math.floor(46 * ratio);
  const y = Math.floor(52 * ratio);
  const width = Math.ceil(2 * ratio);
  const height = Math.ceil(12 * ratio);
  const data = ctx.getImageData(x, y, width, height).data;
  for (let i = 0; i < data.length; i += 4) {
    if (data[i] || data[i + 1] || data[i + 2]) return "steve";
  }
  return "alex";
}

/** 卡片 / 上传预览统一入口。 */
export function renderPreview(canvas, image, kind, model) {
  if (kind === "cape") {
    drawCapeAvatar(canvas, image);
  } else {
    drawSkinAvatar(canvas, image, model);
  }
}
