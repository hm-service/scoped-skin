<script setup lang="ts">
/**
 * 新建一个账号：游戏内名称 / 账号 / 密码 / UUID / 身份组。
 *
 * 入口在账号列表末尾那一行（点一下开这个弹层）。UUID 留空时由后端按离线规则从
 * 游戏内名称推导，建好之后可以在「编辑」里改。
 */
import { t } from "@/i18n";
import { maintainStore } from "@/maintain-store";

const store = maintainStore;
</script>

<template>
  <div v-if="store.state.draftOpen" class="modal-layer" @click.self="store.cancelCreate()">
    <section class="modal" role="dialog" aria-modal="true" :aria-label="t('user.create')">
      <header class="modal-head">
        <h2>{{ t("user.create") }}</h2>
        <button class="icon-btn" type="button" :title="t('common.close')" @click="store.cancelCreate()">
          <svg class="icon"><use href="#i-x"/></svg>
        </button>
      </header>

      <form class="m-form" @submit.prevent="store.create()">
        <label class="field">
          <span>{{ t("field.player-name") }}</span>
          <input v-model.trim="store.state.draft.player_name" type="text" minlength="3" maxlength="16"
                 spellcheck="false" :placeholder="t('user.player-name-hint')" required>
        </label>
        <label class="field">
          <span>{{ t("auth.account") }}</span>
          <input v-model.trim="store.state.draft.username" type="text" maxlength="32"
                 spellcheck="false" autocomplete="off" :placeholder="t('user.username-hint')" required>
        </label>
        <label class="field">
          <span>{{ t("auth.password") }}</span>
          <input v-model="store.state.draft.password" type="password" minlength="8" maxlength="64"
                 autocomplete="new-password" :placeholder="t('user.password-hint')" required>
        </label>
        <label class="field">
          <span>UUID</span>
          <input v-model.trim="store.state.draft.uuid" type="text" spellcheck="false"
                 autocomplete="off" :placeholder="t('user.uuid-hint')"
                 @blur="store.lowerDraftUuid()">
        </label>
        <label class="field">
          <span>{{ t("field.role") }}</span>
          <select v-model="store.state.draft.role">
            <option v-for="role in store.roleOptions" :key="role" :value="role">
              {{ store.roleLabel(role) }}
            </option>
          </select>
        </label>

        <div class="form-actions">
          <button type="submit" class="primary-btn" :disabled="store.state.creating">
            <svg class="icon"><use href="#i-plus"/></svg>
            {{ store.state.creating ? t('user.creating') : t('user.create-submit') }}
          </button>
          <button type="button" class="ghost-btn" @click="store.cancelCreate()">{{ t("common.cancel") }}</button>
        </div>
      </form>
    </section>
  </div>
</template>
