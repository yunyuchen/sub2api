<template>
  <!-- 04 榜单：hairline 全宽表，没有卡片也没有斑马纹。名次列补零对齐，本人行靠左侧色条 +
       淡底 + 「你」标记区分，相对第一名是细条 + 百分比（窄屏折成第二行，条 MUST NOT 隐藏）。 -->
  <div class="rp-lb" role="table" data-testid="leaderboard-rank-list">
    <div class="rp-lb-head rp-lb-row" role="row">
      <span role="columnheader">{{ t('leaderboard.table.rank') }}</span>
      <span role="columnheader">{{ t('leaderboard.table.user') }}</span>
      <span role="columnheader">{{ t('leaderboard.table.relativeToTop') }}</span>
      <span
        class="rp-right"
        role="columnheader"
        data-testid="leaderboard-col-total-tokens"
        :aria-sort="metric === 'total_tokens' ? 'descending' : 'none'"
      >
        {{ t('leaderboard.table.totalTokens') }}
      </span>
      <span
        class="rp-right"
        role="columnheader"
        data-testid="leaderboard-col-successful-requests"
        :aria-sort="metric === 'successful_requests' ? 'descending' : 'none'"
      >
        {{ t('leaderboard.table.successfulRequestsShort') }}
      </span>
      <span
        class="rp-right"
        role="columnheader"
        data-testid="leaderboard-col-cost"
        :aria-sort="metric === 'cost' ? 'descending' : 'none'"
      >
        {{ t('leaderboard.table.cost') }}
      </span>
    </div>

    <!-- 前三名的强调形态按 Rank 值判定，与皮肤无关：并列第 1 时两行都是首行的待遇，
         跳号之后的名次不是。MUST NOT 按行下标或 Ordinal。 -->
    <div
      v-for="row in rows"
      :key="row.entry.ordinal"
      class="rp-lb-row rp-r"
      :class="[row.entry.rank <= 3 ? 'is-top' : '', row.entry.is_self ? 'is-self' : '']"
      role="row"
      :style="{ '--i': row.cascade }"
      :data-testid="row.entry.is_self ? 'leaderboard-row-self' : 'leaderboard-row'"
    >
      <span
        class="rp-lb-rank"
        role="cell"
        :data-testid="row.entry.rank <= 3 ? 'leaderboard-rank-top' : 'leaderboard-rank-plain'"
      >
        {{ paddedRank(row.entry.rank) }}
      </span>

      <span class="rp-lb-name" role="cell">
        <span :class="isAnonymous(row.entry) ? 'rp-anon' : ''">{{ displayName(row.entry) }}</span>
        <span v-if="row.entry.is_self" class="rp-you">{{ t('leaderboard.table.selfBadge') }}</span>
      </span>

      <!-- 相对第一名算不出时（匿名档本人行：第一名的绝对量前端看不到）留占位符，
           MUST NOT 把「算不出」画成 0%——那一行恰恰是最该看清的一行。 -->
      <span class="rp-lb-barcell rp-barwrap" role="cell">
        <span class="rp-bar" aria-hidden="true">
          <span
            v-if="row.bar !== null"
            class="rp-bar-fill rp-grow"
            :style="{ width: `${row.bar}%`, '--i': row.cascade }"
          ></span>
        </span>
        <span
          class="rp-pc"
          :title="row.bar === null ? t('leaderboard.table.relativeUnknown') : undefined"
        >
          {{ row.bar === null ? '—' : `${row.bar}%` }}
        </span>
      </span>

      <span
        class="rp-lb-num rp-lb-tok"
        :class="metric === 'total_tokens' ? '' : 'is-dim'"
        role="cell"
        :title="relativeTitle(row.entry, 'total_tokens')"
      >
        {{ metricText(row.entry, 'total_tokens') }}
      </span>
      <span
        class="rp-lb-num rp-lb-req"
        :class="metric === 'successful_requests' ? '' : 'is-dim'"
        role="cell"
        :title="relativeTitle(row.entry, 'successful_requests')"
      >
        {{ metricText(row.entry, 'successful_requests') }}
      </span>
      <span
        class="rp-lb-num rp-lb-cost"
        :class="metric === 'cost' ? '' : 'is-dim'"
        role="cell"
        :title="relativeTitle(row.entry, 'cost')"
      >
        {{ metricText(row.entry, 'cost') }}
      </span>
    </div>
  </div>

  <!-- 表下只剩一行解读句，两个数：第 1 名是第 2 名的几倍、前三名占全站多少。
       分句缺数据就整句省略，MUST NOT 拼半截话，也 MUST NOT 写死示例数字；两档同形。
       互换那句与「共 N 位活跃 / 竞赛排名 / 本人行标记」整段注脚已删——
       前者是换个口径才成立的假设，后者说的是表格自己已经画出来的东西。 -->
  <div class="rp-lb-foot">
    <p v-if="readout" data-testid="leaderboard-rank-readout">{{ readout }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  formatCompactNumberTrimmed,
  formatCurrency,
  formatNumberLocaleString,
} from '@/utils/format'
import { useLeaderboardDisplayName } from './displayName'
import type {
  LeaderboardEntry,
  LeaderboardMetric,
  LeaderboardSiteSummary,
} from '@/api/leaderboard'

const { t } = useI18n()

const props = withDefaults(
  defineProps<{
    entries: LeaderboardEntry[]
    /** 选中的排名指标：三列都渲染，只有这一列被高亮，相对条也按它算。 */
    metric: LeaderboardMetric
    /**
     * `highlights.site`：解读句里「前三名占全站 N%」的分母。
     * `anonymous` 档下站点级绝对量缺席，那一句随之省略（MUST NOT 用榜内合计冒充全站）。
     */
    site?: LeaderboardSiteSummary | null
  }>(),
  { site: null },
)

/** 名次补零到两位：`01`。三位数名次原样输出，不截断。 */
function paddedRank(rank: number): string {
  return `${rank}`.padStart(2, '0')
}

/** 金额那一列的绝对量是 USD 浮点，与另两列的整数走同一套「缺席 / 相对百分比」规则。 */
function absolute(entry: LeaderboardEntry, column: LeaderboardMetric): number | undefined {
  if (column === 'total_tokens') return entry.total_tokens
  if (column === 'successful_requests') return entry.successful_requests
  return entry.cost
}

function relative(entry: LeaderboardEntry, column: LeaderboardMetric): number | undefined {
  if (column === 'total_tokens') return entry.total_tokens_relative_percent
  if (column === 'successful_requests') return entry.successful_requests_relative_percent
  return entry.cost_relative_percent
}

/**
 * 一列里是不是两种量纲混着出现。`anonymous` 档就是这个形态：后端给**本人行**绝对量、
 * 给**他人行**相对第一名的百分比（leaderboard_service.go 的 `renderNamed || item.isSelf`）。
 */
function detectMixedScale(column: LeaderboardMetric): boolean {
  let absoluteOnly = false
  let relativeOnly = false
  for (const item of props.entries) {
    const hasAbsolute = typeof absolute(item, column) === 'number'
    const hasRelative = typeof relative(item, column) === 'number'
    if (hasAbsolute && !hasRelative) absoluteOnly = true
    else if (hasRelative && !hasAbsolute) relativeOnly = true
  }
  return absoluteOnly && relativeOnly
}

const mixedScale = computed<Record<LeaderboardMetric, boolean>>(() => ({
  total_tokens: detectMixedScale('total_tokens'),
  successful_requests: detectMixedScale('successful_requests'),
  cost: detectMixedScale('cost'),
}))

/**
 * 比较用的量级。两种形态混着出现时（`anonymous` 档）只认相对百分比那把公共尺子，
 * 只有绝对量的那一行记为 null——拿「4.37M tokens」去除以「62(%)」得出的倍数是假的，
 * 任何牵涉到它的分句宁可整句省略（增补 T5：缺什么省什么）。
 */
function magnitude(entry: LeaderboardEntry, column: LeaderboardMetric): number | null {
  const value = absolute(entry, column)
  const percent = relative(entry, column)
  if (mixedScale.value[column]) return typeof percent === 'number' ? percent : null
  if (typeof value === 'number') return value
  return typeof percent === 'number' ? percent : null
}

function isAnonymous(entry: LeaderboardEntry): boolean {
  return !entry.is_self && entry.identity.kind !== 'named'
}

/**
 * 展示名一律由前端渲染：规则收敛在 `displayName.ts`（本人优先自己的昵称，
 * named 用后端给的 username，其余是「第 N 位」假名），用户 id 永不出现在这里。
 */
const { displayName } = useLeaderboardDisplayName()

/**
 * 「档位决定字段是否存在」：anonymous 档他人条目没有绝对值，只有相对百分比。
 * 两者都缺席时给占位符，MUST NOT 把缺席当成 0。
 */
function metricText(entry: LeaderboardEntry, column: LeaderboardMetric): string {
  const value = absolute(entry, column)
  if (typeof value === 'number') {
    if (column === 'total_tokens') return formatCompactNumberTrimmed(value)
    if (column === 'successful_requests') return formatNumberLocaleString(value)
    // 金额是 USD，带货币符号与语言环境（`formatCurrency` 对不足 0.01 的金额自动加小数位）。
    return formatCurrency(value)
  }

  const percent = relative(entry, column)
  if (typeof percent === 'number') return `${percent}%`
  return '—'
}

/** 相对值那一档的数字本身只有「%」，完整语义放在 title 里。 */
function relativeTitle(entry: LeaderboardEntry, column: LeaderboardMetric): string | undefined {
  if (typeof absolute(entry, column) === 'number') return undefined
  const percent = relative(entry, column)
  if (typeof percent !== 'number') return undefined
  return t('leaderboard.table.relativePercent', { percent })
}

/**
 * 细条按选中的 Metric 相对第一名；匿名档他人行直接用后端给的相对百分比。
 * 只有绝对量时才回退到「自己 / 第一名」——匿名档下第一名的绝对量前端拿不到
 * （除非本人自己就是第一名），那时返回 null 走占位符，MUST NOT 返回 0。
 */
function barPercent(entry: LeaderboardEntry): number | null {
  const percent = relative(entry, props.metric)
  if (typeof percent === 'number') return Math.max(0, Math.min(100, percent))

  const value = absolute(entry, props.metric)
  const top = props.entries[0] ? absolute(props.entries[0], props.metric) : undefined
  if (typeof value !== 'number' || typeof top !== 'number' || top <= 0) return null
  return Math.max(0, Math.min(100, Math.round((value / top) * 100)))
}

/**
 * 级联下标按 4 行一档：榜单最长 50 行，逐行递延会让最后一行等到 ~2.9s 才入场，
 * 那时读者早已读到中段。与同目录其它组件（趋势按 3 天、时段按 4 小时）同一套做法。
 */
const CASCADE_ROWS_PER_STEP = 4

interface RankRow {
  entry: LeaderboardEntry
  cascade: number
  /** 相对第一名的百分比；算不出时是 null（占位符 + 不画条）。 */
  bar: number | null
}

const rows = computed<RankRow[]>(() =>
  props.entries.map((entry, index) => ({
    entry,
    cascade: Math.floor(index / CASCADE_ROWS_PER_STEP),
    bar: barPercent(entry),
  })),
)

/** 一位小数的倍数：`1.6` 倍。 */
function ratioText(value: number): string {
  return value >= 10 ? `${Math.round(value)}` : `${Math.round(value * 10) / 10}`
}

/**
 * 第 1 名是第 2 名的几倍。两个量级都要在且大于 0，否则整句省略。
 * 第 1 名的绝对量与「是末名的几倍」都已删：前者就印在表格第一行，后者末名是谁本来就看得见。
 */
const leadClause = computed(() => {
  const list = props.entries
  if (list.length < 2) return ''
  const top = magnitude(list[0], props.metric)
  const second = magnitude(list[1], props.metric)
  if (!top || !second || second <= 0) return ''
  return t('leaderboard.rank.readout.lead', { ratio: ratioText(top / second) })
})

/**
 * 前三名占全站多少。分母是站点级绝对量（`highlights.site`，cost 用 `site.cost`），
 * 匿名档缺席该字段——榜内合计只能算出「占前 50 名」，那是另一个口径，MUST NOT 顶替。
 */
const shareClause = computed(() => {
  const site = props.site
  const total =
    props.metric === 'total_tokens'
      ? site?.total_tokens
      : props.metric === 'successful_requests'
        ? site?.successful_requests
        : site?.cost
  if (typeof total !== 'number' || total <= 0) return ''

  // 不足三行时这句话的主语不成立（「前三名」得真有三名），整句省略。
  const head = props.entries.slice(0, 3)
  if (head.length < 3) return ''
  let sum = 0
  for (const entry of head) {
    const value = absolute(entry, props.metric)
    if (typeof value !== 'number') return ''
    sum += value
  }
  const percent = Math.min(100, Math.round((sum / total) * 100))
  return t('leaderboard.rank.readout.topThreeShare', { percent })
})

/**
 * 解读句 = 两个分句用 ` · ` 连起来，缺数据的分句整句省略（两句都缺时整行不渲染）。
 * 分句自身不带句末标点：这是一行标签式的读数，不是一段话。
 */
const readout = computed(() =>
  [leadClause.value, shareClause.value].filter(Boolean).join(' · '),
)
</script>
