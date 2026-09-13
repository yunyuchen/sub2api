<template>
  <!-- 03 我的位置：左名次 + 模型偏好，右名次折线 + 我 vs 全站。
       viewer 里全是本人的真实数据，任何档位都不裁剪（design D21）；某一格没有数据时只隐藏那一格。 -->
  <!-- 右栏（名次折线 + 我 VS 全站）在没有名次历史时整块缺席：两栏栅格要随之收成一栏，
       MUST NOT 让左栏占三成、右侧留一片空白。 -->
  <div
    class="rp-whoami"
    :class="rankPoints.length > 0 ? '' : 'is-single'"
    data-testid="leaderboard-whoami"
  >
    <div class="rp-r">
      <span class="rp-eyebrow">{{ t('leaderboard.whoami.rankEyebrow') }}</span>

      <template v-if="myRank">
        <div class="rp-rank-v">#{{ myRank.rank }}</div>
        <div class="rp-of">{{ t('leaderboard.myRank.participants', { count: participantLabel }) }}</div>
        <p v-if="hintText" class="rp-hint" data-testid="leaderboard-whoami-hint">{{ hintText }}</p>
      </template>

      <!-- 快照还在算的时候没有名次可给，那不是「本窗口暂无用量」，不能说成后者。 -->
      <template v-else-if="computing">
        <div class="rp-rank-v" data-testid="leaderboard-whoami-computing">—</div>
        <div class="rp-of">{{ t('leaderboard.states.computing') }}</div>
        <p class="rp-hint">{{ t('leaderboard.states.computingHint') }}</p>
      </template>

      <template v-else>
        <div class="rp-rank-v" data-testid="leaderboard-whoami-no-usage">—</div>
        <div class="rp-of">{{ t('leaderboard.myRank.noUsage') }}</div>
        <p class="rp-hint">{{ t('leaderboard.myRank.noUsageHint') }}</p>
        <p class="rp-hint">
          {{ t('leaderboard.myRank.participants', { count: participantLabel }) }}
        </p>
      </template>

      <span class="rp-private">
        <LbIcon name="eye-off" :size="12" />{{ t('leaderboard.whoami.note') }}
      </span>

      <div v-if="models.length > 0" class="rp-subblock" data-testid="leaderboard-whoami-models">
        <span class="rp-eyebrow">{{ t('leaderboard.whoami.models') }}</span>
        <div class="rp-chips">
          <span
            v-for="(model, index) in models"
            :key="model.model"
            class="rp-chip"
            :class="index === 0 ? 'is-lead' : ''"
          >
            {{ model.model }}<span class="rp-chip-p">{{ model.share_percent }}%</span>
          </span>
        </div>
      </div>
    </div>

    <div
      v-if="rankPoints.length > 0"
      class="rp-chartwrap rp-r"
      style="--i: 1"
      data-testid="leaderboard-whoami-rank-history"
    >
      <span class="rp-eyebrow">{{ rankTrendLabel }}</span>
      <LbSparkline :points="rankPoints" invert :label="rankTrendLabel" :height="96" />
      <div class="rp-axis">
        <span class="rp-n">{{ firstDate }}</span>
        <span>{{ rankNarrative }}</span>
        <span class="rp-n">{{ lastDate }}</span>
      </div>

      <div v-if="compareRows.length > 0" class="rp-subblock" data-testid="leaderboard-whoami-compare">
        <span class="rp-eyebrow">{{ t('leaderboard.whoami.compare') }}</span>
        <div class="rp-vs">
          <div v-for="(row, index) in compareRows" :key="row.key" class="rp-vs-row">
            <span>{{ row.label }}</span>
            <span class="rp-vs-bars">
              <span class="rp-vs-line">
                <span class="rp-vs-t">{{ t('leaderboard.whoami.meLabel') }}</span>
                <span class="rp-vs-me">
                  <i class="rp-grow" :style="{ width: `${row.mineWidth}%`, '--i': index }"></i>
                </span>
              </span>
              <span class="rp-vs-line">
                <span class="rp-vs-t">{{ t('leaderboard.whoami.siteLabel') }}</span>
                <span class="rp-vs-all">
                  <i class="rp-grow" :style="{ width: `${row.siteWidth}%`, '--i': index + 1 }"></i>
                </span>
              </span>
            </span>
            <span class="rp-vs-val">
              {{ row.mine }}<em v-if="row.site">{{ row.site }}</em>
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LbIcon from './LbIcon.vue'
import LbSparkline from './LbSparkline.vue'
import {
  formatCompactNumberTrimmed,
  formatCurrency,
  formatNumberLocaleString,
} from '@/utils/format'
import type {
  LeaderboardMyRank,
  LeaderboardSiteSummary,
  LeaderboardViewer,
} from '@/api/leaderboard'

const { t } = useI18n()

const props = defineProps<{
  /** 零用量时为 null。 */
  myRank: LeaderboardMyRank | null
  /** named 档与 Preview 下是精确整数，anonymous 档下是分档字符串（如 `100+`）。 */
  participantCount: number | string
  /** 抑制态（anonymous 档参与人数过少）：只剩本人行，提示语换成抑制态的说明。 */
  suppressed?: boolean
  /** 快照尚未生成：这一格没有名次可给，但 viewer 的三格照常渲染。 */
  computing?: boolean
  /** 本人数据；旧后端不下发时为 null，对应的几格一并隐藏。 */
  viewer: LeaderboardViewer | null
  /** 全站对照值，取 highlights.site 的两个比率；highlights 不可用时为 null。 */
  site: LeaderboardSiteSummary | null
}>()

/**
 * 提示语里的目标名次。后端把它固化在 kind 名字里（`tokens_to_top10`）而不单独下发，
 * 因此这里也只能是同一个常量——改动必须与 `leaderboardHintTopN` 一起改。
 */
const HINT_TOP_N = 10

const participantLabel = computed(() =>
  typeof props.participantCount === 'number'
    ? formatNumberLocaleString(props.participantCount)
    : props.participantCount,
)

/**
 * 提示语按后端下发的 kind 选文案：后端只给数值，档位裁剪也在后端完成——
 * `anonymous` 档拿到的是 `relative_percent` 形态，里面只有相对第一名的百分比，
 * 不含任何他人绝对量。量词由 kind 自带，MUST NOT 再去看选中的 Metric。
 * 所需数值缺席时整行隐藏，MUST NOT 拼一句半截话。
 */
const hintText = computed(() => {
  if (props.suppressed) return t('leaderboard.myRank.suppressedHint')

  // hint 缺席（已经在前 10 之内，或参与人数不足 10 人）时整行不渲染：
  // 「已进前 10」是名次那个大数字已经说完的事，再说一遍只是多一行字。
  const hint = props.myRank?.hint
  if (!hint) return ''

  // 三个 `*_to_top10` 形态共用一句「再 X 即可进入前 N」，量词与格式都由 kind 决定：
  // tokens 用紧凑数字、成功请求用千分位、金额用 `formatCurrency`（`value` 的单位是 USD）。
  if (
    hint.kind === 'tokens_to_top10' ||
    hint.kind === 'requests_to_top10' ||
    hint.kind === 'cost_to_top10'
  ) {
    const value = hint.value
    if (typeof value !== 'number') return ''
    if (hint.kind === 'tokens_to_top10') {
      return t('leaderboard.myRank.hint.gapTokens', {
        gap: formatCompactNumberTrimmed(value),
        rank: HINT_TOP_N,
      })
    }
    if (hint.kind === 'requests_to_top10') {
      return t('leaderboard.myRank.hint.gapRequests', {
        gap: formatNumberLocaleString(value),
        rank: HINT_TOP_N,
      })
    }
    return t('leaderboard.myRank.hint.gapCost', {
      gap: formatCurrency(value),
      rank: HINT_TOP_N,
    })
  }

  if (typeof hint.self !== 'number' || typeof hint.tenth !== 'number') return ''
  return t('leaderboard.myRank.hint.relative', {
    percent: hint.self,
    rank: HINT_TOP_N,
    target: hint.tenth,
  })
})

/** 名次走势：折线按 `invert` 画（名次越小越高），两端是首末日期。 */
const rankHistory = computed(() => props.viewer?.rank_history ?? [])
const rankPoints = computed(() => rankHistory.value.map((point) => point.rank))
const firstDate = computed(() => rankHistory.value[0]?.date ?? '')
const lastDate = computed(() => rankHistory.value[rankHistory.value.length - 1]?.date ?? '')

/**
 * 折线标题里的窗口长度取实际拿到的点数：折线画几个点就说几天，
 * MUST NOT 写死 14——新用户只有两三天历史时两句话会互相打脸。
 * 「数值越小越好」那句坐标轴说明已删：折线下方的「最好 #a · 最差 #b」已经把方向说清楚了。
 */
const rankTrendLabel = computed(() =>
  t('leaderboard.whoami.rankTrend', { span: rankPoints.value.length }),
)

/**
 * 折线下面那句话由数据现算，只剩两个数：最好名次与最差名次。
 * 「近 N 天有 M 天在最好名次上」已删——折线本身就把每一天画出来了。
 */
const rankNarrative = computed(() => {
  const points = rankPoints.value
  if (points.length === 0) return ''
  return t('leaderboard.whoami.rankSummary', {
    best: Math.min(...points),
    worst: Math.max(...points),
  })
})

const models = computed(() => props.viewer?.models ?? [])

interface CompareRow {
  key: string
  label: string
  mine: string
  site: string
  mineWidth: number
  siteWidth: number
}

/** 双条按两者中的较大值归一：较大的那条占满，另一条按比例，MUST NOT 各自归一。 */
function widths(mine: number, site: number | null): { mineWidth: number; siteWidth: number } {
  const peak = Math.max(mine, site ?? 0)
  if (peak <= 0) return { mineWidth: 0, siteWidth: 0 }
  return {
    mineWidth: Math.max(1, Math.round((mine / peak) * 1000) / 10),
    siteWidth: site === null ? 0 : Math.max(1, Math.round((site / peak) * 1000) / 10),
  }
}

const compareRows = computed<CompareRow[]>(() => {
  const rows: CompareRow[] = []

  const mineCache = props.viewer?.cache_hit_rate
  if (typeof mineCache === 'number') {
    const siteCache = typeof props.site?.cache_hit_rate === 'number' ? props.site.cache_hit_rate : null
    rows.push({
      key: 'cacheHitRate',
      label: t('leaderboard.whoami.cacheHitRate'),
      mine: `${Math.round(mineCache * 100)}%`,
      site:
        siteCache === null
          ? ''
          : t('leaderboard.whoami.siteValue', { value: `${Math.round(siteCache * 100)}%` }),
      ...widths(mineCache, siteCache),
    })
  }

  const mineAvg = props.viewer?.avg_tokens_per_request
  if (typeof mineAvg === 'number') {
    const siteAvg =
      typeof props.site?.avg_tokens_per_request === 'number' ? props.site.avg_tokens_per_request : null
    rows.push({
      key: 'avgTokens',
      label: t('leaderboard.whoami.avgTokens'),
      mine: formatCompactNumberTrimmed(Math.round(mineAvg)),
      site:
        siteAvg === null
          ? ''
          : t('leaderboard.whoami.siteValue', {
              value: formatCompactNumberTrimmed(Math.round(siteAvg)),
            }),
      ...widths(mineAvg, siteAvg),
    })
  }

  return rows
})
</script>
