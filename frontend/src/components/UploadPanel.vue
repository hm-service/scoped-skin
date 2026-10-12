<script setup lang="ts">
/** 上传面板：拖拽区（sha256 预检）+ 标签 + 模型 + 提交。 */
import { computed, onMounted, ref, watch } from "vue";

import { renderPreview } from "@/avatar";
import TagCombo from "@/components/TagCombo.vue";
import { t } from "@/i18n";
import { MODEL_OPTIONS, store } from "@/store";
import { byPopularity } from "@/tags";

const canvas = ref<HTMLCanvasElement | null>(null);
const fileInput = ref<HTMLInputElement | null>(null);

const library = computed(() => [...store.state.library].sort(byPopularity));

function draw(): void {
  const element = canvas.value;
  if (!element) return;
  const image = store.pendingImage;
  if (!image) {
  element.getContext("2d")?.clearRect(0, 0, element.width, element.height);
  return;
  }
  renderPreview(element, image, store.pendingKind);
}

onMounted(draw);
watch(() => [store.pendingImage, store.pendingKind], draw);

function pick(): void {
  fileInput.value?.click();
}

function onFile(event: Event): void {
  store.setPendingFile((event.target as HTMLInputElement).files?.[0]);
  (event.target as HTMLInputElement).value = ""; // 允许再次选同一个文件
}

function onDrop(event: DragEvent): void {
  store.state.fileOver = false;
  store.setPendingFile(event.dataTransfer?.files?.[0]);
}
</script>

<template>
  <form class="uploader" novalidate @submit.prevent="store.submitUpload()">
    <div class="dropzone" role="button" tabindex="0" :class="{ over: store.state.fileOver }"
         @click="pick" @keydown.enter.prevent="pick"
         @dragenter.prevent="store.state.fileOver = true"
         @dragover.prevent="store.state.fileOver = true"
         @dragleave.prevent="store.state.fileOver = false"
         @drop.prevent="onDrop">
      <input ref="fileInput" type="file" accept="image/png" hidden @change="onFile">
      <canvas ref="canvas" class="drop-preview" width="96" height="192"></canvas>
      <span class="drop-text">
        <strong>{{ store.dropText.title }}</strong>
        <small>{{ store.dropText.detail }}</small>
      </span>
    </div>

    <div class="form-row">
      <span class="form-label">{{ t("upload.tags") }}</span>
      <div class="tag-field">
        <TagCombo :selected="store.state.selectedTags" :library="store.state.library"
                  @add="store.addTag" @remove="store.removeTag"/>
        <div class="tag-existing-list">
          <button v-for="entry in library" :key="entry.tag" type="button"
                  class="tag-chip ghost" :class="{ on: store.hasTag(entry.tag) }"
                  @click="store.toggleTag(entry.tag)">
            <span>{{ entry.tag }}</span><span class="count">{{ entry.count }}</span>
          </button>
          <span v-if="!library.length" class="tag-empty">{{ t("upload.tags-empty") }}</span>
        </div>
      </div>
    </div>

    <label v-if="store.state.category === 'skin'" class="field field-model">
      <span>{{ t("upload.model") }}</span>
      <select v-model="store.state.model">
        <option v-for="option in MODEL_OPTIONS" :key="option.value" :value="option.value">
          {{ option.label }}
        </option>
      </select>
    </label>

    <div class="form-actions">
      <button type="submit" class="primary-btn" :disabled="store.submitState.disabled">
        {{ store.submitState.label }}
      </button>
      <button v-if="store.canDeletePending" type="button" class="ghost-btn danger"
              @click="store.removePending()">
        <svg class="icon"><use href="#i-trash"/></svg>{{ t("action.delete") }}
      </button>
    </div>
  </form>
</template>

<style scoped>
/* ── 上传面板 ──────────────────────────────────────────────────── */

.uploader {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 2px;
}

.dropzone {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 14px;
  border: 1.5px dashed var(--line-strong);
  border-radius: var(--radius-md);
  background: var(--paper-soft);
  cursor: pointer;
  transition: border-color .15s, background .15s;
}

.dropzone:hover, .dropzone.over { border-color: var(--accent); background: var(--accent-soft); }

.drop-preview {
  width: 76px;
  height: 152px;
  image-rendering: pixelated;
  background: #fff;
  border-radius: 8px;
  object-fit: contain;
  flex: 0 0 auto;
}

.drop-text { display: flex; flex-direction: column; gap: 5px; font-size: 13px; color: var(--ink-soft); }

.drop-text strong { font-size: 14px; color: var(--ink); }

.drop-text small { color: var(--ink-faint); }

.form-row { display: flex; gap: 10px; }

.form-label { flex: 0 0 auto; padding-top: 4px; font-size: 12.5px; color: var(--ink-soft); }

.tag-field { flex: 1 1 auto; min-width: 0; display: flex; flex-direction: column; gap: 8px; }

/* 已有标签：随内容自动撑高，最多约 8 行，再多就自己滚 */
.tag-existing-list {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  max-height: 232px;
  overflow-y: auto;
  scrollbar-width: thin;
}

.field-model { max-width: 240px; }

.field-model select { flex: 1 1 auto; }
</style>
