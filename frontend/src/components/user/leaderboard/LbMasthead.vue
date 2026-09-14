<template>
  <!-- 页头：H1 + 副题 + 唯一一组控制条（由 `LbTitle.vue` 合并进来）。
       站名归站点外壳（页面套 `AppLayout`，侧边栏就是导航），页内 MUST NOT 再提供
       「返回仪表盘」这类跳转入口；主题切换整块删除——明暗由样式层跟随站点的 `html.dark`，
       开关归侧边栏，这里 MUST NOT 读写站点的 `html.dark` 与 `localStorage['theme']`。

       日期在副题、时区在页脚、快照时分在 chip——同一个事实只在页面上出现一次，
       因此这里 MUST NOT 再挂时区，也不挂 WINDOW / METRIC / MODE 这些键名。
       参与人数还没算出来时整段略过它：「正在计算」MUST NOT 渲染成 0 值。 -->
  <header class="rp-phead" data-testid="leaderboard-masthead">
    <div class="rp-r" data-testid="leaderboard-title">
      <h1 data-testid="leaderboard-title-heading">{{ headingText }}</h1>
      <p class="rp-phead-sub" data-testid="leaderboard-title-note">
        <template v-if="ready">
          <span class="rp-n">{{ participantLabel }}</span>
          {{ t('leaderboard.titleBlock.participantsUnit') }}
          <span aria-hidden="true"> · </span>
        </template>
        <span class="rp-n">{{ dateText }}</span>
      </p>
    </div>

    <div class="rp-ctls rp-r" style="--i: 1">
      <span class="rp-seg" role="group" :aria-label="t('leaderboard.windows.label')">
        <!-- 分段自带一个 mono 小标签，因为控制条上并排着两组分段，
             没有标签就分不出哪一组是窗口、哪一组是指标。 -->
        <b>{{ t('leaderboard.windows.label') }}</b>
        <button
          v-for="option in windowOptions"
          :key="option.value"
          type="button"
          :aria-pressed="activeWindow === option.value"
          :title="option.title"
          :data-testid="`leaderboard-window-${option.testid}`"
          @click="emit('select-window', option.value)"
        >
          {{ option.label }}
        </button>
      </span>

      <span class="rp-seg" role="group" :aria-label="t('leaderboard.metrics.label')">
        <b>{{ t('leaderboard.metrics.label') }}</b>
        <button
          v-for="option in metricOptions"
          :key="option.value"
          type="button"
          :aria-pressed="activeMetric === option.value"
          :title="option.title"
          :data-testid="`leaderboard-metric-${option.testid}`"
          @click="emit('select-metric', option.value)"
        >
          {{ option.label }}
        </button>
      </span>

      <!-- 快照 chip：呼吸点 + 「快照 HH:MM」+「距下一次重建」。标签与页脚 colophon 上那个
           共用 `leaderboard.masthead.snapshot`；快照未生成时显示 `snapshotPending`，不留英文。 -->
      <span class="rp-snap" data-testid="leaderboard-masthead-snapshot">
        <span class="rp-pulse" aria-hidden="true"></span>
        {{ t('leaderboard.masthead.snapshot') }} <span class="rp-n">{{ snapshotValue }}</span>
        <span v-if="nextRebuildText" class="rp-snap-rebuild">{{ nextRebuildText }}</span>
      </span>

      <!-- 档位 chip 只在匿名档出现：实名档是常态，给它一个 chip 只是多一句不用读的字。 -->
      <span v-if="isAnonymous" class="rp-chip" data-testid="leaderboard-masthead-mode">
        {{ t('leaderboard.masthead.modeAnonymous') }}
      </span>
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  formatDateOnlyInTimeZone,
  formatNumberLocaleString,
  formatTimeToMinuteInTimeZone,
} from '@/utils/format'
import type { LeaderboardMetric, LeaderboardWindow } from '@/api/leaderboard'
import type { LeaderboardMode } from '@/utils/featureFlags'

const { t } = useI18n()

/** 后台作业的重建周期（分钟），与页脚那句「每 5 分钟重建」是同一个数。 */
const LEADERBOARD_REBUILD_MINUTES = 5

const props = defineProps<{
  activeWindow: LeaderboardWindow
  activeMetric: LeaderboardMetric
  /** 首屏还没拿到响应时为 null，此时不猜档位，也就不渲染档位 chip。 */
  mode: LeaderboardMode | null
  /** Snapshot 尚未生成时为 null。 */
  snapshotUpdatedAt: string | null
  /** 快照已陈旧（作业没按时跑）时为 true，此时「距下一次重建」这句话不再成立。 */
  stale: boolean
  /** 窗口边界所用的站点时区名：快照时分与日期按它渲染，页头本身不再印出时区。 */
  timezone: string
  /**
   * 快照已就绪、响应已到手。为 false 时（加载中或 status = computing）参与人数还没算出来，
   * 副题整段略过它。
   */
  ready: boolean
  /** named 档与 Preview 下是精确整数，anonymous 档下是分档字符串（如 `100+`）。 */
  participantCount: number | string
}>()

const emit = defineEmits<{
  (event: 'select-window', value: LeaderboardWindow): void
  (event: 'select-metric', value: LeaderboardMetric): void
}>()

/** H1 随 Window 换一句：今日 / 本周 / 本月「概览」。窗口字面量已经在控制条的分段上，这里不回显。 */
const headingText = computed(() => t(`leaderboard.titleBlock.heading.${props.activeWindow}`))

const participantLabel = computed(() =>
  typeof props.participantCount === 'number'
    ? formatNumberLocaleString(props.participantCount)
    : props.participantCount,
)

/**
 * 日期固定成 YYYY-MM-DD。按站点时区渲染：这一行说的是「哪一天的榜」，
 * 跨时区查看者看到的必须与页脚的 tz 一致。
 */
const dateText = computed(() =>
  formatDateOnlyInTimeZone(props.snapshotUpdatedAt ?? new Date(), props.timezone),
)

/**
 * 分段上显示 i18n 标签（zh「今日 / 本周 / 本月」），`value` 才是请求参数 `today / week / month`。
 * 界面文字 MUST 走 i18n，只有接口参数保持英文（2026-09-14 用户「顶部也做成中文」）。
 */
const windowOptions = computed(() => [
  { value: 'today' as const, testid: 'today', label: t('leaderboard.windows.today'), title: t('leaderboard.windows.today') },
  { value: 'week' as const, testid: 'week', label: t('leaderboard.windows.week'), title: t('leaderboard.windows.week') },
  { value: 'month' as const, testid: 'month', label: t('leaderboard.windows.month'), title: t('leaderboard.windows.month') },
])

/** 分段上是短标签，完整名字进 title。 */
const metricOptions = computed(() => [
  {
    value: 'total_tokens' as const,
    testid: 'total-tokens',
    label: t('leaderboard.masthead.metricTokens'),
    title: t('leaderboard.metrics.totalTokens'),
  },
  {
    value: 'successful_requests' as const,
    testid: 'successful-requests',
    label: t('leaderboard.masthead.metricRequests'),
    title: t('leaderboard.metrics.successfulRequests'),
  },
  {
    value: 'cost' as const,
    testid: 'cost',
    label: t('leaderboard.masthead.metricCost'),
    title: t('leaderboard.metrics.cost'),
  },
])

/** 只有匿名档才挂 chip；实名档与「还没拿到响应」都不挂。 */
const isAnonymous = computed(() => props.mode === 'anonymous')

/**
 * 快照时间取 HH:MM，且按站点时区渲染：窗口边界是站点时区算的，
 * 用浏览器本地时区会让这个时分与页脚的 tz 自相矛盾。
 * 还没生成时显示一个短词（zh「待生成」），而不是一句话，这一格放不下一句话。
 */
const snapshotValue = computed(() => {
  const pending = t('leaderboard.masthead.snapshotPending')
  if (!props.snapshotUpdatedAt) return pending
  return formatTimeToMinuteInTimeZone(props.snapshotUpdatedAt, props.timezone) || pending
})

/**
 * 「距下一次重建还剩几分钟」，按重建周期现算。
 * 快照还没生成、或已经陈旧（作业没按时跑）时这句话不成立，整段后缀不渲染——
 * MUST NOT 一直挂着画板里的那个示例值。
 */
const nextRebuildText = computed(() => {
  if (!props.snapshotUpdatedAt || props.stale) return ''
  const updatedAt = new Date(props.snapshotUpdatedAt).getTime()
  if (Number.isNaN(updatedAt)) return ''
  const remaining = LEADERBOARD_REBUILD_MINUTES - Math.floor((Date.now() - updatedAt) / 60_000)
  if (remaining <= 0) return ''
  return t('leaderboard.masthead.rebuildIn', { minutes: remaining })
})
</script>
