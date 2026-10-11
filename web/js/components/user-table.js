/** 用户列表：一行一个账号，行尾是编辑 / 删除；自己被标出来且不能删。末尾一行是新建入口。 */
import { maintainStore } from "../maintain-store.js";

export const UserTable = {
  name: "UserTable",
  setup() {
    return { store: maintainStore };
  },
  template: `
    <div class="m-table-wrap">
      <table class="m-table">
        <thead>
          <tr>
            <th>{{ t("field.player-name") }}</th>
            <th>{{ t("auth.account") }}</th>
            <th>UUID</th>
            <th>{{ t("field.role") }}</th>
            <th class="m-col-actions">{{ t("user.actions") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="user in store.state.users" :key="user.uuid"
              :class="{ self: store.isSelf(user) }">
            <td>
              <span class="m-name">{{ user.player_name }}</span>
              <span v-if="store.isSelf(user)" class="m-self-flag">{{ t("user.me") }}</span>
            </td>
            <td>{{ user.username }}</td>
            <td class="m-uuid" :title="user.uuid">{{ user.uuid }}</td>
            <td>
              <span class="m-role" :class="user.role">{{ store.roleLabel(user.role) }}</span>
            </td>
            <td class="m-col-actions">
              <button class="icon-btn" type="button" :title="t('action.edit')"
                      @click="store.startEdit(user)">
                <svg class="icon"><use href="#i-pencil"/></svg>
              </button>
              <button class="icon-btn danger" type="button" :title="t('action.delete')"
                      :disabled="store.isSelf(user)" @click="store.remove(user)">
                <svg class="icon"><use href="#i-trash"/></svg>
              </button>
            </td>
          </tr>
          <tr v-if="!store.state.users.length">
            <td class="m-empty" colspan="5">{{ t("user.empty") }}</td>
          </tr>
        </tbody>
        <tfoot>
          <tr>
            <td colspan="5">
              <button class="ghost-btn" type="button" @click="store.startCreate()">
                <svg class="icon"><use href="#i-plus"/></svg>{{ t("user.create") }}
              </button>
            </td>
          </tr>
        </tfoot>
      </table>
    </div>`,
};
