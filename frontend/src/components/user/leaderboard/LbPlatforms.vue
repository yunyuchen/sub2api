<template>
  <!-- 05 下半通栏：今日成功请求按平台。整块为 null（旧后端 / 来源缺行）或空数组时不渲染。
       条是同一个色相的四级明度，不换色相。 -->
  <div v-if="segments.length" class="rp-plat" data-testid="leaderboard-platforms">
    <span class="rp-eyebrow">{{ t('leaderboard.platforms.note') }}</span>

    <div class="rp-stackbar" aria-hidden="true">
      <i
        v-for="(segment, index) in segments"
        :key="segment.key"
        class="rp-grow"
        :class="segment.tone"
        :style="{ flex: `${segment.share_percent} 0 0`, '--i': index }"
        data-testid="leaderboard-platforms-segment"
      ></i>
    </div>

    <ul class="rp-legend">
      <li
        v-for="segment in segments"
        :key="segment.key"
        data-testid="leaderboard-platforms-legend"
      >
        <span class="rp-sw" :class="segment.tone" aria-hidden="true"></span>
        <span>{{ segment.label }}</span>
        <span class="rp-legend-v">{{ segment.share_percent }}%</span>
        <em v-if="segment.note">{{ segment.note }}</em>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatNumberLocaleString } from '@/utils/format'
import type { LeaderboardPlatform } from '@/api/leaderboard'

const { t } = useI18n()

const props = defineProps<{
  /** `insights.platforms_today`；为 null 或空时整块不渲染。 */
  platforms: LeaderboardPlatform[] | null
}>()

/** 同色相四级明度，超出四段后循环；残差项固定用最弱的一档。 */
const TONES = ['rp-sw1', 'rp-sw2', 'rp-sw3', 'rp-sw4'] as const

interface PlatformSegment {
  key: string
  label: string
  share_percent: number
  tone: string
  /** 右侧小字：实名档是成功请求数；残差项与匿名档都没有这一段。 */
  note: string
}

/**
 * 图例的最后一项「其他」只在给出的各项占比之和小于 100 时出现（各项都取整，和常常差 1–2）。
 * 它是取整残差而不是一个平台，因此 MUST NOT 为它编一个请求数。
 */
const segments = computed<PlatformSegment[]>(() => {
  const source = props.platforms ?? []
  if (source.length === 0) return []

  const list: PlatformSegment[] = source.map((platform, index) => ({
    key: platform.platform,
    label: platform.platform,
    share_percent: platform.share_percent,
    tone: TONES[index % TONES.length],
    note:
      typeof platform.successful_requests === 'number'
        ? t('leaderboard.platforms.legendCount', {
            count: formatNumberLocaleString(platform.successful_requests),
          })
        : '',
  }))

  const sum = list.reduce((total, segment) => total + segment.share_percent, 0)
  if (sum < 100) {
    list.push({
      key: '__residual__',
      label: t('leaderboard.platforms.other'),
      share_percent: Math.round((100 - sum) * 10) / 10,
      tone: TONES[TONES.length - 1],
      // 「取整残差」那句解释已删：图例上一个 `其他 1%` 已经说完了它是什么。
      note: '',
    })
  }
  return list
})
</script>
