<template>
  <!-- 05 左栏：今日全站按模型的成功请求。数据整块缺席（预聚合表没行 / 旧后端）时不渲染
       任何东西，由父级的章 v-if 兜底。 -->
  <div v-if="rows.length" data-testid="leaderboard-model-heat">
    <span class="rp-eyebrow">{{ eyebrow }}</span>
    <ul class="rp-mlist">
      <li
        v-for="(row, index) in rows"
        :key="row.model"
        class="rp-r"
        :style="{ '--i': index }"
        data-testid="leaderboard-model-heat-row"
      >
        <span class="rp-nm">
          <span>{{ row.model }}</span>
          <span class="rp-bar" aria-hidden="true">
            <span
              class="rp-bar-fill rp-grow"
              :style="{ width: `${barPercent(row)}%`, '--i': index }"
            ></span>
          </span>
        </span>
        <!-- 「档位决定字段是否存在」：anonymous 档没有站点级绝对量，这一格整格留空。 -->
        <span class="rp-n rp-c">{{ countText(row) }}</span>
        <span class="rp-n rp-p">{{ row.share_percent }}%</span>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatNumberLocaleString } from '@/utils/format'
import type { LeaderboardModelUsage } from '@/api/leaderboard'

const { t } = useI18n()

const props = defineProps<{
  /** `insights.models_today`；为 null 或空时整块不渲染。 */
  models: LeaderboardModelUsage[] | null
}>()

/** 只展示 Top 8，后端已按次数倒序。 */
const rows = computed(() => (props.models ?? []).slice(0, 8))

const eyebrow = computed(() => t('leaderboard.insights.modelHeat.title'))

/**
 * 计数与榜单同口径（带成功落账过滤），与热力图 / 时段分布的裸计数不同。
 * anonymous 档只有占比，右边那一列已经写着占比，这里就不再重复一遍。
 */
function countText(row: LeaderboardModelUsage): string {
  if (typeof row.successful_requests !== 'number') return ''
  return formatNumberLocaleString(row.successful_requests)
}

/** 条长相对第一名，两档都用 share_percent 算，避免匿名档没有绝对量时条全是 0。 */
function barPercent(row: LeaderboardModelUsage): number {
  const top = rows.value[0]?.share_percent ?? 0
  if (top <= 0) return 0
  return Math.max(0, Math.min(100, Math.round((row.share_percent / top) * 100)))
}
</script>
