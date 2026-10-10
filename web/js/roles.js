/**
 * 身份组：与 DESIGN.md 5.1 的权限矩阵一一对应。
 *
 *   guest  —— 只读：浏览 / 预览
 *   normal —— 上面这些 + 收藏、换装、上传贴图
 *   admin  —— 上面这些 + 管理用户、删除资源
 *
 * 这里只管界面显不显示；真正的闸门在后端（`Depends(current_writer)` /
 * `Depends(current_admin)`），前端拦不住也不该拦。
 */
import { t } from "./i18n.js";

const LABELS = { admin: t("管理员"), normal: t("普通"), guest: t("游客") };

export const roleLabel = (role) => LABELS[role] ?? role;

export const canUpload = (role) => role === "admin" || role === "normal";

export const canWrite = (role) => role === "admin" || role === "normal";

export const canDelete = (role) => role === "admin";
