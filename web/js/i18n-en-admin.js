/**
 * 登录页与维护页（用户管理）的英文文案。
 *
 * 键 = 源码里 `t("…")` 括号中的中文原文，必须一字不差；`{name}` 之类的占位符照抄。
 * 漏翻或写错键由 `make i18n`（tools/check-i18n.mjs）检查。
 *
 * 「皮肤站」「Minecraft Skin Library」两个品牌词两本字典都有（运行时按
 * app → admin 的顺序合并，取值一致），这样本文件的键与本地文件里的 t() 一一对应。
 */
export const admin = {
  // 页面标题（维护页；登录页那条已经由 auth-page.js 用上）
  "用户管理 · 皮肤站": "Accounts · Skin Library",
  // ── 品牌词 ──────────────────────────────────────────────────────
  "皮肤站": "Skin Library",
  "Minecraft Skin Library": "Minecraft Skin Library",

  // ── 登录页 ──────────────────────────────────────────────────────
  "登录 · 皮肤站": "Sign in · Skin Library",
  "账号": "Account",
  "密码": "Password",
  "登录": "Sign in",
  "登录中…": "Signing in…",
  "账号和密码不能为空": "Account and password required",
  "登录失败 ({status})": "Sign in failed ({status})",

  // ── 维护页：顶栏 ────────────────────────────────────────────────
  "回皮肤站": "Back to library",
  "退出登录": "Sign out",
  "{count} 个账号": "{count} accounts",

  // ── 维护页：无权访问 ────────────────────────────────────────────
  "需要管理员身份": "Admin role required",
  "当前身份组没有用户管理权限。": "This role cannot manage users",

  // ── 维护页：新建用户 ────────────────────────────────────────────
  "新建用户": "New account",
  "游戏内名称": "Player name",
  "大小写字母 / 数字 / 下划线，3-16 位": "Letters / digits / underscores, 3–16 chars",
  "游戏内名称只能是大写字母、小写字母、数字或下划线，长度 3-16": "Player name takes letters, digits and underscore, 3–16 characters",
  "UUID 必须是 32 位十六进制字符": "UUID must be 32 hex characters",
  "32 位十六进制，可带横线；建好后也能改": "32 hex characters, dashes allowed; editable later",
  "小写字母 / 数字 / @ / . / _": "Lowercase letters / digits / @ / . / _",
  "账号名最长 32 个字符": "Account name is at most 32 characters",
  "账号名只能是小写字母、数字、@、. 和 _": "Account name takes lowercase letters, digits, @, . and _",
  "8-64 位，大小写字母 / 数字 / 下划线": "8–64 chars: letters / digits / underscore",
  "留空不改；8-64 位大小写字母 / 数字 / 下划线": "Empty keeps it · 8–64 letters / digits / underscore",
  "32 位小写十六进制，留空则自动推导": "32 lowercase hex characters; empty derives it",
  "密码长度 {min}-{max} 位": "Password must be {min}–{max} characters",
  "密码只能是大写字母、小写字母、数字或下划线": "Password takes letters, digits and underscore",
  "身份组": "Role",
  "游客": "Guest",
  "普通": "Normal",
  "管理员": "Admin",
  "创建用户": "Create account",
  "创建中…": "Creating…",

  // ── 维护页：账号列表 ────────────────────────────────────────────
  "账号、游戏内名称、密码不能为空": "Account, player name and password required",
  "账号和游戏内名称都不能为空": "Account and player name required",
  "已创建「{name}」": "Created “{name}”",
  "载入中…": "Loading…",
  "{count} 个": "{count} total",
  "操作": "Actions",
  "我": "me",
  "编辑": "Edit",
  "删除": "Delete",
  "暂无账号": "No accounts",

  // ── 维护页：编辑账号 ────────────────────────────────────────────
  "编辑用户": "Edit account",
  "关闭": "Close",
  "32 位小写十六进制，可带横线": "32 lowercase hex characters, dashes allowed",
  "新密码": "New password",
  "保存": "Save",
  "保存中…": "Saving…",
  "取消": "Cancel",

  // ── 维护页：增删改结果 ──────────────────────────────────────────
  "已保存": "Saved",
  "已退出管理员身份": "Admin role removed",
  "删除用户「{name}」？不可撤销。": "Delete user “{name}”? Cannot be undone",
  "已删除「{name}」": "Deleted “{name}”",
};
