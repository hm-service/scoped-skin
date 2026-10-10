/**
 * 身份组：与 DESIGN.md 5.1 的权限矩阵一一对应。
 *
 *   guest  —— 看 + 换皮肤
 *   normal —— 上面这些 + 上传贴图
 *   admin  —— 上面这些 + 管理用户、删除资源
 *
 * 这里只管界面显不显示；真正的闸门在后端（`Depends(current_admin)` /
 * `Depends(current_uploader)`），前端拦不住也不该拦。
 */
import { t } from "./i18n.js";

const LABELS = { admin: t("管理员"), normal: t("普通"), guest: t("游客") };

export const roleLabel = (role) => LABELS[role] ?? role;

/** 上传贴图：admin / normal。 */
export const canUpload = (role) => role === "admin" || role === "normal";

/** 删除资源：仅 admin。 */
export const canDelete = (role) => role === "admin";
