/**
 * 身份组：与 DESIGN.md 5.1 的权限矩阵一一对应。
 *
 *   guest  —— 只读：浏览 / 预览
 *   normal —— 上面这些 + 收藏、换装、上传贴图
 *   admin  —— 上面这些 + 管理用户、删除资源
 *
 * 这里只管界面显不显示；真正的闸门在后端（api/site_auth.go 的 currentWriter /
 * currentAdmin），前端拦不住也不该拦。
 */
import { t } from "./i18n.js";

const LABELS = { admin: t("role.admin"), normal: t("role.normal"), guest: t("role.guest") };

/** 身份组名单：新建 / 编辑账号的下拉共用这一份，顺序就是界面上看到的顺序。 */
export const ROLES = ["guest", "normal", "admin"];

export const roleLabel = (role) => LABELS[role] ?? role;

// 上传与「收藏 / 换装」在 DESIGN.md 5.1 里是两格，规则今天一致；留两个名字是为了将来能各改各的。
export const canUpload = (role) => role === "admin" || role === "normal";

export const canWrite = (role) => role === "admin" || role === "normal";

export const canDelete = (role) => role === "admin";
