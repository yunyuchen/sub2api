<template>
  <!-- 05 右栏：当前 Window 前 50 名（与榜单本体同一个数）各自用过的全部模型及占比，按成功请求降序。每行一位用户，chip 的左边界对齐成一条轴。
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
        <!-- 头像与榜单本体同一套规则（design D25）：匿名行传空名 + 空地址 → 素色空圆。 -->
        <span class="rp-u" :class="isSelf(row) ? 'is-self' : ''">
          <UserAvatar
            size="xs"
            class="rp-lb-avatar"
            :name="isAnonymousRow(row) ? '' : displayName(row)"
            :avatar-url="avatarUrl(row)"
            data-testid="leaderboard-avatar"
          />
          <span class="rp-lb-nametext">{{ displayName(row) }}</span>
        </span>
        <span class="rp-cs">
          <span
            v-for="model in row.models"
            :key="model.model"
            data-testid="leaderboard-profiles-chip"
          >
            {{ model.model }}<b>{{ shareLabel(model) }}</b>
          </span>
        </span>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import UserAvatar from '@/components/common/UserAvatar.vue'
import { useLeaderboardDisplayName } from './displayName'
import type { LeaderboardProfile, LeaderboardProfileModel } from '@/api/leaderboard'

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

/** 展示名与头像与榜单条目、Highlights、Extremes 完全同一套规则，收敛在 `displayName.ts`。 */
const { displayName, avatarUrl } = useLeaderboardDisplayName()

/**
 * 后端把占比向下取整成整数，不足 1% 会变成 0：这种模型确实被调过，
 * 显示成 0% 会读成「没用过」，所以写成 <1%。
 */
function shareLabel(model: LeaderboardProfileModel): string {
  return model.share_percent <= 0 ? '<1%' : `${model.share_percent}%`
}

/** 本人那一行用强调色，与榜单里的本人行是同一条线索。 */
function isSelf(profile: LeaderboardProfile): boolean {
  return profile.identity.kind === 'self'
}

/** 匿名行：名字是「第 N 位」假名，头像圆圈里 MUST NOT 出现任何字符。 */
function isAnonymousRow(profile: LeaderboardProfile): boolean {
  return profile.identity.kind !== 'self' && profile.identity.kind !== 'named'
}
</script>
