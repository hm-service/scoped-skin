/**
 * 唯一数据源：页面状态 + 全部动作。
 *
 * 组件只读 `store`（`reactive` 包装，模板里直接 `store.xxx`）、只调动作，
 * 视图完全由状态推导 —— 不再各自 `getElementById` 手工改 DOM。
 */
import { api } from "./api.js";
import { getToken, getUser, setToken, toLogin } from "./auth.js";
import { categoryOfSize, detectModel, loadImage, pngSize } from "./avatar.js";
import { t } from "./i18n.js";
import * as roles from "./roles.js";
import { hasTag, shortId } from "./tags.js";
import { createToast } from "./toast.js";
import { computed, reactive, ref } from "./vue.js";

/** 条目 kind → 接口里的模型值（模型已并进 kind，皮肤才有这一档）。 */
const MODEL_LABELS = { "skin.steve": "steve", "skin.alex": "alex" };

/** 模型值的展示名（下拉选项与各处徽标共用一份）；值仍是接口要的 steve / alex。 */
const MODEL_TEXTS = { steve: t("upload.model-steve"), alex: t("upload.model-alex") };
export const MODEL_OPTIONS = Object.entries(MODEL_TEXTS).map(([value, label]) => ({ value, label }));

/** 卡片那种一格宽的窄位置只放大类的「经典 / 纤细」，全名留给下拉与舞台徽标。 */
const MODEL_SHORTS = { "skin.steve": t("model.classic"), "skin.alex": t("model.slim") };

/** 上传表单的粗分类 + 模型 → 接口要的 kind（没有「自动」这一档，模型由前端显式选定）。 */
const kindOf = (category, model) => (category === "cape" ? "cape" : `skin.${model}`);

export const ANIMATIONS = [
  { key: "idle", label: t("pose.idle") },
  { key: "walk", label: t("pose.walk") },
  { key: "run", label: t("pose.run") },
  { key: "fly", label: t("pose.fly") },
];

/** 3D 预览实例的落脚点（skinview3d 的对象不放进响应式状态，避免被代理）。 */
export const viewerRef = { current: null };

/** item.id -> Promise<HTMLImageElement>：卡片缩略图共用一份贴图缓存。 */
const thumbnails = new Map();
const pendingImage = ref(null); // 上传表单里那张待上传的图（留给 2D 预览画）

export function thumbnailFor(item) {
  if (!thumbnails.has(item.id)) thumbnails.set(item.id, loadImage(item.texture_url));
  return thumbnails.get(item.id);
}

function forgetThumbnail(id) {
  thumbnails.delete(id);
}

export function modelLabel(item) {
  if (item.kind === "cape") return t("noun.cape");
  return MODEL_TEXTS[MODEL_LABELS[item.kind]] ?? "";
}

/** 卡片一格的窄位置用：经典 / 纤细 / 披风。 */
export function modelShort(item) {
  if (item.kind === "cape") return t("noun.cape");
  return MODEL_SHORTS[item.kind] ?? "";
}

/** 分块转 base64：一次 apply 整个数组会爆调用栈，64×64 的图也照样分块。 */
function toBase64(bytes) {
  let text = "";
  for (let offset = 0; offset < bytes.length; offset += 0x8000) {
    text += String.fromCharCode(...bytes.subarray(offset, offset + 0x8000));
  }
  return btoa(text);
}

/**
 * 选中文件后读一次，转成 base64 交给后端。
 *
 * 前端不再自己算 sha256：入库的是重新编码过的字节，asset_id 只有服务端算得出来
 * （预检会把它回给我们，界面上显示的就是这个 ID）。
 */
async function readFile(file) {
  return { base64: toBase64(new Uint8Array(await file.arrayBuffer())) };
}

function sameTags(a, b) {
  const norm = (list) => [...list].map((tag) => tag.toLowerCase()).sort().join("|");
  return norm(a) === norm(b);
}

/** 标签上限，与 core.MaxTagsPerAsset / core.MaxTagLength 保持一致（DESIGN.md 4.2）。 */
const MAX_TAGS = 8;
const MAX_TAG_LENGTH = 32;

function createStore() {
  const state = reactive({
    tab: "all", // all | favorites | upload
    category: "skin", // skin | cape（粗分类；条目上的 kind 才是 skin.steve / skin.alex / cape）
    query: "", // 搜索串：按空白切成标签取交集
    items: [],
    stats: null,
    player: getUser(),
    skin: null, // 当前预览的皮肤条目
    cape: null, // 当前预览的披风条目
    backEquipment: "cape", // 背面显示哪一档：cape | elytra | null（隐藏）。默认「有披风就显示」
    equipped: { skin: null, cape: null }, // /api/me 回传的当前装备
    library: [], // 已有标签：[{ tag, count }]
    selectedTags: [], // 上传表单选中的标签
    model: "steve", // 上传表单的模型：steve | alex（披风没有模型）
    fileOver: false, // 拖拽悬停
    uploadOver: false, // 拖到「上传新皮肤」按钮上
    pending: { file: null, name: "", size: 0, url: null, id: null, registered: null, checking: false, submitting: false, editing: false, error: "" },
    hover: { item: null, anchor: null }, // 卡片悬浮面板：显示放不下的那些标签
    reveal: null, // 「把这张卡片滚进视野」的请求：{ id, nonce }（列表虚拟化后卡片不一定在 DOM 里，见 app.js）
    view: { animation: "idle", paused: false, rotate: true, skinVisible: true, outerVisible: true },
    toast: { message: "", kind: "info", visible: false },
  });

  let searchTimer = null;
  let refreshToken = 0; // refresh() 的序号：并发请求只认最后发出的那次响应
  let checkToken = 0; // 预检序号：换了文件 / 类别，晚回来的旧预检不许回写
  let imageToken = 0; // 待上传贴图的解码序号：换了文件，旧的解码结果作废
  let fileToken = 0; // 读 PNG 头的序号：换了文件，晚回来的尺寸结果不许回写

  /* ── 提示 ────────────────────────────────────────────────────── */

  const { toast, toastError } = createToast(state);

  /* ── 派生状态 ────────────────────────────────────────────────── */

  const uploading = computed(() => state.tab === "upload");
  const pendingActive = computed(() => uploading.value && Boolean(state.pending.url));

  /** 身份组派生出来的界面权限（DESIGN.md 5.1，真正的闸门在后端）。 */
  const role = computed(() => state.player?.role ?? "");
  const canUpload = computed(() => roles.canUpload(role.value));
  const canWrite = computed(() => roles.canWrite(role.value)); // 收藏 / 装备：guest 只读
  const canDelete = computed(() => roles.canDelete(role.value));
  const roleLabel = computed(() => roles.roleLabel(role.value));

  const pendingKind = computed(() => kindOf(state.category, state.model));

  /** 右侧 3D 正在预览待上传的那张图（而不是库里选中的那张）。 */
  const stageSkin = computed(() => {
    if (pendingActive.value && state.category === "skin") {
      return { texture_url: state.pending.url, kind: pendingKind.value };
    }
    return state.skin ? { texture_url: state.skin.texture_url, kind: state.skin.kind } : null;
  });

  /** 背面材质的贴图（披风与鞘翅共用同一张）：挑过就带上，显示哪一档由 backEquipment 定。 */
  const stageCapeUrl = computed(() => {
    if (pendingActive.value && state.category === "cape") return state.pending.url;
    return state.cape ? state.cape.texture_url : null;
  });

  /** 背面显示：披风 / 鞘翅 / 隐藏（null）；一张贴图都没有时只能隐藏。 */
  const stageBackEquipment = computed(() => (stageCapeUrl.value ? state.backEquipment : null));

  const stage = computed(() => {
    if (pendingActive.value) {
      return {
        live: true,
        model: state.category === "cape" ? t("noun.cape") : MODEL_TEXTS[state.model] ?? state.model,
        tags: [...state.selectedTags],
        fallback: t("preview.no-tags"),
      };
    }
    if (state.skin) {
      return {
        live: false,
        model: modelLabel(state.skin),
        tags: [...(state.skin.tags ?? [])],
        fallback: shortId(state.skin),
      };
    }
    return { live: false, model: "", tags: [], fallback: t("preview.no-skin") };
  });

  const stageEmpty = computed(() => !stageSkin.value);

  const statsText = computed(() => {
    const stats = state.stats;
    if (!stats) return t("common.loading");
    return t("library.stats", {
      skins: stats.skins,
      capes: stats.capes,
      favorites: stats.favorites,
    });
  });

  const paneTitle = computed(() => {
    if (uploading.value) {
      if (state.pending.editing) {
        return t("library.edit-noun", { noun: state.category === "skin" ? t("noun.skin") : t("noun.cape") });
      }
      return state.category === "skin" ? t("library.upload-skin") : t("library.upload-cape");
    }
    const titles = {
      skin: { all: t("library.skins-all"), favorites: t("library.skins-favorites") },
      cape: { all: t("library.capes-all"), favorites: t("library.capes-favorites") },
    };
    return titles[state.category][state.tab];
  });

  const paneCount = computed(() => {
    if (uploading.value) return "";
    return state.query.trim()
      ? t("library.filtered", { count: state.items.length })
      : t("common.count", { count: state.items.length });
  });

  const emptyKind = computed(() => {
    if (state.query.trim()) return "search";
    return state.tab === "favorites" ? "favorites" : "none";
  });

  const emptyNoun = computed(() => (state.category === "skin" ? t("noun.skin") : t("noun.cape")));

  /**
   * 搜索串切出来的词（与后端 core.SplitQuery 一致：按空白切、大小写不敏感去重）。
   * 空状态提示按它逐词加粗，所以对外就叫 emptyTokens。
   */
  const emptyTokens = computed(() => {
    const seen = new Set();
    const tokens = [];
    for (const token of state.query.split(/\s+/)) {
      const key = token.toLowerCase();
      if (!key || seen.has(key)) continue;
      seen.add(key);
      tokens.push(token);
    }
    return tokens;
  });

  /** 上传表单的按钮文案与可用性（已存在时只有改动过才能提交）。 */
  const submitState = computed(() => {
    const pending = state.pending;
    if (!pending.file) return { label: t("upload.submit"), disabled: true };
    if (pending.error) return { label: t("upload.file-unusable"), disabled: true };
    if (pending.checking) return { label: t("upload.validating"), disabled: true };
    if (pending.submitting) return { label: t("upload.submitting"), disabled: true };
    const existing = pending.registered;
    if (!existing) return { label: t("upload.submit"), disabled: false };
    if (existing.readonly) return { label: t("upload.builtin"), disabled: true };
    // 同一份内容（id 相同）改标签或改模型都算有变化
    const changed = !sameTags(state.selectedTags, existing.tags) || pendingKind.value !== existing.kind;
    return changed ? { label: t("upload.save"), disabled: false } : { label: t("upload.uploaded"), disabled: true };
  });

  /** 编辑一条已入库的记录时才谈得上删除：只读资源不能删，删除本身也只给 admin。 */
  const canDeletePending = computed(() => {
    const item = state.pending.registered;
    return Boolean(canDelete.value && item && !item.readonly);
  });

  const dropText = computed(() => {
    const pending = state.pending;
    if (!pending.file) {
      return { title: t("upload.drop"), detail: t("upload.size-hint") };
    }
    if (pending.error) return { title: pending.name, detail: pending.error };
    const size = `${(pending.size / 1024).toFixed(1)} KB`;
    const head = pending.id
      ? `${size} · sha256 ${pending.id.slice(0, 12)}…`
      : t("upload.checking", { size });
    const existing = pending.registered;
    if (!existing) return { title: pending.name, detail: t("upload.never", { head }) };
    if (existing.readonly) return { title: pending.name, detail: t("upload.builtin-readonly", { head }) };
    const tags = existing.tags.length ? existing.tags.join(" / ") : t("common.none");
    return {
      title: pending.name,
      detail: t("upload.existing", {
        head,
        tags,
        model: modelLabel(existing),
      }),
    };
  });

  /* ── 读取 ────────────────────────────────────────────────────── */

  async function loadTags() {
    const payload = await api("/api/tags");
    state.library = payload.tags;
  }

  async function loadMe() {
    const me = await api("/api/me");
    state.player = me.player;
    state.equipped = { skin: me.skin, cape: me.cape };
    setToken(getToken(), me.player); // 顺手把用户信息写回 localStorage
    if (me.cape) state.cape = me.cape;
  }

  async function refresh() {
    if (uploading.value) {
      state.items = [];
      return;
    }
    const query = state.query.trim();
    const search = query ? `&q=${encodeURIComponent(query)}` : "";
    const token = ++refreshToken;
    const payload = await api(`/api/library?category=${state.category}&tab=${state.tab}${search}`);
    if (token !== refreshToken) return; // 期间又刷新过一次：以那次为准
    state.items = payload.items;
    state.stats = payload.stats;

    // 首次进入：优先恢复 /api/me 里装备的皮肤，否则选中第一个
    if (!state.skin && state.category === "skin" && state.items.length) {
      const pick = state.items.find((item) => item.id === state.equipped.skin?.id) ?? state.items[0];
      select(pick, { persist: false });
    }
    if (!state.cape && state.category === "cape" && state.items.length) {
      state.cape = state.items[0];
    }
  }

  /* ── 选择 / 装备 / 收藏 / 删除 ───────────────────────────────── */

  async function equip(item) {
    try {
      const me = await api("/api/me/equip", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ asset_id: item.id }),
      });
      state.equipped = { skin: me.skin, cape: me.cape };
    } catch (error) {
      toastError(error.message);
    }
  }

  function select(item, { persist = true } = {}) {
    // 不移除悬浮面板：鼠标还停在卡片上，选中不该把详情收走（滚列表 / 换页时才收）
    if (item.category === "skin") {
      state.skin = item;
    } else {
      state.cape = item;
      // 换披风只换贴图，不动「披风 / 鞘翅 / 隐藏」这一档：那是用户按出来的显示选择，
      // 挑一张新披风就把它冲掉会很难用（鞘翅用的也是这张贴图，切完贴图它还得在）。
      // guest 是只读身份组：只能预览，不用提示「已穿上」
      if (canWrite.value) {
        toast(t("toast.equipped", { name: item.tags?.length ? item.tags.join(" · ") : shortId(item) }));
      }
    }
    if (persist && canWrite.value) equip(item);
  }

  async function toggleFavorite(item) {
    if (!canWrite.value) return; // guest 只读：星标本来就不给它显示
    try {
      const updated = await api(`/api/items/${item.id}/favorite`, { method: "POST" });
      item.favorite = updated.favorite;
      if (updated.stats) state.stats = updated.stats;
      if (state.tab === "favorites") await refresh();
      toast(item.favorite ? t("toast.favorited") : t("toast.unfavorited"));
    } catch (error) {
      toastError(error.message);
    }
  }

  /** @returns {Promise<boolean>} 真的删掉了才是 true（取消确认 / 报错都是 false）。 */
  async function remove(item) {
    if (!canDelete.value) {
      toastError(t("upload.delete-admin-only"));
      return false;
    }
    // 先问后端「谁在穿它」：有人用就把名字列出来（删除会让他们变成未装备，得让管理员知道）
    let wearers = [];
    try {
      wearers = (await api(`/api/items/${item.id}/users`)).users ?? [];
    } catch {
      // 读不到名单不该挡删除：退回普通确认
    }
    const noun = item.category === "skin" ? t("noun.skin") : t("noun.cape");
    const prompt = wearers.length
      ? t("asset.delete-in-use", {
          count: wearers.length,
          names: wearers.map((user) => user.player_name).join("、"),
        })
      : t("confirm.delete", { noun });
    if (!confirm(prompt)) return false;
    try {
      const payload = await api(`/api/items/${item.id}`, { method: "DELETE" });
      forgetThumbnail(item.id);
      if (state.skin?.id === item.id) state.skin = null;
      if (state.cape?.id === item.id) {
        state.cape = null;
        state.backEquipment = null;
      }
      if (payload.stats) state.stats = payload.stats;
      await refresh();
      await loadTags();
      toast(wearers.length ? t("asset.deleted-unequipped", { count: wearers.length }) : t("toast.deleted"));
      return true;
    } catch (error) {
      toastError(error.message);
      return false;
    }
  }

  /* ── 卡片悬浮面板 ────────────────────────────────────────────── */

  /** 记下锚点矩形而不是元素本身：面板固定定位，不跟着列表滚动。 */
  function hoverCard(item, event) {
    const rect = event.currentTarget.getBoundingClientRect();
    state.hover.item = item;
    state.hover.anchor = { left: rect.left, top: rect.top, bottom: rect.bottom, width: rect.width };
  }

  /** 只清自己那张：从 A 滑到 B 时，leave(A) 可能晚于 enter(B)。 */
  function unhoverCard(item) {
    if (state.hover.item === item) hideHover();
  }

  function hideHover() {
    state.hover.item = null;
    state.hover.anchor = null;
  }

  /* ── 列表切换 / 搜索 ─────────────────────────────────────────── */

  async function setTab(tab) {
    if (tab === "upload" && !canUpload.value) {
      toastError(t("upload.forbidden"));
      return;
    }
    hideHover();
    const leavingUpload = state.tab === "upload" && tab !== "upload";
    state.tab = tab;
    if (leavingUpload) clearPendingFile();
    await refresh();
  }

  async function setCategory(category) {
    state.category = category;
    hideHover();
    // 皮肤与披风的尺寸要求不同，换类别就把待上传的那张按新类别重验一遍
    recheckPending();
    await refresh();
  }

  function setQuery(query) {
    state.query = query;
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => refresh().catch((error) => toastError(error.message)), 220);
  }

  function submitSearch() {
    clearTimeout(searchTimer);
    refresh().catch((error) => toastError(error.message));
  }

  function clearQuery() {
    state.query = "";
    submitSearch();
  }

  /** 这个标签是不是已经在搜索串里（标签面板据此高亮）。 */
  function queryHasTag(tag) {
    return hasTag(emptyTokens.value, tag);
  }

  /** 标签面板点一下＝把标签加进搜索串（再点一下移出去），然后立刻搜。 */
  function toggleQueryTag(tag) {
    const needle = tag.toLowerCase();
    const kept = emptyTokens.value.filter((token) => token.toLowerCase() !== needle);
    if (kept.length === emptyTokens.value.length) kept.push(tag);
    state.query = kept.join(" ");
    submitSearch();
  }

  /* ── 上传 ────────────────────────────────────────────────────── */

  // 预检把原图（base64）交给后端：尺寸、能不能解码、asset_id 都由它自己算
  async function lookupRegistered(data) {
    const payload = await api("/api/precheck", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ kind: pendingKind.value, data }),
    });
    return { id: payload.asset_id, item: payload.exists ? payload.item : null };
  }

  function addTag(tag) {
    const clean = tag.trim();
    if (!clean || hasTag(state.selectedTags, clean)) return;
    // 与后端一致的上限（DESIGN.md 4.2）：最多 8 个、单条最长 32 字符
    if ([...clean].length > MAX_TAG_LENGTH) {
      toast(t("tag.too-long", { count: MAX_TAG_LENGTH }));
      return;
    }
    if (state.selectedTags.length >= MAX_TAGS) {
      toast(t("tag.max", { count: MAX_TAGS }));
      return;
    }
    state.selectedTags.push(clean);
  }

  function removeTag(tag) {
    state.selectedTags = state.selectedTags.filter((item) => item !== tag);
  }

  function toggleTag(tag) {
    if (hasTag(state.selectedTags, tag)) removeTag(tag);
    else addTag(tag);
  }

  function recheckPending() {
    const pending = state.pending;
    if (!pending.file) return;
    const token = ++checkToken;
    pending.checking = true;
    pending.error = "";
    readFile(pending.file)
      .then(async ({ base64 }) => {
        const { id, item } = await lookupRegistered(base64);
        if (token !== checkToken) return; // 期间换了文件 / 类别：旧预检结果丢掉
        pending.checking = false;
        pending.error = "";
        pending.id = id;
        pending.registered = item;
        if (item) {
          state.selectedTags = [...item.tags];
          // 已注册过的这张带着库里的 kind，把模型回填到下拉
          if (state.category === "skin" && item.category === "skin") state.model = MODEL_LABELS[item.kind];
        }
      })
      .catch((error) => {
        if (token !== checkToken) return;
        pending.checking = false;
        pending.registered = null;
        pending.error = error.message || t("upload.check-failed");
      });
  }

  function clearPendingFile() {
    // 在途的预检、解码与读 PNG 头一律作废，免得它们回写到刚清空的表单上
    checkToken += 1;
    imageToken += 1;
    fileToken += 1;
    const pending = state.pending;
    if (pending.url) URL.revokeObjectURL(pending.url);
    pendingImage.value = null;
    Object.assign(pending, {
      file: null, name: "", size: 0, url: null, id: null,
      registered: null, checking: false, submitting: false, editing: false, error: "",
    });
    state.selectedTags = [];
    state.model = "steve";
  }

  /**
   * 选中一个文件。
   *
   * `model` 是编辑入口用的：那条记录在库里已经存了 steve / alex，重新解码探测不该把它改掉，
   * 所以这里允许显式指定；普通上传仍按图像自动判断（用户也可以手动改下拉）。
   */
  async function setPendingFile(file, { model = "" } = {}) {
    if (!file) return;
    const readToken = ++fileToken;
    // 认 PNG 头而不是 file.type：拖进来的文件不一定带对的 MIME
    const size = await pngSize(file);
    if (readToken !== fileToken) return; // 期间又换了文件
    if (!size) {
      toastError(t("upload.png-only"));
      return;
    }
    clearPendingFile();
    const pending = state.pending;
    pending.file = file;
    pending.name = file.name;
    pending.size = file.size;
    pending.url = URL.createObjectURL(file);

    // 选中文件就立刻交给后端验一遍并按结果更新界面（下面是 2D 预览与模型判断）
    recheckPending();
    const token = imageToken;

    loadImage(pending.url)
      .then((image) => {
        if (token !== imageToken) return; // 又选了别的文件：这次解码结果不再是当前那张
        pendingImage.value = image;
        // 选中文件就直接判断模型并切换下拉（用户仍可手动改）
        if (state.category === "skin") state.model = model || detectModel(image);
      })
      .catch(() => {
        if (token !== imageToken) return;
        toastError(t("upload.read-failed"));
      });
  }

  /**
   * 编辑入口：把库里那条记录搬进上传面板 —— 拉回贴图字节当作「刚选中的文件」，
   * 预检按内容命中同一条 asset，于是提交走的就是「覆盖标签与 kind」那条路。
   */
  async function editItem(item) {
    if (!canWrite.value) {
      toastError(t("upload.forbidden-edit"));
      return;
    }
    if (item.readonly) {
      toastError(t("upload.cannot-edit"));
      return;
    }
    let file;
    try {
      const response = await fetch(item.texture_url);
      if (!response.ok) throw new Error(t("error.request", { status: response.status }));
      const blob = await response.blob();
      file = new File([blob], `${shortId(item).slice(1)}.png`, { type: "image/png" });
    } catch (error) {
      toastError(error.message || t("upload.read-failed"));
      return;
    }
    if (!uploading.value) await setTab("upload");
    state.category = item.category;
    await setPendingFile(file, { model: MODEL_LABELS[item.kind] ?? "" });
    state.pending.editing = true;
  }

  /** 编辑面板里的删除：删掉正在编辑的那条，然后回列表。 */
  async function removePending() {
    const item = state.pending.registered;
    if (!item) return;
    if (!(await remove(item))) return;
    clearPendingFile();
    await setTab("all");
  }

  /**
   * 文件直接拖到「上传新皮肤」按钮上：先按 PNG 尺寸挑皮肤 / 披风再进面板。
   * 与面板里那个拖拽区的唯一区别就是这一步 —— 那边类别由用户在左栏自己选。
   */
  async function dropOnUploadTab(file) {
    state.uploadOver = false;
    if (!file || !canUpload.value) return;
    const size = await pngSize(file);
    if (!size) {
      toastError(t("upload.png-only"));
      return;
    }
    const category = categoryOfSize(size);
    if (state.category !== category) {
      state.category = category;
      toast(t("upload.detected", { noun: category === "skin" ? t("noun.skin") : t("noun.cape") }));
    }
    if (!uploading.value) await setTab("upload");
    setPendingFile(file);
  }

  async function submitUpload() {
    const pending = state.pending;
    if (!pending.file) {
      toastError(t("upload.pick-png"));
      return;
    }
    if (submitState.value.disabled) return;
    const category = state.category;
    const kind = pendingKind.value; // clearPendingFile() 会把模型重置，先取下来
    const body = new FormData();
    body.append("file", pending.file);
    body.append("kind", kind);
    for (const tag of state.selectedTags) body.append("tags", tag);

    pending.submitting = true;
    try {
      const item = await api("/api/skins", { method: "POST", body });
      toast(item.updated ? t("upload.tags-updated") : t("upload.success"));
      clearPendingFile();
      await loadTags();
      thumbnails.set(item.id, loadImage(item.texture_url));
      await setTab("all");
      state.category = category;
      await refresh();
      select(item);
      // 全量渲染时这里是 scrollIntoView，现在是虚拟列表：卡片不一定在 DOM 里，
      // 交给视图层按「第几行」换算滚动位置
      state.reveal = { id: item.id, nonce: (state.reveal?.nonce ?? 0) + 1 };
    } catch (error) {
      toastError(error.message);
    } finally {
      pending.submitting = false;
    }
  }

  /* ── 右侧 3D 面板上的开关 ────────────────────────────────────── */

  const view = state.view;
  const setAnimation = (key) => (view.animation = key);
  const togglePaused = () => (view.paused = !view.paused);
  const toggleRotate = () => (view.rotate = !view.rotate);
  const toggleSkinVisible = () => (view.skinVisible = !view.skinVisible);
  const toggleOuterVisible = () => (view.outerVisible = !view.outerVisible);

  /** 背面三档：披风 / 鞘翅 / 隐藏（null）；鞘翅在游戏里用的就是披风那张贴图。 */
  function setBackEquipment(equipment) {
    state.backEquipment = equipment;
    if (equipment && !stageCapeUrl.value) {
      toast(t("toast.no-cape"));
    }
  }

  // 退出登录就是「清 token 回登录页」，与 api.js 的 401 处理共用同一份实现
  const logout = toLogin;

  return reactive({
    state,
    pendingImage,
    // 身份组
    role,
    canUpload,
    canWrite,
    roleLabel,
    // 派生
    uploading,
    pendingKind,
    stageSkin,
    stageCapeUrl,
    stageBackEquipment,
    stage,
    stageEmpty,
    statsText,
    paneTitle,
    paneCount,
    emptyKind,
    emptyNoun,
    emptyTokens,
    submitState,
    dropText,
    canDeletePending,
    // 动作
    toast,
    toastError,
    loadTags,
    loadMe,
    refresh,
    select,
    toggleFavorite,
    remove,
    setTab,
    setCategory,
    setQuery,
    submitSearch,
    clearQuery,
    queryHasTag,
    toggleQueryTag,
    setPendingFile,
    dropOnUploadTab,
    editItem,
    removePending,
    hoverCard,
    unhoverCard,
    hideHover,
    addTag,
    removeTag,
    toggleTag,
    submitUpload,
    setAnimation,
    togglePaused,
    toggleRotate,
    toggleSkinVisible,
    toggleOuterVisible,
    setBackEquipment,
    logout,
    // 小工具
    hasTag: (tag) => hasTag(state.selectedTags, tag),
    modelLabel,
  });
}

export const store = createStore();
