<template>
  <!-- 内联线条图标：页面里不出现任何 unicode 符号图标（☼ ☾ ▤ ↻ ▲ …），也不引外部图标库。
       图形一律 aria-hidden：图标只当装饰，可访问名挂在包着它的按钮 / 文字上。 -->
  <svg
    class="rp-icon"
    :width="size"
    :height="size"
    viewBox="0 0 16 16"
    fill="none"
    stroke="currentColor"
    stroke-width="1.5"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    focusable="false"
    :data-icon="name"
  >
    <template v-for="(shape, index) in shapes" :key="index">
      <circle
        v-if="shape.kind === 'circle'"
        :cx="shape.cx"
        :cy="shape.cy"
        :r="shape.r"
      ></circle>
      <rect
        v-else-if="shape.kind === 'rect'"
        :x="shape.x"
        :y="shape.y"
        :width="shape.width"
        :height="shape.height"
        :rx="shape.rx"
      ></rect>
      <path v-else :d="shape.d"></path>
    </template>
  </svg>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { LEADERBOARD_ICONS, type LbIconName } from './icons'

const props = withDefaults(
  defineProps<{
    /** 图标名，取值见 `./icons.ts`（照画板生成器的 ICONS 字典移植）。 */
    name: LbIconName
    /** 边长（px），viewBox 固定 16，因此任何尺寸下线宽观感一致。 */
    size?: number
  }>(),
  { size: 14 },
)

/** 名字给错时画一个空 svg 而不是抛错：图标只是装饰，MUST NOT 因此炸掉整块内容。 */
const shapes = computed(() => LEADERBOARD_ICONS[props.name] ?? [])
</script>
