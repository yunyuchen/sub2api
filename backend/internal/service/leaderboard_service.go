package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// Leaderboard（排行榜）的错误。
//
// ErrLeaderboardNotFound 是 off 档下普通用户的唯一回应：404 而不是 403——
// 403 会确认功能存在（design D9）。写法照 channel_monitor_const.go 的 infraerrors 常量。
//
// reason 与 message 都刻意不提 Leaderboard：响应 MUST NOT 透露该路由存在，
// 而带 reason 的错误信封本身就与未注册路由的纯文本 404 可区分。因此这个 sentinel
// 只做进程内信号，线上响应由 handler.AbortLeaderboardNotFound 写成与 gin 未注册
// 路由逐字节一致的 404，不经 response.ErrorFrom 序列化。
var (
	ErrLeaderboardNotFound = infraerrors.NotFound(
		"NOT_FOUND",
		"404 page not found",
	)
	ErrLeaderboardInvalidWindow = infraerrors.BadRequest(
		"LEADERBOARD_INVALID_WINDOW",
		"window must be one of today, week, month",
	)
	ErrLeaderboardInvalidMetric = infraerrors.BadRequest(
		"LEADERBOARD_INVALID_METRIC",
		"metric must be one of total_tokens, successful_requests",
	)
)

// Leaderboard Entry（榜单条目）的身份形态。后端只下发结构化的 kind + username，
// 展示名的文案由前端 i18n 渲染（design D10）：self → 本人的 username（缺席时
// 「当前用户」）、anonymous →「第 Ordinal 位」、named → username。
// 响应里永远不会出现字面量 "Me"。
const (
	LeaderboardIdentitySelf      = "self"
	LeaderboardIdentityAnonymous = "anonymous"
	LeaderboardIdentityNamed     = "named"
)

// Snapshot（榜单快照）的两种状态：ready 表示快照已就绪（哪怕窗口内一个人都没有），
// computing 表示快照缺失（key 不存在或 Redis 不可用），页面显示「正在计算」。
const (
	LeaderboardStatusReady     = "ready"
	LeaderboardStatusComputing = "computing"
)

// 「你的位置」提示的三种形态（见 LeaderboardMyRankHint）。
const (
	LeaderboardHintTokensToTop10   = "tokens_to_top10"
	LeaderboardHintRequestsToTop10 = "requests_to_top10"
	LeaderboardHintRelativePercent = "relative_percent"
)

// leaderboardHintTopN 是提示语里那个目标名次：进了前 10 就没有可说的差距，hint 缺席。
const leaderboardHintTopN = 10

// leaderboardAnonymousMinParticipants 是 anonymous 档展示榜单条目的人数门槛：
// 人再少一点，「假名 + 相对百分比」也接近可辨认，因此只显示本人行（design D1）。
// 该门槛 MUST NOT 在 named 档套用。
const leaderboardAnonymousMinParticipants = 5

// leaderboardParticipantBuckets 是 anonymous 档 Participant Count（参与人数）的下界序列：
// 取不超过实际人数的最大值表达为「N+」（137 → 「100+」），少于 5 时为「<5」（design D1）。
var leaderboardParticipantBuckets = []int64{5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000}

// LeaderboardUserRepository 是 Leaderboard 在请求路径上唯一允许的数据库访问：
// 对 Top 50 加查看者共至多 51 个 id 做一次按 id 的批量查询，用于渲染展示名与参与资格。
type LeaderboardUserRepository interface {
	GetByIDs(ctx context.Context, ids []int64) ([]User, error)
}

// LeaderboardViewerRepository 只服务顶层 viewer（「你的统计」）这一个区块，两条都只查
// **查看者自己**（design D21）：
//   - LeaderboardRankHistory 读 leaderboard_rank_history，是后台作业写下的旁路留痕，
//     与 Snapshot（榜单快照）无关，因此 status 为 computing 时也照常取得到；
//   - LeaderboardViewerModels 是 D8「请求路径只读」的唯一例外：一条带 user_id 过滤的
//     usage_logs 聚合，只扫这一个人的行，结果在 Redis 上缓存 60 秒。
//
// 这个窄接口刻意只有这两个方法：例外的边界写在类型上，扩用一个字段就得先改这里。
type LeaderboardViewerRepository interface {
	LeaderboardRankHistory(ctx context.Context, userID int64, fromDate, toDate time.Time) ([]usagestats.LeaderboardRankHistoryRow, error)
	LeaderboardViewerModels(ctx context.Context, userID int64, start, end time.Time, limit int) ([]usagestats.LeaderboardModelUsageRow, int64, error)
}

// LeaderboardIdentity 是条目的结构化身份。后端 MUST NOT 拼接展示名字符串。
type LeaderboardIdentity struct {
	Kind     string `json:"kind"`
	Username string `json:"username,omitempty"`
}

// LeaderboardEntry 是榜单的一行。
//
// 绝对数值与相对百分比用指针 + omitempty 表达「字段是否存在」：anonymous 档下他人条目的
// total_tokens / successful_requests MUST 缺席而不是清零，客户端因此没有机会把缺席误读成 0。
type LeaderboardEntry struct {
	Rank     int64               `json:"rank"`
	Ordinal  int                 `json:"ordinal"`
	Identity LeaderboardIdentity `json:"identity"`
	IsSelf   bool                `json:"is_self"`

	TotalTokens        *int64 `json:"total_tokens,omitempty"`
	SuccessfulRequests *int64 `json:"successful_requests,omitempty"`

	TotalTokensRelativePercent        *int `json:"total_tokens_relative_percent,omitempty"`
	SuccessfulRequestsRelativePercent *int `json:"successful_requests_relative_percent,omitempty"`
}

// LeaderboardMyRank 是查看者自己的名次与真实数值：即使不进前 50 也返回；
// 查看者在该 Window 内零用量时整个对象为 null（页面提示「本窗口暂无用量」）。
//
// Hint（提示）是「你的位置」那一句话的数值来源，已经在前 10 名之内时缺席。
type LeaderboardMyRank struct {
	Rank               int64 `json:"rank"`
	TotalTokens        int64 `json:"total_tokens"`
	SuccessfulRequests int64 `json:"successful_requests"`

	Hint *LeaderboardMyRankHint `json:"hint,omitempty"`
}

// LeaderboardMyRankHint 是「你的位置」提示语的结构化数值。后端只下发数值，文案由前端
// i18n 渲染——与 identity 同一套思路，后端 MUST NOT 拼接句子（design D18）。
//
// Kind 决定哪几个数有值：
//   - tokens_to_top10 / requests_to_top10（named 档与 Preview）：Value 是按当前 Metric
//     还差多少才够得着第 10 名。单位随 Metric 变，因此分成两个 kind，前端不必再去
//     读顶层的 metric 才能决定量词。
//   - relative_percent（anonymous 档）：Self 与 Tenth 分别是本人与第 10 名相对第一名的
//     整数百分比。他人的绝对量 MUST NOT 出现在提示里，哪怕是差额形式。
type LeaderboardMyRankHint struct {
	Kind  string `json:"kind"`
	Value *int64 `json:"value,omitempty"`
	Self  *int   `json:"self,omitempty"`
	Tenth *int   `json:"tenth,omitempty"`
}

// LeaderboardView 是 GET /api/v1/leaderboard 的响应体（design D14）。
//
// ParticipantCount 的 JSON 类型随档位变化：named 档与 Preview 下是精确整数，
// anonymous 档下是分档字符串（如 "100+" / "<5"）。
type LeaderboardView struct {
	Window   string `json:"window"`
	Metric   string `json:"metric"`
	Mode     string `json:"mode"`
	Preview  bool   `json:"preview"`
	Timezone string `json:"timezone"`

	Status            string     `json:"status"`
	Stale             bool       `json:"stale"`
	SnapshotUpdatedAt *time.Time `json:"snapshot_updated_at"`

	ParticipantCount  any                `json:"participant_count"`
	Entries           []LeaderboardEntry `json:"entries"`
	EntriesSuppressed bool               `json:"entries_suppressed"`
	MyRank            *LeaderboardMyRank `json:"my_rank"`

	// 两个区块都可能整块缺失（快照未就绪、旧快照、预聚合关闭），此时序列化成 null，
	// 页面把对应区块渲染成骨架或干脆隐藏——因此刻意不带 omitempty。
	Highlights *LeaderboardHighlightsView `json:"highlights"`
	Insights   *LeaderboardInsightsView   `json:"insights"`

	// Viewer 是「你的统计」：全部是查看者本人的数据，因此 MUST NOT 随档位裁剪，
	// 也 MUST NOT 随 status 变化——它不出自 Snapshot（design D21）。永远不是 null。
	Viewer *LeaderboardViewerView `json:"viewer"`
}

// 以下是 Highlights（趣味卡）与 Insights（洞察）的**下发**形态，与 leaderboard_insights.go
// 里的存储形态刻意分成两套类型：存储层只有 user_id 与全量数值，下发层只有结构化身份与
// 按档位裁剪后的数值。裁剪只发生在这一处，handler MUST NOT 再裁一次（design D18）。
//
// 绝对量一律用指针 + omitempty 表达「字段是否存在」：anonymous 档下他人与站点级的绝对量
// MUST 缺席而不是清零，写法与 LeaderboardEntry 一致。

// LeaderboardHighlightsView 是某个 Window 的四块趣味卡。
// 三张人物卡在无人满足条件时为 null，MUST NOT 用零值对象顶替；site 始终存在。
// Extremes（之最）与四张卡同源：它们存在同一个 highlights JSON 里，因此快照未就绪时
// 一起为 null，MUST NOT 出现「有之最却没有四张卡」这种自相矛盾的响应（design D20）。
type LeaderboardHighlightsView struct {
	TopTokens   *LeaderboardHighlightUserView `json:"top_tokens"`
	TopRequests *LeaderboardHighlightUserView `json:"top_requests"`
	CacheKing   *LeaderboardCacheKingView     `json:"cache_king"`
	Site        LeaderboardSiteView           `json:"site"`
	Extremes    *LeaderboardExtremesView      `json:"extremes"`
}

// LeaderboardExtremesView 是六张之最小卡。每项无人满足条件时为 null，
// MUST NOT 用零值对象顶替；rising 只可能在 today 窗口有值。
//
// 六项的身份与 Highlights 完全同一套规则：named 档按 Named Participation 决定实名还是匿名，
// anonymous 档用当前 Window × Metric 榜单上的 Ordinal 假名，不在下发的 entries 里时
// ordinal 为 null（前端渲染成「榜外用户」）。
type LeaderboardExtremesView struct {
	NightOwl  *LeaderboardExtremeNightOwlView  `json:"night_owl"`
	Rising    *LeaderboardExtremeRisingView    `json:"rising"`
	Omnivore  *LeaderboardExtremeOmnivoreView  `json:"omnivore"`
	Talker    *LeaderboardExtremeTalkerView    `json:"talker"`
	MaxSingle *LeaderboardExtremeMaxSingleView `json:"max_single"`
	Streak    *LeaderboardExtremeStreakView    `json:"streak"`
}

// LeaderboardExtremeNightOwlView 是夜猫子：night_tokens 是绝对量，anonymous 档缺席；
// night_share_percent 是相对量，两档都给。
type LeaderboardExtremeNightOwlView struct {
	Identity LeaderboardIdentity `json:"identity"`
	Ordinal  *int                `json:"ordinal"`

	NightTokens       *int64 `json:"night_tokens,omitempty"`
	NightSharePercent int    `json:"night_share_percent"`
}

// LeaderboardExtremeRisingView 是进步之星：较昨日的增幅，两档都给。
type LeaderboardExtremeRisingView struct {
	Identity LeaderboardIdentity `json:"identity"`
	Ordinal  *int                `json:"ordinal"`

	ChangePercent int `json:"change_percent"`
}

// LeaderboardExtremeOmnivoreView 是杂食者：用过的不同模型数，两档都给
// （它是个位数量级的计数，不是用量规模）。
type LeaderboardExtremeOmnivoreView struct {
	Identity LeaderboardIdentity `json:"identity"`
	Ordinal  *int                `json:"ordinal"`

	DistinctModels int `json:"distinct_models"`
}

// LeaderboardExtremeTalkerView 是话痨：output 占自身 tokens 的百分比，两档都给。
type LeaderboardExtremeTalkerView struct {
	Identity LeaderboardIdentity `json:"identity"`
	Ordinal  *int                `json:"ordinal"`

	OutputSharePercent int `json:"output_share_percent"`
}

// LeaderboardExtremeMaxSingleView 是单次最大：max_single_tokens 是不折不扣的绝对量，
// anonymous 档缺席，改由 ratio_to_median（相对全体参与者中位数的倍数）表达同一件事。
type LeaderboardExtremeMaxSingleView struct {
	Identity LeaderboardIdentity `json:"identity"`
	Ordinal  *int                `json:"ordinal"`

	MaxSingleTokens *int64  `json:"max_single_tokens,omitempty"`
	RatioToMedian   float64 `json:"ratio_to_median"`
}

// LeaderboardExtremeStreakView 是连续活跃：天数与 Window 无关，三个窗口相同，两档都给。
type LeaderboardExtremeStreakView struct {
	Identity LeaderboardIdentity `json:"identity"`
	Ordinal  *int                `json:"ordinal"`

	Days int `json:"days"`
}

// LeaderboardHighlightUserView 是「今日卷王」与「最勤快」两张卡。
//
// Ordinal 是该用户在当前 Window × Metric 榜单上的行序号（前端渲染成「第 N 位」）：
// 不在下发的 entries 里时为 null，前端渲染成「榜外用户」——MUST NOT 用 Rank 顶替，
// Rank 会并列，会出现两张卡都写「第 1 位」。
type LeaderboardHighlightUserView struct {
	Identity LeaderboardIdentity `json:"identity"`
	Ordinal  *int                `json:"ordinal"`

	TotalTokens        *int64 `json:"total_tokens,omitempty"`
	SuccessfulRequests *int64 `json:"successful_requests,omitempty"`

	SharePercent int `json:"share_percent"`
	LeadPercent  int `json:"lead_percent"`
}

// LeaderboardCacheKingView 是「效率之星」卡：命中率是比率、模型名不是某个人的绝对用量，
// 两档都下发。DominantModel 查不到时是空串，前端隐藏那句文案。
type LeaderboardCacheKingView struct {
	Identity LeaderboardIdentity `json:"identity"`
	Ordinal  *int                `json:"ordinal"`

	CacheHitRate  float64 `json:"cache_hit_rate"`
	DominantModel string  `json:"dominant_model"`
}

// LeaderboardSiteView 是「全站概况」卡。ParticipantCount 的 JSON 类型随档位变化，
// 与顶层的 participant_count 同一条规则（named 精确整数、anonymous 分档字符串）。
//
// AvgTokensPerRequest 是比率（全站该窗口 tokens / 成功请求数），与 CacheHitRate 一样两档都给：
// 页面把它与 viewer.avg_tokens_per_request 并排对比。全站成功请求数为 0 时它在存储层就缺席，
// 这里也就无从下发——MUST NOT 记成 0。
type LeaderboardSiteView struct {
	TotalTokens        *int64 `json:"total_tokens,omitempty"`
	SuccessfulRequests *int64 `json:"successful_requests,omitempty"`

	ParticipantCount    any      `json:"participant_count"`
	CacheHitRate        *float64 `json:"cache_hit_rate,omitempty"`
	PeakHour            *int     `json:"peak_hour,omitempty"`
	AvgTokensPerRequest *float64 `json:"avg_tokens_per_request,omitempty"`
}

// LeaderboardInsightsView 是与 Window 无关的站点级洞察。
// 任一区块的数据源缺失时该区块为 null（切片为 nil），MUST NOT 用 0 填充，
// 也 MUST NOT 让整个响应失败。
//
// Profiles（模型偏好画像）是个例外：它按 Window 算、存在该 Window 的 highlights JSON 里，
// 只在下发时挂到这里——存储按「重建时按什么分组」划分，下发按「页面上属于哪个区块」划分
// （design D22）。连带后果是 status 为 computing 时它随 highlights 一起不可用。
type LeaderboardInsightsView struct {
	ModelsToday []LeaderboardModelView     `json:"models_today"`
	Daily30     []LeaderboardDailyView     `json:"daily_30"`
	HourlyToday []LeaderboardHourlyView    `json:"hourly_today"`
	CacheToday  *LeaderboardCacheTodayView `json:"cache_today"`
	Month       *LeaderboardMonthView      `json:"month"`

	Profiles         []LeaderboardProfileView    `json:"profiles"`
	PlatformsToday   []LeaderboardPlatformView   `json:"platforms_today"`
	WeeklyRhythm     [][]int                     `json:"weekly_rhythm"`
	CompositionToday *LeaderboardCompositionView `json:"composition_today"`
	CacheTrend14     []LeaderboardCacheTrendView `json:"cache_trend_14"`
}

// LeaderboardProfileView 是模型偏好画像的一行：身份规则与榜单条目、Highlights 完全相同，
// 占比两档都给。
type LeaderboardProfileView struct {
	Identity LeaderboardIdentity `json:"identity"`
	Ordinal  *int                `json:"ordinal"`

	Models []LeaderboardProfileModelView `json:"models"`
}

// LeaderboardProfileModelView 是画像里的一个模型及其占该用户该窗口成功请求的百分比。
type LeaderboardProfileModelView struct {
	Model        string `json:"model"`
	SharePercent int    `json:"share_percent"`
}

// LeaderboardPlatformView 是今日平台分布的一行。successful_requests 是站点级绝对量，
// anonymous 档缺席；share_percent 两档都给，页面的堆叠条靠它渲染。
type LeaderboardPlatformView struct {
	Platform           string `json:"platform"`
	SuccessfulRequests *int64 `json:"successful_requests,omitempty"`
	SharePercent       int    `json:"share_percent"`
}

// LeaderboardCompositionView 是今日 Token 构成：四段绝对量是站点级的，anonymous 档一个都不留；
// 四段百分比两档都给。百分比向下取整，因此四段之和可能是 99——页面按段宽渲染，不做凑整。
type LeaderboardCompositionView struct {
	InputTokens         *int64 `json:"input_tokens,omitempty"`
	OutputTokens        *int64 `json:"output_tokens,omitempty"`
	CacheCreationTokens *int64 `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     *int64 `json:"cache_read_tokens,omitempty"`

	InputPercent         int `json:"input_percent"`
	OutputPercent        int `json:"output_percent"`
	CacheCreationPercent int `json:"cache_creation_percent"`
	CacheReadPercent     int `json:"cache_read_percent"`
}

// LeaderboardCacheTrendView 是缓存命中率趋势的一天。命中率是比率，两档都给。
type LeaderboardCacheTrendView struct {
	Date         string  `json:"date"`
	CacheHitRate float64 `json:"cache_hit_rate"`
}

// LeaderboardViewerView 是「你的统计」：四部分全是查看者**本人**的数据，
// 因此在任何档位下都是真实值，MUST NOT 裁剪（design D21）。
//
// 它也不随 status 变化：rank_history 来自 Postgres 的旁路留痕，models 来自那条只查本人的
// 例外聚合，两者都不依赖 Snapshot。没有历史时 rank_history 是空数组（MUST NOT 是 null），
// 本窗口零用量时 models 是空数组且两个比率缺席。
type LeaderboardViewerView struct {
	RankHistory []LeaderboardRankPointView   `json:"rank_history"`
	Models      []LeaderboardViewerModelView `json:"models"`

	CacheHitRate        *float64 `json:"cache_hit_rate,omitempty"`
	AvgTokensPerRequest *float64 `json:"avg_tokens_per_request,omitempty"`
}

// LeaderboardRankPointView 是名次走势的一天：按 Total Tokens 的名次，名次越小越靠前。
type LeaderboardRankPointView struct {
	Date string `json:"date"`
	Rank int    `json:"rank"`
}

// LeaderboardViewerModelView 是本人模型偏好的一行。successful_requests 是本人自己的数，
// 因此两档都是精确值，不用指针——缺席在这里没有任何意义。
type LeaderboardViewerModelView struct {
	Model              string `json:"model"`
	SuccessfulRequests int64  `json:"successful_requests"`
	SharePercent       int    `json:"share_percent"`
}

// LeaderboardModelView 是今日模型热度的一行。successful_requests 是成功落账口径，
// 与下面两个区块的 requests（预聚合表的裸计数，含失败占位行）不是一回事，因此不同名。
type LeaderboardModelView struct {
	Model              string `json:"model"`
	SuccessfulRequests *int64 `json:"successful_requests,omitempty"`
	SharePercent       int    `json:"share_percent"`
}

// LeaderboardDailyView 是近 30 天的一天；RelativePercent 相对这段里 Total Tokens 最高的
// 一天，热力图色阶与趋势条宽共用它，因此两档都下发。
type LeaderboardDailyView struct {
	Date            string `json:"date"`
	Requests        *int64 `json:"requests,omitempty"`
	TotalTokens     *int64 `json:"total_tokens,omitempty"`
	RelativePercent int    `json:"relative_percent"`
}

// LeaderboardHourlyView 是今日的一个小时桶；峰值小时就是 RelativePercent 为 100 的那个。
type LeaderboardHourlyView struct {
	Hour            int    `json:"hour"`
	Requests        *int64 `json:"requests,omitempty"`
	RelativePercent int    `json:"relative_percent"`
}

// LeaderboardCacheTodayView 是今日全站缓存命中：命中率是比率，两档都给。
type LeaderboardCacheTodayView struct {
	CacheHitRate    float64 `json:"cache_hit_rate"`
	CacheReadTokens *int64  `json:"cache_read_tokens,omitempty"`
	InputTokens     *int64  `json:"input_tokens,omitempty"`
}

// LeaderboardMonthView 是「本月累计」与「较上月」；上月的绝对量从来不在结构里，
// 因此任何档位都无从下发。
type LeaderboardMonthView struct {
	TotalTokens   *int64 `json:"total_tokens,omitempty"`
	ChangePercent int    `json:"change_percent"`
}

// LeaderboardService 组装用户侧的榜单响应：只读 Snapshot，永不触发聚合、永不回源查
// usage_logs。数值与名次随快照冻结，展示名与参与资格按响应时刻的 users 当前状态渲染。
//
// viewerRepo 只服务顶层 viewer 那一块，两条查询都只涉及查看者本人（design D21）；
// 为 nil 时该块降级为空数组，榜单本体不受影响。
type LeaderboardService struct {
	cache      LeaderboardCache
	userRepo   LeaderboardUserRepository
	viewerRepo LeaderboardViewerRepository
}

// NewLeaderboardService 创建用户侧榜单查询服务。
func NewLeaderboardService(cache LeaderboardCache, userRepo LeaderboardUserRepository, viewerRepo LeaderboardViewerRepository) *LeaderboardService {
	return &LeaderboardService{cache: cache, userRepo: userRepo, viewerRepo: viewerRepo}
}

// leaderboardKeptEntry 是通过参与资格筛选、准备下发的一行（Ordinal 尚未分配）。
type leaderboardKeptEntry struct {
	userID   int64
	rank     int64
	metrics  LeaderboardUserMetrics
	isSelf   bool
	identity LeaderboardIdentity
}

// leaderboardIdentitySlot 是某个 user_id 在本次响应里已经渲染好的身份与行序号。
// Highlights 的领先者直接复用它：那批 users 已经查过一次，MUST NOT 为了几张卡再查一次
// （请求路径上只允许一次按 id 的批量查询，design D8）。
type leaderboardIdentitySlot struct {
	identity LeaderboardIdentity
	ordinal  int
}

// Query 返回某个 Window × Metric 的榜单视图。
//
// mode 由路由 guard 从带短 TTL 缓存的读取器取得后透传进来（读不到时是 off）；这里再
// 归一化一次并自行判定 off 档的可见性——隐私控制必须在服务端强制，不能只靠 guard。
func (s *LeaderboardService) Query(
	ctx context.Context,
	viewerID int64,
	window LeaderboardWindow,
	metric LeaderboardMetric,
	mode string,
	viewerIsAdmin bool,
) (*LeaderboardView, error) {
	if s == nil {
		return nil, errors.New("排行榜服务未初始化")
	}

	mode = normalizeLeaderboardMode(mode)
	preview := false
	if mode == LeaderboardModeOff {
		// off 档：普通用户一律 404（连功能存在都不确认），管理员进入 Preview。
		if !viewerIsAdmin {
			return nil, ErrLeaderboardNotFound
		}
		preview = true
	}
	// Preview 按 named 档的身份形态与数值精度渲染：管理员要预览的正是开启后最开放的形态，
	// 否则他仍要靠「先开后关」才能看到效果（design D9）。
	renderNamed := mode == LeaderboardModeNamed || preview

	if metric != LeaderboardMetricTotalTokens && metric != LeaderboardMetricSuccessfulRequests {
		return nil, ErrLeaderboardInvalidMetric
	}
	now := timezone.Now()
	windowStart, windowEnd, ok := LeaderboardWindowBounds(window, now)
	if !ok {
		return nil, ErrLeaderboardInvalidWindow
	}
	todayStart, _, _ := LeaderboardWindowBounds(LeaderboardWindowToday, now)

	view := &LeaderboardView{
		Window:           string(window),
		Metric:           string(metric),
		Mode:             mode,
		Preview:          preview,
		Timezone:         timezone.Name(),
		Status:           LeaderboardStatusComputing,
		ParticipantCount: leaderboardParticipantCount(0, mode),
		Entries:          []LeaderboardEntry{},
	}
	// viewer 在任何分支之前就组装好：它不出自 Snapshot，因此 MUST NOT 随「正在计算」、
	// 抑制态或 Redis 抖动一起消失，也 MUST NOT 为 null（design D21）。
	view.Viewer = s.viewerView(ctx, viewerID, window, windowStart, windowEnd, todayStart)
	if s.cache == nil {
		return view, nil
	}

	// Insights（洞察）是站点级的、与 Window 无关的一份数据，key 永远按今日窗口起点取，
	// 且不随本 Window 的快照状态变化：「正在计算」的榜单旁边照样可以有洞察区块。
	// 读不到（旧快照、作业没跑过、Redis 抖动）就整块为 null，页面隐藏对应区块。
	if insights, insightsErr := s.cache.Insights(ctx, todayStart); insightsErr == nil {
		view.Insights = leaderboardInsightsView(insights, renderNamed)
	}

	// 快照缺失（key 不存在）或 Redis 不可用都归到「正在计算」：不空榜、不 500、不回源。
	updatedAt, exists, err := s.cache.UpdatedAt(ctx, window, windowStart)
	if err != nil || !exists {
		return view, nil
	}
	view.Status = LeaderboardStatusReady
	snapshotUpdatedAt := updatedAt
	view.SnapshotUpdatedAt = &snapshotUpdatedAt
	view.Stale = now.Sub(updatedAt) > LeaderboardSnapshotStaleAfter

	count, err := s.cache.ParticipantCount(ctx, window, windowStart, metric)
	if err != nil {
		return leaderboardComputingView(view, mode), nil
	}
	view.ParticipantCount = leaderboardParticipantCount(count, mode)

	// anonymous 档人数过少时不下发任何条目，只保留本人行信息（design D1）。
	suppressed := mode == LeaderboardModeAnonymous && count < leaderboardAnonymousMinParticipants
	view.EntriesSuppressed = suppressed

	var top []LeaderboardScoreEntry
	if !suppressed {
		top, err = s.cache.TopEntries(ctx, window, windowStart, metric, LeaderboardTopEntryLimit)
		if err != nil {
			return leaderboardComputingView(view, mode), nil
		}
	}

	// Rank 是竞争排名：严格高于自己的人数加一，并列同名次、其后跳号。
	// top 已按分数倒序，且任何分数严格更高的用户必然也在 top 内，因此可以就地推出名次，
	// 不必为每一行再打一次 ZCOUNT。查看者若在榜内，My Rank 直接复用同一个值，
	// 两处因此永不互相矛盾。
	ranks := leaderboardCompetitionRanks(top)

	ids := make([]int64, 0, len(top)+1)
	viewerInTop := false
	for i := range top {
		ids = append(ids, top[i].UserID)
		if top[i].UserID == viewerID {
			viewerInTop = true
		}
	}
	if !viewerInTop && viewerID > 0 {
		ids = append(ids, viewerID)
	}

	metricsByUser, err := s.cache.MetricsOf(ctx, window, windowStart, ids)
	if err != nil {
		return leaderboardComputingView(view, mode), nil
	}

	// My Rank：查看者在榜内时复用榜内名次，否则用 ZCOUNT 单独算一次。
	if viewerMetrics, hasUsage := metricsByUser[viewerID]; hasUsage && viewerID > 0 {
		// 「你 vs 全站」的两个比率取自本人在该 Window Hash 里的那一行，不额外查库；
		// 本窗口零用量（连 Hash 行都没有）时两个字段缺席，MUST NOT 记成 0。
		leaderboardFillViewerRatios(view.Viewer, viewerMetrics)

		viewerRank := int64(0)
		found := false
		if viewerInTop {
			for i := range top {
				if top[i].UserID == viewerID {
					viewerRank, found = ranks[i], true
					break
				}
			}
		} else {
			viewerRank, found, err = s.cache.RankOf(ctx, window, windowStart, metric, viewerID)
			if err != nil {
				return leaderboardComputingView(view, mode), nil
			}
		}
		if found {
			view.MyRank = &LeaderboardMyRank{
				Rank:               viewerRank,
				TotalTokens:        viewerMetrics.TotalTokens,
				SuccessfulRequests: viewerMetrics.SuccessfulRequests,
			}
			view.MyRank.Hint = leaderboardMyRankHint(top, viewerMetrics.Metric(metric), viewerRank, metric, renderNamed)
		}
	}

	if suppressed || len(top) == 0 {
		return view, nil
	}

	entries, slots, err := s.renderEntries(ctx, top, ranks, metricsByUser, viewerID, renderNamed)
	if err != nil {
		return nil, err
	}
	view.Entries = entries

	// Highlights（趣味卡）与榜单同一轮写入、同一批 RENAME，因此这里读到的必然与上面的
	// entries 同源。读不到只丢这一块（页面把四张卡渲染成骨架），而不把整页退回
	// 「正在计算」——榜单本体已经完整，没必要一起作废。
	if highlights, highlightsErr := s.cache.Highlights(ctx, window, windowStart); highlightsErr == nil {
		view.Highlights = leaderboardHighlightsView(highlights, slots, viewerID, renderNamed, mode)
		// 画像存在 highlights 里、下发在 insights 下（design D22）：它的可用性只随 highlights 走，
		// 与站点级 insights 那一个 key 无关。insights 读不到时不该把已经读到的画像一起丢掉，
		// 补一个空壳 insights 挂上去即可——其余字段仍是缺席，页面照旧隐藏那些区块。
		if highlights != nil && len(highlights.Profiles) > 0 {
			if view.Insights == nil {
				view.Insights = &LeaderboardInsightsView{}
			}
			view.Insights.Profiles = leaderboardProfilesView(highlights.Profiles, slots, viewerID)
		}
	}
	return view, nil
}

// viewerView 组装顶层 viewer。它在任何档位下都是真实值：里面全是查看者自己的数据，
// 与 D18 的「他人与站点级绝对量按档位裁剪」无关（design D21）。
//
// 两块各自独立降级：名次历史读不动就是空数组，模型偏好读不动也是空数组，
// MUST NOT 因此让整个响应失败。viewerID 非正（理论上不会发生，handler 已拦）时同样返回空壳。
func (s *LeaderboardService) viewerView(
	ctx context.Context,
	viewerID int64,
	window LeaderboardWindow,
	windowStart, windowEnd, todayStart time.Time,
) *LeaderboardViewerView {
	view := &LeaderboardViewerView{
		RankHistory: []LeaderboardRankPointView{},
		Models:      []LeaderboardViewerModelView{},
	}
	if s == nil || viewerID <= 0 {
		return view
	}
	view.RankHistory = s.viewerRankHistory(ctx, viewerID, todayStart)
	view.Models = s.viewerModels(ctx, viewerID, window, windowStart, windowEnd)
	return view
}

// viewerRankHistory 取查看者本人近 14 天在 today 窗口按 Total Tokens 的名次。
//
// 只查一个 user_id：响应 MUST NOT 包含任何其他用户的名次历史。这张表由快照作业逐轮覆盖写，
// 与 Snapshot 无关，因此 status 为 computing 时照常有值。
func (s *LeaderboardService) viewerRankHistory(ctx context.Context, viewerID int64, todayStart time.Time) []LeaderboardRankPointView {
	points := []LeaderboardRankPointView{}
	if s.viewerRepo == nil {
		return points
	}
	from, to := leaderboardRankHistoryRange(todayStart)
	rows, err := s.viewerRepo.LeaderboardRankHistory(ctx, viewerID, from, to)
	if err != nil {
		logger.LegacyPrintf("service.leaderboard",
			"[Leaderboard] 名次走势读取失败，该块降级为空数组 (user_id=%d): %v", viewerID, err)
		return points
	}
	for _, row := range rows {
		points = append(points, LeaderboardRankPointView{
			// 日期串直接格式化 DATE 值本身：它落库时就是站点时区下的那一天，
			// MUST NOT 再做一次时区转换，否则非 UTC 站点上会整体偏一天。
			Date: row.SnapshotDate.Format(leaderboardInsightDateLayout),
			Rank: row.RankTotalTokens,
		})
	}
	return points
}

// viewerModels 取查看者本人在当前 Window 的 Top 5 模型及占比。
//
// 这是 design D8「请求路径只读」的唯一例外，边界写死三条：只查自己、只服务这一个字段、
// 结果按 (user_id, window, 窗口起点) 在 Redis 上缓存 60 秒。缓存命中时 MUST NOT 再查一次库；
// 缓存不可用只是少一层，回源照做；回源失败则整块降级为空数组，MUST NOT 影响响应其余部分。
func (s *LeaderboardService) viewerModels(
	ctx context.Context,
	viewerID int64,
	window LeaderboardWindow,
	windowStart, windowEnd time.Time,
) []LeaderboardViewerModelView {
	models := []LeaderboardViewerModelView{}

	var (
		rows  []usagestats.LeaderboardModelUsageRow
		total int64
		found bool
	)
	if s.cache != nil {
		cachedRows, cachedTotal, cached, err := s.cache.ViewerModels(ctx, viewerID, window, windowStart)
		switch {
		case err != nil:
			// Redis 不可用只是少一层缓存：这一块本来就允许回源，因此继续往下走。
			logger.LegacyPrintf("service.leaderboard",
				"[Leaderboard] 模型偏好缓存读取失败，本次回源 (user_id=%d, window=%s): %v", viewerID, window, err)
		case cached:
			rows, total, found = cachedRows, cachedTotal, true
		}
	}

	if !found {
		if s.viewerRepo == nil {
			return models
		}
		queried, queriedTotal, err := s.viewerRepo.LeaderboardViewerModels(ctx, viewerID, windowStart, windowEnd, leaderboardViewerModelsLimit)
		if err != nil {
			logger.LegacyPrintf("service.leaderboard",
				"[Leaderboard] 模型偏好聚合失败，该块降级为空数组 (user_id=%d, window=%s): %v", viewerID, window, err)
			return models
		}
		rows, total = queried, queriedTotal
		if s.cache != nil {
			// 零行也照样回填：一个本窗口没有用量的查看者每分钟只该触达一次数据库。
			if setErr := s.cache.SetViewerModels(ctx, viewerID, window, windowStart, windowEnd, rows, total); setErr != nil {
				logger.LegacyPrintf("service.leaderboard",
					"[Leaderboard] 模型偏好缓存回填失败，不影响本次响应 (user_id=%d, window=%s): %v", viewerID, window, setErr)
			}
		}
	}

	for _, row := range rows {
		models = append(models, LeaderboardViewerModelView{
			Model:              row.Model,
			SuccessfulRequests: row.SuccessfulRequests,
			SharePercent:       leaderboardRelativePercent(row.SuccessfulRequests, total),
		})
	}
	return models
}

// leaderboardFillViewerRatios 把本人的缓存命中率与平均每请求 tokens 填进 viewer。
// 两个分母为 0 时对应字段缺席而不是 0：「没有可算的比率」与「比率是 0」是两回事。
func leaderboardFillViewerRatios(viewer *LeaderboardViewerView, metrics LeaderboardUserMetrics) {
	if viewer == nil {
		return
	}
	if rate, ok := leaderboardCacheHitRate(metrics.InputTokens, metrics.CacheReadTokens); ok {
		viewer.CacheHitRate = &rate
	}
	if metrics.SuccessfulRequests > 0 {
		avg := float64(metrics.TotalTokens) / float64(metrics.SuccessfulRequests)
		viewer.AvgTokensPerRequest = &avg
	}
}

// leaderboardMyRankHint 算出「你的位置」那一句提示的数值：进前 10 还差多少。
//
// 查看者已经在前 10 名之内、或参与人数不足 10 人（人人都在前 10）时没有可说的差距，
// 返回 nil；anonymous 档只给相对第一名的百分比，他人的绝对量连差额形式都不出现
// （design D18）。
//
// top 是按当前 Metric 倒序的前 50 名分数，因此第 10 名的分数就是 top[9]：追平它
// 即意味着严格高于自己的人至多 9 个，名次因此 ≤ 10。
func leaderboardMyRankHint(
	top []LeaderboardScoreEntry,
	viewerScore int64,
	viewerRank int64,
	metric LeaderboardMetric,
	renderNamed bool,
) *LeaderboardMyRankHint {
	if viewerRank <= leaderboardHintTopN || len(top) < leaderboardHintTopN {
		return nil
	}
	tenthScore := top[leaderboardHintTopN-1].Score

	if !renderNamed {
		self := leaderboardRelativePercent(viewerScore, top[0].Score)
		tenth := leaderboardRelativePercent(tenthScore, top[0].Score)
		return &LeaderboardMyRankHint{Kind: LeaderboardHintRelativePercent, Self: &self, Tenth: &tenth}
	}

	gap := tenthScore - viewerScore
	if gap <= 0 {
		// 追平第 10 名的分数就已经进前 10，理论上不会走到这里；真走到了宁可不提示，
		// 也不给一句「再多 0 个就能进前 10」。
		return nil
	}
	kind := LeaderboardHintTokensToTop10
	if metric == LeaderboardMetricSuccessfulRequests {
		kind = LeaderboardHintRequestsToTop10
	}
	return &LeaderboardMyRankHint{Kind: kind, Value: &gap}
}

// renderEntries 按响应时刻的 users 当前状态剔除不合格用户、判定身份形态并填数值。
//
// 剔除 MUST NOT 让剩余条目的 Rank 重算，也 MUST NOT 从第 51 名起回填（entries 因此可以
// 少于 50 条）；Ordinal 在剔除完成后按剩余条目从 1 连续分配。
// 第二个返回值是 user_id → 身份与行序号的索引，供 Highlights 复用同一次查询的结果。
func (s *LeaderboardService) renderEntries(
	ctx context.Context,
	top []LeaderboardScoreEntry,
	ranks []int64,
	metricsByUser map[int64]LeaderboardUserMetrics,
	viewerID int64,
	renderNamed bool,
) ([]LeaderboardEntry, map[int64]leaderboardIdentitySlot, error) {
	ids := make([]int64, 0, len(top))
	for i := range top {
		ids = append(ids, top[i].UserID)
	}
	users, err := s.lookupUsers(ctx, ids)
	if err != nil {
		return nil, nil, err
	}

	kept := make([]leaderboardKeptEntry, 0, len(top))
	var maxTokens, maxRequests int64
	for i := range top {
		userID := top[i].UserID
		user, ok := users[userID]
		if !ok || !leaderboardUserEligible(user) {
			// 快照冻结时合格、此刻已被禁用 / 软删除：渲染时剔除，且不下发其任何身份信息。
			continue
		}
		metrics := metricsByUser[userID]
		metrics.UserID = userID

		isSelf := viewerID > 0 && userID == viewerID
		identity := LeaderboardIdentity{Kind: LeaderboardIdentityAnonymous}
		switch {
		case isSelf:
			identity.Kind = LeaderboardIdentitySelf
		case renderNamed && isLeaderboardNamedEligible(user):
			identity.Kind = LeaderboardIdentityNamed
			identity.Username = strings.TrimSpace(user.Username)
		}

		if metrics.TotalTokens > maxTokens {
			maxTokens = metrics.TotalTokens
		}
		if metrics.SuccessfulRequests > maxRequests {
			maxRequests = metrics.SuccessfulRequests
		}
		kept = append(kept, leaderboardKeptEntry{
			userID:   userID,
			rank:     ranks[i],
			metrics:  metrics,
			isSelf:   isSelf,
			identity: identity,
		})
	}

	entries := make([]LeaderboardEntry, 0, len(kept))
	slots := make(map[int64]leaderboardIdentitySlot, len(kept))
	for i, item := range kept {
		entry := LeaderboardEntry{
			Rank:     item.rank,
			Ordinal:  i + 1,
			Identity: item.identity,
			IsSelf:   item.isSelf,
		}
		slots[item.userID] = leaderboardIdentitySlot{identity: item.identity, ordinal: entry.Ordinal}
		if renderNamed || item.isSelf {
			// named 档与 Preview 下全部精确；anonymous 档下本人行也始终是真实数值。
			tokens, requests := item.metrics.TotalTokens, item.metrics.SuccessfulRequests
			entry.TotalTokens = &tokens
			entry.SuccessfulRequests = &requests
		} else {
			tokensPercent := leaderboardRelativePercent(item.metrics.TotalTokens, maxTokens)
			requestsPercent := leaderboardRelativePercent(item.metrics.SuccessfulRequests, maxRequests)
			entry.TotalTokensRelativePercent = &tokensPercent
			entry.SuccessfulRequestsRelativePercent = &requestsPercent
		}
		entries = append(entries, entry)
	}
	return entries, slots, nil
}

// lookupUsers 做请求路径上唯一的那次数据库访问：按 id 批量取用户。
func (s *LeaderboardService) lookupUsers(ctx context.Context, ids []int64) (map[int64]*User, error) {
	out := make(map[int64]*User, len(ids))
	if s.userRepo == nil || len(ids) == 0 {
		return out, nil
	}
	users, err := s.userRepo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range users {
		out[users[i].ID] = &users[i]
	}
	return out, nil
}

// leaderboardUserEligible 判定参与资格：被禁用与已软删除的用户不参与展示。
// 管理员账号照常参与，不做任何角色分支。
func leaderboardUserEligible(u *User) bool {
	if u == nil {
		return false
	}
	if u.DeletedAt != nil {
		return false
	}
	return u.Status != StatusDisabled
}

// leaderboardCompetitionRanks 按倒序的分数列推出竞争排名：并列同名次，其后跳号（1、1、3）。
func leaderboardCompetitionRanks(top []LeaderboardScoreEntry) []int64 {
	ranks := make([]int64, len(top))
	for i := range top {
		if i > 0 && top[i].Score == top[i-1].Score {
			ranks[i] = ranks[i-1]
			continue
		}
		ranks[i] = int64(i + 1)
	}
	return ranks
}

// leaderboardRelativePercent 是相对该 Metric 第一名的整数百分比：第一名为 100。
// 向下取整，避免 99.6% 被四舍五入成 100 而冒充第一名。
func leaderboardRelativePercent(value, max int64) int {
	if max <= 0 || value <= 0 {
		return 0
	}
	percent := int(value * 100 / max)
	if percent > 100 {
		return 100
	}
	return percent
}

// leaderboardParticipantCount 按档位决定 participant_count 的表达形式：
// named 档与 Preview 精确整数，anonymous 档只给分档字符串。
func leaderboardParticipantCount(count int64, mode string) any {
	if mode == LeaderboardModeAnonymous {
		return leaderboardParticipantBucket(count)
	}
	if count < 0 {
		return int64(0)
	}
	return count
}

// leaderboardParticipantBucket 取下界序列中不超过实际人数的最大值，表达为「N+」。
func leaderboardParticipantBucket(count int64) string {
	bucket := int64(0)
	for _, lower := range leaderboardParticipantBuckets {
		if count >= lower {
			bucket = lower
		}
	}
	if bucket == 0 {
		return "<5"
	}
	return strconv.FormatInt(bucket, 10) + "+"
}

// leaderboardComputingView 把一份已开头的视图退回「正在计算」：读到一半 Redis 出错时，
// 宁可整体显示「正在计算」，也不给出半份自相矛盾的榜单。
func leaderboardComputingView(view *LeaderboardView, mode string) *LeaderboardView {
	view.Status = LeaderboardStatusComputing
	view.SnapshotUpdatedAt = nil
	view.Stale = false
	view.ParticipantCount = leaderboardParticipantCount(0, mode)
	view.Entries = []LeaderboardEntry{}
	view.EntriesSuppressed = false
	view.MyRank = nil
	// 趣味卡与榜单同源：榜单退回「正在计算」时它（连同 extremes）也一起消失，
	// 页面把卡片渲染成骨架。Insights 是站点级的、viewer 是查看者自己的，两者都不随
	// 快照状态变化，因此刻意保留（design D17 / D21）。
	view.Highlights = nil
	return view
}

// leaderboardHighlightsView 把存储层的 Highlights 裁成下发形态（design D18）。
//
// 两件事在这里一次做完：身份用已渲染好的 entries 索引补上（榜外即「榜外用户」），
// 数值按档位裁剪——anonymous 档下他人与站点级的绝对量一个都不留，只剩 share_percent、
// lead_percent、cache_hit_rate、peak_hour 与分档后的 participant_count。
func leaderboardHighlightsView(
	highlights *LeaderboardHighlights,
	slots map[int64]leaderboardIdentitySlot,
	viewerID int64,
	renderNamed bool,
	mode string,
) *LeaderboardHighlightsView {
	if highlights == nil {
		return nil
	}

	view := &LeaderboardHighlightsView{
		TopTokens:   leaderboardHighlightUserView(highlights.TopTokens, slots, viewerID, renderNamed),
		TopRequests: leaderboardHighlightUserView(highlights.TopRequests, slots, viewerID, renderNamed),
		Site: LeaderboardSiteView{
			ParticipantCount: leaderboardParticipantCount(highlights.Site.ParticipantCount, mode),
		},
	}
	if rate := highlights.Site.CacheHitRate; rate != nil {
		siteRate := *rate
		view.Site.CacheHitRate = &siteRate
	}
	if hour := highlights.Site.PeakHour; hour != nil {
		peakHour := *hour
		view.Site.PeakHour = &peakHour
	}
	// 平均每请求 tokens 是比率，与命中率同一条规则：两档都下发。
	if avg := highlights.Site.AvgTokensPerRequest; avg != nil {
		siteAvg := *avg
		view.Site.AvgTokensPerRequest = &siteAvg
	}
	view.Extremes = leaderboardExtremesView(highlights.Extremes, slots, viewerID, renderNamed)
	if renderNamed {
		tokens, requests := highlights.Site.TotalTokens, highlights.Site.SuccessfulRequests
		view.Site.TotalTokens, view.Site.SuccessfulRequests = &tokens, &requests
	}

	if king := highlights.CacheKing; king != nil {
		identity, ordinal := leaderboardHighlightIdentity(king.UserID, slots, viewerID)
		view.CacheKing = &LeaderboardCacheKingView{
			Identity:      identity,
			Ordinal:       ordinal,
			CacheHitRate:  king.CacheHitRate,
			DominantModel: king.DominantModel,
		}
	}
	return view
}

// leaderboardHighlightUserView 裁剪一张人物卡：两个相对量两档都给，两个绝对量只在
// named 档与 Preview 下给——查看者本人恰好是领先者时也照这条裁，那张卡是给所有人看的
// 同一份数据，不是「我的数据」。
func leaderboardHighlightUserView(
	leader *LeaderboardHighlightUser,
	slots map[int64]leaderboardIdentitySlot,
	viewerID int64,
	renderNamed bool,
) *LeaderboardHighlightUserView {
	if leader == nil {
		return nil
	}
	identity, ordinal := leaderboardHighlightIdentity(leader.UserID, slots, viewerID)
	view := &LeaderboardHighlightUserView{
		Identity:     identity,
		Ordinal:      ordinal,
		SharePercent: leader.SharePercent,
		LeadPercent:  leader.LeadPercent,
	}
	if renderNamed {
		tokens, requests := leader.TotalTokens, leader.SuccessfulRequests
		view.TotalTokens, view.SuccessfulRequests = &tokens, &requests
	}
	return view
}

// leaderboardExtremesView 裁剪六张之最小卡（design D20）。
//
// 身份与 Highlights 共用 leaderboardHighlightIdentity：同一批已渲染的 entries 索引，
// 因此 MUST NOT 为这六张卡再查一次 users。数值只有两个是绝对量——night_tokens 与
// max_single_tokens，anonymous 档下缺席；其余六个相对量与比率两档都给（design D18）。
//
// extremes 为 nil（空窗口或旧快照）时整块为 null；窗口非空但六项都无人达标时，
// 存储层给的是一个六项全 nil 的结构，这里照样渲染成 `"extremes": {…全 null}`——
// 「算过了没人达标」与「这一轮根本没算」在响应上是两回事。
func leaderboardExtremesView(
	extremes *LeaderboardExtremes,
	slots map[int64]leaderboardIdentitySlot,
	viewerID int64,
	renderNamed bool,
) *LeaderboardExtremesView {
	if extremes == nil {
		return nil
	}
	view := &LeaderboardExtremesView{}

	if owl := extremes.NightOwl; owl != nil {
		identity, ordinal := leaderboardHighlightIdentity(owl.UserID, slots, viewerID)
		card := &LeaderboardExtremeNightOwlView{
			Identity:          identity,
			Ordinal:           ordinal,
			NightSharePercent: owl.NightSharePercent,
		}
		if renderNamed {
			nightTokens := owl.NightTokens
			card.NightTokens = &nightTokens
		}
		view.NightOwl = card
	}

	// rising 只可能在 today 窗口有值：其余窗口存储层就没有这一项，这里也就无从渲染。
	if rising := extremes.Rising; rising != nil {
		identity, ordinal := leaderboardHighlightIdentity(rising.UserID, slots, viewerID)
		view.Rising = &LeaderboardExtremeRisingView{
			Identity:      identity,
			Ordinal:       ordinal,
			ChangePercent: rising.ChangePercent,
		}
	}

	if omnivore := extremes.Omnivore; omnivore != nil {
		identity, ordinal := leaderboardHighlightIdentity(omnivore.UserID, slots, viewerID)
		view.Omnivore = &LeaderboardExtremeOmnivoreView{
			Identity:       identity,
			Ordinal:        ordinal,
			DistinctModels: omnivore.DistinctModels,
		}
	}

	if talker := extremes.Talker; talker != nil {
		identity, ordinal := leaderboardHighlightIdentity(talker.UserID, slots, viewerID)
		view.Talker = &LeaderboardExtremeTalkerView{
			Identity:           identity,
			Ordinal:            ordinal,
			OutputSharePercent: talker.OutputSharePercent,
		}
	}

	if maxSingle := extremes.MaxSingle; maxSingle != nil {
		identity, ordinal := leaderboardHighlightIdentity(maxSingle.UserID, slots, viewerID)
		card := &LeaderboardExtremeMaxSingleView{
			Identity:      identity,
			Ordinal:       ordinal,
			RatioToMedian: maxSingle.RatioToMedian,
		}
		if renderNamed {
			maxSingleTokens := maxSingle.MaxSingleTokens
			card.MaxSingleTokens = &maxSingleTokens
		}
		view.MaxSingle = card
	}

	if streak := extremes.Streak; streak != nil {
		identity, ordinal := leaderboardHighlightIdentity(streak.UserID, slots, viewerID)
		view.Streak = &LeaderboardExtremeStreakView{
			Identity: identity,
			Ordinal:  ordinal,
			Days:     streak.Days,
		}
	}
	return view
}

// leaderboardProfilesView 裁剪模型偏好画像：身份按档位（与榜单条目同一套规则），
// 模型占比两档都给。存储层没有这一块时返回 nil，下发即 `"profiles": null`。
func leaderboardProfilesView(
	profiles []LeaderboardProfile,
	slots map[int64]leaderboardIdentitySlot,
	viewerID int64,
) []LeaderboardProfileView {
	if len(profiles) == 0 {
		return nil
	}
	views := make([]LeaderboardProfileView, 0, len(profiles))
	for _, profile := range profiles {
		identity, ordinal := leaderboardHighlightIdentity(profile.UserID, slots, viewerID)
		models := make([]LeaderboardProfileModelView, 0, len(profile.Models))
		for _, model := range profile.Models {
			models = append(models, LeaderboardProfileModelView(model))
		}
		views = append(views, LeaderboardProfileView{Identity: identity, Ordinal: ordinal, Models: models})
	}
	return views
}

// leaderboardHighlightIdentity 把 Highlights 里的 user_id 换成结构化身份与行序号。
//
// 只认 entries 里已经渲染过的那批用户：领先者不在前 50、或已被剔除（禁用 / 软删除）时
// 既没有 Ordinal 也没有可下发的身份，一律退回匿名形态（前端渲染成「榜外用户」）——
// MUST NOT 为此再查一次 users，更 MUST NOT 把已下架用户的 username 从别处补回来。
func leaderboardHighlightIdentity(
	userID int64,
	slots map[int64]leaderboardIdentitySlot,
	viewerID int64,
) (LeaderboardIdentity, *int) {
	if slot, ok := slots[userID]; ok {
		ordinal := slot.ordinal
		return slot.identity, &ordinal
	}
	if viewerID > 0 && userID == viewerID {
		// 查看者本人排在 50 名开外时仍然认得出自己，页面照样渲染「当前用户」。
		return LeaderboardIdentity{Kind: LeaderboardIdentitySelf}, nil
	}
	return LeaderboardIdentity{Kind: LeaderboardIdentityAnonymous}, nil
}

// leaderboardInsightsView 把存储层的 Insights 裁成下发形态。
//
// 相对量（relative_percent）与比率（cache_hit_rate、change_percent）两档都给：
// 热力图的色阶、趋势条的宽度与柱高都靠它们，named 档同样要用。
func leaderboardInsightsView(insights *LeaderboardInsights, renderNamed bool) *LeaderboardInsightsView {
	if insights == nil {
		return nil
	}
	view := &LeaderboardInsightsView{}

	if len(insights.ModelsToday) > 0 {
		models := make([]LeaderboardModelView, 0, len(insights.ModelsToday))
		for _, row := range insights.ModelsToday {
			model := LeaderboardModelView{Model: row.Model, SharePercent: row.SharePercent}
			if renderNamed {
				requests := row.SuccessfulRequests
				model.SuccessfulRequests = &requests
			}
			models = append(models, model)
		}
		view.ModelsToday = models
	}

	if len(insights.Daily30) > 0 {
		daily := make([]LeaderboardDailyView, 0, len(insights.Daily30))
		for _, row := range insights.Daily30 {
			day := LeaderboardDailyView{Date: row.Date, RelativePercent: row.RelativePercent}
			if renderNamed {
				requests, tokens := row.Requests, row.TotalTokens
				day.Requests, day.TotalTokens = &requests, &tokens
			}
			daily = append(daily, day)
		}
		view.Daily30 = daily
	}

	if len(insights.HourlyToday) > 0 {
		hourly := make([]LeaderboardHourlyView, 0, len(insights.HourlyToday))
		for _, row := range insights.HourlyToday {
			bucket := LeaderboardHourlyView{Hour: row.Hour, RelativePercent: row.RelativePercent}
			if renderNamed {
				requests := row.Requests
				bucket.Requests = &requests
			}
			hourly = append(hourly, bucket)
		}
		view.HourlyToday = hourly
	}

	if cache := insights.CacheToday; cache != nil {
		today := &LeaderboardCacheTodayView{CacheHitRate: cache.CacheHitRate}
		if renderNamed {
			cacheRead, input := cache.CacheReadTokens, cache.InputTokens
			today.CacheReadTokens, today.InputTokens = &cacheRead, &input
		}
		view.CacheToday = today
	}

	if month := insights.Month; month != nil {
		view.Month = &LeaderboardMonthView{ChangePercent: month.ChangePercent}
		if renderNamed {
			tokens := month.TotalTokens
			view.Month.TotalTokens = &tokens
		}
	}

	if len(insights.PlatformsToday) > 0 {
		platforms := make([]LeaderboardPlatformView, 0, len(insights.PlatformsToday))
		for _, row := range insights.PlatformsToday {
			platform := LeaderboardPlatformView{Platform: row.Platform, SharePercent: row.SharePercent}
			if renderNamed {
				requests := row.SuccessfulRequests
				platform.SuccessfulRequests = &requests
			}
			platforms = append(platforms, platform)
		}
		view.PlatformsToday = platforms
	}

	// 周内节奏两档完全相同：0–4 的等级本来就是色阶，不含任何绝对规模。
	// 这里按行拷贝一份，避免下发结构与解码出来的存储结构共享底层数组。
	if len(insights.WeeklyRhythm) > 0 {
		rhythm := make([][]int, 0, len(insights.WeeklyRhythm))
		for _, row := range insights.WeeklyRhythm {
			rhythm = append(rhythm, append([]int(nil), row...))
		}
		view.WeeklyRhythm = rhythm
	}

	if composition := insights.CompositionToday; composition != nil {
		today := &LeaderboardCompositionView{
			InputPercent:         composition.InputPercent,
			OutputPercent:        composition.OutputPercent,
			CacheCreationPercent: composition.CacheCreationPercent,
			CacheReadPercent:     composition.CacheReadPercent,
		}
		if renderNamed {
			input, output := composition.InputTokens, composition.OutputTokens
			cacheCreation, cacheRead := composition.CacheCreationTokens, composition.CacheReadTokens
			today.InputTokens, today.OutputTokens = &input, &output
			today.CacheCreationTokens, today.CacheReadTokens = &cacheCreation, &cacheRead
		}
		view.CompositionToday = today
	}

	if len(insights.CacheTrend14) > 0 {
		trend := make([]LeaderboardCacheTrendView, 0, len(insights.CacheTrend14))
		for _, row := range insights.CacheTrend14 {
			trend = append(trend, LeaderboardCacheTrendView(row))
		}
		view.CacheTrend14 = trend
	}
	return view
}
