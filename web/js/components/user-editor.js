/** 改一个账号：UUID / 账号名 / 游戏内名称 / 身份组 / 新密码（留空 = 不改）。 */
import { maintainStore } from "../maintain-store.js";

export const UserEditor = {
  name: "UserEditor",
  setup() {
    return { store: maintainStore };
  },
  template: `
    <div v-if="store.state.editing" class="modal-layer" @click.self="store.cancelEdit()">
      <section class="modal" role="dialog" aria-modal="true" :aria-label="t('编辑用户')">
        <header class="modal-head">
          <h2>{{ t("编辑用户") }}</h2>
          <button class="icon-btn" type="button" :title="t('关闭')" @click="store.cancelEdit()">
            <svg class="icon"><use href="#i-x"/></svg>
          </button>
        </header>

        <form class="m-form" @submit.prevent="store.saveEdit()">
          <label class="field">
            <span>{{ t("游戏内名称") }}</span>
            <input v-model.trim="store.state.editing.player_name" type="text" maxlength="16"
                   spellcheck="false" :placeholder="t('小写字母 / 数字 / 下划线')">
          </label>
          <label class="field">
            <span>{{ t("账号") }}</span>
            <input v-model.trim="store.state.editing.username" type="text" maxlength="32"
                   spellcheck="false" autocomplete="off">
          </label>
          <label class="field">
            <span>UUID</span>
            <input v-model.trim="store.state.editing.uuid" type="text" spellcheck="false"
                   autocomplete="off" :placeholder="t('32 位十六进制，可带横线')">
          </label>
          <label class="field">
            <span>{{ t("身份组") }}</span>
            <select v-model="store.state.editing.role">
              <option value="guest">{{ t("游客") }}</option>
              <option value="normal">{{ t("普通") }}</option>
              <option value="admin">{{ t("管理员") }}</option>
            </select>
          </label>
          <label class="field">
            <span>{{ t("新密码") }}</span>
            <input v-model="store.state.editing.password" type="password" autocomplete="new-password"
                   :placeholder="t('留空不改')">
          </label>

          <p class="m-hint">{{ t("改 UUID / 账号 / 密码后，对方需要重新登录。") }}</p>

          <div class="form-actions">
            <button type="submit" class="primary-btn" :disabled="store.state.saving">
              <svg class="icon"><use href="#i-check"/></svg>
              {{ store.state.saving ? t('保存中…') : t('保存') }}
            </button>
            <button type="button" class="ghost-btn" @click="store.cancelEdit()">{{ t("取消") }}</button>
          </div>
        </form>
      </section>
    </div>`,
};
