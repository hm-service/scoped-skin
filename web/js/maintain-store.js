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

const emptyDraft = () => ({ username: "", player_name: "", password: "", role: "normal" });

function createStore() {
  const state = reactive({
    me: getUser(),
    users: [],
    loading: true,
    denied: false,
    creating: false,
    saving: false,
    draft: emptyDraft(),
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

  async function create() {
    const draft = state.draft;
    if (!draft.username.trim() || !draft.player_name.trim() || !draft.password) {
      toastError(t("账号、游戏内名称、密码不能为空"));
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
        }),
      });
      applySnapshot(payload);
      state.draft = emptyDraft();
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
          uuid: editing.uuid,
          username: editing.username.trim(),
          player_name: editing.player_name.trim(),
          role: editing.role,
          ...(editing.password ? { password: editing.password } : {}),
        }),
      });
      applySnapshot(payload);
      state.editing = null;
      if (selfEdit && credentialsChanged) {
        toast(t("凭据已改，请重新登录"));
        setTimeout(toLogin, 900);
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
