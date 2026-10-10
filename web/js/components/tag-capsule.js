/** 标签 capsule：配色由标签内容决定（同一个标签永远同色）。 */
import { tagColor } from "../tags.js";

export const TagCapsule = {
  name: "TagCapsule",
  props: { tag: { type: String, required: true } },
  computed: {
    color() {
      return tagColor(this.tag);
    },
  },
  template: `<span class="tag" :class="color">{{ tag }}</span>`,
};
