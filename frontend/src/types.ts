/**
 * 接口 JSON 的形状：与 api/site_*.go 的响应、db/models.go 的模型一一对应。
 *
 * 只放跨模块共用的类型；模块自己内部的形状（比如上传表单的草稿）写在模块里。
 */

/** 身份组：DESIGN.md 5.1 的权限矩阵。 */
export type Role = "guest" | "normal" | "admin";

/** 库里那一列 kind（模型已并进皮肤这一档）。 */
export type Kind = "skin.steve" | "skin.alex" | "cape";

/** 由 kind 推出的粗分类：前端分类与 Yggdrasil 的 SKIN / CAPE 分支都用它。 */
export type Category = "skin" | "cape";

/** 上传表单能选的模型；披风没有这一档。 */
export type Model = "steve" | "alex";

/** 左栏的列表档位。 */
export type Tab = "all" | "favorites" | "upload";

/** 3D 预览背面显示哪一档；null 表示隐藏。 */
export type BackEquipment = "cape" | "elytra" | null;

export interface Player {
  uuid: string;
  role: Role;
  player_name: string;
  username: string;
  skin: string;
  cape: string;
}

/** 一条贴图资源（DESIGN.md 3.6）。 */
export interface Item {
  id: string;
  kind: Kind;
  category: Category;
  tags: string[];
  upload_at: number;
  readonly: boolean;
  width: number;
  height: number;
  size: number;
  favorite: boolean;
  texture_url: string;
}

export interface TagCount {
  tag: string;
  count: number;
}

export interface Stats {
  skins: number;
  capes: number;
  favorites: number;
}

/** 「谁在穿这张贴图」的一条记录（删资源前给管理员看的）。 */
export interface AssetUser {
  uuid: string;
  player_name: string;
  slot: Category;
}

/* ── 各接口的响应体 ─────────────────────────────────────────────── */

export interface LoginResponse {
  token: string;
  player: Player;
}

export interface MeResponse {
  player: Player;
  skin: Item | null;
  cape: Item | null;
}

export interface TagsResponse {
  tags: TagCount[];
}

export interface LibraryResponse {
  items: Item[];
  stats: Stats;
}

export interface PrecheckResponse {
  asset_id: string;
  exists: boolean;
  item: Item | null;
}

export interface UploadResponse extends Item {
  created: boolean;
  updated: boolean;
}

export interface FavoriteResponse extends Item {
  stats: Stats;
}

export interface DeleteItemResponse {
  deleted: string;
  stats: Stats;
}

export interface AssetUsersResponse {
  users: AssetUser[];
}

export interface UsersResponse {
  users: Player[];
  /** 发起这次请求的账号 uuid；写操作里没有它。 */
  me?: string;
  /** 新建 / 编辑时回传的那一条。 */
  user?: Player;
}

export interface ErrorResponse {
  detail?: string;
}
