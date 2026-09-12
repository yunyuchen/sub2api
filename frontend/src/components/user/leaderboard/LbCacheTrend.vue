<template>
  <!-- 07 下半右：全站缓存的唯一一块。大号数字是今日命中率（缺席时退到趋势末点），
       右边是近 14 天的折线。页面上 MUST NOT 再出现第二个缓存区块——
       「今日命中率」与「趋势末点」通常是同一个数，画两遍只是同一句话说两次。
       两个来源都没有数据时整块不渲染，MUST NOT 画一条代表 0 的平线。 -->
  <div v-if="hasRate" class="rp-r" style="--i: 3" data-testid="leaderboard-cache-trend">
    <span class="rp-eyebrow">{{ t('leaderboard.cacheTrend.note') }}</span>

    <div class="rp-cache-row">
      <div>
        <div class="rp-big" data-testid="leaderboard-cache-trend-rate">{{ rateText }}%</div>
        <div class="rp-k" data-testid="leaderboard-cache-trend-sub">{{ subText }}</div>
      </div>
      <div>
        <LbSparkline
          v-if="values.length"
          :points="values"
          :height="64"
          :label="t('leaderboard.cacheTrend.note')"
        />
        <div v-if="axis" class="rp-axis" aria-hidden="true">
          <span class="rp-n">{{ axis.from }}</span>
          <span class="rp-n">{{ axis.to }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LbSparkline from './LbSparkline.vue'
import { useCountUp } from '@/composables/useCountUp'
import { formatCompactNumberTrimmed } from '@/utils/format'
import type { LeaderboardCacheToday, LeaderboardCacheTrendPoint } from '@/api/leaderboard'

const { t } = useI18n()

const props = defineProps<{
  /** `insights.cache_today`；命中率两档都下发，两个绝对量只有 named 档有。 */
  cache: LeaderboardCacheToday | null
  /** `insights.cache_trend_14`；为 null 或空时只少一条折线，不影响大号数字。 */
  points: LeaderboardCacheTrendPoint[] | null
}>()

/** cache_hit_rate 在两个来源里都是 0–1 的比率，整块按一位小数的百分比渲染。 */
const values = computed(() =>
  (props.points ?? [])
    .map((point) => point.cache_hit_rate * 100)
    .filter((value) => Number.isFinite(value)),
)

/** 大号数字优先用今日命中率；旧后端只给趋势时退到末点，两个都没有则整块不渲染。 */
const rateTarget = computed(() => {
  const today = props.cache?.cache_hit_rate
  if (typeof today === 'number') return today * 100
  return values.value[values.value.length - 1] ?? 0
})
const hasRate = computed(
  () => typeof props.cache?.cache_hit_rate === 'number' || values.value.length > 0,
)

const { current: rate } = useCountUp(rateTarget)
const rateText = computed(() => rate.value.toFixed(1))

/** `YYYY-MM-DD` → `MM-DD`：折线两端的刻度。 */
function shortDate(date: string): string {
  return date.length >= 10 ? date.slice(5) : date
}

const axis = computed(() => {
  const list = props.points ?? []
  if (list.length < 2) return null
  return { from: shortDate(list[0].date), to: shortDate(list[list.length - 1].date) }
})

/**
 * 大号数字下面那一格：有趋势时只是一个「今日」标签（`区间 a–b%` 已删，
 * 那两个数右边的折线已经画出来了），只有今日一个数时退到按档位二选一的口径句。
 * 这里用定值而不是 countUp 的中间值——一句话不该跟着数字跳。
 */
const subText = computed(() => {
  if (values.value.length >= 2) return t('leaderboard.cacheTrend.range')
  const hits = props.cache?.cache_read_tokens
  const inputs = props.cache?.input_tokens
  if (typeof hits === 'number' && typeof inputs === 'number') {
    return t('leaderboard.cacheTrend.subNamed', {
      hits: formatCompactNumberTrimmed(hits),
      inputs: formatCompactNumberTrimmed(inputs),
    })
  }
  return t('leaderboard.cacheTrend.sub', { rate: rateTarget.value.toFixed(1) })
})
</script>
