<template>
  <!-- 一章 = 章名行（章号内联在章名前 + 副题或工具条）+ 内容。章号是固定编号而不是序号：
       某一章因数据缺失整章不渲染时，其余章号 MUST NOT 重排（页面出现 01 02 04… 是正确行为）。
       左侧边注栏已按用户反馈（2026-09-12）整体去掉，内容通栏。 -->
  <section class="rp-chapter" :data-testid="`leaderboard-chapter-${num}`">
    <div class="rp-h2row">
      <h2 class="rp-h2"><span class="rp-num">{{ num }}</span>{{ title }}</h2>
      <!-- 工具条（窗口 / 指标 / 刷新）与小字副题占同一个位置：给了 tools 就以它为准。 -->
      <slot name="tools">
        <span v-if="subtitle" class="rp-h2sub">{{ subtitle }}</span>
      </slot>
    </div>
    <div class="rp-body">
      <slot />
    </div>
  </section>
</template>

<script setup lang="ts">
defineProps<{
  /** 固定章号，两位字符串（`01`–`07`）；补零由调用方给出，组件不做格式化。 */
  num: string
  /** 章名，走 i18n（`leaderboard.chapters.NN.name`）。 */
  title: string
  /** 章名右侧的小字副题；给了 `tools` 插槽时不渲染。 */
  subtitle?: string
}>()
</script>
