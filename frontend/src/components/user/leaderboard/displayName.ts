/**
 * 榜单展示名的唯一出处。
 *
 * 后端只下发结构化 `identity`（`self` / `named` / `anonymous`）与榜单序号 `ordinal`，
 * 从不拼展示名字符串，也永不下发 user_id 与邮箱——展示名一律由这里渲染：
 *
 *   - `self`（本人行与本人相关的卡片）：优先显示自己在个人资料里的 `username`，
 *     只有 username 缺席或为空白时才回退到「当前用户」。本人行的「你」标记与这里无关，
 *     由各组件自己渲染，MUST NOT 把它拼进展示名。
 *   - `named`：直接用后端给的 `username`；`named` 但 username 缺席时回退匿名形态，
 *     不拼任何兜底代号。
 *   - `anonymous`：用榜单序号的假名「第 N 位」（`leaderboard.identity.anonymous`）。
 *     N 是 `ordinal`（榜内连续序号），不是 user_id，也不是会并列跳号的 `rank`。
 *   - `ordinal` 为 null（领先者不在下发的前 50 条里）：「榜外用户」。
 *
 * 「本人看自己的名字」不受站点档位影响：匿名档只约束**他人**的展示形态，本人这一行
 * 本来就只有自己能看见，所以两档同规则，组件不需要再看 mode。
 *
 * 头像（design D25）与展示名走同一套身份规则，因此 `avatarUrl` 也收敛在这里：
 * named 才可能有（后端下发的 64px 小图），self 从自己的个人资料取，anonymous 恒无。
 * 用户关掉「实名参与」后名字与头像一起消失，不另设开关。
 */
import { computed, type ComputedRef } from 'vue'
import { useI18n } from 'vue-i18n'

import { useAuthStore } from '@/stores/auth'
import type { LeaderboardIdentity } from '@/api/leaderboard'

/**
 * 任何带结构化身份的榜单实体：榜单条目、Highlights / Extremes 的持有人、模型画像行。
 * 榜单条目的 `ordinal` 恒为数字，其余几种在「不在前 50」时是 null，这里统一按可空处理。
 */
export interface LeaderboardIdentityHolder {
  identity: LeaderboardIdentity
  ordinal?: number | null
}

export interface LeaderboardDisplayName {
  /** 本人的展示名：有 username 用 username，否则「当前用户」。 */
  selfDisplayName: ComputedRef<string>
  /** 按 identity 与 ordinal 渲染任意一行的展示名。 */
  displayName: (holder: LeaderboardIdentityHolder) => string
  /** 按 identity 取任意一行的头像地址；没有头像时返回空串（组件回退首字母或空圆）。 */
  avatarUrl: (holder: LeaderboardIdentityHolder) => string
}

export function useLeaderboardDisplayName(): LeaderboardDisplayName {
  const { t } = useI18n()
  const authStore = useAuthStore()

  const selfDisplayName = computed(() => {
    // 只认非空白的 username；OAuth 建号等场景下它可能是空串，那时仍走「当前用户」。
    const username = authStore.user?.username?.trim()
    return username ? username : t('channelMonitorV2.currentUser')
  })

  function displayName(holder: LeaderboardIdentityHolder): string {
    const identity = holder.identity
    if (identity.kind === 'self') return selfDisplayName.value
    if (identity.kind === 'named' && identity.username) return identity.username
    if (typeof holder.ordinal === 'number') {
      return t('leaderboard.identity.anonymous', { ordinal: holder.ordinal })
    }
    return t('leaderboard.identity.outOfRank')
  }

  /**
   * 头像地址（design D25）。与展示名同一套身份规则，所以同住这里：
   *
   *   - `self`：从**自己的个人资料**取（auth store 的 `user.avatar_url`）。后端 MUST NOT 为
   *     本人行下发头像——本人行只有自己看得见，没必要为此把原图搬进榜单响应。
   *   - `named`：用后端给的 `identity.avatar_url`，那是 64px 小图的 data URL。
   *     只有内嵌头像才有小图；外链头像（remote_url）不上榜，缺席时回退首字母。
   *   - `anonymous`：恒为空串。给匿名行配头像等于把匿名档拆穿。
   */
  function avatarUrl(holder: LeaderboardIdentityHolder): string {
    const identity = holder.identity
    if (identity.kind === 'self') return authStore.user?.avatar_url?.trim() || ''
    if (identity.kind === 'named') return identity.avatar_url?.trim() || ''
    return ''
  }

  return { selfDisplayName, displayName, avatarUrl }
}
