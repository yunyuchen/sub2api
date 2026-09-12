<template>
  <!-- 07 上半：左 14 天柱（取 daily_30 的尾部 14 条），右「本月累计」。
       纵轴的断轴规则见下面的常量注释：阈值与三段比例是固定常量，MUST NOT 随数据自适应。 -->
  <div
    v-if="columns.length"
    class="rp-trendwrap"
    :class="monthMeta ? '' : 'is-single'"
    data-testid="leaderboard-trend"
  >
    <div class="rp-r">
      <span class="rp-eyebrow">{{ t('leaderboard.insights.trend.title') }}</span>

      <div class="rp-plot">
        <div class="rp-bars">
          <!-- 级联按 3 天一档：14 根柱逐根递延会把入场拖得太长。 -->
          <span
            v-for="(column, index) in columns"
            :key="column.date"
            class="rp-col"
            :class="[column.hot ? 'is-hot' : '', column.peak ? 'is-peak' : '']"
            :title="column.title"
            data-testid="leaderboard-trend-col"
          >
            <span class="rp-n rp-v">{{ column.label }}</span>
            <!-- 柱高的百分比对 `.rp-barbox`（列高减去读数行）求值：100% 正好是柱区满格，
                 MUST NOT 直接挂在列上——那会让读数行挤掉顶部一段量程。 -->
            <span class="rp-barbox">
              <span
                class="rp-b rp-growy"
                :style="{ height: `${column.height}%`, '--i': Math.floor(index / 3) }"
              ></span>
            </span>
          </span>
        </div>

        <div
          v-if="axisBreak"
          class="rp-breakline"
          :style="{ bottom: breaklineBottom }"
          aria-hidden="true"
          data-testid="leaderboard-trend-breakline"
        >
          <span>{{ axisBreak.label }}</span>
        </div>
      </div>

      <div class="rp-bar-ticks" aria-hidden="true">
        <span v-for="(column, index) in columns" :key="column.date">
          {{ index % 2 === 0 ? column.tick : '' }}
        </span>
      </div>

    </div>

    <div
      v-if="monthMeta"
      class="rp-monthbox rp-r"
      style="--i: 1"
      data-testid="leaderboard-trend-month"
    >
      <span class="rp-eyebrow">{{ monthMeta.label }}</span>
      <div class="rp-n rp-month-v">{{ monthMeta.value }}</div>
      <div v-if="dayOverDay" class="rp-month-d">{{ dayOverDay }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatCompactNumberTrimmed } from '@/utils/format'
import type { LeaderboardDailyBucket, LeaderboardMonth } from '@/api/leaderboard'

const { t } = useI18n()

const props = defineProps<{
  /** `insights.daily_30`，只取尾部 14 条；为 null 或空时整块不渲染。 */
  daily: LeaderboardDailyBucket[] | null
  /** `insights.month`；为 null 时右边那一格整体不渲染。 */
  month: LeaderboardMonth | null
}>()

/**
 * 断轴的三个固定常量（design D23 / 增补 T2）：
 * 取 14 天里第三大的值 `T`，当最大值 **严格大于** `5T` 时纵轴在 `T` 处压缩——
 * `T` 以下占柱区 `BREAK_LOW` 高、中间 `BREAK_GAP` 画断轴虚线、`T` 以上占剩下的 22%。
 * 阈值与三段比例 MUST NOT 随数据自适应，少于 3 个数据点时没有 `T`，一律线性。
 */
const BREAK_RATIO = 5
const BREAK_LOW = 66
const BREAK_GAP = 12
const BREAK_LINE_BOTTOM = BREAK_LOW + BREAK_GAP / 2

/**
 * 读数行（`.rp-v` 13px）与列内 gap（4px）之和。柱子的百分比由 `.rp-barbox` 兜住，
 * 断轴虚线却挂在 `.rp-plot` 上（整列高），因此这里把同一个百分比换算过去：
 * `bottom: calc(B% - B/100 × 17px)` 等价于「柱区高度的 B%」，两个断点下都精确。
 */
const BAR_READOUT_H = 17
const breaklineBottom = `calc(${BREAK_LINE_BOTTOM}% - ${(BREAK_LINE_BOTTOM * BAR_READOUT_H) / 100}px)`

const rows = computed(() => (props.daily ?? []).slice(-14))

/** `YYYY-MM-DD` → `MM-DD`，刻度与 title 都用它。 */
function shortDate(date: string): string {
  return date.length >= 10 ? date.slice(5) : date
}

/** 排序与比例用的数值：优先绝对量，缺席时退到相对百分比（同一把尺子，比值仍然成立）。 */
function magnitude(bucket: LeaderboardDailyBucket): number {
  return typeof bucket.total_tokens === 'number' ? bucket.total_tokens : bucket.relative_percent
}

/** named 档给 tokens 绝对量，anonymous 档只有相对最高日的百分比。 */
function valueText(bucket: LeaderboardDailyBucket): string {
  if (typeof bucket.total_tokens === 'number') return formatCompactNumberTrimmed(bucket.total_tokens)
  return `${bucket.relative_percent}%`
}

/** 断轴标注里的那个 `T`：口径跟着柱子走，绝对量缺席时写成百分比。 */
function magnitudeText(value: number): string {
  const named = rows.value.some((bucket) => typeof bucket.total_tokens === 'number')
  return named ? formatCompactNumberTrimmed(Math.round(value)) : `${Math.round(value)}%`
}

const values = computed(() => rows.value.map(magnitude))
const maxValue = computed(() => values.value.reduce((max, value) => Math.max(max, value), 0))

/**
 * 断点值 `T` = 第三大的值。少于 3 个点、`T` 为 0、或最大值没有超过 `5T` 时不断轴。
 * 「恰好等于 `5T`」不断轴：阈值是严格大于。
 */
const breakValue = computed<number | null>(() => {
  const sorted = [...values.value].sort((left, right) => right - left)
  if (sorted.length < 3) return null
  const third = sorted[2]
  if (third <= 0) return null
  return maxValue.value > BREAK_RATIO * third ? third : null
})

interface TrendColumn {
  date: string
  tick: string
  title: string
  label: string
  height: number
  hot: boolean
  /** 最高的那一根：中窄屏列宽放不下两个读数，样式只留它一个。 */
  peak: boolean
}

const columns = computed<TrendColumn[]>(() => {
  const max = maxValue.value
  const brk = breakValue.value
  return rows.value.map((bucket) => {
    const value = magnitude(bucket)
    const hot = brk === null ? value >= max && max > 0 : value > brk
    let height: number
    if (brk === null) {
      height = max > 0 ? (value / max) * 100 : 0
    } else if (value <= brk) {
      height = (value / brk) * BREAK_LOW
    } else {
      // 断点以上的一段单独归一：断点映到 66 + 12，最大值映到 100。
      const span = max - brk
      const ratio = span > 0 ? (value - brk) / span : 1
      height = BREAK_LOW + BREAK_GAP + ratio * (100 - BREAK_LOW - BREAK_GAP)
    }
    return {
      date: bucket.date,
      tick: shortDate(bucket.date),
      title: `${shortDate(bucket.date)} · ${valueText(bucket)}`,
      // 读数只标在压缩段之上的那几天；不断轴时只标最高的一天，免得 14 个数字糊成一片。
      label: hot ? valueText(bucket) : '',
      height: Math.round(Math.max(2, height) * 1000) / 1000,
      hot,
      peak: max > 0 && value >= max,
    }
  })
})

/**
 * 断轴的标注（图上虚线旁的 `轴在 <T> 压缩`）；不断轴时整块不渲染。
 * 图下那句解释「最高一天是第三高那天的 N 倍…」已删：标注本身已经说明轴被压缩了。
 */
const axisBreak = computed(() => {
  const brk = breakValue.value
  if (brk === null) return null
  return { label: t('leaderboard.insights.trend.axisBreak.label', { value: magnitudeText(brk) }) }
})

function signedPercent(percent: number): string {
  return percent > 0 ? `+${percent}%` : `${percent}%`
}

/**
 * 右边那一格：named 档给「本月累计 165M」，anonymous 档没有站点级绝对量，
 * 换成「本月累计 · 较上月 +18%」。
 */
const monthMeta = computed(() => {
  const month = props.month
  if (!month) return null
  if (typeof month.total_tokens === 'number') {
    return {
      label: t('leaderboard.insights.trend.monthTotal'),
      value: formatCompactNumberTrimmed(month.total_tokens),
    }
  }
  return {
    label: `${t('leaderboard.insights.trend.monthTotal')} · ${t('leaderboard.insights.trend.monthChange')}`,
    value: signedPercent(month.change_percent),
  }
})

/** 「较前一日」由最后两天现算；不足两天或前一天为 0 时整行省略，MUST NOT 写成 0%。 */
const dayOverDay = computed(() => {
  const list = values.value
  if (list.length < 2) return ''
  const previous = list[list.length - 2]
  const latest = list[list.length - 1]
  if (previous <= 0) return ''
  return t('leaderboard.insights.trend.sub', {
    change: signedPercent(Math.round((latest / previous - 1) * 100)),
  })
})
</script>
