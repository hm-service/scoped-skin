/**
 * 主应用（皮肤库 / 上传 / 3D 预览 / 顶栏）的文案表，与后端 i18n/i18n.go 同一套策略：
 *
 * - 键是点号分层的**符号名**（如 library.search-placeholder），不是中文原文；
 * - 每条同时给中英两栏 zh / en，源语言是简体中文；
 * - 模板里写 {{ t("library.search-placeholder") }}；带变量时
 *   t("toast.equipped", { name })，占位符照抄为 {name}；
 * - 「皮肤站」「Minecraft Skin Library」等跨页共用的键只在这里定义一次，
 *   维护页的 t() 直接引用同一个键（运行时两张表合并成一张 catalog）。
 *
 * 漏配 / 多配 / 缺栏由 `make i18n`（tools/check-i18n.mjs）检查。
 */
export const app = {
  "title.library": { zh: "皮肤站 · Minecraft Skin Library", en: "Skin Library" },
  // 骨架与顶栏
  "brand.site": { zh: "皮肤站", en: "Skin Library" },
  "nav.switch-language": { zh: "切换到 {language}", en: "Switch to {language}" },
  // 品牌副标：主页与维护页各一句，都是品牌词（中英同文）
  "brand.sub": { zh: "Minecraft Skin Library", en: "Minecraft Skin Library" },
  "brand.sub-maintain": { zh: "Maintain", en: "Maintain" },
  "nav.accounts": { zh: "用户管理", en: "User management" },
  "nav.signout": { zh: "退出登录", en: "Sign out" },

  // 身份组
  "role.admin": { zh: "管理员", en: "Admin" },
  "role.normal": { zh: "普通", en: "Normal" },
  "role.guest": { zh: "游客", en: "Guest" },

  // 顶栏计数 / 皮肤库骨架
  "library.textures": { zh: "贴图库", en: "Textures" },
  "library.favorites": { zh: "我的收藏", en: "Favorites" },
  "library.all": { zh: "全部", en: "All" },
  "library.upload-skin": { zh: "上传新皮肤", en: "Upload" },
  "library.upload-cape": { zh: "上传新披风", en: "Upload" },
  "library.edit-noun": { zh: "编辑{noun}", en: "Edit {noun}" },
  "library.category": { zh: "类别", en: "Category" },
  "noun.skin": { zh: "皮肤", en: "skin" },
  "noun.cape": { zh: "披风", en: "cape" },
  "library.skins-all": { zh: "全部皮肤", en: "All skins" },
  "library.skins-favorites": { zh: "收藏的皮肤", en: "Favorite skins" },
  "library.capes-all": { zh: "全部披风", en: "All capes" },
  "library.capes-favorites": { zh: "收藏的披风", en: "Favorite capes" },
  "library.search-placeholder": { zh: "搜索标签 · 空格分隔取交集", en: "Search tags · space-separated AND" },
  "library.tags-all": { zh: "全部标签", en: "All tags" },
  "library.tags-hint": { zh: "点击标签加入搜索", en: "Click a tag to add it to the search" },
  "library.tags-empty": { zh: "还没有任何标签", en: "No tags yet" },
  "library.clear": { zh: "清除", en: "Clear" },
  "common.loading": { zh: "载入中…", en: "Loading…" },
  "library.stats": { zh: "皮肤 {skins} · 披风 {capes} · 收藏 {favorites}", en: "Skins {skins} · Capes {capes} · Favorites {favorites}" },
  "library.filtered": { zh: "筛出 {count} 个", en: "{count} filtered" },
  "common.count": { zh: "{count} 个", en: "{count} total" },

  // 空状态
  "empty.no-match": { zh: "没有同时带上", en: "No {noun} matching " },
  "empty.tag": { zh: "「{tag}」", en: "“{tag}”" },
  "empty.no-match-tail": { zh: "的{noun}。", en: "." },
  "empty.no-favorites": { zh: "还没有收藏{noun}。", en: "No favorite {noun} yet" },
  "empty.favorite-hint": { zh: "点卡片右上角的星标收藏。", en: "Click the star on a card to favorite" },
  "empty.library": { zh: "{noun}库是空的", en: "The {noun} library is empty" },
  "empty.upload-hint": { zh: "，去「上传新皮肤」传一张。", en: ". Upload one" },
  "common.period": { zh: "。", en: "." },

  // 卡片
  "action.favorite": { zh: "收藏", en: "Favorite" },
  "action.unfavorite": { zh: "取消收藏", en: "Unfavorite" },
  "action.delete": { zh: "删除", en: "Delete" },
  "action.edit": { zh: "编辑", en: "Edit" },
  "card.load-failed": { zh: "无法加载", en: "Cannot load" },
  "card.builtin": { zh: "内置", en: "Built-in" },

  // 上传面板
  "upload.tags": { zh: "标签", en: "Tags" },
  "upload.tags-empty": { zh: "暂无标签，直接输入创建", en: "No tags yet, type to create" },
  "upload.model": { zh: "模型", en: "Model" },
  "upload.model-steve": { zh: "Steve（经典）", en: "Steve (Classic)" },
  "upload.model-alex": { zh: "Alex（纤细）", en: "Alex (Slim)" },
  "model.classic": { zh: "经典", en: "Classic" },
  "model.slim": { zh: "纤细", en: "Slim" },
  "upload.drop": { zh: "点击或拖入 PNG", en: "Click or drop a PNG" },
  "upload.size-hint": { zh: "64×64 皮肤 / 64×32 披风", en: "64×64 skin / 64×32 cape" },
  "upload.checking": { zh: "{size} · 正在校验…", en: "{size} · Checking…" },
  "upload.never": { zh: "{head} · 还没有上传过", en: "{head} · Not uploaded yet" },
  "upload.builtin-readonly": { zh: "{head} · 内置资源不可修改", en: "{head} · Built-in, read-only" },
  "common.none": { zh: "无", en: "None" },
  "upload.existing": { zh: "{head} · 已上传过 · 标签 {tags} · 模型 {model}", en: "{head} · Uploaded · Tags {tags} · Model {model}" },
  "upload.submit": { zh: "上传", en: "Upload" },
  "upload.file-unusable": { zh: "文件不可用", en: "File not usable" },
  "upload.validating": { zh: "校验中…", en: "Checking…" },
  "upload.submitting": { zh: "提交中…", en: "Submitting…" },
  "upload.builtin": { zh: "内置资源", en: "Built-in" },
  "upload.cannot-edit": { zh: "内置资源不可修改", en: "Built-in, read-only" },
  "upload.save": { zh: "保存修改", en: "Save changes" },
  "upload.uploaded": { zh: "已上传", en: "Uploaded" },
  "upload.pick-png": { zh: "先选一个 PNG 文件", en: "Pick a PNG first" },
  "upload.png-only": { zh: "只支持 PNG 文件", en: "PNG files only" },
  "upload.read-failed": { zh: "贴图读取失败", en: "Texture load failed" },
  "upload.check-failed": { zh: "校验失败", en: "Checking failed" },
  "upload.tags-updated": { zh: "已更新标签 / 模型", en: "Tags / model updated" },
  "upload.success": { zh: "上传成功", en: "Uploaded" },
  "upload.forbidden": { zh: "当前身份组不能上传贴图", en: "This role cannot upload textures" },
  "upload.forbidden-edit": { zh: "当前身份组不能修改资源", en: "This role cannot modify textures" },
  "upload.detected": { zh: "按尺寸识别为{noun}，已切换", en: "Detected a {noun} by size, switched" },
  "upload.delete-admin-only": { zh: "只有管理员能删除资源", en: "Only admins can delete" },

  // 标签输入
  "tag.remove": { zh: "移除", en: "Remove" },
  "tag.add-more": { zh: "继续添加…", en: "Add more…" },
  "tag.placeholder": { zh: "输入标签后回车，或从下拉选择", en: "Type a tag and press Enter, or pick below" },
  "tag.no-match": { zh: "没有匹配，回车新建", en: "No match, press Enter to create" },
  "tag.create": { zh: "新建标签 {tag}", en: "Create tag {tag}" },
  "tag.create-quoted": { zh: "新建「{tag}」", en: "Create “{tag}”" },
  "tag.max": { zh: "标签最多 {count} 个", en: "At most {count} tags" },
  "tag.too-long": { zh: "单个标签最长 {count} 个字符", en: "Each tag is at most {count} characters" },
  "tag.usage": { zh: "用过 {count} 次", en: "used {count} times" },

  // 3D 预览
  "preview.title": { zh: "3D 预览", en: "3D preview" },
  "preview.pending": { zh: "待上传", en: "Pending upload" },
  "preview.auto-rotate": { zh: "自动旋转", en: "Auto-rotate" },
  "preview.reset": { zh: "重置视角", en: "Reset view" },
  "preview.hint": { zh: "拖拽旋转 · 滚轮缩放 · 中键平移 · 中键双击重置", en: "Drag to rotate · scroll to zoom · middle-drag to pan · double-middle-click to reset" },
  "asset.delete-in-use": { zh: "有 {count} 个账号正在使用：{names}。删除后这些账号会取消装备，确定删除？", en: "In use by {count} accounts: {names}. They will be unequipped. Delete anyway?" },
  "asset.deleted-unequipped": { zh: "已删除（{count} 个账号已取消装备）", en: "Deleted · {count} accounts unequipped" },
  "preview.pick-skin": { zh: "选一张皮肤开始预览", en: "Pick a skin to preview" },
  "preview.toggle-skin": { zh: "显示 / 隐藏皮肤", en: "Show / hide skin" },
  "preview.toggle-overlay": { zh: "显示 / 隐藏外层", en: "Show / hide overlay" },
  "preview.back-equipment": { zh: "背面装备", en: "Back equipment" },
  "preview.elytra": { zh: "鞘翅", en: "Elytra" },
  "preview.hide": { zh: "隐藏", en: "Hide" },
  "preview.pause-toggle": { zh: "暂停 / 继续", en: "Pause / resume" },
  "preview.resume": { zh: "继续", en: "Resume" },
  "preview.pause": { zh: "暂停", en: "Pause" },
  "noun.overlay": { zh: "外层", en: "overlay" },
  "pose.idle": { zh: "站立", en: "Idle" },
  "pose.walk": { zh: "行走", en: "Walk" },
  "pose.run": { zh: "奔跑", en: "Run" },
  "pose.fly": { zh: "飞行", en: "Fly" },
  "preview.no-tags": { zh: "未添加标签", en: "No tags" },
  "preview.no-skin": { zh: "未选择皮肤", en: "No skin selected" },

  // 提示与错误兜底
  "toast.equipped": { zh: "已穿上「{name}」", en: "Now wearing “{name}”" },
  "toast.favorited": { zh: "已加入收藏", en: "Added to favorites" },
  "toast.unfavorited": { zh: "已取消收藏", en: "Removed from favorites" },
  "confirm.delete": { zh: "删除这个{noun}？", en: "Delete this {noun}?" },
  "toast.deleted": { zh: "已删除", en: "Deleted" },
  "toast.no-cape": { zh: "还没选披风，去「披风」挑一张", en: "No cape selected, pick one in “cape”" },
  "toast.session-expired": { zh: "登录状态已失效", en: "Session expired" },
  "error.request": { zh: "请求失败 ({status})", en: "Request failed ({status})" },
  "error.image-load": { zh: "图片加载失败：{url}", en: "Image load failed: {url}" },
  "error.skin-texture": { zh: "皮肤贴图加载失败", en: "Skin texture load failed" },
  "error.cape-texture": { zh: "披风贴图加载失败", en: "Cape texture load failed" },
};
