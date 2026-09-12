<template>
  <!-- 06 右：今日 24 个小时桶（与近 30 天热力图同源），柱下只留峰值那一句。
       「主要落在 HH:00 — HH:00」、由 7 × 24 矩阵与 30 天数据现算的节奏说明、
       以及预聚合口径那句注脚都已删——柱形本身已经把这三件事画出来了。 -->
  <div v-if="buckets.length" class="rp-r" style="--i: 2" data-testid="leaderboard-hourly">
    <span class="rp-eyebrow">{{ t('leaderboard.insights.hourly.title') }}</span>

    <div class="rp-hbars">
      <!-- 级联按 4 小时一档，逐根柱递延会让 24 根柱的入场拖得太长。 -->
      <span
        v-for="bucket in buckets"
        :key="bucket.hour"
        class="rp-b rp-growy"
        :class="barTone(bucket)"
        :style="{ height: `${barHeight(bucket)}%`, '--i': Math.floor(bucket.hour / 4) }"
        :title="barTitle(bucket)"
        :data-testid="isPeak(bucket) ? 'leaderboard-hourly-peak' : 'leaderboard-hourly-bar'"
      ></span>
    </div>

    <div class="rp-hticks" aria-hidden="true">
      <span
        v-for="hour in HOURS_PER_DAY"
        :key="hour"
        :class="peakBucket && peakBucket.hour === hour - 1 ? 'is-on' : ''"
      >
        {{ (hour - 1) % 3 === 0 ? hour - 1 : '' }}
      </span>
    </div>

    <p v-if="peakLine" class="rp-peaknote" data-testid="leaderboard-hourly-callout">
      <LbIcon name="pulse" :size="13" />{{ peakLine }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LbIcon from './LbIcon.vue'
import { formatNumberLocaleString } from '@/utils/format'
import type { LeaderboardHourlyBucket } from '@/api/leaderboard'

const { t } = useI18n()

const props = defineProps<{
  /** `insights.hourly_today`；为 null 或空时整块不渲染。桶边界已是站点时区，不再做映射。 */
  hourly: LeaderboardHourlyBucket[] | null
}>()

const HOURS_PER_DAY = 24

const buckets = computed(() =>
  [...(props.hourly ?? [])].sort((left, right) => left.hour - right.hour),
)

const peakPercent = computed(() =>
  buckets.value.reduce((max, bucket) => Math.max(max, bucket.relative_percent), 0),
)

/** 峰值桶：后端把峰值那一桶定为 100，这里仍按实际最大值判定，避免来源没归一时全无高亮。 */
const peakBucket = computed(() => buckets.value.find((bucket) => isPeak(bucket)) ?? null)

function isPeak(bucket: LeaderboardHourlyBucket): boolean {
  return peakPercent.value > 0 && bucket.relative_percent === peakPercent.value
}

/** 0 的那几个小时画成一条底线而不是留空：空白读起来像「缺数据」。 */
function barTone(bucket: LeaderboardHourlyBucket): string {
  if (bucket.relative_percent <= 0) return 'is-zero'
  return isPeak(bucket) ? 'is-peak' : ''
}

function barHeight(bucket: LeaderboardHourlyBucket): number {
  return Math.max(0, Math.min(100, bucket.relative_percent))
}

function paddedHour(hour: number): string {
  return `${hour}`.padStart(2, '0')
}

function barTitle(bucket: LeaderboardHourlyBucket): string {
  if (typeof bucket.requests === 'number') {
    return t('leaderboard.insights.hourly.barRequests', {
      hour: paddedHour(bucket.hour),
      count: formatNumberLocaleString(bucket.requests),
    })
  }
  return t('leaderboard.insights.hourly.barRelative', {
    hour: paddedHour(bucket.hour),
    percent: bucket.relative_percent,
  })
}

/**
 * 柱下唯一那一句：named 档是「峰值 HH:00 · N 次」，anonymous 档只有小时本身。
 * 达到门槛的时段区间那半句已删：24 根柱已经把「忙的时段在哪一段」画出来了。
 */
const peakLine = computed(() => {
  const peak = peakBucket.value
  if (!peak) return ''
  return typeof peak.requests === 'number'
    ? t('leaderboard.insights.hourly.peakWithRequests', {
        hour: paddedHour(peak.hour),
        count: formatNumberLocaleString(peak.requests),
      })
    : t('leaderboard.insights.hourly.peak', { hour: paddedHour(peak.hour) })
})
</script>
