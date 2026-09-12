/**
 * Leaderboard（排行榜）API。
 *
 * 只有一个只读接口 `GET /leaderboard`，Window（榜单窗口）与 Metric（排名指标）
 * 由查询参数表达，两者都有默认值（today / total_tokens），非法值由后端 400。
 * 响应类型就地定义在本文件并 export（同 `channelMonitorV2.ts` 的做法）。
 *
 * 档位对字段的影响 —— 「档位决定字段是否存在」，而不是把字段清零：
 *   - `anonymous` 档下他人条目 **没有** `total_tokens` / `successful_requests`，
 *     只有 `total_tokens_relative_percent` / `successful_requests_relative_percent`；
 *     本人条目（`is_self`）与 `my_rank` 始终是真实绝对值。
 *   - `named` 档与 `off` 档 Preview（预览）下所有数值精确。
 * 因此绝对值字段与相对百分比字段都是可选的，读取时 MUST NOT 把缺席当成 0。
 */

import { apiClient } from './client'
// 类型-only 引用：编译期擦除，不会把 pinia store 拉进 api 层。
import type { LeaderboardMode } from '@/utils/featureFlags'

/** 榜单窗口：今日 / 本周 / 本月，边界按站点时区计算，周一起算。 */
export type LeaderboardWindow = 'today' | 'week' | 'month'

/** 排名指标：只有这两项，金额既不展示也不作为排序项。 */
export type LeaderboardMetric = 'total_tokens' | 'successful_requests'

/** `computing` 表示 Snapshot（榜单快照）尚未生成，不是空榜也不是错误。 */
export type LeaderboardStatus = 'ready' | 'computing'

/** 身份形态。后端不拼展示名字符串，文案一律由前端 i18n 渲染。 */
export type LeaderboardIdentityKind = 'self' | 'anonymous' | 'named'

/**
 * 结构化身份。展示名的渲染规则收敛在 `@/components/user/leaderboard/displayName.ts`：
 * `self` → 本人自己的昵称（auth store 的 `user.username`），缺席时才回退「当前用户」；
 * `anonymous` → 「第 Ordinal 位」；`named` → 直接用 `username`。
 * 响应永不下发 user_id 与邮箱，展示名里也永不出现用户 id。
 */
export interface LeaderboardIdentity {
  kind: LeaderboardIdentityKind
  /** 仅 `named` 形态携带。 */
  username?: string
}

export interface LeaderboardEntry {
  /** 竞争排名：严格高于自己的合格用户数 + 1，并列同名次其后跳号（1、1、3）。 */
  rank: number
  /** 榜内连续序号，1..N 唯一不跳号；只用于匿名展示名，MUST NOT 当成名次。 */
  ordinal: number
  identity: LeaderboardIdentity
  is_self: boolean
  /** `anonymous` 档的他人条目缺席该字段。 */
  total_tokens?: number
  /** `anonymous` 档的他人条目缺席该字段。 */
  successful_requests?: number
  /** 仅 `anonymous` 档的他人条目出现：相对该 Metric 第一名的 0–100 整数百分比。 */
  total_tokens_relative_percent?: number
  /** 仅 `anonymous` 档的他人条目出现：相对该 Metric 第一名的 0–100 整数百分比。 */
  successful_requests_relative_percent?: number
}

/**
 * 「你的位置」那一句提示所需的数值。文案由前端按 `kind` 选，后端只下发数值（design D18）：
 *   - `tokens_to_top10` / `requests_to_top10`（`named` 档与 Preview）：`value` 是按当前 Metric
 *     还差多少才够得着第 10 名。量词由 kind 自己带，读取时 MUST NOT 再去看顶层的 `metric`；
 *   - `relative_percent`（`anonymous` 档）：`self` 与 `tenth` 分别是本人与第 10 名相对第一名的
 *     整数百分比。他人的绝对量连差额形式都不出现。
 * 目标名次固定是前 10（写在 kind 名字里），后端不单独下发。本人已经在前 10 之内、或参与人数
 * 不足 10 人时整个 `hint` 缺席——那不是「没话说」，页面按 `rank` 自己补一句「已进前 10」。
 * 所需数值缺席时隐藏这一行，MUST NOT 拼一句半截话。
 */
export type LeaderboardMyRankHintKind =
  | 'tokens_to_top10'
  | 'requests_to_top10'
  | 'relative_percent'

export interface LeaderboardMyRankHint {
  kind: LeaderboardMyRankHintKind
  /** 仅两个 `*_to_top10` 形态：还差多少个该 Metric 的绝对量。 */
  value?: number
  /** 仅 `relative_percent` 形态：本人相对第一名的 0–100 整数百分比。 */
  self?: number
  /** 仅 `relative_percent` 形态：第 10 名相对第一名的 0–100 整数百分比。 */
  tenth?: number
}

/** 查看者在该 Window 内零用量时整体为 null，而不是给一个巨大的名次。 */
export interface LeaderboardMyRank {
  rank: number
  total_tokens: number
  successful_requests: number
  hint?: LeaderboardMyRankHint | null
}

/**
 * Highlights（趣味卡）与 Insights（洞察）的档位裁剪同 `entries`：
 * `anonymous` 档下任何**他人的**或**站点级的**绝对量字段都缺席，只留相对量与比率，
 * 读取时 MUST NOT 把缺席当成 0。相对量（share / lead / relative / change）与
 * `cache_hit_rate`、`peak_hour` 两档都下发。
 */
export interface LeaderboardHighlightUser {
  identity: LeaderboardIdentity
  /** 该用户在当前 Window + Metric 榜单上的 Ordinal；不在前 50 时为 null（渲染成「榜外用户」）。 */
  ordinal: number | null
  /** `anonymous` 档缺席。 */
  total_tokens?: number
  /** `anonymous` 档缺席。 */
  successful_requests?: number
  /** 占全站该 Metric 的 0–100 整数百分比。 */
  share_percent: number
  /** 比第 2 名多出的整数百分比。 */
  lead_percent: number
}

export interface LeaderboardCacheKing {
  identity: LeaderboardIdentity
  ordinal: number | null
  /** 0–1 的比率，页面按一位小数的百分比渲染。 */
  cache_hit_rate: number
  /** 该用户该窗口请求最多的模型；无法确定时是空串。 */
  dominant_model: string
}

export interface LeaderboardSiteSummary {
  /** `anonymous` 档缺席。 */
  total_tokens?: number
  /** `anonymous` 档缺席。 */
  successful_requests?: number
  /** 与顶层 `participant_count` 同一套规则：`named` 精确整数，`anonymous` 分档字符串。 */
  participant_count: number | string
  cache_hit_rate?: number
  /** 全站峰值小时（0–23）。 */
  peak_hour?: number
  /** 全站该窗口 tokens / 成功请求数。比率，两档都下发；成功请求数为 0 时缺席。 */
  avg_tokens_per_request?: number
}

/**
 * Extremes（之最）的六张小卡。身份与 `ordinal` 的规则同榜单条目与 Highlights：
 * 领先者不在下发的 `entries` 里时 `ordinal` 为 null（渲染成「榜外用户」）。
 * 绝对量（`night_tokens` / `max_single_tokens`）在 `anonymous` 档缺席，
 * 相对量（share / change / ratio / days / distinct_models）两档都下发。
 */
export interface LeaderboardExtremeBase {
  identity: LeaderboardIdentity
  ordinal: number | null
}

export interface LeaderboardExtremeNightOwl extends LeaderboardExtremeBase {
  /** `anonymous` 档缺席。 */
  night_tokens?: number
  /** 其 0–6 点 tokens 占其自身该窗口 tokens 的 0–100 整数百分比。 */
  night_share_percent: number
}

export interface LeaderboardExtremeRising extends LeaderboardExtremeBase {
  /** 今日相对昨日的整数增幅百分比。 */
  change_percent: number
}

export interface LeaderboardExtremeOmnivore extends LeaderboardExtremeBase {
  distinct_models: number
}

export interface LeaderboardExtremeTalker extends LeaderboardExtremeBase {
  /** output_tokens / total_tokens 的 0–100 整数百分比。 */
  output_share_percent: number
}

export interface LeaderboardExtremeMaxSingle extends LeaderboardExtremeBase {
  /** `anonymous` 档缺席。 */
  max_single_tokens?: number
  /** 相对全体参与者各自单次最大值之中位数的倍数，一位小数。 */
  ratio_to_median: number
}

export interface LeaderboardExtremeStreak extends LeaderboardExtremeBase {
  days: number
}

/** 无人满足条件时该项为 null，MUST NOT 用零值对象顶替。 */
export interface LeaderboardExtremes {
  night_owl: LeaderboardExtremeNightOwl | null
  /** 只在 `window=today` 时可能有值，其余窗口必然为 null，页面不渲染那张卡。 */
  rising: LeaderboardExtremeRising | null
  omnivore: LeaderboardExtremeOmnivore | null
  talker: LeaderboardExtremeTalker | null
  max_single: LeaderboardExtremeMaxSingle | null
  streak: LeaderboardExtremeStreak | null
}

export interface LeaderboardHighlights {
  top_tokens: LeaderboardHighlightUser | null
  top_requests: LeaderboardHighlightUser | null
  /** 该窗口无人达到成功请求门槛时为 null。 */
  cache_king: LeaderboardCacheKing | null
  site: LeaderboardSiteSummary
  /** 旧后端不下发；六项各自可为 null。 */
  extremes?: LeaderboardExtremes | null
}

/** 今日模型热度：计数带成功落账过滤，与榜单同口径。 */
export interface LeaderboardModelUsage {
  model: string
  /** `anonymous` 档缺席。 */
  successful_requests?: number
  share_percent: number
}

/**
 * 近 30 天日桶。来源是仪表盘预聚合表，`requests` 是裸 COUNT(*)（含失败请求的占位记录），
 * 与榜单和模型热度的 Successful Requests 不是同一个口径，页面要注明。
 */
export interface LeaderboardDailyBucket {
  /** `YYYY-MM-DD`，按站点时区。 */
  date: string
  /** `anonymous` 档缺席。 */
  requests?: number
  /** `anonymous` 档缺席。 */
  total_tokens?: number
  /** 相对这 30 天里最高一天的 0–100 整数百分比，两档都下发。 */
  relative_percent: number
}

/** 今日 24 个小时桶，口径同 `LeaderboardDailyBucket`。 */
export interface LeaderboardHourlyBucket {
  /** 0–23，按站点时区。 */
  hour: number
  /** `anonymous` 档缺席。 */
  requests?: number
  /** 相对峰值小时的 0–100 整数百分比；峰值小时就是取 100 的那个桶。 */
  relative_percent: number
}

export interface LeaderboardCacheToday {
  /** 0–1 的比率。 */
  cache_hit_rate: number
  /** `anonymous` 档缺席。 */
  cache_read_tokens?: number
  /** `anonymous` 档缺席。 */
  input_tokens?: number
}

export interface LeaderboardMonth {
  /** `anonymous` 档缺席。 */
  total_tokens?: number
  /** 较上月的整数百分比，可为负；上月绝对量永不下发。 */
  change_percent: number
}

/** 模型偏好画像：当前 Window 前 50 名（与榜单本体同一个数）各自的 Top 3 模型。占比两档都下发。 */
export interface LeaderboardProfileModel {
  model: string
  share_percent: number
}

export interface LeaderboardProfile {
  identity: LeaderboardIdentity
  /** 规则同榜单条目：不在下发的 `entries` 里时为 null。 */
  ordinal: number | null
  models: LeaderboardProfileModel[]
}

/** 今日请求按平台。`successful_requests` 在 `anonymous` 档缺席。 */
export interface LeaderboardPlatform {
  platform: string
  successful_requests?: number
  share_percent: number
}

/** 今日 token 构成：四段占比两档都下发，四个绝对量只在 `named` 档与 Preview 下出现。 */
export interface LeaderboardComposition {
  input_tokens?: number
  output_tokens?: number
  cache_creation_tokens?: number
  cache_read_tokens?: number
  input_percent: number
  output_percent: number
  cache_creation_percent: number
  cache_read_percent: number
}

/** 近 14 天每天的缓存命中率，0–1 的比率，两档都下发。 */
export interface LeaderboardCacheTrendPoint {
  /** `YYYY-MM-DD`，按站点时区。 */
  date: string
  cache_hit_rate: number
}

/**
 * 站点级洞察，与 Window 无关（模型热度、时段与缓存都是「今日」语义），
 * 唯一的例外是 `profiles`——它按当前 Window 计算，存在该 Window 的 highlights 里，
 * 因此 `status` 为 `computing` 时它随 highlights 一起不可用。
 * 预聚合表缺行时对应区块整块为 null，页面隐藏该区块而不是用 0 填充。
 */
export interface LeaderboardInsights {
  models_today: LeaderboardModelUsage[] | null
  daily_30: LeaderboardDailyBucket[] | null
  hourly_today: LeaderboardHourlyBucket[] | null
  cache_today: LeaderboardCacheToday | null
  month: LeaderboardMonth | null
  /** 以下五块是 v2 新增；旧后端不下发，来源缺失时为 null。 */
  profiles?: LeaderboardProfile[] | null
  platforms_today?: LeaderboardPlatform[] | null
  /** 7 × 24 的 0–4 等级，行序周一起；两档完全相同。 */
  weekly_rhythm?: number[][] | null
  composition_today?: LeaderboardComposition | null
  cache_trend_14?: LeaderboardCacheTrendPoint[] | null
}

/** 名次走势的一个点：`rank` 越小名次越高，画折线时要翻转纵轴。 */
export interface LeaderboardRankPoint {
  /** `YYYY-MM-DD`，按站点时区。 */
  date: string
  rank: number
}

export interface LeaderboardViewerModel {
  model: string
  successful_requests: number
  share_percent: number
}

/**
 * 查看者**本人**的数据。任何档位下都是真实值，不受档位裁剪影响，
 * 也不随 `status` 变化（不出自 Snapshot）。没有历史时 `rank_history` 是空数组，
 * 本窗口零用量时 `models` 是空数组且两个比率字段缺席。
 */
export interface LeaderboardViewer {
  rank_history: LeaderboardRankPoint[]
  models: LeaderboardViewerModel[]
  cache_hit_rate?: number
  /** 本人该窗口 tokens / 成功请求数。 */
  avg_tokens_per_request?: number
}

export interface LeaderboardResponse {
  /** 回显实际生效的取值。 */
  window: LeaderboardWindow
  metric: LeaderboardMetric
  /** Preview 下回显 `off`。 */
  mode: LeaderboardMode
  /** 仅 `off` 档的管理员为 true，页面据此显示「普通用户不可见」横幅。 */
  preview: boolean
  /** 窗口边界所用的站点时区名，页面常驻展示。 */
  timezone: string
  status: LeaderboardStatus
  /** snapshot_updated_at 距当前超过 15 分钟为 true，此时仍展示数据但要给警告。 */
  stale: boolean
  /** `status === 'computing'` 时为 null。 */
  snapshot_updated_at: string | null
  /**
   * 参与人数。类型随档位变化：`named` 档与 Preview 下是精确整数，
   * `anonymous` 档下是分档字符串（如 `100+`，少于 5 为 `<5`）。
   */
  participant_count: number | string
  entries: LeaderboardEntry[]
  /**
   * 仅 `anonymous` 档因参与人数过少而不下发条目时为 true。
   * 抑制态与空态的 `entries` 都是空数组，必须靠这个显式信号区分，
   * MUST NOT 从「entries 是空数组」反推。
   */
  entries_suppressed: boolean
  my_rank: LeaderboardMyRank | null
  /** `status === 'computing'` 时为 null；旧后端不下发该字段，页面按骨架处理。 */
  highlights?: LeaderboardHighlights | null
  /** 不随快照状态变化，来源缺失时按区块各自为 null；旧后端不下发该字段。 */
  insights?: LeaderboardInsights | null
  /**
   * 查看者本人的数据。后端 MUST NOT 下发 null，但旧后端根本不下发该字段，
   * 因此类型上仍是可选，读取时按「缺席即这一块不渲染」处理。
   */
  viewer?: LeaderboardViewer | null
}

export interface GetLeaderboardParams {
  window?: LeaderboardWindow
  metric?: LeaderboardMetric
}

/**
 * 读取榜单。省略参数时由后端按 today / total_tokens 取默认值。
 * `off` 档下普通用户会拿到 404（不确认功能存在），管理员拿到 preview 响应。
 */
export async function getLeaderboard(
  params: GetLeaderboardParams = {},
  signal?: AbortSignal
): Promise<LeaderboardResponse> {
  const query: Record<string, string> = {}
  if (params.window) query.window = params.window
  if (params.metric) query.metric = params.metric

  const { data } = await apiClient.get<LeaderboardResponse>('/leaderboard', {
    params: query,
    signal,
  })
  return data
}
