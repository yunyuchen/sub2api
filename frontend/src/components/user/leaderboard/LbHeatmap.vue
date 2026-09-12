<template>
  <!-- 06 顶部通栏：近 30 天按天的条形热力，一行 30 格 + 起止日期刻度。
       来源是仪表盘预聚合表（计数是裸 COUNT(*)），口径差异写在 CONTEXT.md 里，
       页面上不再为它挂一句注脚：格子只表达「这一天忙不忙」。 -->
  <div v-if="cells.length" class="rp-r" data-testid="leaderboard-heatmap">
    <span class="rp-eyebrow">{{ t('leaderboard.insights.heatmap.title') }}</span>

    <div class="rp-heat30">
      <!-- 级联按周走（每 7 格一档），逐格递延会让一行 30 格的入场拖得太长。 -->
      <i
        v-for="(cell, index) in cells"
        :key="cell.date"
        class="rp-growy"
        :class="cell.levelClass"
        :title="cell.title"
        :style="{ '--i': Math.floor(index / 7) }"
        data-testid="leaderboard-heatmap-cell"
      ></i>
    </div>

    <div class="rp-heat-ticks" aria-hidden="true">
      <span class="rp-n is-start">{{ firstLabel }}</span>
      <span class="rp-n is-end">{{ lastLabel }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatNumberLocaleString } from '@/utils/format'
import type { LeaderboardDailyBucket } from '@/api/leaderboard'

const { t } = useI18n()

const props = defineProps<{
  /** `insights.daily_30`；为 null 或空时整块不渲染。 */
  daily: LeaderboardDailyBucket[] | null
}>()

interface HeatmapCell {
  date: string
  /** 0 档不加类，直接用格子的底色（`--heat-0`）；其余是同色相的四级明度。 */
  levelClass: string
  title: string
}

/** 该日是否有用量：匿名档没有绝对量，只能看相对百分比。 */
function hasUsage(bucket: LeaderboardDailyBucket): boolean {
  if (bucket.relative_percent > 0) return true
  return (bucket.requests ?? 0) > 0 || (bucket.total_tokens ?? 0) > 0
}

/** 色阶按相对最高日的百分比分桶；有用量但四舍五入成 0% 的日子仍给最低一档，不当成空白。 */
function levelOf(bucket: LeaderboardDailyBucket): number {
  if (!hasUsage(bucket)) return 0
  const percent = bucket.relative_percent
  if (percent <= 25) return 1
  if (percent <= 50) return 2
  if (percent <= 75) return 3
  return 4
}

function titleOf(bucket: LeaderboardDailyBucket): string {
  if (typeof bucket.requests === 'number') {
    return t('leaderboard.insights.heatmap.cellRequests', {
      date: bucket.date,
      count: formatNumberLocaleString(bucket.requests),
    })
  }
  return t('leaderboard.insights.heatmap.cellRelative', {
    date: bucket.date,
    percent: bucket.relative_percent,
  })
}

/** 一行 30 格，取最近 30 天；来源不足 30 天时有几天画几天。 */
const cells = computed<HeatmapCell[]>(() =>
  (props.daily ?? []).slice(-30).map((bucket) => {
    const level = levelOf(bucket)
    return {
      date: bucket.date,
      levelClass: level > 0 ? `rp-lv${level}` : '',
      title: titleOf(bucket),
    }
  }),
)

/** `YYYY-MM-DD` → `MM-DD`：刻度只标起止两端，中间由格子自己说话。 */
function shortDate(date: string): string {
  return date.length >= 10 ? date.slice(5) : date
}

const firstLabel = computed(() => (cells.value[0] ? shortDate(cells.value[0].date) : ''))
const lastLabel = computed(() => {
  const last = cells.value[cells.value.length - 1]
  return last ? shortDate(last.date) : ''
})
</script>
