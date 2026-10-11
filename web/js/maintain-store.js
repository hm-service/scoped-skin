/**
 * 维护页（/maintain）的唯一数据源：用户列表 + 新建 / 编辑 / 删除动作。
 *
 * 闸门在后端：`/api/admin/*` 只认 `role = admin` 的 token，其它一律 403；
 * 前端拿到 403 就把页面切成「无权访问」，不是靠隐藏入口来挡人。
 * 改凭据（uuid / 用户名 / 密码）会让自己的旧 token 立刻失效，这里改完自己就
 * 主动回登录页，不让用户对着一个已经失效的会话操作。
 */
import { api } from "./api.js";
import { getToken, getUser, setToken, toLogin } from "./auth.js";
import { t } from "./i18n.js";
import { roleLabel } from "./roles.js";
import { createToast } from "./toast.js";
import { computed, reactive } from "./vue.js";

const emptyDraft = () => ({ username: "", player_name: "", password: "", role: "normal", uuid: "" });

/** 与 core 的规则保持一致：角色名 ^[A-Za-z0-9_]{3,16}$、UUID 32 位十六进制（可带横线）。
 *  前端先拦一道只是为了不用等一次往返，真正的闸门仍在后端。 */
const PLAYER_NAME_PATTERN = /^[A-Za-z0-9_]{3,16}$/;
const USERNAME_PATTERN = /^[a-z0-9@._]+$/;
const PASSWORD_PATTERN = /^[A-Za-z0-9_]+$/;
const MAX_USERNAME_LENGTH = 32;
const MIN_PASSWORD_LENGTH = 8;
const MAX_PASSWORD_LENGTH = 64;
// UUID：本地故意宽松（带 `i`），大写不在这里拦 —— 失焦时折成小写、发请求前再折一次。
const UUID_PATTERN = /^(?:[0-9a-f]{32}|[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12})$/i;

/** 校验一组账号字段；返回错误文案，通过则返回空串。密码留空表示不改（编辑时）。 */
function validateAccount({ username, player_name, uuid, password }) {
  if ([...username].length > MAX_USERNAME_LENGTH) return t("账号名最长 32 个字符");
  if (!USERNAME_PATTERN.test(username)) return t("账号名只能是小写字母、数字、@、. 和 _");
  if (!PLAYER_NAME_PATTERN.test(player_name)) return t("游戏内名称只能是大写字母、小写字母、数字或下划线，长度 3-16");
  if (uuid && !UUID_PATTERN.test(uuid)) return t("UUID 必须是 32 位十六进制字符");
  if (password) {
    const length = [...password].length;
    if (length < MIN_PASSWORD_LENGTH || length > MAX_PASSWORD_LENGTH) {
      return t("密码长度 {min}-{max} 位", { min: MIN_PASSWORD_LENGTH, max: MAX_PASSWORD_LENGTH });
    }
    if (!PASSWORD_PATTERN.test(password)) return t("密码只能是大写字母、小写字母、数字或下划线");
  }
  return "";
}

function createStore() {
  const state = reactive({
    me: getUser(),
    users: [],
    loading: true,
    denied: false,
    creating: false,
    saving: false,
    draft: emptyDraft(), // 新建表单的值
    draftOpen: false, // 新建弹层开着没有（入口在账号列表末尾那一行）
    editing: null, // { uuid, username, player_name, role, password, original }
    toast: { message: "", kind: "info", visible: false },
  });

  const { toast, toastError } = createToast(state);

  /* ── 派生 ────────────────────────────────────────────────────── */

  const me = computed(() => state.users.find((user) => user.uuid === state.me?.uuid) ?? state.me);
  const isSelf = (user) => Boolean(state.me && user.uuid === state.me.uuid);

  /** 后端每次写操作都回一份整表快照，前端直接换掉，不做局部拼接。 */
  function applySnapshot(payload) {
    state.users = payload.users;
    if (!payload.me) return;
    const mine = state.users.find((user) => user.uuid === payload.me);
    if (mine) {
      state.me = mine;
      setToken(getToken(), mine); // 顺手刷新本地缓存的角色
    }
  }

  function fail(error) {
    if (error.status === 403) {
      state.denied = true;
      return;
    }
    toastError(error.message);
  }

  /* ── 读取 ────────────────────────────────────────────────────── */

  async function load() {
    // 不看本地缓存的角色：它可能已经过期（比如刚从 normal 提成 admin，还没重新登录），
    // 直接问后端 —— 非管理员会拿到 403，再由 fail() 置 denied。
    state.loading = true;
    try {
      applySnapshot(await api("/api/admin/users"));
    } catch (error) {
      fail(error);
    } finally {
      state.loading = false;
    }
  }

  /* ── 新建 / 编辑 / 删除 ──────────────────────────────────────── */

  /** 打开新建弹层：每次都从空表单开始，免得上次没提交的内容又冒出来。 */
  function startCreate() {
    state.draft = emptyDraft();
    state.draftOpen = true;
  }

  function cancelCreate() {
    state.draftOpen = false;
  }

  /**
   * UUID 的规范形式：32 位无横线小写（与库里存的一致）。
   *
   * 输入框失焦时折成它 —— 大小写与横线都折，不折的话「看到的」和「存下来的」不一样，
   * 容易让人以为横线也进了库。不边打边改，免得光标乱跳。
   */
  const canonicalUuid = (value) => value.trim().toLowerCase().replaceAll("-", "");

  function lowerDraftUuid() {
    state.draft.uuid = canonicalUuid(state.draft.uuid);
  }

  function lowerEditingUuid() {
    if (state.editing) state.editing.uuid = canonicalUuid(state.editing.uuid);
  }

  async function create() {
    const draft = state.draft;
    if (!draft.username.trim() || !draft.player_name.trim() || !draft.password) {
      toastError(t("账号、游戏内名称、密码不能为空"));
      return;
    }
    const invalid = validateAccount({
      username: draft.username.trim(),
      player_name: draft.player_name.trim(),
      uuid: draft.uuid.trim(),
      password: draft.password,
    });
    if (invalid) {
      toastError(invalid);
      return;
    }
    state.creating = true;
    try {
      const payload = await api("/api/admin/users", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          username: draft.username.trim(),
          player_name: draft.player_name.trim(),
          password: draft.password,
          role: draft.role,
          uuid: canonicalUuid(draft.uuid), // 空串 = 让后端按离线规则推导
        }),
      });
      applySnapshot(payload);
      state.draft = emptyDraft();
      state.draftOpen = false;
      toast(t("已创建「{name}」", { name: payload.user.player_name }));
    } catch (error) {
      fail(error);
    } finally {
      state.creating = false;
    }
  }

  function startEdit(user) {
    state.editing = {
      uuid: user.uuid,
      username: user.username,
      player_name: user.player_name,
      role: user.role,
      password: "",
      original: { uuid: user.uuid, username: user.username },
    };
  }

  function cancelEdit() {
    state.editing = null;
  }

  async function saveEdit() {
    const editing = state.editing;
    if (!editing) return;
    if (!editing.username.trim() || !editing.player_name.trim()) {
      toastError(t("账号和游戏内名称都不能为空"));
      return;
    }
    const invalid = validateAccount({
      username: editing.username.trim(),
      player_name: editing.player_name.trim(),
      uuid: editing.uuid.trim(),
      password: editing.password,
    });
    if (invalid) {
      toastError(invalid);
      return;
    }
    const selfEdit = state.me?.uuid === editing.original.uuid;
    const credentialsChanged =
      editing.password !== "" ||
      editing.username !== editing.original.username ||
      editing.uuid !== editing.original.uuid;

    state.saving = true;
    try {
      // 路径用的是「原始 uuid」（账号身份），body 里才是要改成的新 uuid
      const payload = await api(`/api/admin/users/${editing.original.uuid}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          uuid: canonicalUuid(editing.uuid),
          username: editing.username.trim(),
          player_name: editing.player_name.trim(),
          role: editing.role,
          ...(editing.password ? { password: editing.password } : {}),
        }),
      });
      applySnapshot(payload);
      state.editing = null;
      // 改的是自己的凭据：旧 token 已经失效，直接回登录页
      if (selfEdit && credentialsChanged) {
        toLogin();
        return;
      }
      if (selfEdit && editing.role !== "admin") {
        toast(t("已退出管理员身份"));
        setTimeout(() => location.replace("/"), 900);
        return;
      }
      toast(t("已保存"));
    } catch (error) {
      fail(error);
    } finally {
      state.saving = false;
    }
  }

  async function remove(user) {
    if (!confirm(t("删除用户「{name}」？不可撤销。", { name: user.player_name }))) return;
    try {
      applySnapshot(await api(`/api/admin/users/${user.uuid}`, { method: "DELETE" }));
      toast(t("已删除「{name}」", { name: user.player_name }));
    } catch (error) {
      fail(error);
    }
  }

  // 退出登录就是「清 token 回登录页」，与 api.js 的 401 处理共用同一份实现
  const logout = toLogin;

  return reactive({
    state,
    me,
    isSelf,
    roleLabel,
    load,
    startCreate,
    cancelCreate,
    lowerDraftUuid,
    lowerEditingUuid,
    create,
    startEdit,
    cancelEdit,
    saveEdit,
    remove,
    logout,
    toast,
    toastError,
  });
}

export const maintainStore = createStore();
