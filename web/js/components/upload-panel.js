/** 上传面板：拖拽区（sha256 预检）+ 标签 + 模型 + 提交。 */
import { renderPreview } from "../avatar.js";
import { t } from "../i18n.js";
import { MODEL_OPTIONS, store } from "../store.js";
import { byPopularity } from "../tags.js";
import { computed, onMounted, ref, watch } from "../vue.js";
import { TagCombo } from "./tag-combo.js";

export const UploadPanel = {
  name: "UploadPanel",
  components: { TagCombo },
  setup() {
    const canvas = ref(null);
    const fileInput = ref(null);

    const library = computed(() => [...store.state.library].sort(byPopularity));

    function draw() {
      const element = canvas.value;
      if (!element) return;
      const image = store.pendingImage;
      if (!image) {
        element.getContext("2d").clearRect(0, 0, element.width, element.height);
        return;
      }
      renderPreview(element, image, store.pendingKind);
    }

    onMounted(draw);
    watch(() => [store.pendingImage, store.pendingKind], draw);

    function pick() {
      fileInput.value?.click();
    }

    function onFile(event) {
      store.setPendingFile(event.target.files?.[0]);
      event.target.value = ""; // 允许再次选同一个文件
    }

    function onDrop(event) {
      store.state.fileOver = false;
      store.setPendingFile(event.dataTransfer?.files?.[0]);
    }

    return { store, canvas, fileInput, library, pick, onFile, onDrop, MODEL_OPTIONS };
  },
  template: `
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
    </form>`,
};
