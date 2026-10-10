/**
 * 唯一数据源：页面状态 + 全部动作。
 *
 * 组件只读 `store`（`reactive` 包装，模板里直接 `store.xxx`）、只调动作，
 * 视图完全由状态推导 —— 不再各自 `getElementById` 手工改 DOM。
 */
import { api } from "./api.js";
import { getToken, getUser, setToken, toLogin } from "./auth.js";
import { detectModel, loadImage } from "./avatar.js";
import { t } from "./i18n.js";
import * as roles from "./roles.js";
import { hasTag, shortId } from "./tags.js";
import { createToast } from "./toast.js";
import { computed, nextTick, reactive, ref } from "./vue.js";

/** 条目 kind → 展示用的模型名（模型已并进 kind，皮肤才有这一档）。 */
const MODEL_LABELS = { "skin.steve": "steve", "skin.alex": "alex" };

/** 上传表单的粗分类 + 模型 → 接口要的 kind（没有「自动」这一档，模型由前端显式选定）。 */
const kindOf = (category, model) => (category === "cape" ? "cape" : `skin.${model}`);

export const ANIMATIONS = [
  { key: "idle", label: t("站立") },
  { key: "walk", label: t("行走") },
  { key: "run", label: t("奔跑") },
  { key: "fly", label: t("飞行") },
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
  if (item.kind === "cape") return t("披风");
  return MODEL_LABELS[item.kind];
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
const MAX_TAGS = 16;
const MAX_TAG_LENGTH = 64;

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
    capeVisible: false, // 没挑披风之前就不显示，按钮状态与画面保持一致
    equipped: { skin: null, cape: null }, // /api/me 回传的当前装备
    library: [], // 已有标签：[{ tag, count }]
    selectedTags: [], // 上传表单选中的标签
    model: "steve", // 上传表单的模型：steve | alex（披风没有模型）
    fileOver: false, // 拖拽悬停
    pending: { file: null, name: "", size: 0, url: null, id: null, registered: null, checking: false, submitting: false, error: "" },
    view: { animation: "idle", paused: false, rotate: true, skinVisible: true, outerVisible: true },
    toast: { message: "", kind: "info", visible: false },
  });

  let searchTimer = null;
  let refreshToken = 0; // refresh() 的序号：并发请求只认最后发出的那次响应
  let checkToken = 0; // 预检序号：换了文件 / 类别，晚回来的旧预检不许回写
  let imageToken = 0; // 待上传贴图的解码序号：换了文件，旧的解码结果作废

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

  const stageCapeUrl = computed(() => {
    if (pendingActive.value && state.category === "cape") return state.pending.url;
    return state.capeVisible && state.cape ? state.cape.texture_url : null;
  });

  const stage = computed(() => {
    if (pendingActive.value) {
      return {
        live: true,
        model: state.category === "cape" ? t("披风") : state.model,
        tags: [...state.selectedTags],
        fallback: t("未添加标签"),
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
    return { live: false, model: "", tags: [], fallback: t("未选择皮肤") };
  });

  const stageEmpty = computed(() => !stageSkin.value);

  const statsText = computed(() => {
    const stats = state.stats;
    if (!stats) return t("载入中…");
    return t("皮肤 {skins} · 披风 {capes} · 收藏 {favorites}", {
      skins: stats.skins,
      capes: stats.capes,
      favorites: stats.favorites,
    });
  });

  const paneTitle = computed(() => {
    if (uploading.value) return state.category === "skin" ? t("上传新皮肤") : t("上传新披风");
    const titles = {
      skin: { all: t("全部皮肤"), favorites: t("收藏的皮肤") },
      cape: { all: t("全部披风"), favorites: t("收藏的披风") },
    };
    return titles[state.category][state.tab];
  });

  const paneCount = computed(() => {
    if (uploading.value) return "";
    return state.query.trim()
      ? t("筛出 {count} 个", { count: state.items.length })
      : t("{count} 个", { count: state.items.length });
  });

  const emptyKind = computed(() => {
    if (state.query.trim()) return "search";
    return state.tab === "favorites" ? "favorites" : "none";
  });

  const emptyNoun = computed(() => (state.category === "skin" ? t("皮肤") : t("披风")));
  const emptyTokens = computed(() => state.query.trim().split(/\s+/).filter(Boolean));

  /** 上传表单的按钮文案与可用性（已存在时只有改动过才能提交）。 */
  const submitState = computed(() => {
    const pending = state.pending;
    if (!pending.file) return { label: t("上传"), disabled: true };
    if (pending.error) return { label: t("文件不可用"), disabled: true };
    if (pending.checking) return { label: t("校验中…"), disabled: true };
    if (pending.submitting) return { label: t("提交中…"), disabled: true };
    const existing = pending.registered;
    if (!existing) return { label: t("上传"), disabled: false };
    if (existing.readonly) return { label: t("内置资源"), disabled: true };
    // 同一份内容（id 相同）改标签或改模型都算有变化
    const changed = !sameTags(state.selectedTags, existing.tags) || pendingKind.value !== existing.kind;
    return changed ? { label: t("保存修改"), disabled: false } : { label: t("已上传"), disabled: true };
  });

  const dropText = computed(() => {
    const pending = state.pending;
    if (!pending.file) {
      return { title: t("点击或拖入 PNG"), detail: t("64×64 皮肤 / 64×32 披风") };
    }
    if (pending.error) return { title: pending.name, detail: pending.error };
    const size = `${(pending.size / 1024).toFixed(1)} KB`;
    const head = pending.id
      ? `${size} · sha256 ${pending.id.slice(0, 12)}…`
      : t("{size} · 正在校验…", { size });
    const existing = pending.registered;
    if (!existing) return { title: pending.name, detail: t("{head} · 还没有上传过", { head }) };
    if (existing.readonly) return { title: pending.name, detail: t("{head} · 内置资源不可修改", { head }) };
    const tags = existing.tags.length ? existing.tags.join(" / ") : t("无");
    return {
      title: pending.name,
      detail: t("{head} · 已上传过 · 标签 {tags} · 模型 {model}", {
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
    if (me.cape) {
      state.cape = me.cape;
      state.capeVisible = true;
    }
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
    if (item.category === "skin") {
      state.skin = item;
    } else {
      state.cape = item;
      state.capeVisible = true;
      // guest 是只读身份组：只能预览，不用提示「已穿上」
      if (canWrite.value) {
        toast(t("已穿上「{name}」", { name: item.tags?.length ? item.tags.join(" · ") : shortId(item) }));
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
      toast(item.favorite ? t("已加入收藏") : t("已取消收藏"));
    } catch (error) {
      toastError(error.message);
    }
  }

  async function remove(item) {
    if (!canDelete.value) {
      toastError(t("只有管理员能删除资源"));
      return;
    }
    if (!confirm(t("删除这个{noun}？", { noun: item.category === "skin" ? t("皮肤") : t("披风") }))) return;
    try {
      const payload = await api(`/api/items/${item.id}`, { method: "DELETE" });
      forgetThumbnail(item.id);
      if (state.skin?.id === item.id) state.skin = null;
      if (state.cape?.id === item.id) {
        state.cape = null;
        state.capeVisible = false;
      }
      if (payload.stats) state.stats = payload.stats;
      await refresh();
      await loadTags();
      toast(t("已删除"));
    } catch (error) {
      toastError(error.message);
    }
  }

  /* ── 列表切换 / 搜索 ─────────────────────────────────────────── */

  async function setTab(tab) {
    if (tab === "upload" && !canUpload.value) {
      toastError(t("当前身份组不能上传贴图"));
      return;
    }
    const leavingUpload = state.tab === "upload" && tab !== "upload";
    state.tab = tab;
    if (leavingUpload) clearPendingFile();
    await refresh();
  }

  async function setCategory(category) {
    state.category = category;
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
    // 与后端一致的上限（DESIGN.md 4.2）：最多 16 个、单条最长 64 字符
    if ([...clean].length > MAX_TAG_LENGTH) {
      toast(t("单个标签最长 {count} 个字符", { count: MAX_TAG_LENGTH }));
      return;
    }
    if (state.selectedTags.length >= MAX_TAGS) {
      toast(t("标签最多 {count} 个", { count: MAX_TAGS }));
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
        pending.error = error.message || t("校验失败");
      });
  }

  function clearPendingFile() {
    // 在途的预检与解码一律作废，免得它们回写到刚清空的表单上
    checkToken += 1;
    imageToken += 1;
    const pending = state.pending;
    if (pending.url) URL.revokeObjectURL(pending.url);
    pendingImage.value = null;
    Object.assign(pending, {
      file: null, name: "", size: 0, url: null, id: null,
      registered: null, checking: false, submitting: false, error: "",
    });
    state.selectedTags = [];
    state.model = "steve";
  }

  function setPendingFile(file) {
    if (!file) return;
    if (file.type !== "image/png") {
      toastError(t("只支持 PNG 文件"));
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
        if (state.category === "skin") state.model = detectModel(image);
      })
      .catch(() => {
        if (token !== imageToken) return;
        toastError(t("贴图读取失败"));
      });
  }

  async function submitUpload() {
    const pending = state.pending;
    if (!pending.file) {
      toastError(t("先选一个 PNG 文件"));
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
      toast(item.updated ? t("已更新标签 / 模型") : t("上传成功"));
      clearPendingFile();
      await loadTags();
      thumbnails.set(item.id, loadImage(item.texture_url));
      await setTab("all");
      state.category = category;
      await refresh();
      select(item);
      await nextTick();
      document.querySelector(`#grid .card[data-id="${item.id}"]`)?.scrollIntoView({ block: "nearest" });
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
  const toggleCapeVisible = () => {
    state.capeVisible = !state.capeVisible;
    if (state.capeVisible && !state.cape && !pendingActive.value) {
      toast(t("还没选披风，去「披风」挑一张"));
    }
  };

  // 退出登录就是「清 token 回登录页」，与 api.js 的 401 处理共用同一份实现
  const logout = toLogin;

  return reactive({
    state,
    pendingImage,
    // 身份组
    role,
    canUpload,
    canWrite,
    canDelete,
    roleLabel,
    // 派生
    uploading,
    pendingActive,
    pendingKind,
    stageSkin,
    stageCapeUrl,
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
    // 动作
    toast,
    toastError,
    loadTags,
    loadMe,
    refresh,
    select,
    equip,
    toggleFavorite,
    remove,
    setTab,
    setCategory,
    setQuery,
    submitSearch,
    clearQuery,
    setPendingFile,
    clearPendingFile,
    addTag,
    removeTag,
    toggleTag,
    submitUpload,
    setAnimation,
    togglePaused,
    toggleRotate,
    toggleSkinVisible,
    toggleOuterVisible,
    toggleCapeVisible,
    logout,
    // 小工具
    hasTag: (tag) => hasTag(state.selectedTags, tag),
    modelLabel,
    shortId,
  });
}

export const store = createStore();
