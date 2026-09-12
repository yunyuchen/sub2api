package service

import (
	"math"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// 本文件是 Highlights（趣味卡）与 Insights（洞察）的**存储层**形状：它们和 Snapshot
// （榜单快照）一样在后台作业里算好、以 JSON 串写进 Redis，请求路径只读（design D17）。
//
// 与响应视图刻意分成两套类型：这里只有 user_id 与数值，MUST NOT 出现 username、邮箱与
// 任何金额——身份仍在响应组装时按 users 当前状态渲染（design D8），档位裁剪也只发生在
// 响应组装那一处（design D18）。

const (
	// leaderboardCacheKingMinRequests 是效率之星的参评门槛：该 Window 的 Successful Requests
	// 必须**大于**这个数。几次请求就能刷出 100% 命中率，门槛低了这张卡就只是噪声。
	leaderboardCacheKingMinRequests = 5

	// leaderboardTopModelsLimit 是今日模型热度的条数（mockup 的 Top 8）。
	leaderboardTopModelsLimit = 8

	// leaderboardDailyInsightDays 是近 30 天活跃度 / 用量趋势的天数（含今日）。
	leaderboardDailyInsightDays = 30

	// leaderboardHoursPerDay 是今日时段分布的桶数：有数据时补齐 0–23 点，
	// 缺的小时是「今天这个点真的没有请求」，不是「数据缺失」。
	leaderboardHoursPerDay = 24

	// leaderboardInsightDateLayout 是 daily_30 的日期串格式；桶边界已是站点时区，
	// 因此直接格式化 DATE 值本身，MUST NOT 再做一次时区转换。
	leaderboardInsightDateLayout = "2006-01-02"

	// leaderboardTalkerMinRequests 是话痨的参评门槛：该 Window 的 Successful Requests
	// 必须**不少于**这个数。一两次请求就能刷出极端的 output 占比，门槛低了这张卡只是噪声。
	// 与 leaderboardCacheKingMinRequests 是同一类阈值，但边界不同（这里含等于）。
	leaderboardTalkerMinRequests = 5

	// leaderboardStreakLookbackDays 是连续活跃的回溯天数，与名次历史的保留期同一个数。
	leaderboardStreakLookbackDays = 90

	// leaderboardProfilesLimit 是模型偏好画像的用户数：该 Window Total Tokens 前 50 名，
	// 与榜单本体的 Top 50（LeaderboardTopEntryLimit）是同一个数——榜上有名的人在画像里都能找到自己。
	leaderboardProfilesLimit = LeaderboardTopEntryLimit

	// leaderboardRhythmLookbackDays 是周内节奏的回溯天数（近 4 周）。
	leaderboardRhythmLookbackDays = 28

	// leaderboardRhythmLevels 是周内节奏热力图的等级数：0–4 共五档，两档完全相同。
	leaderboardRhythmLevels = 5

	// leaderboardCacheTrendDays 是缓存命中率趋势的天数（含今日）。
	leaderboardCacheTrendDays = 14

	// leaderboardWeekdaysPerWeek 是周内节奏的行数：7 行，行序周一起。
	leaderboardWeekdaysPerWeek = 7

	// leaderboardRankHistoryDays 是名次走势下发的天数（含今日）。
	leaderboardRankHistoryDays = 14

	// leaderboardRankHistoryRetentionDays 是名次历史的保留期，与连续活跃的回溯窗口同一个数：
	// 一个数管两处，也给 14 天的走势留足余量。
	leaderboardRankHistoryRetentionDays = 90

	// leaderboardViewerModelsLimit 是「你的模型偏好」下发的条数（本人 Top 5）。
	leaderboardViewerModelsLimit = 5
)

// LeaderboardHighlights 是一个 Window（榜单窗口）的四块趣味卡数据，整块存成一个 JSON 串。
// 无人满足条件的卡为 nil，MUST NOT 用零值对象顶替。
//
// Extremes（之最）与 Profiles（模型偏好画像）都是**按 Window** 算的，因此和四张卡存在
// 同一个 JSON 串里：同一轮写入、同一批 RENAME、同一个 TTL，一次请求读到的四者必然同源
// （design D20 / D22）。下发时 Profiles 按页面区块挂到 insights.profiles 下——
// 存储按「重建时按什么分组」划分，下发按「页面上属于哪个区块」划分。
type LeaderboardHighlights struct {
	TopTokens   *LeaderboardHighlightUser `json:"top_tokens,omitempty"`
	TopRequests *LeaderboardHighlightUser `json:"top_requests,omitempty"`
	CacheKing   *LeaderboardCacheKing     `json:"cache_king,omitempty"`
	Site        LeaderboardSiteSummary    `json:"site"`
	Extremes    *LeaderboardExtremes      `json:"extremes,omitempty"`
	Profiles    []LeaderboardProfile      `json:"profiles,omitempty"`
}

// LeaderboardExtremes 是一个 Window 的六项之最。每项无人满足条件时为 nil，
// MUST NOT 用零值对象顶替——「没有夜猫子」与「夜猫子是 0 tokens」是两回事。
type LeaderboardExtremes struct {
	NightOwl  *LeaderboardExtremeNightOwl  `json:"night_owl,omitempty"`
	Rising    *LeaderboardExtremeRising    `json:"rising,omitempty"`
	Omnivore  *LeaderboardExtremeOmnivore  `json:"omnivore,omitempty"`
	Talker    *LeaderboardExtremeTalker    `json:"talker,omitempty"`
	MaxSingle *LeaderboardExtremeMaxSingle `json:"max_single,omitempty"`
	Streak    *LeaderboardExtremeStreak    `json:"streak,omitempty"`
}

// LeaderboardExtremeNightOwl 是夜猫子：该窗口 0–6 点（站点时区）Total Tokens 最多者。
// NightSharePercent 是其夜间 tokens 占自身该窗口 tokens 的整数百分比（相对量，两档都下发）；
// NightTokens 是绝对量，在 anonymous 档被响应层裁掉。
type LeaderboardExtremeNightOwl struct {
	UserID            int64 `json:"user_id"`
	NightTokens       int64 `json:"night_tokens"`
	NightSharePercent int   `json:"night_share_percent"`
}

// LeaderboardExtremeRising 是进步之星：今日 Total Tokens 相对昨日增幅最大者。
// 只在 today 窗口存在——聚合里只有「昨日」一组基线列，没有同口径的「上周 / 上月」。
type LeaderboardExtremeRising struct {
	UserID        int64 `json:"user_id"`
	ChangePercent int   `json:"change_percent"`
}

// LeaderboardExtremeOmnivore 是杂食者：该窗口用过的不同模型数最多者。
type LeaderboardExtremeOmnivore struct {
	UserID         int64 `json:"user_id"`
	DistinctModels int   `json:"distinct_models"`
}

// LeaderboardExtremeTalker 是话痨：output_tokens / total_tokens 最高者，
// 参评门槛是该窗口成功请求数不少于 leaderboardTalkerMinRequests。
type LeaderboardExtremeTalker struct {
	UserID             int64 `json:"user_id"`
	OutputSharePercent int   `json:"output_share_percent"`
}

// LeaderboardExtremeMaxSingle 是单次最大：单次请求 tokens 最大者。
// MaxSingleTokens 是绝对量，anonymous 档缺席；RatioToMedian 是相对全体参与者
// 「各自单次最大值」中位数的倍数（一位小数），两档都下发。
type LeaderboardExtremeMaxSingle struct {
	UserID          int64   `json:"user_id"`
	MaxSingleTokens int64   `json:"max_single_tokens"`
	RatioToMedian   float64 `json:"ratio_to_median"`
}

// LeaderboardExtremeStreak 是连续活跃：截至今日或昨日连续有用量天数最长者。
// 它与 Window 无关，三个窗口的这一项内容相同。
type LeaderboardExtremeStreak struct {
	UserID int64 `json:"user_id"`
	Days   int   `json:"days"`
}

// LeaderboardProfile 是模型偏好画像的一行：某个用户与他在该窗口用过的全部模型（按成功请求降序，不截断）。
// 只有 user_id 与占比，身份仍在响应组装时按 users 当前状态渲染。
type LeaderboardProfile struct {
	UserID int64                     `json:"user_id"`
	Models []LeaderboardProfileModel `json:"models"`
}

// LeaderboardProfileModel 是画像里的一个模型：占该用户该窗口成功请求的整数百分比。
type LeaderboardProfileModel struct {
	Model        string `json:"model"`
	SharePercent int    `json:"share_percent"`
}

// LeaderboardHighlightUser 是某个 Metric（排名指标）的领先者。
//
// SharePercent 是其占全站该 Metric 的整数百分比，LeadPercent 是比第 2 名多出的整数百分比；
// 两个相对量在任何档位都下发，绝对量则在 anonymous 档被响应层裁掉。
type LeaderboardHighlightUser struct {
	UserID             int64 `json:"user_id"`
	TotalTokens        int64 `json:"total_tokens"`
	SuccessfulRequests int64 `json:"successful_requests"`
	SharePercent       int   `json:"share_percent"`
	LeadPercent        int   `json:"lead_percent"`
}

// LeaderboardCacheKing 是效率之星：该窗口 Cache Hit Rate（缓存命中率）最高的用户。
// CacheHitRate 是 0–1 的比率；DominantModel 是该用户该窗口成功请求最多的模型，
// 由一条只对这一个用户执行的聚合得出（design D17），查不到时为空串。
type LeaderboardCacheKing struct {
	UserID        int64   `json:"user_id"`
	CacheHitRate  float64 `json:"cache_hit_rate"`
	DominantModel string  `json:"dominant_model"`
}

// LeaderboardSiteSummary 是全站概况卡。
//
// CacheHitRate 按该窗口所有参与者的 input / cache_read 汇总；分母为 0 时字段缺席，
// MUST NOT 记成 0。PeakHour 来自今日的小时桶（见 LeaderboardInsights.HourlyToday），
// 预聚合缺行时缺席。
// AvgTokensPerRequest 是全站该窗口 tokens / 成功请求数，供「你 vs 全站」并排对比；
// 它是比率，按 D18 两档都下发，全站成功请求数为 0 时字段缺席（同 CacheHitRate 的写法）。
type LeaderboardSiteSummary struct {
	TotalTokens         int64    `json:"total_tokens"`
	SuccessfulRequests  int64    `json:"successful_requests"`
	ParticipantCount    int64    `json:"participant_count"`
	CacheHitRate        *float64 `json:"cache_hit_rate,omitempty"`
	PeakHour            *int     `json:"peak_hour,omitempty"`
	AvgTokensPerRequest *float64 `json:"avg_tokens_per_request,omitempty"`
}

// LeaderboardInsights 是与 Window 无关的站点级洞察，整块存成一个 JSON 串。
// 任一区块的数据源缺行时该区块为 nil（切片为空、指针为 nil），MUST NOT 用 0 填充，
// 也 MUST NOT 让整轮作业失败（design D17）。
type LeaderboardInsights struct {
	ModelsToday []LeaderboardModelInsight  `json:"models_today,omitempty"`
	Daily30     []LeaderboardDailyInsight  `json:"daily_30,omitempty"`
	HourlyToday []LeaderboardHourlyInsight `json:"hourly_today,omitempty"`
	CacheToday  *LeaderboardCacheInsight   `json:"cache_today,omitempty"`
	Month       *LeaderboardMonthInsight   `json:"month,omitempty"`

	// v2 新增的四块（design D22）。profiles 不在这里：它是按 Window 算的，
	// 存在该 Window 的 Highlights JSON 里，只在下发时才挂到 insights 下。
	PlatformsToday   []LeaderboardPlatformInsight   `json:"platforms_today,omitempty"`
	WeeklyRhythm     [][]int                        `json:"weekly_rhythm,omitempty"`
	CompositionToday *LeaderboardCompositionInsight `json:"composition_today,omitempty"`
	CacheTrend14     []LeaderboardCacheTrendInsight `json:"cache_trend_14,omitempty"`
}

// LeaderboardPlatformInsight 是今日平台分布的一行。SuccessfulRequests 是成功落账口径，
// 是站点级绝对量，在 anonymous 档被响应层裁掉；SharePercent 两档都下发。
type LeaderboardPlatformInsight struct {
	Platform           string `json:"platform"`
	SuccessfulRequests int64  `json:"successful_requests"`
	SharePercent       int    `json:"share_percent"`
}

// LeaderboardCompositionInsight 是今日 Token 构成：四段绝对量与四段占比。
// 绝对量是站点级的，anonymous 档缺席；四个百分比两档都下发。
// 来源是今日窗口聚合的站点合计，MUST NOT 另查一次。
type LeaderboardCompositionInsight struct {
	InputTokens          int64 `json:"input_tokens"`
	OutputTokens         int64 `json:"output_tokens"`
	CacheCreationTokens  int64 `json:"cache_creation_tokens"`
	CacheReadTokens      int64 `json:"cache_read_tokens"`
	InputPercent         int   `json:"input_percent"`
	OutputPercent        int   `json:"output_percent"`
	CacheCreationPercent int   `json:"cache_creation_percent"`
	CacheReadPercent     int   `json:"cache_read_percent"`
}

// LeaderboardCacheTrendInsight 是缓存命中率趋势的一天：cache_read /(input + cache_read)。
// 命中率是比率，两档都下发。
type LeaderboardCacheTrendInsight struct {
	Date         string  `json:"date"`
	CacheHitRate float64 `json:"cache_hit_rate"`
}

// LeaderboardModelInsight 是今日模型热度的一行。SuccessfulRequests 是成功落账口径
// （actual_cost > 0），与下面两张预聚合表来的 Requests 不是一个口径，因此不同名。
type LeaderboardModelInsight struct {
	Model              string `json:"model"`
	SuccessfulRequests int64  `json:"successful_requests"`
	SharePercent       int    `json:"share_percent"`
}

// LeaderboardDailyInsight 是近 30 天的一天。
//
// Requests 来自 usage_dashboard_daily.total_requests，是**全部请求**的裸 COUNT(*)（含失败
// 请求的占位记录），与榜单的 Successful Requests 不是同一个口径。
// RelativePercent 相对这 30 天里 Total Tokens 最高的一天，热力图色阶与趋势条宽都用它，
// 因此口径统一取 tokens（趋势卡展示的数值也是 tokens）。
type LeaderboardDailyInsight struct {
	Date            string `json:"date"`
	Requests        int64  `json:"requests"`
	TotalTokens     int64  `json:"total_tokens"`
	RelativePercent int    `json:"relative_percent"`
}

// LeaderboardHourlyInsight 是今日的一个小时桶。Requests 口径同 LeaderboardDailyInsight；
// RelativePercent 相对峰值小时，峰值小时就是 RelativePercent 为 100 的那个桶。
type LeaderboardHourlyInsight struct {
	Hour            int   `json:"hour"`
	Requests        int64 `json:"requests"`
	RelativePercent int   `json:"relative_percent"`
}

// LeaderboardCacheInsight 是今日全站缓存命中，由今日小时桶的同名列汇总而来，不另查一次。
type LeaderboardCacheInsight struct {
	CacheHitRate    float64 `json:"cache_hit_rate"`
	CacheReadTokens int64   `json:"cache_read_tokens"`
	InputTokens     int64   `json:"input_tokens"`
}

// LeaderboardMonthInsight 是「本月累计」与「较上月」。
// 上月的绝对量只参与算 ChangePercent，MUST NOT 出现在这个结构里，因此也无从下发。
type LeaderboardMonthInsight struct {
	TotalTokens   int64 `json:"total_tokens"`
	ChangePercent int   `json:"change_percent"`
}

// computeLeaderboardHighlights 由一个 Window 的全部快照条目算出四块趣味卡。
//
// 遍历只做一次：领先者、效率之星与全站汇总同源，因此页面上的四张卡不会互相矛盾。
// peakHour 来自今日的小时桶（与 Window 无关），预聚合缺行时传 nil。
// entries 为空（该窗口没有任何合格用量）时返回 nil——空窗口没有可言说的 Highlights。
func computeLeaderboardHighlights(entries []LeaderboardUserMetrics, peakHour *int) *LeaderboardHighlights {
	if len(entries) == 0 {
		return nil
	}

	var (
		siteTokens    int64
		siteRequests  int64
		siteInput     int64
		siteCacheRead int64

		topTokens, secondTokens     int64
		topRequests, secondRequests int64
		topTokensUser               *LeaderboardUserMetrics
		topRequestsUser             *LeaderboardUserMetrics

		cacheKingUser *LeaderboardUserMetrics
		cacheKingRate float64
	)

	for i := range entries {
		entry := entries[i]
		siteTokens += entry.TotalTokens
		siteRequests += entry.SuccessfulRequests
		siteInput += entry.InputTokens
		siteCacheRead += entry.CacheReadTokens

		// 第 2 名取「除冠军那一个条目之外的最大值」：两人并列第一时 lead_percent 为 0，
		// 与 mockup 的「比并列第 2 名多 0%」一致。
		switch {
		case entry.TotalTokens > topTokens:
			secondTokens = topTokens
			topTokens = entry.TotalTokens
			topTokensUser = &entries[i]
		case entry.TotalTokens > secondTokens:
			secondTokens = entry.TotalTokens
		}
		if entry.TotalTokens == topTokens && topTokensUser != nil && entry.UserID < topTokensUser.UserID {
			// 同值时固定取 user_id 较小者，避免同一份数据每轮换人。
			topTokensUser = &entries[i]
		}

		switch {
		case entry.SuccessfulRequests > topRequests:
			secondRequests = topRequests
			topRequests = entry.SuccessfulRequests
			topRequestsUser = &entries[i]
		case entry.SuccessfulRequests > secondRequests:
			secondRequests = entry.SuccessfulRequests
		}
		if entry.SuccessfulRequests == topRequests && topRequestsUser != nil && entry.UserID < topRequestsUser.UserID {
			topRequestsUser = &entries[i]
		}

		// 效率之星：请求数够多才参评，且该窗口 input + cache_read 为 0 的用户没有命中率。
		if entry.SuccessfulRequests <= leaderboardCacheKingMinRequests {
			continue
		}
		rate, ok := leaderboardCacheHitRate(entry.InputTokens, entry.CacheReadTokens)
		if !ok {
			continue
		}
		if cacheKingUser == nil || rate > cacheKingRate ||
			(rate == cacheKingRate && entry.UserID < cacheKingUser.UserID) {
			cacheKingUser, cacheKingRate = &entries[i], rate
		}
	}

	highlights := &LeaderboardHighlights{
		Site: LeaderboardSiteSummary{
			TotalTokens:        siteTokens,
			SuccessfulRequests: siteRequests,
			ParticipantCount:   int64(len(entries)),
			PeakHour:           peakHour,
		},
	}
	if rate, ok := leaderboardCacheHitRate(siteInput, siteCacheRead); ok {
		highlights.Site.CacheHitRate = &rate
	}
	// 全站平均每请求 tokens：成功请求数为 0 时字段缺席，MUST NOT 记成 0——
	// 「今天还没有成功请求」与「每个请求 0 tokens」是两回事，写法与 CacheHitRate 一致。
	if siteRequests > 0 {
		avg := float64(siteTokens) / float64(siteRequests)
		highlights.Site.AvgTokensPerRequest = &avg
	}

	if topTokensUser != nil && topTokens > 0 {
		highlights.TopTokens = &LeaderboardHighlightUser{
			UserID:             topTokensUser.UserID,
			TotalTokens:        topTokensUser.TotalTokens,
			SuccessfulRequests: topTokensUser.SuccessfulRequests,
			SharePercent:       leaderboardRelativePercent(topTokensUser.TotalTokens, siteTokens),
			LeadPercent:        leaderboardLeadPercent(topTokens, secondTokens),
		}
	}
	if topRequestsUser != nil && topRequests > 0 {
		highlights.TopRequests = &LeaderboardHighlightUser{
			UserID:             topRequestsUser.UserID,
			TotalTokens:        topRequestsUser.TotalTokens,
			SuccessfulRequests: topRequestsUser.SuccessfulRequests,
			SharePercent:       leaderboardRelativePercent(topRequestsUser.SuccessfulRequests, siteRequests),
			LeadPercent:        leaderboardLeadPercent(topRequests, secondRequests),
		}
	}
	if cacheKingUser != nil {
		// DominantModel 由调用方补：那是一条按 (user_id, model) 的聚合，只对这一个用户查一次。
		highlights.CacheKing = &LeaderboardCacheKing{UserID: cacheKingUser.UserID, CacheHitRate: cacheKingRate}
	}
	return highlights
}

// leaderboardCacheHitRate 是 cache_read /(input + cache_read)，分母为 0 时 ok=false：
// 「没有命中率」与「命中率是 0」是两回事，后者会被读成「一次缓存都没命中」。
func leaderboardCacheHitRate(inputTokens, cacheReadTokens int64) (float64, bool) {
	denominator := inputTokens + cacheReadTokens
	if denominator <= 0 {
		return 0, false
	}
	return float64(cacheReadTokens) / float64(denominator), true
}

// leaderboardLeadPercent 是第 1 名比第 2 名多出的整数百分比（向下取整）。
// 并列时为 0；没有第 2 名或第 2 名为 0 时封顶按 100（「领先一倍」）表达，
// 不给无穷大，也不留一个能反推绝对量的超大数。
func leaderboardLeadPercent(first, second int64) int {
	if first <= 0 {
		return 0
	}
	if second <= 0 {
		return 100
	}
	if first <= second {
		return 0
	}
	return int((first - second) * 100 / second)
}

// buildLeaderboardModelInsights 把今日按模型的聚合行转成模型热度区块。
// total 是今日全站成功请求总数（不止 Top N），即 share_percent 的分母。
func buildLeaderboardModelInsights(rows []usagestats.LeaderboardModelUsageRow, total int64) []LeaderboardModelInsight {
	if len(rows) == 0 {
		return nil
	}
	insights := make([]LeaderboardModelInsight, 0, len(rows))
	for _, row := range rows {
		insights = append(insights, LeaderboardModelInsight{
			Model:              row.Model,
			SuccessfulRequests: row.SuccessfulRequests,
			SharePercent:       leaderboardRelativePercent(row.SuccessfulRequests, total),
		})
	}
	return insights
}

// buildLeaderboardDailyInsights 把日桶转成近 30 天区块。
// 缺的天不补 0：预聚合保留期没覆盖到的日期就是「没有数据」，不是「那天没有用量」。
func buildLeaderboardDailyInsights(rows []usagestats.LeaderboardDailyBucketRow) []LeaderboardDailyInsight {
	if len(rows) == 0 {
		return nil
	}
	var maxTokens int64
	for _, row := range rows {
		if row.TotalTokens > maxTokens {
			maxTokens = row.TotalTokens
		}
	}
	insights := make([]LeaderboardDailyInsight, 0, len(rows))
	for _, row := range rows {
		insights = append(insights, LeaderboardDailyInsight{
			Date:            row.Date.Format(leaderboardInsightDateLayout),
			Requests:        row.Requests,
			TotalTokens:     row.TotalTokens,
			RelativePercent: leaderboardRelativePercent(row.TotalTokens, maxTokens),
		})
	}
	return insights
}

// buildLeaderboardHourlyInsights 把今日的小时桶补齐成 0–23 点。
//
// 整块缺行时返回 nil（预聚合未启用）；有行时缺的小时按 0 请求补齐——那是「今天这个点没有
// 请求」的真实值，与「整块没有数据」是两回事。桶起点本来就是站点时区，因此小时取
// bucket_start 在站点时区下的 Hour()，不做二次映射。
func buildLeaderboardHourlyInsights(rows []usagestats.LeaderboardHourlyBucketRow) []LeaderboardHourlyInsight {
	if len(rows) == 0 {
		return nil
	}
	requestsByHour := make([]int64, leaderboardHoursPerDay)
	var maxRequests int64
	for _, row := range rows {
		hour := row.BucketStart.In(timezone.Location()).Hour()
		if hour < 0 || hour >= leaderboardHoursPerDay {
			continue
		}
		requestsByHour[hour] += row.Requests
		if requestsByHour[hour] > maxRequests {
			maxRequests = requestsByHour[hour]
		}
	}
	insights := make([]LeaderboardHourlyInsight, 0, leaderboardHoursPerDay)
	for hour := 0; hour < leaderboardHoursPerDay; hour++ {
		insights = append(insights, LeaderboardHourlyInsight{
			Hour:            hour,
			Requests:        requestsByHour[hour],
			RelativePercent: leaderboardRelativePercent(requestsByHour[hour], maxRequests),
		})
	}
	return insights
}

// buildLeaderboardCacheInsight 由今日小时桶汇总出全站命中率，不另查一次。
// 全天 input + cache_read 为 0 时整块为 nil，而不是 0%。
func buildLeaderboardCacheInsight(rows []usagestats.LeaderboardHourlyBucketRow) *LeaderboardCacheInsight {
	if len(rows) == 0 {
		return nil
	}
	var inputTokens, cacheReadTokens int64
	for _, row := range rows {
		inputTokens += row.InputTokens
		cacheReadTokens += row.CacheReadTokens
	}
	rate, ok := leaderboardCacheHitRate(inputTokens, cacheReadTokens)
	if !ok {
		return nil
	}
	return &LeaderboardCacheInsight{
		CacheHitRate:    rate,
		CacheReadTokens: cacheReadTokens,
		InputTokens:     inputTokens,
	}
}

// buildLeaderboardMonthInsight 由本月与上月两段日桶算出「本月累计」与「较上月」。
// 上月只出一个百分比：它的绝对量不进结构体，因此也无从被下发。
// 本月没有任何日桶时整块为 nil；上月没有数据时不宣称增长（change_percent 为 0）。
func buildLeaderboardMonthInsight(current, previous []usagestats.LeaderboardDailyBucketRow) *LeaderboardMonthInsight {
	if len(current) == 0 {
		return nil
	}
	var currentTokens, previousTokens int64
	for _, row := range current {
		currentTokens += row.TotalTokens
	}
	for _, row := range previous {
		previousTokens += row.TotalTokens
	}
	change := 0
	if previousTokens > 0 {
		// 向零取整，与其余百分比一致；可为负。
		change = int((currentTokens - previousTokens) * 100 / previousTokens)
	}
	return &LeaderboardMonthInsight{TotalTokens: currentTokens, ChangePercent: change}
}

// leaderboardPeakHour 取今日请求最多的小时（RelativePercent 为 100 的桶）。
// 今日一个请求都没有时没有峰值时段，返回 nil。
func leaderboardPeakHour(hourly []LeaderboardHourlyInsight) *int {
	var peak *int
	var peakRequests int64
	for i := range hourly {
		if hourly[i].Requests > peakRequests {
			peakRequests = hourly[i].Requests
			hour := hourly[i].Hour
			peak = &hour
		}
	}
	return peak
}

// leaderboardDailyInsightRange 是近 30 天区块的查询区间 [from, to)：含今日在内共 30 天。
func leaderboardDailyInsightRange(todayStart time.Time) (from, to time.Time) {
	return todayStart.AddDate(0, 0, -(leaderboardDailyInsightDays - 1)), todayStart.AddDate(0, 0, 1)
}

// computeLeaderboardExtremes 由一个 Window 的全部快照条目算出六项之最（design D20）。
//
// 遍历只做一次：六项里有五项都来自同一批 Hash 数值，因此页面上的六张卡不会互相矛盾。
// streak 与 Window 无关（来自 usage_dashboard_daily_users 的一条查询），由调用方一次算好后
// 三个窗口共用；为 nil 或 Days 为 0 时该项缺席。
//
// window 参与判定的只有一处：rising 只在 today 窗口存在——聚合里只有「昨日」一组基线列，
// 没有同口径的「上周 / 上月」，week / month 下这一项必然是 nil。
//
// entries 为空时返回 nil：空窗口没有可言说的之最。窗口非空但六项都无人满足条件时
// 返回的是一个六项全 nil 的结构（序列化成 `"extremes": {}`），而不是 nil——
// 「这一轮算过了、没人达标」与「这一轮根本没算」在响应上是两回事，前端都按「不渲染卡」处理。
func computeLeaderboardExtremes(entries []LeaderboardUserMetrics, window LeaderboardWindow, streak *usagestats.LeaderboardStreakRow) *LeaderboardExtremes {
	if len(entries) == 0 {
		return nil
	}

	var (
		nightOwl       *LeaderboardUserMetrics
		risingUser     *LeaderboardUserMetrics
		risingPercent  int
		omnivore       *LeaderboardUserMetrics
		talker         *LeaderboardUserMetrics
		talkerShare    float64
		maxSingleUser  *LeaderboardUserMetrics
		maxSingleValue int64
	)
	// 单次最大的中位数要的是「全体参与者各自的单次最大值」，因此这里单独收集一份。
	maxSingles := make([]int64, 0, len(entries))

	for i := range entries {
		entry := entries[i]

		// 夜猫子：0–6 点 tokens 最多者。同值时固定取 user_id 较小者，避免同一份数据每轮换人。
		if entry.NightTokens > 0 &&
			(nightOwl == nil || entry.NightTokens > nightOwl.NightTokens ||
				(entry.NightTokens == nightOwl.NightTokens && entry.UserID < nightOwl.UserID)) {
			nightOwl = &entries[i]
		}

		// 进步之星：昨日 tokens 大于 0 且今日大于昨日才参评，否则「昨天零用量」会算出无穷增幅。
		if window == LeaderboardWindowToday && entry.YesterdayTokens > 0 && entry.TotalTokens > entry.YesterdayTokens {
			change := int((entry.TotalTokens - entry.YesterdayTokens) * 100 / entry.YesterdayTokens)
			if risingUser == nil || change > risingPercent ||
				(change == risingPercent && entry.UserID < risingUser.UserID) {
				risingUser, risingPercent = &entries[i], change
			}
		}

		// 杂食者：用过的不同模型数最多者。
		if entry.DistinctModels > 0 &&
			(omnivore == nil || entry.DistinctModels > omnivore.DistinctModels ||
				(entry.DistinctModels == omnivore.DistinctModels && entry.UserID < omnivore.UserID)) {
			omnivore = &entries[i]
		}

		// 话痨：output / total 最高者，成功请求不足门槛的不参评。
		if entry.SuccessfulRequests >= leaderboardTalkerMinRequests && entry.TotalTokens > 0 && entry.OutputTokens > 0 {
			share := float64(entry.OutputTokens) / float64(entry.TotalTokens)
			if talker == nil || share > talkerShare ||
				(share == talkerShare && entry.UserID < talker.UserID) {
				talker, talkerShare = &entries[i], share
			}
		}

		// 单次最大：单次请求 tokens 最大者，同时攒下中位数的样本。
		if entry.MaxSingleTokens > 0 {
			maxSingles = append(maxSingles, entry.MaxSingleTokens)
			if maxSingleUser == nil || entry.MaxSingleTokens > maxSingleValue ||
				(entry.MaxSingleTokens == maxSingleValue && entry.UserID < maxSingleUser.UserID) {
				maxSingleUser, maxSingleValue = &entries[i], entry.MaxSingleTokens
			}
		}
	}

	extremes := &LeaderboardExtremes{}
	if nightOwl != nil {
		extremes.NightOwl = &LeaderboardExtremeNightOwl{
			UserID:            nightOwl.UserID,
			NightTokens:       nightOwl.NightTokens,
			NightSharePercent: leaderboardRelativePercent(nightOwl.NightTokens, nightOwl.TotalTokens),
		}
	}
	if risingUser != nil {
		extremes.Rising = &LeaderboardExtremeRising{UserID: risingUser.UserID, ChangePercent: risingPercent}
	}
	if omnivore != nil {
		extremes.Omnivore = &LeaderboardExtremeOmnivore{UserID: omnivore.UserID, DistinctModels: omnivore.DistinctModels}
	}
	if talker != nil {
		extremes.Talker = &LeaderboardExtremeTalker{
			UserID:             talker.UserID,
			OutputSharePercent: leaderboardRelativePercent(talker.OutputTokens, talker.TotalTokens),
		}
	}
	if maxSingleUser != nil {
		// 中位数为 0 时整项缺席：anonymous 档只有 ratio_to_median 可用，
		// 分母没了这张卡就没有可下发的相对量，给 0 倍反而是个假事实。
		if median := leaderboardMedian(maxSingles); median > 0 {
			extremes.MaxSingle = &LeaderboardExtremeMaxSingle{
				UserID:          maxSingleUser.UserID,
				MaxSingleTokens: maxSingleValue,
				RatioToMedian:   leaderboardRoundOneDecimal(float64(maxSingleValue) / median),
			}
		}
	}
	if streak != nil && streak.Days > 0 {
		extremes.Streak = &LeaderboardExtremeStreak{UserID: streak.UserID, Days: streak.Days}
	}
	return extremes
}

// leaderboardMedian 返回一组正整数的中位数（偶数个时取中间两个的平均）。
// 会就地排序传入的切片——调用方给的是本函数专用的样本副本。
func leaderboardMedian(values []int64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	mid := len(values) / 2
	if len(values)%2 == 1 {
		return float64(values[mid])
	}
	return float64(values[mid-1]+values[mid]) / 2
}

// leaderboardRoundOneDecimal 保留一位小数：ratio_to_median 只需要「几倍」这个量级，
// 多给小数位反而能反推出更精确的绝对量。
func leaderboardRoundOneDecimal(value float64) float64 {
	return math.Round(value*10) / 10
}

// buildLeaderboardProfiles 由「Top N 名的 user_id」与一条 (user_id, model) 聚合的结果
// 拼出模型偏好画像：每人列出该窗口用过的全部模型及占比，按成功请求降序，不截断。
//
// rows 必须已按 (user_id, successful_requests DESC, model ASC) 排好序——仓储层的 ORDER BY
// 就是这个顺序，因此这里不再排一次。占比的分母是该用户该窗口的成功请求总数。
// 某人在该窗口没有任何成功请求时不出现在结果里，而不是给一行空模型。
func buildLeaderboardProfiles(userIDs []int64, rows []usagestats.LeaderboardUserModelUsageRow) []LeaderboardProfile {
	if len(userIDs) == 0 || len(rows) == 0 {
		return nil
	}
	totals := make(map[int64]int64, len(userIDs))
	byUser := make(map[int64][]usagestats.LeaderboardUserModelUsageRow, len(userIDs))
	for _, row := range rows {
		totals[row.UserID] += row.SuccessfulRequests
		byUser[row.UserID] = append(byUser[row.UserID], row)
	}

	profiles := make([]LeaderboardProfile, 0, len(userIDs))
	for _, userID := range userIDs {
		userRows, ok := byUser[userID]
		if !ok || totals[userID] <= 0 {
			continue
		}
		models := make([]LeaderboardProfileModel, 0, len(userRows))
		for _, row := range userRows {
			models = append(models, LeaderboardProfileModel{
				Model:        row.Model,
				SharePercent: leaderboardRelativePercent(row.SuccessfulRequests, totals[userID]),
			})
		}
		if len(models) == 0 {
			continue
		}
		profiles = append(profiles, LeaderboardProfile{UserID: userID, Models: models})
	}
	if len(profiles) == 0 {
		return nil
	}
	return profiles
}

// leaderboardTopUserIDsByTokens 取该 Window Total Tokens 前 limit 名的 user_id。
// 同值时按 user_id 升序，与 Highlights 的「同值取 user_id 较小者」是同一条稳定规则，
// 避免同一份数据每轮换一批人进画像。
func leaderboardTopUserIDsByTokens(entries []LeaderboardUserMetrics, limit int) []int64 {
	if len(entries) == 0 || limit <= 0 {
		return nil
	}
	ordered := make([]LeaderboardUserMetrics, 0, len(entries))
	for _, entry := range entries {
		if entry.TotalTokens > 0 {
			ordered = append(ordered, entry)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].TotalTokens != ordered[j].TotalTokens {
			return ordered[i].TotalTokens > ordered[j].TotalTokens
		}
		return ordered[i].UserID < ordered[j].UserID
	})
	if len(ordered) > limit {
		ordered = ordered[:limit]
	}
	userIDs := make([]int64, 0, len(ordered))
	for _, entry := range ordered {
		userIDs = append(userIDs, entry.UserID)
	}
	return userIDs
}

// buildLeaderboardPlatformInsights 把今日按平台的聚合行转成平台分布区块。
// total 是今日全站成功请求总数（不止返回的这几行），即 share_percent 的分母。
func buildLeaderboardPlatformInsights(rows []usagestats.LeaderboardPlatformUsageRow, total int64) []LeaderboardPlatformInsight {
	if len(rows) == 0 {
		return nil
	}
	insights := make([]LeaderboardPlatformInsight, 0, len(rows))
	for _, row := range rows {
		insights = append(insights, LeaderboardPlatformInsight{
			Platform:           row.Platform,
			SuccessfulRequests: row.SuccessfulRequests,
			SharePercent:       leaderboardRelativePercent(row.SuccessfulRequests, total),
		})
	}
	return insights
}

// buildLeaderboardWeeklyRhythm 把近 28 天按 (周几, 小时) 的平均请求数转成 7 × 24 的 0–4 等级。
//
// 整块缺行（预聚合未启用 / 保留期没覆盖）时返回 nil，由上层整块置 null；有行时，某个格子
// 没有对应行意思是「那个时段这 28 天没有请求」，等级为 0——这是真实的 0，不是缺数据
// （design D22）。行序周一起（ISO 周几 1–7 映射到下标 0–6）。
//
// 等级按该 28 天里的最大值等分五档：最大值那一格必然是 4，非零但很小的格子至少是 1，
// 因此「有一点用量」与「完全没有」在色阶上分得开。
func buildLeaderboardWeeklyRhythm(rows []usagestats.LeaderboardWeekdayHourRow) [][]int {
	if len(rows) == 0 {
		return nil
	}
	grid := make([][]int64, leaderboardWeekdaysPerWeek)
	for i := range grid {
		grid[i] = make([]int64, leaderboardHoursPerDay)
	}
	var maxRequests int64
	for _, row := range rows {
		weekday := row.Weekday - 1
		if weekday < 0 || weekday >= leaderboardWeekdaysPerWeek {
			continue
		}
		if row.Hour < 0 || row.Hour >= leaderboardHoursPerDay {
			continue
		}
		grid[weekday][row.Hour] = row.Requests
		if row.Requests > maxRequests {
			maxRequests = row.Requests
		}
	}

	levels := make([][]int, leaderboardWeekdaysPerWeek)
	for weekday := range levels {
		levels[weekday] = make([]int, leaderboardHoursPerDay)
		for hour := range levels[weekday] {
			levels[weekday][hour] = leaderboardRhythmLevel(grid[weekday][hour], maxRequests)
		}
	}
	return levels
}

// leaderboardRhythmLevel 把一格的平均请求数换算成 0–4 的等级。
// 0 永远是 0 级；其余按相对最大值的比例向上取档，保证最大值那一格是 4、
// 而任何非零的格子至少是 1。
func leaderboardRhythmLevel(value, max int64) int {
	if value <= 0 || max <= 0 {
		return 0
	}
	level := int((value*int64(leaderboardRhythmLevels-1) + max - 1) / max)
	if level < 1 {
		level = 1
	}
	if level > leaderboardRhythmLevels-1 {
		level = leaderboardRhythmLevels - 1
	}
	return level
}

// buildLeaderboardCompositionInsight 由今日窗口聚合的站点合计算出 Token 构成。
// 四段全为 0（今日没有任何用量）时整块为 nil，而不是四个 0%。
// 百分比向下取整，因此四段之和可能是 99——页面按段宽渲染，不做凑整。
func buildLeaderboardCompositionInsight(input, output, cacheCreation, cacheRead int64) *LeaderboardCompositionInsight {
	total := input + output + cacheCreation + cacheRead
	if total <= 0 {
		return nil
	}
	return &LeaderboardCompositionInsight{
		InputTokens:          input,
		OutputTokens:         output,
		CacheCreationTokens:  cacheCreation,
		CacheReadTokens:      cacheRead,
		InputPercent:         leaderboardRelativePercent(input, total),
		OutputPercent:        leaderboardRelativePercent(output, total),
		CacheCreationPercent: leaderboardRelativePercent(cacheCreation, total),
		CacheReadPercent:     leaderboardRelativePercent(cacheRead, total),
	}
}

// buildLeaderboardCacheTrendInsights 从近 30 天的日桶里切出近 leaderboardCacheTrendDays 天的
// 每日缓存命中率。近 14 天是近 30 天的子集，因此复用同一批行，MUST NOT 另查一次。
//
// 某天 input + cache_read 为 0 时那一天没有命中率，直接跳过而不是记成 0%；
// 整段一天都没有时返回 nil，由上层整块置 null。
func buildLeaderboardCacheTrendInsights(rows []usagestats.LeaderboardDailyBucketRow, from time.Time) []LeaderboardCacheTrendInsight {
	if len(rows) == 0 {
		return nil
	}
	// 下界按日期串比较而不是 time.Before：日桶的 DATE 值被驱动读成 UTC 零点，
	// 而 from 是站点时区的窗口起点，直接比瞬间会在非 UTC 站点上多算 / 少算一天。
	fromDate := from.Format(leaderboardInsightDateLayout)
	insights := make([]LeaderboardCacheTrendInsight, 0, leaderboardCacheTrendDays)
	for _, row := range rows {
		date := row.Date.Format(leaderboardInsightDateLayout)
		if date < fromDate {
			continue
		}
		rate, ok := leaderboardCacheHitRate(row.InputTokens, row.CacheReadTokens)
		if !ok {
			continue
		}
		insights = append(insights, LeaderboardCacheTrendInsight{Date: date, CacheHitRate: rate})
	}
	if len(insights) == 0 {
		return nil
	}
	return insights
}

// leaderboardCacheTrendRange 是缓存命中率趋势的下界（含今日共 leaderboardCacheTrendDays 天）。
func leaderboardCacheTrendRange(todayStart time.Time) time.Time {
	return todayStart.AddDate(0, 0, -(leaderboardCacheTrendDays - 1))
}

// leaderboardRhythmRange 是周内节奏的查询区间 [from, to)：近 leaderboardRhythmLookbackDays 天，
// 不含今日——今天还没过完，把半天的小时桶混进平均值会让当日那几格系统性偏低。
func leaderboardRhythmRange(todayStart time.Time) (from, to time.Time) {
	return todayStart.AddDate(0, 0, -leaderboardRhythmLookbackDays), todayStart
}

// leaderboardStreakRange 是连续活跃的查询区间 [from, to)：回溯 leaderboardStreakLookbackDays 天，
// 上界取今日的次日零点，因此「截至今日或昨日」在 SQL 里就是末日不早于 to::date - 2。
func leaderboardStreakRange(todayStart time.Time) (from, to time.Time) {
	return todayStart.AddDate(0, 0, -(leaderboardStreakLookbackDays - 1)), todayStart.AddDate(0, 0, 1)
}

// leaderboardRankHistoryRange 是名次走势的查询区间 [from, to)：含今日共 leaderboardRankHistoryDays 天。
// 保留期（leaderboardRankHistoryRetentionDays）远大于它，因此这段永远取得到完整的行。
func leaderboardRankHistoryRange(todayStart time.Time) (from, to time.Time) {
	return todayStart.AddDate(0, 0, -(leaderboardRankHistoryDays - 1)), todayStart.AddDate(0, 0, 1)
}
