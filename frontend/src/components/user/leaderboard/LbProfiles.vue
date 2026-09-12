<template>
  <!-- 05 右栏：当前 Window 前 50 名（与榜单本体同一个数）各自的 Top 3 模型。每行一位用户，chip 的左边界对齐成一条轴。
       profiles 与 extremes 同住该 Window 的 highlights，因此 status = computing 时随之缺席；
       整块为 null（旧后端 / 快照在算）或空数组时不渲染任何东西。 -->
  <div v-if="rows.length" class="rp-prof" data-testid="leaderboard-profiles">
    <span class="rp-eyebrow">{{ t('leaderboard.profiles.note') }}</span>
    <ul>
      <li
        v-for="(row, index) in rows"
        :key="index"
        class="rp-r"
        :style="{ '--i': Math.floor(index / CASCADE_ROWS_PER_STEP) }"
        data-testid="leaderboard-profiles-row"
      >
        <span class="rp-u" :class="isSelf(row) ? 'is-self' : ''">{{ displayName(row) }}</span>
        <span class="rp-cs">
          <span
            v-for="model in row.models"
            :key="model.model"
            data-testid="leaderboard-profiles-chip"
          >
            {{ model.model }}<b>{{ model.share_percent }}%</b>
          </span>
          <!-- Top 3 之和不足 100% 时如实说还有别的模型，MUST NOT 把这一行读成「只用这三个」。 -->
          <span v-if="hasMore(row)" class="rp-more" data-testid="leaderboard-profiles-more">
            {{ t('leaderboard.profiles.more') }}
          </span>
        </span>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useLeaderboardDisplayName } from './displayName'
import type { LeaderboardProfile } from '@/api/leaderboard'

const { t } = useI18n()

const props = defineProps<{
  /** `insights.profiles`；为 null 或空时整块不渲染。占比两档都下发，这里没有绝对量可漏。 */
  profiles: LeaderboardProfile[] | null
}>()

/** 后端已按当前 Window 的 Total Tokens 取前 50 名（`leaderboardProfilesLimit`，与榜单 Top 50 同一个数），这里只兜一层底。 */
const PROFILE_ROWS_LIMIT = 50
const rows = computed(() => (props.profiles ?? []).slice(0, PROFILE_ROWS_LIMIT))

/**
 * 级联下标按 4 行一档，与榜单本体（`LbRankList.vue`）同一套做法：
 * 50 行逐行递延会让末行等到 ~3s 才入场，那时读者早已读到中段。
 */
const CASCADE_ROWS_PER_STEP = 4

/** 展示名与榜单条目、Highlights、Extremes 完全同一套规则，收敛在 `displayName.ts`。 */
const { displayName } = useLeaderboardDisplayName()

/** 后端每人最多给 3 个模型（`leaderboard_insights.go` 的 `leaderboardProfileModelsLimit`）。 */
const PROFILE_MODELS_LIMIT = 3
/** 三个占比各自取整最多差 1.5 个百分点，留 2 个点的余量免得把残差当成「还有别的模型」。 */
const PROFILE_RESIDUAL_TOLERANCE = 2

/**
 * Top 3 的占比之和明显不足 100%：这一行还有没进 Top 3 的模型。
 * 不足 3 个时后端已经把这个人的模型给全了，不可能有第 4 个。
 */
function hasMore(profile: LeaderboardProfile): boolean {
  if (profile.models.length < PROFILE_MODELS_LIMIT) return false
  const sum = profile.models.reduce((total, model) => total + model.share_percent, 0)
  return sum < 100 - PROFILE_RESIDUAL_TOLERANCE
}

/** 本人那一行用强调色，与榜单里的本人行是同一条线索。 */
function isSelf(profile: LeaderboardProfile): boolean {
  return profile.identity.kind === 'self'
}
</script>
