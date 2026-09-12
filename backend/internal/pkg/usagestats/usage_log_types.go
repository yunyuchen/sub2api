// Package usagestats provides types for usage statistics and reporting.
package usagestats

import "time"

const (
	ModelSourceRequested = "requested"
	ModelSourceUpstream  = "upstream"
	ModelSourceMapping   = "mapping"
)

func IsValidModelSource(source string) bool {
	switch source {
	case ModelSourceRequested, ModelSourceUpstream, ModelSourceMapping:
		return true
	default:
		return false
	}
}

func NormalizeModelSource(source string) string {
	if IsValidModelSource(source) {
		return source
	}
	return ModelSourceRequested
}

// DashboardStats 仪表盘统计
type DashboardStats struct {
	// 用户统计
	TotalUsers    int64 `json:"total_users"`
	TodayNewUsers int64 `json:"today_new_users"` // 今日新增用户数
	ActiveUsers   int64 `json:"active_users"`    // 今日有请求的用户数
	// 小时活跃用户数（UTC 当前小时）
	HourlyActiveUsers int64 `json:"hourly_active_users"`

	// 预聚合新鲜度
	StatsUpdatedAt string `json:"stats_updated_at"`
	StatsStale     bool   `json:"stats_stale"`

	// API Key 统计
	TotalAPIKeys  int64 `json:"total_api_keys"`
	ActiveAPIKeys int64 `json:"active_api_keys"` // 状态为 active 的 API Key 数

	// 账户统计
	TotalAccounts     int64 `json:"total_accounts"`
	NormalAccounts    int64 `json:"normal_accounts"`    // 正常账户数 (schedulable=true, status=active)
	ErrorAccounts     int64 `json:"error_accounts"`     // 异常账户数 (status=error)
	RateLimitAccounts int64 `json:"ratelimit_accounts"` // 限流账户数
	OverloadAccounts  int64 `json:"overload_accounts"`  // 过载账户数

	// 累计 Token 使用统计
	TotalRequests            int64   `json:"total_requests"`
	TotalInputTokens         int64   `json:"total_input_tokens"`
	TotalOutputTokens        int64   `json:"total_output_tokens"`
	TotalCacheCreationTokens int64   `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64   `json:"total_cache_read_tokens"`
	TotalTokens              int64   `json:"total_tokens"`
	TotalCost                float64 `json:"total_cost"`         // 累计标准计费
	TotalActualCost          float64 `json:"total_actual_cost"`  // 累计实际扣除
	TotalAccountCost         float64 `json:"total_account_cost"` // 累计账号成本

	// 今日 Token 使用统计
	TodayRequests            int64   `json:"today_requests"`
	TodayInputTokens         int64   `json:"today_input_tokens"`
	TodayOutputTokens        int64   `json:"today_output_tokens"`
	TodayCacheCreationTokens int64   `json:"today_cache_creation_tokens"`
	TodayCacheReadTokens     int64   `json:"today_cache_read_tokens"`
	TodayTokens              int64   `json:"today_tokens"`
	TodayCost                float64 `json:"today_cost"`         // 今日标准计费
	TodayActualCost          float64 `json:"today_actual_cost"`  // 今日实际扣除
	TodayAccountCost         float64 `json:"today_account_cost"` // 今日账号成本

	// 系统运行统计
	AverageDurationMs float64 `json:"average_duration_ms"` // 平均响应时间

	// 性能指标
	Rpm int64 `json:"rpm"` // 近5分钟平均每分钟请求数
	Tpm int64 `json:"tpm"` // 近5分钟平均每分钟Token数
}

// TrendDataPoint represents a single point in trend data
type TrendDataPoint struct {
	Date                string  `json:"date"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	Cost                float64 `json:"cost"`        // 标准计费
	ActualCost          float64 `json:"actual_cost"` // 实际扣除
}

// ModelStat represents usage statistics for a single model
type ModelStat struct {
	Model               string  `json:"model"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	Cost                float64 `json:"cost"`         // 标准计费
	ActualCost          float64 `json:"actual_cost"`  // 实际扣除
	AccountCost         float64 `json:"account_cost"` // 账号成本
}

// EndpointStat represents usage statistics for a single request endpoint.
type EndpointStat struct {
	Endpoint    string  `json:"endpoint"`
	Requests    int64   `json:"requests"`
	TotalTokens int64   `json:"total_tokens"`
	Cost        float64 `json:"cost"`        // 标准计费
	ActualCost  float64 `json:"actual_cost"` // 实际扣除
}

// GroupUsageSummary represents today's, yesterday's, and cumulative cost for a single group.
type GroupUsageSummary struct {
	GroupID       int64   `json:"group_id"`
	TodayCost     float64 `json:"today_cost"`
	YesterdayCost float64 `json:"yesterday_cost"`
	TotalCost     float64 `json:"total_cost"`
}

// GroupStat represents usage statistics for a single group
type GroupStat struct {
	GroupID     int64   `json:"group_id"`
	GroupName   string  `json:"group_name"`
	Requests    int64   `json:"requests"`
	TotalTokens int64   `json:"total_tokens"`
	Cost        float64 `json:"cost"`         // 标准计费
	ActualCost  float64 `json:"actual_cost"`  // 实际扣除
	AccountCost float64 `json:"account_cost"` // 账号成本
}

// UserUsageTrendPoint represents user usage trend data point
type UserUsageTrendPoint struct {
	Date       string  `json:"date"`
	UserID     int64   `json:"user_id"`
	Email      string  `json:"email"`
	Username   string  `json:"username"`
	Requests   int64   `json:"requests"`
	Tokens     int64   `json:"tokens"`
	Cost       float64 `json:"cost"`        // 标准计费
	ActualCost float64 `json:"actual_cost"` // 实际扣除
}

// UserSpendingRankingItem represents a user spending ranking row.
type UserSpendingRankingItem struct {
	UserID     int64   `json:"user_id"`
	Email      string  `json:"email"`
	Username   string  `json:"username"`
	ActualCost float64 `json:"actual_cost"` // 实际扣除
	Requests   int64   `json:"requests"`
	Tokens     int64   `json:"tokens"`
}

// UserSpendingRankingResponse represents ranking rows plus total spend for the time range.
type UserSpendingRankingResponse struct {
	Ranking         []UserSpendingRankingItem `json:"ranking"`
	TotalActualCost float64                   `json:"total_actual_cost"`
	TotalRequests   int64                     `json:"total_requests"`
	TotalTokens     int64                     `json:"total_tokens"`
}

// UserBreakdownItem represents per-user usage breakdown within a dimension (group, model, endpoint).
type UserBreakdownItem struct {
	UserID       int64   `json:"user_id"`
	Email        string  `json:"email"`
	Username     string  `json:"username"`
	Requests     int64   `json:"requests"`
	InputTokens  int64   `json:"input_tokens"`  // 输入 token 累计
	OutputTokens int64   `json:"output_tokens"` // 输出 token 累计
	CacheTokens  int64   `json:"cache_tokens"`  // 缓存创建 + 读取 token 累计
	TotalTokens  int64   `json:"total_tokens"`  // 输入+输出+缓存 token 累计
	Cost         float64 `json:"cost"`          // 标准计费
	ActualCost   float64 `json:"actual_cost"`   // 实际扣除
	AccountCost  float64 `json:"account_cost"`  // 账号成本
}

// LeaderboardAggregateRow 是 Leaderboard（排行榜）Snapshot（榜单快照）重建时
// 一条 SQL 同时产出的三个 Window（榜单窗口）聚合行：一次扫描 usage_logs，
// 每个窗口各给出 Total Tokens（总 tokens）与 Successful Requests（成功请求数）两个数。
//
// 与 UserBreakdownItem 的区别：这里不含身份（email / username）与任何金额，
// 因为 Snapshot 只记录 user_id 与数值，身份在响应组装时按 users 当前状态渲染。
// Requests 一律是「成功落账」口径（actual_cost > 0），不是裸 COUNT(*)。
//
// InputTokens 与 CacheReadTokens 两列只用来算 Cache Hit Rate（缓存命中率）
// = cache_read /(input + cache_read)，MUST NOT 参与排名：ZSET 仍然只有两个 Metric。
//
// v2 起每个窗口另出六个数、外加两个与窗口无关的「昨日」数，合计十四个：它们只喂
// Extremes（之最）与 Token 构成，同样 MUST NOT 参与排名。口径分两类，刻意不统一：
//   - token 求和（Output / CacheCreation / Night / Yesterday）沿用既有 token 列的口径，
//     即窗口内所有行求和，这样它们与 InputTokens / CacheReadTokens / TotalTokens 以及
//     管理端 User Breakdown（用户用量明细）的同名列对得上，占比算出来分子分母同源；
//   - 按请求取值的列（DistinctModels / MaxSingleTokens / MediaRequests / YesterdayRequests）
//     带成功落账过滤（actual_cost > 0），否则「刷失败请求」就能改写杂食者与单次最大。
type LeaderboardAggregateRow struct {
	UserID               int64 `json:"user_id"`
	TodayTokens          int64 `json:"today_tokens"`            // 今日窗口 Total Tokens
	TodayRequests        int64 `json:"today_requests"`          // 今日窗口 Successful Requests
	TodayInputTokens     int64 `json:"today_input_tokens"`      // 今日窗口输入 tokens（只用于命中率）
	TodayCacheReadTokens int64 `json:"today_cache_read_tokens"` // 今日窗口缓存读取 tokens（只用于命中率）
	WeekTokens           int64 `json:"week_tokens"`             // 本周窗口 Total Tokens
	WeekRequests         int64 `json:"week_requests"`           // 本周窗口 Successful Requests
	WeekInputTokens      int64 `json:"week_input_tokens"`       // 本周窗口输入 tokens（只用于命中率）
	WeekCacheReadTokens  int64 `json:"week_cache_read_tokens"`  // 本周窗口缓存读取 tokens（只用于命中率）
	MonthTokens          int64 `json:"month_tokens"`            // 本月窗口 Total Tokens
	MonthRequests        int64 `json:"month_requests"`          // 本月窗口 Successful Requests
	MonthInputTokens     int64 `json:"month_input_tokens"`      // 本月窗口输入 tokens（只用于命中率）
	MonthCacheReadTokens int64 `json:"month_cache_read_tokens"` // 本月窗口缓存读取 tokens（只用于命中率）

	TodayOutputTokens        int64 `json:"today_output_tokens"`         // 今日窗口输出 tokens（话痨 / Token 构成）
	TodayCacheCreationTokens int64 `json:"today_cache_creation_tokens"` // 今日窗口缓存创建 tokens（Token 构成）
	TodayNightTokens         int64 `json:"today_night_tokens"`          // 今日窗口 0–6 点（站点时区）的 Total Tokens
	TodayDistinctModels      int   `json:"today_distinct_models"`       // 今日窗口用过的不同模型数（杂食者）
	TodayMaxSingleTokens     int64 `json:"today_max_single_tokens"`     // 今日窗口单次请求 tokens 的最大值
	TodayMediaRequests       int64 `json:"today_media_requests"`        // 今日窗口有图片 / 视频计数的成功请求数（只存不发）

	WeekOutputTokens        int64 `json:"week_output_tokens"`
	WeekCacheCreationTokens int64 `json:"week_cache_creation_tokens"`
	WeekNightTokens         int64 `json:"week_night_tokens"`
	WeekDistinctModels      int   `json:"week_distinct_models"`
	WeekMaxSingleTokens     int64 `json:"week_max_single_tokens"`
	WeekMediaRequests       int64 `json:"week_media_requests"`

	MonthOutputTokens        int64 `json:"month_output_tokens"`
	MonthCacheCreationTokens int64 `json:"month_cache_creation_tokens"`
	MonthNightTokens         int64 `json:"month_night_tokens"`
	MonthDistinctModels      int   `json:"month_distinct_models"`
	MonthMaxSingleTokens     int64 `json:"month_max_single_tokens"`
	MonthMediaRequests       int64 `json:"month_media_requests"`

	// 「昨日」两列与 Window 无关，只给今日窗口的进步之星当基线；
	// 它们存在的代价是把扫描下界从 min(月初, 周一) 放宽到 min(月初, 周一, 昨日起点)。
	YesterdayTokens   int64 `json:"yesterday_tokens"`
	YesterdayRequests int64 `json:"yesterday_requests"`
}

// LeaderboardModelUsageRow 是「今日按模型聚合」的一行：Insights（洞察）的模型热度用。
// SuccessfulRequests 与 LeaderboardAggregateRow 同为成功落账口径（actual_cost > 0），
// 因此字段名叫 successful_requests，与两张预聚合表的裸 COUNT(*)（requests）刻意不同名。
type LeaderboardModelUsageRow struct {
	Model              string `json:"model"`
	SuccessfulRequests int64  `json:"successful_requests"`
}

// LeaderboardDailyBucketRow 是 usage_dashboard_daily 的一个日桶（桶边界已是站点时区）。
// Requests 是该表的 total_requests，即**全部请求**的裸 COUNT(*)——含失败请求的占位记录，
// 与榜单的 Successful Requests 不是同一个口径，因此字段名叫 Requests。
//
// 两个 token 列用于算「每天的缓存命中率」（cache_trend_14），与 TotalTokens 同一次读取：
// 近 14 天是近 30 天的子集，因此趋势那一块从同一批日桶里切出来，MUST NOT 另查一次。
type LeaderboardDailyBucketRow struct {
	Date            time.Time `json:"date"`
	Requests        int64     `json:"requests"`
	TotalTokens     int64     `json:"total_tokens"`
	InputTokens     int64     `json:"input_tokens"`
	CacheReadTokens int64     `json:"cache_read_tokens"`
}

// LeaderboardHourlyBucketRow 是 usage_dashboard_hourly 的一个小时桶（桶边界已是站点时区）。
// Requests 的口径同 LeaderboardDailyBucketRow；两个 token 列用于算今日全站命中率。
type LeaderboardHourlyBucketRow struct {
	BucketStart     time.Time `json:"bucket_start"`
	Requests        int64     `json:"requests"`
	InputTokens     int64     `json:"input_tokens"`
	CacheReadTokens int64     `json:"cache_read_tokens"`
}

// LeaderboardStreakRow 是「连续活跃」之最的结果：截至今日或昨日、连续有用量天数最长的那个人。
// 来源是 usage_dashboard_daily_users（每天每人一行），不回去扫 usage_logs。
// 无人满足时返回零值，Days 为 0 即「没有这张卡」。
type LeaderboardStreakRow struct {
	UserID int64 `json:"user_id"`
	Days   int   `json:"days"`
}

// LeaderboardRankHistoryRow 是 leaderboard_rank_history 的一行：某人某天在今日窗口的两个名次。
// 只有 user_id、日期与名次，没有身份、没有数值、没有任何金额。
type LeaderboardRankHistoryRow struct {
	UserID                 int64     `json:"user_id"`
	SnapshotDate           time.Time `json:"snapshot_date"`
	RankTotalTokens        int       `json:"rank_total_tokens"`
	RankSuccessfulRequests int       `json:"rank_successful_requests"`
}

// LeaderboardPlatformUsageRow 是「今日按平台聚合」的一行，供 Insights（洞察）的平台分布使用。
// SuccessfulRequests 与榜单同为成功落账口径（actual_cost > 0）。
type LeaderboardPlatformUsageRow struct {
	Platform           string `json:"platform"`
	SuccessfulRequests int64  `json:"successful_requests"`
}

// LeaderboardUserModelUsageRow 是「指定 user_id 集合在窗口内按 (user_id, model) 聚合」的一行，
// 供模型偏好画像使用：一条 SQL 出前 8 名各自的全部模型，MUST NOT 逐人各查一次。
type LeaderboardUserModelUsageRow struct {
	UserID             int64  `json:"user_id"`
	Model              string `json:"model"`
	SuccessfulRequests int64  `json:"successful_requests"`
}

// LeaderboardWeekdayHourRow 是「近 28 天按 (周几, 小时) 求平均」的一格，供周内节奏热力图使用。
// Weekday 用 ISO 周几（1 = 周一 … 7 = 周日），Requests 是该格的平均请求数（向下取整）；
// 该口径来自 usage_dashboard_hourly 的裸计数，含失败请求的占位记录。
type LeaderboardWeekdayHourRow struct {
	Weekday  int   `json:"weekday"`
	Hour     int   `json:"hour"`
	Requests int64 `json:"requests"`
}

// UserBreakdownDimension specifies the dimension to filter for user breakdown.
type UserBreakdownDimension struct {
	GroupID      int64  // filter by group_id (>0 to enable)
	Model        string // filter by model name (non-empty to enable)
	ModelType    string // "requested", "upstream", or "mapping"
	Endpoint     string // filter by endpoint value (non-empty to enable)
	EndpointType string // "inbound", "upstream", or "path"
	// Additional filter conditions
	UserID             int64  // filter by user_id (>0 to enable)
	APIKeyID           int64  // filter by api_key_id (>0 to enable)
	AccountID          int64  // filter by account_id (>0 to enable)
	RequestType        *int16 // filter by request_type (non-nil to enable)
	Stream             *bool  // filter by stream flag (non-nil to enable)
	NativeCompactionV2 *bool  // filter by native compaction v2 flag (non-nil to enable)
	BillingType        *int8  // filter by billing_type (non-nil to enable)
	// SortBy 指定排序列(空 = 默认按 actual_cost)。合法值由 repo 层 allowlist 校验。
	SortBy string
}

// APIKeyUsageTrendPoint represents API key usage trend data point
type APIKeyUsageTrendPoint struct {
	Date     string `json:"date"`
	APIKeyID int64  `json:"api_key_id"`
	KeyName  string `json:"key_name"`
	Requests int64  `json:"requests"`
	Tokens   int64  `json:"tokens"`
}

// APIKeyDailyUsagePoint represents one day of usage for a single API key.
type APIKeyDailyUsagePoint struct {
	Date             string  `json:"date"`
	Requests         int64   `json:"requests"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	Cost             float64 `json:"cost"`        // 标准计费
	ActualCost       float64 `json:"actual_cost"` // 实际扣除
}

// UserDashboardStats 用户仪表盘统计
type UserDashboardStats struct {
	// API Key 统计
	TotalAPIKeys  int64 `json:"total_api_keys"`
	ActiveAPIKeys int64 `json:"active_api_keys"`

	// 累计 Token 使用统计
	TotalRequests            int64   `json:"total_requests"`
	TotalInputTokens         int64   `json:"total_input_tokens"`
	TotalOutputTokens        int64   `json:"total_output_tokens"`
	TotalCacheCreationTokens int64   `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64   `json:"total_cache_read_tokens"`
	TotalTokens              int64   `json:"total_tokens"`
	TotalCost                float64 `json:"total_cost"`        // 累计标准计费
	TotalActualCost          float64 `json:"total_actual_cost"` // 累计实际扣除

	// 今日 Token 使用统计
	TodayRequests            int64   `json:"today_requests"`
	TodayInputTokens         int64   `json:"today_input_tokens"`
	TodayOutputTokens        int64   `json:"today_output_tokens"`
	TodayCacheCreationTokens int64   `json:"today_cache_creation_tokens"`
	TodayCacheReadTokens     int64   `json:"today_cache_read_tokens"`
	TodayTokens              int64   `json:"today_tokens"`
	TodayCost                float64 `json:"today_cost"`        // 今日标准计费
	TodayActualCost          float64 `json:"today_actual_cost"` // 今日实际扣除

	// 性能统计
	AverageDurationMs float64 `json:"average_duration_ms"`

	// 性能指标
	Rpm int64 `json:"rpm"` // 近5分钟平均每分钟请求数
	Tpm int64 `json:"tpm"` // 近5分钟平均每分钟Token数

	// 按"有效平台"维度拆分（与 ops 路径口径一致：group.platform 优先，否则 account.platform）
	ByPlatform []PlatformDashboardStats `json:"by_platform,omitempty"`
}

// PlatformDashboardStats 单个平台的用量明细。
type PlatformDashboardStats struct {
	Platform        string  `json:"platform"`
	TotalRequests   int64   `json:"total_requests"`
	TotalTokens     int64   `json:"total_tokens"`
	TotalActualCost float64 `json:"total_actual_cost"`
	TodayRequests   int64   `json:"today_requests"`
	TodayTokens     int64   `json:"today_tokens"`
	TodayActualCost float64 `json:"today_actual_cost"`
}

// UsageLogFilters represents filters for usage log queries
type UsageLogFilters struct {
	UserID    int64
	APIKeyID  int64
	AccountID int64
	GroupID   int64
	RequestID string
	Model     string
	// ModelFilterSource controls how Model is matched. Empty preserves raw usage_logs.model semantics.
	ModelFilterSource     string
	RequestType           *int16
	Stream                *bool
	NativeCompactionV2    *bool
	BillingType           *int8
	BillingMode           string
	UpstreamModelMismatch *bool
	StartTime             *time.Time
	EndTime               *time.Time
	// ExactTotal requests exact COUNT(*) for pagination. Default false for fast large-table paging.
	ExactTotal bool
}

// UsageStats represents usage statistics
type UsageStats struct {
	TotalRequests            int64          `json:"total_requests"`
	TotalInputTokens         int64          `json:"total_input_tokens"`
	TotalOutputTokens        int64          `json:"total_output_tokens"`
	TotalCacheTokens         int64          `json:"total_cache_tokens"`
	TotalCacheCreationTokens int64          `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64          `json:"total_cache_read_tokens"`
	TotalTokens              int64          `json:"total_tokens"`
	TotalCost                float64        `json:"total_cost"`
	TotalActualCost          float64        `json:"total_actual_cost"`
	TotalAccountCost         *float64       `json:"total_account_cost,omitempty"`
	AverageDurationMs        float64        `json:"average_duration_ms"`
	Endpoints                []EndpointStat `json:"endpoints,omitempty"`
	UpstreamEndpoints        []EndpointStat `json:"upstream_endpoints,omitempty"`
	EndpointPaths            []EndpointStat `json:"endpoint_paths,omitempty"`
}

// PlatformUsage 表示某用户/某 API key 在单个"有效平台"维度的用量明细。
// Platform 取值与 ops 路径口径一致：优先 groups.platform，否则 accounts.platform。
type PlatformUsage struct {
	Platform        string  `json:"platform"`
	TodayActualCost float64 `json:"today_actual_cost"`
	TotalActualCost float64 `json:"total_actual_cost"`
}

// BatchUserUsageStats represents usage stats for a single user
type BatchUserUsageStats struct {
	UserID          int64           `json:"user_id"`
	TodayActualCost float64         `json:"today_actual_cost"`
	TotalActualCost float64         `json:"total_actual_cost"`
	ByPlatform      []PlatformUsage `json:"by_platform,omitempty"`
}

// BatchAPIKeyUsageStats represents usage stats for a single API key
type BatchAPIKeyUsageStats struct {
	APIKeyID        int64   `json:"api_key_id"`
	TodayActualCost float64 `json:"today_actual_cost"`
	TotalActualCost float64 `json:"total_actual_cost"`
}

// AccountUsageHistory represents daily usage history for an account
type AccountUsageHistory struct {
	Date       string  `json:"date"`
	Label      string  `json:"label"`
	Requests   int64   `json:"requests"`
	Tokens     int64   `json:"tokens"`
	Cost       float64 `json:"cost"`        // 标准计费（total_cost）
	ActualCost float64 `json:"actual_cost"` // 账号口径费用（total_cost * account_rate_multiplier）
	UserCost   float64 `json:"user_cost"`   // 用户口径费用（actual_cost，受分组倍率影响）
}

// AccountUsageSummary represents summary statistics for an account
type AccountUsageSummary struct {
	Days              int     `json:"days"`
	ActualDaysUsed    int     `json:"actual_days_used"`
	TotalCost         float64 `json:"total_cost"`      // 账号口径费用
	TotalUserCost     float64 `json:"total_user_cost"` // 用户口径费用
	TotalStandardCost float64 `json:"total_standard_cost"`
	TotalRequests     int64   `json:"total_requests"`
	TotalTokens       int64   `json:"total_tokens"`
	AvgDailyCost      float64 `json:"avg_daily_cost"` // 账号口径日均
	AvgDailyUserCost  float64 `json:"avg_daily_user_cost"`
	AvgDailyRequests  float64 `json:"avg_daily_requests"`
	AvgDailyTokens    float64 `json:"avg_daily_tokens"`
	AvgDurationMs     float64 `json:"avg_duration_ms"`
	Today             *struct {
		Date     string  `json:"date"`
		Cost     float64 `json:"cost"`
		UserCost float64 `json:"user_cost"`
		Requests int64   `json:"requests"`
		Tokens   int64   `json:"tokens"`
	} `json:"today"`
	HighestCostDay *struct {
		Date     string  `json:"date"`
		Label    string  `json:"label"`
		Cost     float64 `json:"cost"`
		UserCost float64 `json:"user_cost"`
		Requests int64   `json:"requests"`
	} `json:"highest_cost_day"`
	HighestRequestDay *struct {
		Date     string  `json:"date"`
		Label    string  `json:"label"`
		Requests int64   `json:"requests"`
		Cost     float64 `json:"cost"`
		UserCost float64 `json:"user_cost"`
	} `json:"highest_request_day"`
}

// AccountUsageStatsResponse represents the full usage statistics response for an account
type AccountUsageStatsResponse struct {
	History           []AccountUsageHistory `json:"history"`
	Summary           AccountUsageSummary   `json:"summary"`
	Models            []ModelStat           `json:"models"`
	Endpoints         []EndpointStat        `json:"endpoints"`
	UpstreamEndpoints []EndpointStat        `json:"upstream_endpoints"`
}
