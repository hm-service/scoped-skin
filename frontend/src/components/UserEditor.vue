<script setup lang="ts">
/** 改一个账号：UUID / 账号名 / 游戏内名称 / 身份组 / 新密码（留空 = 不改）。 */
import { t } from "@/i18n";
import { maintainStore } from "@/maintain-store";

const store = maintainStore;
</script>

<template>
  <div v-if="store.state.editing" class="modal-layer" @click.self="store.cancelEdit()">
    <section class="modal" role="dialog" aria-modal="true" :aria-label="t('user.edit-title')">
      <header class="modal-head">
        <h2>{{ t("user.edit-title") }}</h2>
        <button class="icon-btn" type="button" :title="t('common.close')" @click="store.cancelEdit()">
          <svg class="icon"><use href="#i-x"/></svg>
        </button>
      </header>

      <form class="m-form" @submit.prevent="store.saveEdit()">
        <label class="field">
          <span>{{ t("field.player-name") }}</span>
          <input v-model.trim="store.state.editing.player_name" type="text" minlength="3" maxlength="16"
                 spellcheck="false" :placeholder="t('user.player-name-hint')">
        </label>
        <label class="field">
          <span>{{ t("auth.account") }}</span>
          <input v-model.trim="store.state.editing.username" type="text" maxlength="32"
                 spellcheck="false" autocomplete="off">
        </label>
        <label class="field">
          <span>UUID</span>
          <input v-model.trim="store.state.editing.uuid" type="text" spellcheck="false"
                 autocomplete="off" :placeholder="t('user.uuid-hint-edit')"
                 @blur="store.lowerEditingUuid()">
        </label>
        <label class="field">
          <span>{{ t("field.role") }}</span>
          <select v-model="store.state.editing.role">
            <option v-for="role in store.roleOptions" :key="role" :value="role">
              {{ store.roleLabel(role) }}
            </option>
          </select>
        </label>
        <label class="field">
          <span>{{ t("user.new-password") }}</span>
          <input v-model="store.state.editing.password" type="password" minlength="8" maxlength="64"
                 autocomplete="new-password" :placeholder="t('user.password-hint-edit')">
        </label>

        <div class="form-actions">
          <button type="submit" class="primary-btn" :disabled="store.state.saving">
            <svg class="icon"><use href="#i-check"/></svg>
            {{ store.state.saving ? t('user.saving') : t('user.save') }}
          </button>
          <button type="button" class="ghost-btn" @click="store.cancelEdit()">{{ t("common.cancel") }}</button>
        </div>
      </form>
    </section>
  </div>
</template>
