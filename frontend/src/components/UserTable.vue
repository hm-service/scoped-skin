<script setup lang="ts">
/** 用户列表：一行一个账号，行尾是编辑 / 删除；自己被标出来且不能删。末尾一行是新建入口。 */
import { t } from "@/i18n";
import { maintainStore } from "@/maintain-store";

const store = maintainStore;
</script>

<template>
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
  </div>
</template>

<style scoped>
.m-table-wrap { overflow-x: auto; }

.m-table { width: 100%; border-collapse: collapse; font-size: 13px; }

.m-table th {
  padding: 6px 10px;
  border-bottom: 1px solid var(--line-strong);
  text-align: left;
  font-size: 11.5px;
  font-weight: 600;
  color: var(--ink-faint);
  letter-spacing: .04em;
  white-space: nowrap;
}

.m-table td { padding: 8px 10px; border-bottom: 1px solid var(--line); vertical-align: middle; }

.m-table tbody tr:last-child td { border-bottom: none; }

.m-table tbody tr:hover { background: var(--paper-soft); }

.m-table tbody tr.self { background: var(--accent-soft); }

.m-table tbody tr.self:hover { background: var(--accent-soft); }

/* 列表末尾那一行：新建入口 */
.m-table tfoot td { padding: 8px 10px; border-top: 1px solid var(--line); }

.m-table tfoot .ghost-btn { display: inline-flex; align-items: center; gap: 6px; }

.m-name { font-weight: 600; }

.m-self-flag {
  margin-left: 6px;
  padding: 1px 6px;
  border-radius: 999px;
  background: var(--accent);
  color: #fff;
  font-size: 10px;
}

.m-uuid { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11.5px; color: var(--ink-soft); }

.m-role { padding: 2px 8px; border-radius: 999px; background: var(--paper-sunk); color: var(--ink-soft); font-size: 11.5px; }

.m-role.admin { background: var(--accent-soft); color: var(--accent-ink); font-weight: 600; }

.m-col-actions { width: 1%; text-align: right; white-space: nowrap; }

.m-col-actions .icon-btn { display: inline-grid; }

.m-col-actions .icon-btn + .icon-btn { margin-left: 6px; }

.m-empty { padding: 22px; text-align: center; color: var(--ink-faint); }

@media (max-width: 720px) {
  .m-table th:nth-child(3), .m-table td:nth-child(3) { display: none; }
}
</style>
