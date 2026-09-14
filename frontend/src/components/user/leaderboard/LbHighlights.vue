<template>
  <!-- 01 高亮：快照还在计算、或后端没下发 highlights 时走骨架，而不是三栏空壳。 -->
  <div v-if="!highlights" class="rp-lead-grid" data-testid="leaderboard-highlights-skeleton">
    <div v-for="index in 3" :key="index">
      <div class="rp-skel" style="width: 60%"></div>
      <div class="rp-skel" style="height: 26px; width: 80%"></div>
      <div class="rp-skel" style="width: 45%"></div>
    </div>
  </div>

  <div v-else class="rp-lead-grid" data-testid="leaderboard-highlights">
    <!-- 左：该窗口 tokens 第一，外加与第 2 名的距离 -->
    <div
      v-if="highlights.top_tokens"
      class="rp-lead rp-r"
      data-testid="leaderboard-highlight-top-tokens"
    >
      <span class="rp-eyebrow">
        {{
          t('leaderboard.highlights.topTokensEyebrow', {
            label: t(`leaderboard.highlights.topTokens.${activeWindow}`),
          })
        }}
      </span>
      <!-- tokens 领先者是本章唯一带头像的人（design D25）：下面对比条里的 `.rp-cmp-t` 是同一个
           名字的第二次出现，MUST NOT 也挂一张——同一张脸在同一块里出现两次只会更吵。
           匿名档传空名 + 空地址，画出来是素色空圆。 -->
      <div class="rp-who has-avatar">
        <UserAvatar
          size="xs"
          class="rp-lb-avatar"
          :name="isAnonymousHolder(highlights.top_tokens) ? '' : displayName(highlights.top_tokens)"
          :avatar-url="avatarUrl(highlights.top_tokens)"
          data-testid="leaderboard-avatar"
        />
        <span class="rp-lb-nametext">{{ displayName(highlights.top_tokens) }}</span>
        <span v-if="isSelf(highlights.top_tokens)" class="rp-you">
          {{ t('leaderboard.table.selfBadge') }}
        </span>
      </div>
      <div class="rp-big">
        {{ topTokensBig }}<small>{{ topTokensUnit }}</small>
      </div>

      <div class="rp-cmp">
        <!-- 这一行是该窗口 tokens 的第一名，未必是查看者本人：修饰类表达「冠军行」，
             「本人」那条线索只由榜单与 `.rp-you` 徽标给，两者 MUST NOT 混用同一个类名。 -->
        <div class="rp-cmp-row is-lead">
          <span class="rp-cmp-t">{{ displayName(highlights.top_tokens) }}</span>
          <span class="rp-cmp-track">
            <i class="rp-grow" style="width: 100%; --i: 0"></i>
          </span>
          <span class="rp-cmp-v">{{ topValueText }}</span>
        </div>
        <div v-if="runnerUp" class="rp-cmp-row" data-testid="leaderboard-highlight-runner-up">
          <span class="rp-cmp-t">{{ t('leaderboard.highlights.runnerUp') }}</span>
          <span class="rp-cmp-track">
            <i class="rp-grow" :style="{ width: `${runnerUp.width}%`, '--i': 1 }"></i>
          </span>
          <span class="rp-cmp-v">{{ runnerUp.label }}</span>
        </div>
        <p class="rp-say">{{ leadSay }}</p>
      </div>
    </div>

    <!-- 中：效率之星与最勤快，一上一下 -->
    <div class="rp-stack">
      <div
        v-if="highlights.cache_king"
        class="rp-mini rp-r"
        style="--i: 1"
        data-testid="leaderboard-highlight-cache-king"
      >
        <span class="rp-eyebrow">
          {{
            t('leaderboard.highlights.cacheKingEyebrow', {
              label: t('leaderboard.highlights.cacheKing'),
            })
          }}
        </span>
        <div class="rp-mini-v">
          {{ cacheHitRate.toFixed(1) }}%<small>{{
            t('leaderboard.highlights.unitCacheHitRate')
          }}</small>
        </div>
        <div class="rp-who">
          {{ displayName(highlights.cache_king) }}
          <span v-if="isSelf(highlights.cache_king)" class="rp-you">
            {{ t('leaderboard.table.selfBadge') }}
          </span>
        </div>
        <div v-if="highlights.cache_king.dominant_model" class="rp-note-line">
          {{
            t('leaderboard.highlights.dominantModel', {
              model: highlights.cache_king.dominant_model,
            })
          }}
        </div>
      </div>

      <div
        v-if="highlights.top_requests"
        class="rp-mini rp-r"
        style="--i: 2"
        data-testid="leaderboard-highlight-top-requests"
      >
        <span class="rp-eyebrow">
          {{
            t('leaderboard.highlights.topRequestsEyebrow', {
              label: t('leaderboard.highlights.topRequests'),
            })
          }}
        </span>
        <div class="rp-mini-v">
          {{ topRequestsBig }}<small>{{ topRequestsUnit }}</small>
        </div>
        <div class="rp-who">
          {{ displayName(highlights.top_requests) }}
          <span v-if="isSelf(highlights.top_requests)" class="rp-you">
            {{ t('leaderboard.table.selfBadge') }}
          </span>
        </div>
        <div v-if="topRequestsNote" class="rp-note-line">{{ topRequestsNote }}</div>
      </div>
    </div>

    <!-- 右：全站引导点线对账单。绝对量在匿名档缺席，那几行整行不渲染而不是填 0。 -->
    <div class="rp-siteblock" data-testid="leaderboard-highlight-site">
      <span class="rp-eyebrow">{{ t(`leaderboard.highlights.site.${activeWindow}`) }}</span>
      <ul class="rp-ll">
        <li
          v-for="(row, index) in siteRows"
          :key="row.key"
          class="rp-r"
          :style="{ '--i': index + 2 }"
        >
          <span>{{ row.label }}</span>
          <span class="rp-dots" aria-hidden="true"></span>
          <span class="rp-ll-v">{{ row.value }}</span>
        </li>
      </ul>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useCountUp } from '@/composables/useCountUp'
import { formatCompactNumberTrimmed, formatNumberLocaleString } from '@/utils/format'
import UserAvatar from '@/components/common/UserAvatar.vue'
import { useLeaderboardDisplayName } from './displayName'
import type {
  LeaderboardCacheKing,
  LeaderboardHighlightUser,
  LeaderboardHighlights,
  LeaderboardWindow,
} from '@/api/leaderboard'

const { t } = useI18n()

const props = defineProps<{
  /** 为 null 时（status = computing 或旧后端）整章走骨架；单块为 null 时只隐藏那一块。 */
  highlights: LeaderboardHighlights | null
  activeWindow: LeaderboardWindow
}>()

/** 展示名与头像与榜单条目、Extremes、Profiles 完全同一套规则，收敛在 `displayName.ts`。 */
const { displayName, avatarUrl } = useLeaderboardDisplayName()

function isSelf(holder: LeaderboardHighlightUser | LeaderboardCacheKing): boolean {
  return holder.identity.kind === 'self'
}

/** 匿名档的领先者：名字是「第 N 位」假名，头像圆圈里 MUST NOT 出现任何字符。 */
function isAnonymousHolder(holder: LeaderboardHighlightUser | LeaderboardCacheKing): boolean {
  return holder.identity.kind !== 'self' && holder.identity.kind !== 'named'
}

/** 「档位决定字段是否存在」：绝对量在则实名档口径，缺席则只能给占比。 */
const hasTopTokensAbsolute = computed(
  () => typeof props.highlights?.top_tokens?.total_tokens === 'number',
)
const hasTopRequestsAbsolute = computed(
  () => typeof props.highlights?.top_requests?.successful_requests === 'number',
)

const topTokensTarget = computed(() =>
  hasTopTokensAbsolute.value
    ? (props.highlights?.top_tokens?.total_tokens ?? 0)
    : (props.highlights?.top_tokens?.share_percent ?? 0),
)
const topRequestsTarget = computed(() =>
  hasTopRequestsAbsolute.value
    ? (props.highlights?.top_requests?.successful_requests ?? 0)
    : (props.highlights?.top_requests?.share_percent ?? 0),
)
const cacheHitRateTarget = computed(
  () => (props.highlights?.cache_king?.cache_hit_rate ?? 0) * 100,
)

const { current: topTokensValue } = useCountUp(topTokensTarget)
const { current: topRequestsValue } = useCountUp(topRequestsTarget)
const { current: cacheHitRate } = useCountUp(cacheHitRateTarget)

/** 大数字与单位分两截：匿名档没有绝对量，大数字换成占全站的比例。 */
const topTokensBig = computed(() =>
  hasTopTokensAbsolute.value
    ? formatCompactNumberTrimmed(Math.round(topTokensValue.value))
    : `${Math.round(topTokensValue.value)}%`,
)
const topTokensUnit = computed(() =>
  hasTopTokensAbsolute.value
    ? t('leaderboard.highlights.unitTokens')
    : t('leaderboard.highlights.unitShareTokens'),
)
const topRequestsBig = computed(() =>
  hasTopRequestsAbsolute.value
    ? formatNumberLocaleString(Math.round(topRequestsValue.value))
    : `${Math.round(topRequestsValue.value)}%`,
)
const topRequestsUnit = computed(() =>
  hasTopRequestsAbsolute.value
    ? t('leaderboard.highlights.unitRequests')
    : t('leaderboard.highlights.unitShareRequests'),
)

/** 对比双条里本人那一行的读数：实名档是绝对量，匿名档是基准 100%。 */
const topValueText = computed(() => {
  if (!props.highlights?.top_tokens) return ''
  return hasTopTokensAbsolute.value
    ? formatCompactNumberTrimmed(props.highlights.top_tokens.total_tokens ?? 0)
    : '100%'
})

/**
 * 第 2 名那一行由 `lead_percent`（第 1 名比第 2 名多出的百分比）反推，不另取 entries：
 * 第 2 名 = 第 1 名 ÷ (1 + lead/100)。lead 是整数，反推值与真实第 2 名有取整误差，
 * 因此只用于条长与紧凑读数（M / K 级），MUST NOT 拿去做别的计算。
 * lead 为 0（并列）时两条一样长，句子换成「与第 2 名并列」。
 */
const runnerUp = computed<{ width: number; label: string } | null>(() => {
  const top = props.highlights?.top_tokens
  if (!top || typeof top.lead_percent !== 'number') return null
  const ratio = 100 / (100 + Math.max(0, top.lead_percent))
  const width = Math.max(1, Math.round(ratio * 1000) / 10)
  if (hasTopTokensAbsolute.value) {
    return { width, label: formatCompactNumberTrimmed(Math.round((top.total_tokens ?? 0) * ratio)) }
  }
  return { width, label: `${Math.round(ratio * 100)}%` }
})

/**
 * 一句现算的解读，两档同形：`领先第 2 名 N% · 占全站 N%`。
 * 实名档的 tokens 差值已经由上面那两条对比条说清楚，句子里 MUST NOT 再重复一遍；
 * 并列（lead = 0）时「领先 0%」是句废话，换成「与第 2 名并列」。
 */
const leadSay = computed(() => {
  const top = props.highlights?.top_tokens
  if (!top) return ''
  const lead = top.lead_percent
  const share = top.share_percent
  if (!lead) return t('leaderboard.highlights.leadSayTie', { share })
  return t('leaderboard.highlights.leadSay', { lead, share })
})

/**
 * 最勤快那一格的注脚：实名档大数字已经是请求数，因此补一句占比；匿名档大数字就是占比，
 * 只补领先幅度。并列（lead = 0）时换成「与第 2 名并列」而不是「多 0%」。
 */
const topRequestsNote = computed(() => {
  const top = props.highlights?.top_requests
  if (!top) return ''
  const lead = top.lead_percent
  const leadText = lead
    ? t('leaderboard.highlights.leadPercent', { percent: lead })
    : t('leaderboard.highlights.tiedWithSecond')
  if (!hasTopRequestsAbsolute.value) return leadText
  return `${t('leaderboard.highlights.shareOfSite', { percent: top.share_percent })} · ${leadText}`
})

interface SiteRow {
  key: string
  label: string
  value: string
}

/**
 * 全站五行的引导点线。绝对量在匿名档缺席，那几行**整行不渲染**而不是填 0；
 * 比率与时刻两档都下发，因此匿名档下仍然留着活跃人数、峰值时段与命中率三行。
 */
const siteRows = computed<SiteRow[]>(() => {
  const site = props.highlights?.site
  if (!site) return []
  const rows: SiteRow[] = []
  if (typeof site.total_tokens === 'number') {
    rows.push({
      key: 'totalTokens',
      label: t('leaderboard.highlights.siteRows.totalTokens'),
      value: formatCompactNumberTrimmed(site.total_tokens),
    })
  }
  if (typeof site.successful_requests === 'number') {
    rows.push({
      key: 'successfulRequests',
      label: t('leaderboard.highlights.siteRows.successfulRequests'),
      value: t('leaderboard.highlights.rowRequests', {
        count: formatNumberLocaleString(site.successful_requests),
      }),
    })
  }
  if (site.participant_count !== undefined && site.participant_count !== null) {
    rows.push({
      key: 'participants',
      label: t('leaderboard.highlights.siteRows.participants'),
      value:
        typeof site.participant_count === 'number'
          ? t('leaderboard.highlights.rowParticipants', {
              count: formatNumberLocaleString(site.participant_count),
            })
          : String(site.participant_count),
    })
  }
  if (typeof site.peak_hour === 'number') {
    rows.push({
      key: 'peakHour',
      label: t('leaderboard.highlights.siteRows.peakHour'),
      value: `${`${site.peak_hour}`.padStart(2, '0')}:00`,
    })
  }
  if (typeof site.cache_hit_rate === 'number') {
    rows.push({
      key: 'cacheHitRate',
      label: t('leaderboard.highlights.siteRows.cacheHitRate'),
      value: `${(site.cache_hit_rate * 100).toFixed(1)}%`,
    })
  }
  return rows
})
</script>
