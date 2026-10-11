/**
 * 登录页与维护页（用户管理）的文案表，策略同 web/js/i18n-app.js：符号键 + 中英两栏。
 *
 * 跨页共用的键（品牌词、身份组、common.loading 等）由 i18n-app.js 定义，
 * 本文件不重复定义；本页的 t() 可以直接引用它们（运行时两表合并）。
 *
 * 漏配 / 多配 / 缺栏由 `make i18n`（tools/check-i18n.mjs）检查。
 */
export const admin = {
  "title.accounts": { zh: "用户管理 · 皮肤站", en: "Accounts · Skin Library" },

  // ── 登录页 ──────────────────────────────────────────────────────
  "title.login": { zh: "登录 · 皮肤站", en: "Sign in · Skin Library" },
  "auth.account": { zh: "账号", en: "Account" },
  "auth.password": { zh: "密码", en: "Password" },
  "auth.submit": { zh: "登录", en: "Sign in" },
  "auth.submitting": { zh: "登录中…", en: "Signing in…" },
  "auth.empty": { zh: "账号和密码不能为空", en: "Account and password required" },
  "auth.failed": { zh: "登录失败 ({status})", en: "Sign in failed ({status})" },

  // ── 维护页：顶栏 ────────────────────────────────────────────────
  "nav.back": { zh: "回皮肤站", en: "Back to library" },
  "maintain.count": { zh: "{count} 个账号", en: "{count} accounts" },

  // ── 维护页：无权访问 ────────────────────────────────────────────
  "maintain.admin-required": { zh: "需要管理员身份", en: "Admin role required" },
  "maintain.no-permission": { zh: "当前身份组没有用户管理权限。", en: "This role cannot manage users" },

  // ── 维护页：新建用户 ────────────────────────────────────────────
  "user.create": { zh: "新建用户", en: "New account" },
  "field.player-name": { zh: "游戏内名称", en: "Player name" },
  "user.player-name-hint": { zh: "大小写字母 / 数字 / 下划线，3-16 位", en: "Letters / digits / underscores, 3–16 chars" },
  "valid.player-name": { zh: "游戏内名称只能是大写字母、小写字母、数字或下划线，长度 3-16", en: "Player name takes letters, digits and underscore, 3–16 characters" },
  "valid.uuid": { zh: "UUID 必须是 32 位十六进制字符", en: "UUID must be 32 hex characters" },
  "user.username-hint": { zh: "小写字母 / 数字 / @ / . / _", en: "Lowercase letters / digits / @ / . / _" },
  "valid.username-long": { zh: "账号名最长 32 个字符", en: "Account name is at most 32 characters" },
  "valid.username": { zh: "账号名只能是小写字母、数字、@、. 和 _", en: "Account name takes lowercase letters, digits, @, . and _" },
  "user.password-hint": { zh: "8-64 位，大小写字母 / 数字 / 下划线", en: "8–64 chars: letters / digits / underscore" },
  "user.password-hint-edit": { zh: "留空不改；8-64 位大小写字母 / 数字 / 下划线", en: "Empty keeps it · 8–64 letters / digits / underscore" },
  "user.uuid-hint": { zh: "32 位小写十六进制，横线会被去掉；留空则自动推导", en: "32 lowercase hex, dashes stripped; empty derives it" },
  "valid.password-length": { zh: "密码长度 {min}-{max} 位", en: "Password must be {min}–{max} characters" },
  "valid.password-charset": { zh: "密码只能是大写字母、小写字母、数字或下划线", en: "Password takes letters, digits and underscore" },
  "field.role": { zh: "身份组", en: "Role" },
  "user.create-submit": { zh: "创建用户", en: "Create account" },
  "user.creating": { zh: "创建中…", en: "Creating…" },

  // ── 维护页：账号列表 ────────────────────────────────────────────
  "user.create-empty": { zh: "账号、游戏内名称、密码不能为空", en: "Account, player name and password required" },
  "user.edit-empty": { zh: "账号和游戏内名称都不能为空", en: "Account and player name required" },
  "user.created": { zh: "已创建「{name}」", en: "Created “{name}”" },
  "user.actions": { zh: "操作", en: "Actions" },
  "user.me": { zh: "我", en: "me" },
  "user.empty": { zh: "暂无账号", en: "No accounts" },

  // ── 维护页：编辑账号 ────────────────────────────────────────────
  "user.edit-title": { zh: "编辑用户", en: "Edit account" },
  "common.close": { zh: "关闭", en: "Close" },
  "user.uuid-hint-edit": { zh: "32 位小写十六进制，横线会被去掉", en: "32 lowercase hex characters, dashes stripped" },
  "user.new-password": { zh: "新密码", en: "New password" },
  "user.save": { zh: "保存", en: "Save" },
  "user.saving": { zh: "保存中…", en: "Saving…" },
  "common.cancel": { zh: "取消", en: "Cancel" },

  // ── 维护页：增删改结果 ──────────────────────────────────────────
  "user.saved": { zh: "已保存", en: "Saved" },
  "user.role-removed": { zh: "已退出管理员身份", en: "Admin role removed" },
  "user.delete-confirm": { zh: "删除用户「{name}」？不可撤销。", en: "Delete user “{name}”? Cannot be undone" },
  "user.deleted": { zh: "已删除「{name}」", en: "Deleted “{name}”" },
};
