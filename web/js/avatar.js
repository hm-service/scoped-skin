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

const PNG_MAGIC = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];

/**
 * 读 PNG 头 24 字节拿到尺寸（与后端 core.HeaderSize 同一套读法）。
 * 不是 PNG 就回 null —— 拖拽进来的文件不一定带对的 MIME，认字节比认 type 可靠。
 *
 * @returns {Promise<{width: number, height: number}|null>}
 */
export async function pngSize(file) {
  let header;
  try {
    header = new Uint8Array(await file.slice(0, 24).arrayBuffer());
  } catch {
    return null; // 读不出来（目录、权限、文件已被移走）就当它不是 PNG
  }
  if (header.length < 24 || PNG_MAGIC.some((byte, index) => header[index] !== byte)) return null;
  if (String.fromCharCode(...header.subarray(12, 16)) !== "IHDR") return null;
  const view = new DataView(header.buffer);
  return { width: view.getUint32(16), height: view.getUint32(20) };
}

/**
 * 尺寸 → 粗分类：宽是高的两倍就是 64×32 那张披风比例（HD 倍数也认），其余按皮肤走。
 * 不合规的尺寸不在这里拦，交给后端预检报错 —— 尺寸规则只写一份。
 */
export function categoryOfSize({ width, height }) {
  return width === height * 2 ? "cape" : "skin";
}

/** 正面视图的绘制顺序：腿 → 身体 → 手臂 → 头，外层（overlay）紧随其后。 */
function bodyParts(kind) {
  const slim = kind === "skin.alex";
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

export function loadImage(url) {
  return new Promise((resolve, reject) => {
    const image = new Image();
    image.crossOrigin = "anonymous";
    image.onload = () => resolve(image);
    image.onerror = () => reject(new Error(t("error.image-load", { url })));
    image.src = url;
  });
}

function drawSkinAvatar(canvas, image, kind) {
  const factor = image.naturalWidth / 64;
  canvas.width = 16 * SCALE;
  canvas.height = 32 * SCALE;
  const ctx = canvas.getContext("2d");
  ctx.imageSmoothingEnabled = false;
  ctx.clearRect(0, 0, canvas.width, canvas.height);

  for (const [rect, dx, dy] of bodyParts(kind)) {
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

/** 卡片 / 上传预览统一入口：kind 就是接口里的 skin.steve / skin.alex / cape。 */
export function renderPreview(canvas, image, kind) {
  if (kind === "cape") {
    drawCapeAvatar(canvas, image);
  } else {
    drawSkinAvatar(canvas, image, kind);
  }
}
