<template>
  <div class="rp" :class="{ dark: isDark }">
    <!-- 全屏独立页：不再套 AppLayout，视觉 token 作用域就是这个根节点（design D15/D16/D23）。
         v3 报表皮肤里**亮色是基色**，站点处于暗色时追加 `.dark`，状态由 html.dark 推导
         （方向与 v2 相反）。注释放在根节点之内，避免多一个根节点让本组件变成 fragment。 -->
    <div class="rp-page">
      <LbMasthead
        :site-name="siteName"
        :active-window="activeWindow"
        :active-metric="activeMetric"
        :mode="data?.mode ?? null"
        :snapshot-updated-at="data?.snapshot_updated_at ?? null"
        :stale="Boolean(data?.stale)"
        :timezone="data?.timezone ?? siteTimezoneFallback"
        @select-window="selectWindow"
        @select-metric="selectMetric"
        @theme-change="onThemeChange"
      />

      <LbStates v-if="isPreview" variant="preview" />

      <LbTitle
        :active-window="activeWindow"
        :ready="isReady && !loading"
        :participant-count="participantCount"
        :snapshot-updated-at="data?.snapshot_updated_at ?? null"
        :timezone="data?.timezone ?? siteTimezoneFallback"
      />

      <LbStates v-if="isStale" variant="stale" />

      <!-- 章号是固定编号而不是序号：某一章因数据缺失整章不渲染时，其余章号 MUST NOT 重排
           （页面上出现 01 02 04 05 06 07 是正确行为）。
           章名右侧的小字副题已按「全页文案瘦身」整体去掉，只有 04 章那一格让位给榜单工具条。 -->

      <!-- 01 高亮：骨架只代表「还在算」。快照已就绪却没有 highlights（抑制态、空窗口、
           旧快照）时整章隐藏，不留一章永远转不完的骨架。 -->
      <LbChapter
        v-if="showHighlights"
        num="01"
        :title="t(`leaderboard.chapters.01.name.${activeWindow}`)"
      >
        <LbHighlights :highlights="highlights" :active-window="activeWindow" />
      </LbChapter>

      <!-- 02 之最：整块随 highlights 走（同一个 key、同一批快照），单项为 null 时只少一格。 -->
      <LbChapter
        v-if="hasExtremes"
        num="02"
        :title="t('leaderboard.chapters.02.name')"
      >
        <LbExtremes :extremes="extremes" />
      </LbChapter>

      <!-- 03 我的位置：数据缺失时整章不渲染，后面的章号照旧。 -->
      <LbChapter
        v-if="showWhoami"
        num="03"
        :title="t('leaderboard.chapters.03.name')"
      >
        <LbWhoami
          :my-rank="myRank"
          :participant-count="participantCount"
          :suppressed="isSuppressed"
          :computing="isComputing"
          :viewer="viewer"
          :site="highlights?.site ?? null"
        />
      </LbChapter>

      <!-- 04 榜单：非正常态都落在这一章里，章本身照常渲染。 -->
      <LbChapter
        num="04"
        :title="t('leaderboard.chapters.04.name')"
      >
        <!-- 工具条与报头的两个分段是同一份状态、同一组请求参数（MUST NOT 各自维护）；
             testid 另起一套，免得与报头上的那两组撞名。 -->
        <template #tools>
          <div class="rp-tools">
            <span class="rp-seg" role="group" :aria-label="t('leaderboard.windows.label')">
              <button
                v-for="option in windowOptions"
                :key="option.value"
                type="button"
                :aria-pressed="activeWindow === option.value"
                :title="option.title"
                :data-testid="`leaderboard-rank-window-${option.testid}`"
                @click="selectWindow(option.value)"
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
                :data-testid="`leaderboard-rank-metric-${option.testid}`"
                @click="selectMetric(option.value)"
              >
                {{ option.label }}
              </button>
            </span>

            <button
              type="button"
              class="rp-ghost"
              :title="t('common.refresh')"
              :aria-label="t('common.refresh')"
              :disabled="loading"
              data-testid="leaderboard-refresh"
              @click="load(false)"
            >
              <LbIcon name="refresh" :size="13" />{{ t('common.refresh') }}
            </button>
          </div>
        </template>

        <div v-if="loading" data-testid="leaderboard-loading">
          <div v-for="row in 4" :key="row" class="rp-skel"></div>
        </div>

        <div v-else-if="loadFailed" class="rp-empty" data-testid="leaderboard-error">
          <p>{{ t('leaderboard.states.loadFailed') }}</p>
          <button type="button" class="rp-btn" style="margin-top: 10px" @click="load(false)">
            {{ t('common.refresh') }}
          </button>
        </div>

        <!-- 「正在计算」不是空榜也不是报错 -->
        <LbStates v-else-if="isComputing" variant="computing" />

        <!-- 抑制态由 entries_suppressed 判定，不从「entries 是空数组」反推 -->
        <LbStates v-else-if="isSuppressed" variant="suppressed" />

        <div v-else-if="isEmpty" class="rp-empty" data-testid="leaderboard-empty">
          <p>{{ t('leaderboard.states.empty') }}</p>
          <p class="rp-hint">{{ t('leaderboard.states.emptyHint') }}</p>
        </div>

        <!-- `site` 只用于解读句里「前三名占全站 N%」那一句的分母；匿名档缺席时那句自动省略。 -->
        <LbRankList
          v-else
          :entries="entries"
          :metric="activeMetric"
          :site="highlights?.site ?? null"
        />
      </LbChapter>

      <!-- 洞察区块：来源缺行时后端按区块各自给 null，这里隐藏该章而不是用 0 填充。 -->
      <LbChapter
        v-if="showModelsRow"
        num="05"
        :title="t('leaderboard.chapters.05.name')"
      >
        <!-- 两栏里只剩一栏有内容时收成一栏：MUST NOT 让单独一块占三成宽、右侧空一片。 -->
        <div class="rp-two" :class="{ 'is-single': modelsRowSingle }">
          <LbModelHeat :models="modelsToday" />
          <LbProfiles :profiles="profiles" />
        </div>
        <LbPlatforms :platforms="platformsToday" />
      </LbChapter>

      <LbChapter
        v-if="showActivityRow"
        num="06"
        :title="t('leaderboard.chapters.06.name')"
      >
        <LbHeatmap :daily="daily30" />
        <div class="rp-rhythm" :class="{ 'is-single': activityRowSingle }">
          <LbRhythm :rhythm="weeklyRhythm" />
          <LbHourly :hourly="hourlyToday" />
        </div>
      </LbChapter>

      <LbChapter
        v-if="showTrendRow"
        num="07"
        :title="t('leaderboard.chapters.07.name')"
      >
        <LbTrend :daily="daily30" :month="month" />
        <!-- 两类 token 口径：构成在左，缓存在右（同一个数不画两遍）。 -->
        <div class="rp-comp" :class="{ 'is-single': trendRowSingle }">
          <LbComposition :composition="compositionToday" />
          <LbCacheTrend :cache="cacheToday" :points="cacheTrend14" />
        </div>
      </LbChapter>

      <LbFooter
        v-if="data"
        :timezone="data.timezone"
        :snapshot-updated-at="data.snapshot_updated_at"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import LbCacheTrend from '@/components/user/leaderboard/LbCacheTrend.vue'
import LbChapter from '@/components/user/leaderboard/LbChapter.vue'
import LbComposition from '@/components/user/leaderboard/LbComposition.vue'
import LbExtremes from '@/components/user/leaderboard/LbExtremes.vue'
import LbFooter from '@/components/user/leaderboard/LbFooter.vue'
import LbHeatmap from '@/components/user/leaderboard/LbHeatmap.vue'
import LbHighlights from '@/components/user/leaderboard/LbHighlights.vue'
import LbHourly from '@/components/user/leaderboard/LbHourly.vue'
import LbIcon from '@/components/user/leaderboard/LbIcon.vue'
import LbMasthead from '@/components/user/leaderboard/LbMasthead.vue'
import LbModelHeat from '@/components/user/leaderboard/LbModelHeat.vue'
import LbPlatforms from '@/components/user/leaderboard/LbPlatforms.vue'
import LbProfiles from '@/components/user/leaderboard/LbProfiles.vue'
import LbRankList from '@/components/user/leaderboard/LbRankList.vue'
import LbRhythm from '@/components/user/leaderboard/LbRhythm.vue'
import LbStates from '@/components/user/leaderboard/LbStates.vue'
import LbTitle from '@/components/user/leaderboard/LbTitle.vue'
import LbTrend from '@/components/user/leaderboard/LbTrend.vue'
import LbWhoami from '@/components/user/leaderboard/LbWhoami.vue'
import { getLeaderboard } from '@/api/leaderboard'
import type {
  LeaderboardMetric,
  LeaderboardResponse,
  LeaderboardWindow,
} from '@/api/leaderboard'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
// 页面级样式，不进全局入口：变量只定义在 .rp 这一层。
import '@/styles/leaderboard-tokens.css'

const { t } = useI18n()
const appStore = useAppStore()

const data = ref<LeaderboardResponse | null>(null)
const loading = ref(true)
const loadFailed = ref(false)
const activeWindow = ref<LeaderboardWindow>('today')
const activeMetric = ref<LeaderboardMetric>('total_tokens')
/**
 * 主题不是页面私有状态：初值从站点既有的 `html.dark` 读，之后由报头的分段开关回传。
 * 根节点据此追加 `.dark`（亮色是基色，没有 `.dark` 就是亮色）。
 */
const isDark = ref(
  typeof document !== 'undefined' && document.documentElement.classList.contains('dark'),
)

let controller: AbortController | null = null
let sequence = 0

const siteName = computed(() => appStore.siteName || 'spool')

/**
 * 04 章工具条上的两个分段。取值与短标签跟报头完全一致（那边也是同一组 i18n 键），
 * 状态与请求参数同源——两处切换的是同一个 ref，MUST NOT 各自维护一份。
 */
const windowOptions = computed(() => [
  { value: 'today' as const, testid: 'today', title: t('leaderboard.windows.today') },
  { value: 'week' as const, testid: 'week', title: t('leaderboard.windows.week') },
  { value: 'month' as const, testid: 'month', title: t('leaderboard.windows.month') },
])
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
/** 还没拿到响应时报头上的 tz 先留一个占位符，MUST NOT 猜一个时区名。 */
const siteTimezoneFallback = '...'

/**
 * Preview（预览）由后端显式下发的 `preview` 字段判定，MUST NOT 从 `mode === 'off'` 反推：
 * 「这一份响应是不是预览」是后端的结论，页面照抄即可；标题块下的档位说明另读 mode。
 */
const isPreview = computed(() => data.value?.preview === true)
const isReady = computed(() => data.value?.status === 'ready')
const isComputing = computed(() => data.value?.status === 'computing')
/** 陈旧只在有数据时才有意义；数据照常展示，只是加一条警告。 */
const isStale = computed(() => Boolean(data.value?.stale) && isReady.value && !loading.value)
/** 抑制态与空态的 entries 都是空数组，靠 entries_suppressed 这个显式信号区分。 */
const isSuppressed = computed(() => isReady.value && data.value?.entries_suppressed === true)
const isEmpty = computed(
  () => isReady.value && !isSuppressed.value && (data.value?.entries.length ?? 0) === 0,
)
const entries = computed(() => data.value?.entries ?? [])
const myRank = computed(() => data.value?.my_rank ?? null)
const participantCount = computed(() => data.value?.participant_count ?? 0)
/** 快照未就绪或后端尚未下发时为 null；是否因此摆骨架由 showHighlights 决定。 */
const highlights = computed(() => data.value?.highlights ?? null)
/** 之最与 profiles 都放在该窗口的 highlights 里，因此 computing 时随之不可用。 */
const extremes = computed(() => highlights.value?.extremes ?? null)
/** 六项全是 null 时连章头都不留，免得出现一个空章。 */
const hasExtremes = computed(() =>
  Boolean(extremes.value && Object.values(extremes.value).some((item) => item !== null)),
)
/** 本人数据不出自快照，因此 computing 时照常渲染；旧后端不下发时为 null。 */
const viewer = computed(() => data.value?.viewer ?? null)
/**
 * 01 章的去留：有数据就渲染；没有数据时只有「还在算」（加载中或 computing）才摆骨架，
 * 快照已就绪却没有 highlights（抑制态、空窗口、旧快照）时整章隐藏——骨架不是空态的替身。
 */
const showHighlights = computed(() => {
  if (loadFailed.value) return false
  if (highlights.value) return true
  return loading.value || isComputing.value
})
/**
 * 「我的位置」跟着「拿到响应」走而不是「快照已就绪」：viewer 不出自快照，
 * computing 时名次那一格换成「正在计算」，其余照常。抑制态也要显示本人。
 */
const showWhoami = computed(() => Boolean(data.value) && !loading.value && !loadFailed.value)

/**
 * 洞察数据与快照状态无关（不随 computing 消失），来源缺行时后端按区块各自给 null，
 * 页面隐藏该章而不是用 0 填充。
 */
const insights = computed(() => data.value?.insights ?? null)
const modelsToday = computed(() => insights.value?.models_today ?? null)
const daily30 = computed(() => insights.value?.daily_30 ?? null)
const hourlyToday = computed(() => insights.value?.hourly_today ?? null)
const cacheToday = computed(() => insights.value?.cache_today ?? null)
const month = computed(() => insights.value?.month ?? null)
/**
 * v2 新增的五块。`profiles` 按 Window 计算、与 extremes 同住该 Window 的 highlights，
 * 因此只有它会随 status = computing 一起缺席；其余四块与快照状态无关。
 */
const profiles = computed(() => insights.value?.profiles ?? null)
const platformsToday = computed(() => insights.value?.platforms_today ?? null)
const weeklyRhythm = computed(() => insights.value?.weekly_rhythm ?? null)
const compositionToday = computed(() => insights.value?.composition_today ?? null)
const cacheTrend14 = computed(() => insights.value?.cache_trend_14 ?? null)
/**
 * 两栏区块的「只剩一栏」判定，条件与各子组件自己的 v-if 一一对应
 * （改任一侧的渲染条件时这里要跟着改，否则栅格会留下一个空列）。
 */
const hasModelsToday = computed(() => Boolean(modelsToday.value?.length))
const hasProfiles = computed(() => Boolean(profiles.value?.length))
const modelsRowSingle = computed(() => hasModelsToday.value !== hasProfiles.value)
const hasRhythm = computed(() => Boolean(weeklyRhythm.value?.length))
const hasHourly = computed(() => Boolean(hourlyToday.value?.length))
const activityRowSingle = computed(() => hasRhythm.value !== hasHourly.value)
const hasComposition = computed(() => Boolean(compositionToday.value))
const hasCacheTrend = computed(() => Boolean(cacheToday.value || cacheTrend14.value?.length))
const trendRowSingle = computed(() => hasComposition.value !== hasCacheTrend.value)

/** 一章里所有区块都没数据时连章都不渲染，免得留下一整段空白。 */
const showModelsRow = computed(
  () => hasModelsToday.value || hasProfiles.value || Boolean(platformsToday.value?.length),
)
const showActivityRow = computed(
  () => Boolean(daily30.value?.length) || hasRhythm.value || hasHourly.value,
)
const showTrendRow = computed(() =>
  Boolean(
    daily30.value?.length ||
      cacheToday.value ||
      compositionToday.value ||
      cacheTrend14.value?.length,
  ),
)

async function load(silent = true) {
  controller?.abort()
  const request = new AbortController()
  controller = request
  const id = ++sequence
  if (!silent || !data.value) loading.value = true
  try {
    const next = await getLeaderboard(
      { window: activeWindow.value, metric: activeMetric.value },
      request.signal,
    )
    if (id !== sequence) return
    data.value = next
    loadFailed.value = false
  } catch (error) {
    const e = error as { name?: string; code?: string }
    if (e?.name === 'AbortError' || e?.name === 'CanceledError' || e?.code === 'ERR_CANCELED') return
    if (id !== sequence) return
    loadFailed.value = true
    appStore.showError(extractApiErrorMessage(error, t('leaderboard.states.loadFailed')))
  } finally {
    if (id === sequence) loading.value = false
  }
}

function selectWindow(value: LeaderboardWindow) {
  if (activeWindow.value === value) return
  activeWindow.value = value
  void load(false)
}

function selectMetric(value: LeaderboardMetric) {
  if (activeMetric.value === value) return
  activeMetric.value = value
  void load(false)
}

/** 报头切换主题后回传：站点的 html.dark 已经由报头改过，这里只同步根节点的修饰 class。 */
function onThemeChange(dark: boolean) {
  isDark.value = dark
}

onMounted(() => {
  void load(false)
})

onBeforeUnmount(() => {
  controller?.abort()
  controller = null
})
</script>
