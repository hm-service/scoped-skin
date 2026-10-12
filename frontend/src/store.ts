/**
 * 唯一数据源：页面状态 + 全部动作。
 *
 * 组件只读 `store`（`reactive` 包装，模板里直接 `store.xxx`）、只调动作，
 * 视图完全由状态推导 —— 不再各自 `getElementById` 手工改 DOM。
 */
import { computed, reactive, ref } from "vue";

import { api } from "@/api";
import { getToken, getUser, setToken, toLogin } from "@/auth";
import { categoryOfSize, detectModel, loadImage, pngSize } from "@/avatar";
import { t } from "@/i18n";
import * as roles from "@/roles";
import { hasTag, shortId } from "@/tags";
import { createToast, type ToastKind } from "@/toast";
import type {
  AssetUsersResponse,
  BackEquipment,
  Category,
  DeleteItemResponse,
  FavoriteResponse,
  Item,
  Kind,
  LibraryResponse,
  MeResponse,
  Model,
  Player,
  PrecheckResponse,
  Stats,
  Tab,
  TagCount,
  TagsResponse,
  UploadResponse,
} from "@/types";
import type { Preview3D } from "@/viewer";

/** 条目 kind → 接口里的模型值（模型已并进 kind，披风没有这一档）。 */
const MODEL_LABELS: Partial<Record<Kind, Model>> = { "skin.steve": "steve", "skin.alex": "alex" };

/** 模型值的展示名（下拉选项与各处徽标共用一份）；值仍是接口要的 steve / alex。 */
const MODEL_TEXTS: Record<Model, string> = { steve: t("upload.model-steve"), alex: t("upload.model-alex") };
export const MODEL_OPTIONS = Object.entries(MODEL_TEXTS).map(([value, label]) => ({
  value: value as Model,
  label,
}));

/** 卡片那种一格宽的窄位置只放大类的「经典 / 纤细」，全名留给下拉与舞台徽标。 */
const MODEL_SHORTS: Record<string, string> = { "skin.steve": t("model.classic"), "skin.alex": t("model.slim") };

/** 上传表单的粗分类 + 模型 → 接口要的 kind（没有「自动」这一档，模型由前端显式选定）。 */
const kindOf = (category: Category, model: Model): Kind => (category === "cape" ? "cape" : `skin.${model}`);

export const ANIMATIONS = [
  { key: "idle", label: t("pose.idle") },
  { key: "walk", label: t("pose.walk") },
  { key: "run", label: t("pose.run") },
  { key: "fly", label: t("pose.fly") },
];

/** 3D 预览实例的落脚点（skinview3d 的对象不放进响应式状态，避免被代理）。 */
export const viewerRef: { current: Preview3D | null } = { current: null };

/** 待上传的那张图（2D 预览用）。 */
type PendingImage = HTMLImageElement | null;

/** item.id -> Promise<HTMLImageElement>：卡片缩略图共用一份贴图缓存。 */
const thumbnails = new Map<string, Promise<HTMLImageElement>>();
const pendingImage = ref<PendingImage>(null); // 上传表单里那张待上传的图（留给 2D 预览画）

export function thumbnailFor(item: Item): Promise<HTMLImageElement> {
  if (!thumbnails.has(item.id)) thumbnails.set(item.id, loadImage(item.texture_url));
  return thumbnails.get(item.id) as Promise<HTMLImageElement>;
}

function forgetThumbnail(id: string): void {
  thumbnails.delete(id);
}

export function modelLabel(item: Item): string {
  if (item.kind === "cape") return t("noun.cape");
  const model = MODEL_LABELS[item.kind];
  return model ? MODEL_TEXTS[model] : "";
}

/** 卡片一格的窄位置用：经典 / 纤细 / 披风。 */
export function modelShort(item: Item): string {
  if (item.kind === "cape") return t("noun.cape");
  return MODEL_SHORTS[item.kind] ?? "";
}

/** 分块转 base64：一次 apply 整个数组会爆调用栈，64×64 的图也照样分块。 */
function toBase64(bytes: Uint8Array): string {
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
async function readFile(file: File): Promise<{ base64: string }> {
  return { base64: toBase64(new Uint8Array(await file.arrayBuffer())) };
}

function sameTags(a: readonly string[], b: readonly string[]): boolean {
  const norm = (list: readonly string[]) => [...list].map((tag) => tag.toLowerCase()).sort().join("|");
  return norm(a) === norm(b);
}

/** 标签上限，与 core.MaxTagsPerAsset / core.MaxTagLength 保持一致（DESIGN.md 4.2）。 */
const MAX_TAGS = 8;
const MAX_TAG_LENGTH = 32;

/** 上传表单里那条待入库的记录。 */
interface Pending {
  file: File | null;
  name: string;
  size: number;
  url: string | null;
  id: string | null;
  registered: Item | null;
  checking: boolean;
  submitting: boolean;
  editing: boolean;
  error: string;
}

/** 卡片悬浮面板的位置：锚点矩形，不跟着列表滚动。 */
interface HoverAnchor {
  left: number;
  top: number;
  bottom: number;
  width: number;
}

interface StoreState {
  tab: Tab;
  category: Category;
  query: string;
  items: Item[];
  stats: Stats | null;
  player: Player | null;
  skin: Item | null;
  cape: Item | null;
  backEquipment: BackEquipment;
  equipped: { skin: Item | null; cape: Item | null };
  library: TagCount[];
  selectedTags: string[];
  model: Model;
  fileOver: boolean;
  uploadOver: boolean;
  pending: Pending;
  hover: { item: Item | null; anchor: HoverAnchor | null };
  reveal: { id: string; nonce: number } | null;
  view: { animation: string; paused: boolean; rotate: boolean; skinVisible: boolean; outerVisible: boolean };
  toast: { message: string; kind: ToastKind; visible: boolean };
}

/** 右侧 3D 舞台正在展示的皮肤（接口要的形状）。 */
export interface StageSkin {
  texture_url: string;
  kind: Kind;
}

function createStore() {
  const state = reactive<StoreState>({
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
    pending: {
      file: null,
      name: "",
      size: 0,
      url: null,
      id: null,
      registered: null,
      checking: false,
      submitting: false,
      editing: false,
      error: "",
    },
    hover: { item: null, anchor: null }, // 卡片悬浮面板：显示放不下的那些标签
    reveal: null, // 「把这张卡片滚进视野」的请求：{ id, nonce }（列表虚拟化后卡片不一定在 DOM 里，见 App.vue）
    view: { animation: "idle", paused: false, rotate: true, skinVisible: true, outerVisible: true },
    toast: { message: "", kind: "info", visible: false },
  });

  let searchTimer: ReturnType<typeof setTimeout> | null = null;
  let refreshToken = 0; // refresh() 的序号：并发请求只认最后发出的那次响应
  let checkToken = 0; // 预检序号：换了文件 / 类别，晚回来的旧预检不许回写
  let imageToken = 0; // 待上传贴图的解码序号：换了文件，旧的解码结果作废
  let fileToken = 0; // 读 PNG 头的序号：换了文件，晚回来的尺寸结果不许回写

  /* ── 提示 ────────────────────────────────────────────────────── */

  const { toast, toastError } = createToast(state);

  /* ── 派生状态 ────────────────────────────────────────────────── */

  const uploading = computed(() => state.tab === "upload");

  /** 身份组派生出来的界面权限（DESIGN.md 5.1，真正的闸门在后端）。 */
  const role = computed(() => state.player?.role ?? "");
  const canUpload = computed(() => roles.canUpload(role.value));
  const canWrite = computed(() => roles.canWrite(role.value)); // 收藏 / 装备：guest 只读
  const canDelete = computed(() => roles.canDelete(role.value));
  const roleLabel = computed(() => roles.roleLabel(role.value));

  const pendingKind = computed(() => kindOf(state.category, state.model));

  /** 待上传那张图的地址：面板里选过文件、而且正停在上传档位时才算数（否则回 null）。 */
  const pendingUrl = computed<string | null>(() => {
    const url = state.pending.url;
    return uploading.value && url ? url : null;
  });

  /**
   * 鼠标停在哪张卡上，舞台上就预览哪张 —— 只读，不动选中、更不装备。
   * 三个优先级从高到低：待上传的那张 > 鼠标停着的那张 > 库里选中的那张。
   */
  const hovered = computed(() => state.hover.item);

  /** 右侧 3D 正在预览的皮肤（可能是待上传的、鼠标停着的，或选中的那张）。 */
  const stageSkin = computed<StageSkin | null>(() => {
    const url = pendingUrl.value;
    if (url && state.category === "skin") {
      return { texture_url: url, kind: pendingKind.value };
    }
    const item = hovered.value;
    if (item && item.category === "skin") {
      return { texture_url: item.texture_url, kind: item.kind };
    }
    return state.skin ? { texture_url: state.skin.texture_url, kind: state.skin.kind } : null;
  });

  /** 背面材质的贴图（披风与鞘翅共用同一张）：挑过就带上，显示哪一档由 backEquipment 定。 */
  const stageCapeUrl = computed<string | null>(() => {
    const url = pendingUrl.value;
    if (url && state.category === "cape") return url;
    const item = hovered.value;
    if (item && item.category === "cape") return item.texture_url;
    return state.cape ? state.cape.texture_url : null;
  });

  /**
   * 背面显示：披风 / 鞘翅 / 隐藏（null）；一张贴图都没有时只能隐藏。
   * 鼠标停在披风卡上时，隐藏档当成披风（不然预览看不见），披风与鞘翅档照用户选的来
   * —— 鞘翅用的也是这张贴图，展示方式不该被预览冲掉。
   */
  const stageBackEquipment = computed<BackEquipment>(() => {
    if (!stageCapeUrl.value) return null;
    if (hovered.value?.category !== "cape") return state.backEquipment;

    let equipment: BackEquipment = state.backEquipment;
    if (equipment === null) equipment = "cape";
    return equipment;
  });

  /** 舞台顶部那排徽标跟着预览走：显示哪张就说哪张的模型与标签。 */
  const stage = computed(() => {
    if (pendingUrl.value) {
      return {
        live: true,
        model: state.category === "cape" ? t("noun.cape") : (MODEL_TEXTS[state.model] ?? state.model),
        tags: [...state.selectedTags],
        fallback: t("preview.no-tags"),
      };
    }
    const shown = hovered.value ?? state.skin;
    if (shown) {
      return {
        live: false,
        model: modelLabel(shown),
        tags: [...(shown.tags ?? [])],
        fallback: shortId(shown),
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
    const titles: Record<Category, { all: string; favorites: string }> = {
      skin: { all: t("library.skins-all"), favorites: t("library.skins-favorites") },
      cape: { all: t("library.capes-all"), favorites: t("library.capes-favorites") },
    };
    return state.tab === "favorites" ? titles[state.category].favorites : titles[state.category].all;
  });

  const paneCount = computed(() => {
    if (uploading.value) return "";
    return state.query.trim()
      ? t("library.filtered", { count: state.items.length })
      : t("common.count", { count: state.items.length });
  });

  const emptyKind = computed<"search" | "favorites" | "none">(() => {
    if (state.query.trim()) return "search";
    return state.tab === "favorites" ? "favorites" : "none";
  });

  const emptyNoun = computed(() => (state.category === "skin" ? t("noun.skin") : t("noun.cape")));

  /**
   * 搜索串切出来的词（与后端 core.SplitQuery 一致：按空白切、大小写不敏感去重）。
   * 空状态提示按它逐词加粗，所以对外就叫 emptyTokens。
   */
  const emptyTokens = computed<string[]>(() => {
    const seen = new Set<string>();
    const tokens: string[] = [];
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

  async function loadTags(): Promise<void> {
    const payload = await api<TagsResponse>("/api/tags");
    state.library = payload.tags;
  }

  async function loadMe(): Promise<void> {
    const me = await api<MeResponse>("/api/me");
    state.player = me.player;
    state.equipped = { skin: me.skin, cape: me.cape };
    setToken(getToken(), me.player); // 顺手把用户信息写回 localStorage
    if (me.cape) state.cape = me.cape;
  }

  async function refresh(): Promise<void> {
    if (uploading.value) {
      state.items = [];
      return;
    }
    const query = state.query.trim();
    const search = query ? `&q=${encodeURIComponent(query)}` : "";
    const token = ++refreshToken;
    const payload = await api<LibraryResponse>(
      `/api/library?category=${state.category}&tab=${state.tab}${search}`,
    );
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

  async function equip(item: Item): Promise<void> {
    try {
      const me = await api<MeResponse>("/api/me/equip", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ asset_id: item.id }),
      });
      state.equipped = { skin: me.skin, cape: me.cape };
    } catch (error) {
      toastError(error instanceof Error ? error.message : String(error));
    }
  }

  function select(item: Item, { persist = true }: { persist?: boolean } = {}): void {
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
    if (persist && canWrite.value) void equip(item);
  }

  async function toggleFavorite(item: Item): Promise<void> {
    if (!canWrite.value) return; // guest 只读：星标本来就不给它显示
    try {
      const updated = await api<FavoriteResponse>(`/api/items/${item.id}/favorite`, { method: "POST" });
      item.favorite = updated.favorite;
      if (updated.stats) state.stats = updated.stats;
      if (state.tab === "favorites") await refresh();
      toast(item.favorite ? t("toast.favorited") : t("toast.unfavorited"));
    } catch (error) {
      toastError(error instanceof Error ? error.message : String(error));
    }
  }

  /** @returns 真的删掉了才是 true（取消确认 / 报错都是 false）。 */
  async function remove(item: Item): Promise<boolean> {
    if (!canDelete.value) {
      toastError(t("upload.delete-admin-only"));
      return false;
    }
    // 先问后端「谁在穿它」：有人用就把名字列出来（删除会让他们变成未装备，得让管理员知道）
    let wearers: AssetUsersResponse["users"] = [];
    try {
      wearers = (await api<AssetUsersResponse>(`/api/items/${item.id}/users`)).users ?? [];
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
      const payload = await api<DeleteItemResponse>(`/api/items/${item.id}`, { method: "DELETE" });
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
      toastError(error instanceof Error ? error.message : String(error));
      return false;
    }
  }

  /* ── 卡片悬浮面板 ────────────────────────────────────────────── */

  /** 记下锚点矩形而不是元素本身：面板固定定位，不跟着列表滚动。 */
  function hoverCard(item: Item, event: MouseEvent): void {
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    state.hover.item = item;
    state.hover.anchor = { left: rect.left, top: rect.top, bottom: rect.bottom, width: rect.width };
  }

  /** 只清自己那张：从 A 滑到 B 时，leave(A) 可能晚于 enter(B)。 */
  function unhoverCard(item: Item): void {
    if (state.hover.item === item) hideHover();
  }

  function hideHover(): void {
    state.hover.item = null;
    state.hover.anchor = null;
  }

  /* ── 列表切换 / 搜索 ─────────────────────────────────────────── */

  async function setTab(tab: StoreState["tab"]): Promise<void> {
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

  async function setCategory(category: Category): Promise<void> {
    state.category = category;
    hideHover();
    // 皮肤与披风的尺寸要求不同，换类别就把待上传的那张按新类别重验一遍
    recheckPending();
    await refresh();
  }

  function setQuery(query: string): void {
    state.query = query;
    clearTimeout(searchTimer ?? undefined);
    searchTimer = setTimeout(() => void refresh().catch((error: unknown) => fail(error)), 220);
  }

  function submitSearch(): void {
    clearTimeout(searchTimer ?? undefined);
    void refresh().catch((error: unknown) => fail(error));
  }

  function clearQuery(): void {
    state.query = "";
    submitSearch();
  }

  /** 这个标签是不是已经在搜索串里（标签面板据此高亮）。 */
  function queryHasTag(tag: string): boolean {
    return hasTag(emptyTokens.value, tag);
  }

  /** 标签面板点一下＝把标签加进搜索串（再点一下移出去），然后立刻搜。 */
  function toggleQueryTag(tag: string): void {
    const needle = tag.toLowerCase();
    const kept = emptyTokens.value.filter((token) => token.toLowerCase() !== needle);
    if (kept.length === emptyTokens.value.length) kept.push(tag);
    state.query = kept.join(" ");
    submitSearch();
  }

  /* ── 上传 ────────────────────────────────────────────────────── */

  // 预检把原图（base64）交给后端：尺寸、能不能解码、asset_id 都由它自己算
  async function lookupRegistered(data: string): Promise<{ id: string; item: Item | null }> {
    const payload = await api<PrecheckResponse>("/api/precheck", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ kind: pendingKind.value, data }),
    });
    return { id: payload.asset_id, item: payload.exists ? payload.item : null };
  }

  function addTag(tag: string): void {
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

  function removeTag(tag: string): void {
    state.selectedTags = state.selectedTags.filter((item) => item !== tag);
  }

  function toggleTag(tag: string): void {
    if (hasTag(state.selectedTags, tag)) removeTag(tag);
    else addTag(tag);
  }

  function recheckPending(): void {
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
          // 皮肤条目的 kind 只有 skin.steve / skin.alex 两档，披风没有模型这一说
          if (state.category === "skin" && item.category === "skin") {
            state.model = item.kind === "skin.alex" ? "alex" : "steve";
          }
        }
      })
      .catch((error: unknown) => {
        if (token !== checkToken) return;
        pending.checking = false;
        pending.registered = null;
        pending.error = (error instanceof Error ? error.message : "") || t("upload.check-failed");
      });
  }

  function clearPendingFile(): void {
    // 在途的预检、解码与读 PNG 头一律作废，免得它们回写到刚清空的表单上
    checkToken += 1;
    imageToken += 1;
    fileToken += 1;
    const pending = state.pending;
    if (pending.url) URL.revokeObjectURL(pending.url);
    pendingImage.value = null;
    Object.assign(pending, {
      file: null,
      name: "",
      size: 0,
      url: null,
      id: null,
      registered: null,
      checking: false,
      submitting: false,
      editing: false,
      error: "",
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
  async function setPendingFile(file: File | undefined, { model }: { model?: Model | "" } = {}): Promise<void> {
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
  async function editItem(item: Item): Promise<void> {
    if (!canWrite.value) {
      toastError(t("upload.forbidden-edit"));
      return;
    }
    if (item.readonly) {
      toastError(t("upload.cannot-edit"));
      return;
    }
    let file: File;
    try {
      const response = await fetch(item.texture_url);
      if (!response.ok) throw new Error(t("error.request", { status: response.status }));
      const blob = await response.blob();
      file = new File([blob], `${shortId(item).slice(1)}.png`, { type: "image/png" });
    } catch (error) {
      toastError((error instanceof Error ? error.message : "") || t("upload.read-failed"));
      return;
    }
    if (!uploading.value) await setTab("upload");
    state.category = item.category;
    await setPendingFile(file, { model: MODEL_LABELS[item.kind] ?? "" });
    state.pending.editing = true;
  }

  /** 编辑面板里的删除：删掉正在编辑的那条，然后回列表。 */
  async function removePending(): Promise<void> {
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
  async function dropOnUploadTab(file: File | undefined): Promise<void> {
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
    await setPendingFile(file);
  }

  async function submitUpload(): Promise<void> {
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
      const item = await api<UploadResponse>("/api/skins", { method: "POST", body });
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
      fail(error);
    } finally {
      pending.submitting = false;
    }
  }

  /* ── 右侧 3D 面板上的开关 ────────────────────────────────────── */

  const view = state.view;
  const setAnimation = (key: string) => (view.animation = key);
  const togglePaused = () => (view.paused = !view.paused);
  const toggleRotate = () => (view.rotate = !view.rotate);
  const toggleSkinVisible = () => (view.skinVisible = !view.skinVisible);
  const toggleOuterVisible = () => (view.outerVisible = !view.outerVisible);

  /** 背面三档：披风 / 鞘翅 / 隐藏（null）；鞘翅在游戏里用的就是披风那张贴图。 */
  function setBackEquipment(equipment: BackEquipment): void {
    state.backEquipment = equipment;
    if (equipment && !stageCapeUrl.value) {
      toast(t("toast.no-cape"));
    }
  }

  // 退出登录就是「清 token 回登录页」，与 api.ts 的 401 处理共用同一份实现
  const logout = toLogin;

  /** 把 unknown 的异常统一成一句提示文案。 */
  function fail(error: unknown): void {
    toastError(error instanceof Error ? error.message : String(error));
  }

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
    hasTag: (tag: string) => hasTag(state.selectedTags, tag),
    modelLabel,
  });
}

export const store = createStore();
