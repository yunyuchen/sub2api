<template>
  <!-- 一章 = 章头行（章号 chip + 章名，下方一条细线）+ 内容。章号是固定编号而不是序号：
       某一章因数据缺失整章不渲染时，其余章号 MUST NOT 重排（页面出现 01 02 05… 是正确行为）。
       左侧边注栏已按用户反馈（2026-09-12）整体去掉，内容通栏。
       spool 皮肤：章号 chip 从 h2 里移出来，与章名基线对齐；类名仍叫 `.rp-num`，
       页面级 spec 靠 `.rp-chapter .rp-num` 取章号序列。
       工具条插槽已删除：窗口 / 指标只在页头一处，刷新按钮在榜单窗口的标题栏里。 -->
  <section class="rp-chapter" :data-testid="`leaderboard-chapter-${num}`">
    <div class="rp-h2row">
      <span class="rp-num">{{ num }}</span>
      <h2 class="rp-h2">{{ title }}</h2>
      <span v-if="subtitle" class="rp-h2sub">{{ subtitle }}</span>
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
  /** 章名右侧的小字副题；当前没人传，保留给以后用。 */
  subtitle?: string
}>()
</script>
