/**
 * 新建一个账号：游戏内名称 / 账号 / 密码 / 身份组。
 *
 * 入口在账号列表末尾那一行（点一下开这个弹层）。没有 UUID 字段 —— 后端按离线规则从
 * 游戏内名称推导，建好之后可以在「编辑」里改（DESIGN.md 4.1）。
 */
import { maintainStore } from "../maintain-store.js";

export const UserCreate = {
  name: "UserCreate",
  setup() {
    return { store: maintainStore };
  },
  template: `
    <div v-if="store.state.draftOpen" class="modal-layer" @click.self="store.cancelCreate()">
      <section class="modal" role="dialog" aria-modal="true" :aria-label="t('新建用户')">
        <header class="modal-head">
          <h2>{{ t("新建用户") }}</h2>
          <button class="icon-btn" type="button" :title="t('关闭')" @click="store.cancelCreate()">
            <svg class="icon"><use href="#i-x"/></svg>
          </button>
        </header>

        <form class="m-form" @submit.prevent="store.create()">
          <label class="field">
            <span>{{ t("游戏内名称") }}</span>
            <input v-model.trim="store.state.draft.player_name" type="text" minlength="3" maxlength="16"
                   spellcheck="false" :placeholder="t('大小写字母 / 数字 / 下划线，3-16 位')" required>
          </label>
          <label class="field">
            <span>{{ t("账号") }}</span>
            <input v-model.trim="store.state.draft.username" type="text" maxlength="32"
                   spellcheck="false" autocomplete="off" :placeholder="t('小写字母 / 数字 / @ / .')" required>
          </label>
          <label class="field">
            <span>{{ t("密码") }}</span>
            <input v-model="store.state.draft.password" type="password"
                   autocomplete="new-password" :placeholder="t('登录密码')" required>
          </label>
          <label class="field">
            <span>UUID</span>
            <input v-model.trim="store.state.draft.uuid" type="text" spellcheck="false"
                   autocomplete="off" :placeholder="t('留空则由游戏内名称推导')">
          </label>
          <label class="field">
            <span>{{ t("身份组") }}</span>
            <select v-model="store.state.draft.role">
              <option value="guest">{{ t("游客") }}</option>
              <option value="normal">{{ t("普通") }}</option>
              <option value="admin">{{ t("管理员") }}</option>
            </select>
          </label>
          <p class="m-hint">{{ t("32 位十六进制，可带横线；建好后也能改") }}</p>

          <div class="form-actions">
            <button type="submit" class="primary-btn" :disabled="store.state.creating">
              <svg class="icon"><use href="#i-plus"/></svg>
              {{ store.state.creating ? t('创建中…') : t('创建用户') }}
            </button>
            <button type="button" class="ghost-btn" @click="store.cancelCreate()">{{ t("取消") }}</button>
          </div>
        </form>
      </section>
    </div>`,
};
