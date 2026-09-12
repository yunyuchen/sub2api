<template>
  <!-- 02 六个之最：三列两行的 hairline 网格，每格「小标题 / 大数字 + 同行单位 / 用户」。
       整块为 null（status = computing 或旧后端）时不渲染，单项为 null 时那一格直接缺席——
       阶梯位移由 `--i` 现算，因此缺席的项不会在序列里留一个洞（非 today 窗口没有进步之星）。
       用户名一行下面只留「单次最大」的中位数倍数，「只在 today 窗口」那类注脚已删：
       进步之星本来就只在 today 窗口才有格子，缺席即是说明。 -->
  <ul v-if="items.length > 0" class="rp-sup" data-testid="leaderboard-extremes">
    <li
      v-for="(item, index) in items"
      :key="item.key"
      class="rp-r"
      :style="{ '--i': index }"
      :data-testid="item.testid"
    >
      <div class="rp-sup-t">{{ item.label }}</div>
      <div class="rp-sup-v">
        {{ item.value }}<small>{{ item.unit }}</small>
      </div>
      <div class="rp-sup-who">
        {{ item.who }}<em v-if="item.extra"> · {{ item.extra }}</em>
      </div>
    </li>
  </ul>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatCompactNumberTrimmed, formatNumberLocaleString } from '@/utils/format'
import { useLeaderboardDisplayName } from './displayName'
import type { LeaderboardExtremes } from '@/api/leaderboard'

const { t } = useI18n()

const props = defineProps<{
  /** 为 null 时整块不渲染（status = computing 随 highlights 一起不可用，或旧后端）。 */
  extremes: LeaderboardExtremes | null
}>()

/** 展示名与榜单条目、Highlights、Profiles 完全同一套规则，收敛在 `displayName.ts`。 */
const { displayName } = useLeaderboardDisplayName()

interface ExtremeItem {
  key: string
  testid: string
  label: string
  value: string
  unit: string
  who: string
  extra?: string
}

const items = computed<ExtremeItem[]>(() => {
  const extremes = props.extremes
  if (!extremes) return []
  const list: ExtremeItem[] = []

  if (extremes.night_owl) {
    list.push({
      key: 'nightOwl',
      testid: 'leaderboard-extreme-night-owl',
      label: t('leaderboard.extremes.nightOwl.label'),
      value: `${extremes.night_owl.night_share_percent}%`,
      unit: t('leaderboard.extremes.nightOwl.unit'),
      who: displayName(extremes.night_owl),
    })
  }

  if (extremes.rising) {
    list.push({
      key: 'rising',
      testid: 'leaderboard-extreme-rising',
      label: t('leaderboard.extremes.rising.label'),
      value: `+${extremes.rising.change_percent}%`,
      unit: t('leaderboard.extremes.rising.unit'),
      who: displayName(extremes.rising),
    })
  }

  if (extremes.omnivore) {
    list.push({
      key: 'omnivore',
      testid: 'leaderboard-extreme-omnivore',
      label: t('leaderboard.extremes.omnivore.label'),
      value: formatNumberLocaleString(extremes.omnivore.distinct_models),
      unit: t('leaderboard.extremes.omnivore.unit'),
      who: displayName(extremes.omnivore),
    })
  }

  if (extremes.talker) {
    list.push({
      key: 'talker',
      testid: 'leaderboard-extreme-talker',
      label: t('leaderboard.extremes.talker.label'),
      value: `${extremes.talker.output_share_percent}%`,
      unit: t('leaderboard.extremes.talker.unit'),
      who: displayName(extremes.talker),
    })
  }

  if (extremes.max_single) {
    // 「档位决定字段是否存在」：anonymous 档没有单次最大 tokens，只能说「是中位数的几倍」。
    const max = extremes.max_single
    const hasAbsolute = typeof max.max_single_tokens === 'number'
    list.push({
      key: 'maxSingle',
      testid: 'leaderboard-extreme-max-single',
      label: t('leaderboard.extremes.maxSingle.label'),
      value: hasAbsolute
        ? formatCompactNumberTrimmed(max.max_single_tokens ?? 0)
        : max.ratio_to_median.toFixed(1),
      unit: hasAbsolute
        ? t('leaderboard.extremes.maxSingle.unit')
        : t('leaderboard.extremes.maxSingle.ratioUnit'),
      who: displayName(max),
      extra: hasAbsolute
        ? t('leaderboard.extremes.maxSingle.medianNote', { ratio: max.ratio_to_median.toFixed(1) })
        : undefined,
    })
  }

  if (extremes.streak) {
    list.push({
      key: 'streak',
      testid: 'leaderboard-extreme-streak',
      label: t('leaderboard.extremes.streak.label'),
      value: formatNumberLocaleString(extremes.streak.days),
      unit: t('leaderboard.extremes.streak.unit'),
      who: displayName(extremes.streak),
    })
  }

  return list
})
</script>
