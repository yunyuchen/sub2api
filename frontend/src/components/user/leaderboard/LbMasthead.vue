<template>
  <!-- 报头：一行两端对齐，左刊头右拨盘，高度收紧，底部一根 `--line-hard`。
       日期进标题块副题、时区进页脚、快照时分进右侧 chip——同一个事实只在页面上出现一次，
       因此这里 MUST NOT 再挂日期行、时分 · tz 行，也不挂 WINDOW / METRIC / MODE / TZ 这些键名。
       窗口 / 指标在报头与榜单工具条两处都有，状态同源（都由页面持有）。 -->
  <header class="rp-masthead" data-testid="leaderboard-masthead">
    <div class="rp-brand rp-r">
      <b>{{ siteName }}</b>
      <span class="rp-brand-mark">{{ t('leaderboard.masthead.brand') }}</span>
    </div>

    <div class="rp-dials rp-r" style="--i: 1">
      <span class="rp-seg" role="group" :aria-label="t('leaderboard.windows.label')">
        <button
          v-for="option in windowOptions"
          :key="option.value"
          type="button"
          :aria-pressed="activeWindow === option.value"
          :title="option.title"
          :data-testid="`leaderboard-window-${option.testid}`"
          @click="emit('select-window', option.value)"
        >
          {{ option.value }}
        </button>
      </span>

      <span class="rp-seg" role="group" :aria-label="t('leaderboard.metrics.label')">
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

      <!-- 档位 chip 只在匿名档出现：实名档是常态，给它一个 chip 只是多一句不用读的字。 -->
      <span v-if="isAnonymous" class="rp-tag" data-testid="leaderboard-masthead-mode">
        {{ t('leaderboard.masthead.modeAnonymous') }}
      </span>

      <!-- 快照 chip：呼吸点 + 时分 + 「距下一次重建」，不再写 `snapshot` 这个键名。 -->
      <span class="rp-snap" data-testid="leaderboard-masthead-snapshot">
        <span class="rp-pulse" aria-hidden="true"></span>
        <span class="rp-n">{{ snapshotValue }}</span>
        <span v-if="nextRebuildText" class="rp-snap-rebuild">{{ nextRebuildText }}</span>
      </span>

      <span class="rp-sep"></span>

      <span class="rp-seg" role="group" :aria-label="t('leaderboard.masthead.theme')">
        <button
          type="button"
          :aria-pressed="!isDark"
          :title="t('nav.lightMode')"
          data-testid="leaderboard-theme-light"
          @click="setTheme(false)"
        >
          <LbIcon name="sun" :size="13" />{{ t('leaderboard.masthead.themeLight') }}
        </button>
        <button
          type="button"
          :aria-pressed="isDark"
          :title="t('nav.darkMode')"
          data-testid="leaderboard-theme-dark"
          @click="setTheme(true)"
        >
          <LbIcon name="moon" :size="13" />{{ t('leaderboard.masthead.themeDark') }}
        </button>
      </span>

      <button
        type="button"
        class="rp-ghost"
        data-testid="leaderboard-back-to-dashboard"
        @click="backToDashboard"
      >
        <LbIcon name="arrow-left" :size="13" />{{ t('leaderboard.masthead.backToDashboard') }}
      </button>
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import LbIcon from './LbIcon.vue'
import { formatTimeToMinuteInTimeZone } from '@/utils/format'
import type { LeaderboardMetric, LeaderboardWindow } from '@/api/leaderboard'
import type { LeaderboardMode } from '@/utils/featureFlags'

const { t } = useI18n()
const router = useRouter()

/** 后台作业的重建周期（分钟），与页脚那句「每 5 分钟重建」是同一个数。 */
const LEADERBOARD_REBUILD_MINUTES = 5

const props = defineProps<{
  /** 站名，由页面从 appStore 读出后传进来，组件本身保持纯展示。 */
  siteName: string
  activeWindow: LeaderboardWindow
  activeMetric: LeaderboardMetric
  /** 首屏还没拿到响应时为 null，此时不猜档位，也就不渲染档位 chip。 */
  mode: LeaderboardMode | null
  /** Snapshot 尚未生成时为 null。 */
  snapshotUpdatedAt: string | null
  /** 快照已陈旧（作业没按时跑）时为 true，此时「距下一次重建」这句话不再成立。 */
  stale: boolean
  /** 窗口边界所用的站点时区名：快照时分按它渲染，报头本身不再印出时区。 */
  timezone: string
}>()

const emit = defineEmits<{
  (event: 'select-window', value: LeaderboardWindow): void
  (event: 'select-metric', value: LeaderboardMetric): void
  /** 主题切换后把新状态交给页面，页面据此决定根节点要不要带 `.dark`。 */
  (event: 'theme-change', dark: boolean): void
}>()

const windowOptions = computed(() => [
  { value: 'today' as const, testid: 'today', title: t('leaderboard.windows.today') },
  { value: 'week' as const, testid: 'week', title: t('leaderboard.windows.week') },
  { value: 'month' as const, testid: 'month', title: t('leaderboard.windows.month') },
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
 * 还没生成时用一个字面量而不是一句话，这一格放不下一句话。
 */
const snapshotValue = computed(() => {
  if (!props.snapshotUpdatedAt) return 'pending'
  return formatTimeToMinuteInTimeZone(props.snapshotUpdatedAt, props.timezone) || 'pending'
})

/**
 * `(+Nm)` 是「距下一次重建还剩几分钟」，按重建周期现算。
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

/**
 * 主题状态复用站点既有的那一套（html.dark + localStorage['theme']），
 * 语义与 AppSidebar 的 toggleTheme() 一致，因此在本页切换后回到其它页面不会出现两套主题。
 */
const isDark = ref(
  typeof document !== 'undefined' && document.documentElement.classList.contains('dark'),
)

function setTheme(dark: boolean) {
  if (isDark.value === dark) return
  isDark.value = dark
  document.documentElement.classList.toggle('dark', dark)
  try {
    localStorage.setItem('theme', dark ? 'dark' : 'light')
  } catch {
    // localStorage 不可用时只在本次会话内生效
  }
  emit('theme-change', dark)
}

function backToDashboard() {
  void router.push('/dashboard')
}
</script>
