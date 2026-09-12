<template>
  <!-- 07 下半左：今日四类 tokens 的构成。四段占比两档都下发，四个绝对量只有 named 档
       与 Preview 才有。整块为 null 时不渲染。四段相加即当日总 tokens。 -->
  <div v-if="segments.length" class="rp-r" style="--i: 2" data-testid="leaderboard-composition">
    <span class="rp-eyebrow">{{ t('leaderboard.composition.note') }}</span>

    <div class="rp-stackbar" aria-hidden="true">
      <i
        v-for="(segment, index) in segments"
        :key="segment.key"
        class="rp-grow"
        :class="segment.tone"
        :style="{ flex: `${segment.percent} 0 0`, '--i': index }"
        data-testid="leaderboard-composition-segment"
      ></i>
    </div>

    <ul class="rp-legend">
      <li
        v-for="segment in segments"
        :key="segment.key"
        data-testid="leaderboard-composition-legend"
      >
        <span class="rp-sw" :class="segment.tone" aria-hidden="true"></span>
        <span>{{ segment.label }}</span>
        <span class="rp-legend-v">{{ segment.percent }}%</span>
        <em v-if="segment.amount">{{ segment.amount }}</em>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatCompactNumberTrimmed } from '@/utils/format'
import type { LeaderboardComposition } from '@/api/leaderboard'

const { t } = useI18n()

const props = defineProps<{
  /** `insights.composition_today`；为 null 时整块不渲染。 */
  composition: LeaderboardComposition | null
}>()

interface CompositionSegment {
  key: string
  label: string
  percent: number
  tone: string
  /** `anonymous` 档缺席；MUST NOT 把缺席当成 0。 */
  amount: string
}

/** 四段的次序照 mockup，配色是同一个色相的四级明度（输入最深，缓存读取最浅）。 */
const segments = computed<CompositionSegment[]>(() => {
  const value = props.composition
  if (!value) return []
  const source = [
    { key: 'input', percent: value.input_percent, tokens: value.input_tokens },
    { key: 'output', percent: value.output_percent, tokens: value.output_tokens },
    {
      key: 'cacheCreation',
      percent: value.cache_creation_percent,
      tokens: value.cache_creation_tokens,
    },
    { key: 'cacheRead', percent: value.cache_read_percent, tokens: value.cache_read_tokens },
  ]
  return source.map((item, index) => ({
    key: item.key,
    label: t(`leaderboard.composition.${item.key}`),
    percent: item.percent,
    tone: `rp-sw${index + 1}`,
    amount: typeof item.tokens === 'number' ? formatCompactNumberTrimmed(item.tokens) : '',
  }))
})
</script>
