package service

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/google/uuid"
)

// LeaderboardWindow 是 Leaderboard（排行榜）的 Window（榜单窗口）：固定三档，
// 边界一律按站点时区计算，周从周一开始；没有自定义日期，也没有「全部时间」。
type LeaderboardWindow string

const (
	LeaderboardWindowToday LeaderboardWindow = "today"
	LeaderboardWindowWeek  LeaderboardWindow = "week"
	LeaderboardWindowMonth LeaderboardWindow = "month"
)

// LeaderboardMetric 是 Metric（排名指标）：三项——Total Tokens、Successful Requests 与 Cost（消费金额）。
// Cost 是该窗口 actual_cost（实际计费金额，USD）之和；绝对金额与 tokens 同一套档位规则：
// named 档与 Preview 下下发，anonymous 档下只给相对第一名的百分比。
type LeaderboardMetric string

const (
	LeaderboardMetricTotalTokens        LeaderboardMetric = "total_tokens"
	LeaderboardMetricSuccessfulRequests LeaderboardMetric = "successful_requests"
	LeaderboardMetricCost               LeaderboardMetric = "cost"
)

// leaderboardCostMicrosPerUSD 是金额在 Snapshot 里的定点单位：1 USD = 1e6 micros。
// ZSET 分数与 Hash 段都是整数，金额用 micros 存，读出时再换回 USD。
const leaderboardCostMicrosPerUSD = 1_000_000

// LeaderboardCostMicros 把 USD 金额换成 micros（四舍五入）。
func LeaderboardCostMicros(usd float64) int64 {
	return int64(math.Round(usd * leaderboardCostMicrosPerUSD))
}

// LeaderboardCostUSD 把 micros 换回 USD。
func LeaderboardCostUSD(micros int64) float64 {
	return float64(micros) / leaderboardCostMicrosPerUSD
}

const (
	// LeaderboardTopEntryLimit 是 Leaderboard Entry（榜单条目）的固定上限，不提供长度选择器。
	LeaderboardTopEntryLimit = 50
	// LeaderboardSnapshotInterval 是 Snapshot（榜单快照）的重建周期（design D6）。
	LeaderboardSnapshotInterval = 5 * time.Minute
	// LeaderboardSnapshotStaleAfter 超过这个时长未推进即视为陈旧，响应仍给数据但带标记。
	LeaderboardSnapshotStaleAfter = 15 * time.Minute
	// LeaderboardSnapshotMaxTTL 是 Redis key 的硬上限：作业停摆时旧快照最多再服务一小时。
	LeaderboardSnapshotMaxTTL = 60 * time.Minute

	leaderboardSnapshotTimingWheelKey = "leaderboard:snapshot"
	// leaderboardSnapshotLeaderLockKey 保证多副本部署中每个周期只有一个实例执行聚合。
	leaderboardSnapshotLeaderLockKey = "leaderboard:snapshot:leader"
	// TTL 必须覆盖一次月窗口聚合的最坏耗时，避免任务中途失锁。
	leaderboardSnapshotLeaderLockTTL = 5 * time.Minute
	// 聚合跑在后台，即使慢也不阻塞任何请求；超时只是兜底。
	defaultLeaderboardSnapshotTimeout = 2 * time.Minute
)

// ParseLeaderboardWindow 归一化 window 查询参数，非法值返回 ok=false（由调用方给 400）。
func ParseLeaderboardWindow(raw string) (LeaderboardWindow, bool) {
	switch LeaderboardWindow(strings.ToLower(strings.TrimSpace(raw))) {
	case LeaderboardWindowToday:
		return LeaderboardWindowToday, true
	case LeaderboardWindowWeek:
		return LeaderboardWindowWeek, true
	case LeaderboardWindowMonth:
		return LeaderboardWindowMonth, true
	default:
		return "", false
	}
}

// ParseLeaderboardMetric 归一化 metric 查询参数，非法值返回 ok=false（由调用方给 400）。
func ParseLeaderboardMetric(raw string) (LeaderboardMetric, bool) {
	switch LeaderboardMetric(strings.ToLower(strings.TrimSpace(raw))) {
	case LeaderboardMetricTotalTokens:
		return LeaderboardMetricTotalTokens, true
	case LeaderboardMetricSuccessfulRequests:
		return LeaderboardMetricSuccessfulRequests, true
	case LeaderboardMetricCost:
		return LeaderboardMetricCost, true
	default:
		return "", false
	}
}

// LeaderboardWindowBounds 返回某个 Window 在站点时区下的半开区间 [start, end)。
// 边界用 timezone 包计算（周一起算），页面用 timezone.Name() 标注计算时区。
func LeaderboardWindowBounds(window LeaderboardWindow, now time.Time) (start, end time.Time, ok bool) {
	switch window {
	case LeaderboardWindowToday:
		start = timezone.StartOfDay(now)
		return start, start.AddDate(0, 0, 1), true
	case LeaderboardWindowWeek:
		start = timezone.StartOfWeek(now)
		return start, start.AddDate(0, 0, 7), true
	case LeaderboardWindowMonth:
		start = timezone.StartOfMonth(now)
		return start, start.AddDate(0, 1, 0), true
	default:
		return time.Time{}, time.Time{}, false
	}
}

// LeaderboardUserMetrics 是 Snapshot 里一个用户在某个 Window 下的数值。
// Snapshot 只记录 user_id 与数值，不记录身份。
//
// TotalTokens、SuccessfulRequests 与 CostMicros 是三个 Metric（排名指标），各进一个 ZSET；
// 其余只用来算 Cache Hit Rate（缓存命中率）、Extremes（之最）与 Token 构成，只进 Hash，
// MUST NOT 参与排名（design D17 / D20）。CostMicros 是该窗口 actual_cost 之和的定点值
// （1 USD = 1e6），占 Hash 的第 13 段。
//
// MediaRequests 本轮只存不展示：它占住 Hash 的第 10 段，但没有任何响应字段读它。
// YesterdayTokens / YesterdayRequests 与 Window 无关（三个窗口里都是同一个「昨日」），
// 只给今日窗口的进步之星当基线。
type LeaderboardUserMetrics struct {
	UserID              int64
	TotalTokens         int64
	SuccessfulRequests  int64
	InputTokens         int64
	CacheReadTokens     int64
	OutputTokens        int64
	CacheCreationTokens int64
	NightTokens         int64
	DistinctModels      int
	MaxSingleTokens     int64
	MediaRequests       int64
	YesterdayTokens     int64
	YesterdayRequests   int64
	CostMicros          int64
}

// Metric 返回该用户在指定 Metric 下的值（Cost 返回 micros）。
func (m LeaderboardUserMetrics) Metric(metric LeaderboardMetric) int64 {
	switch metric {
	case LeaderboardMetricSuccessfulRequests:
		return m.SuccessfulRequests
	case LeaderboardMetricCost:
		return m.CostMicros
	default:
		return m.TotalTokens
	}
}

// LeaderboardScoreEntry 是从某个 Window × Metric 的 ZSET 上倒序取到的一行。
type LeaderboardScoreEntry struct {
	UserID int64
	Score  int64
}

// LeaderboardSnapshotWindow 是一个 Window 的完整快照，用于整体替换（先写临时 key 再 RENAME）。
// Highlights 与榜单同一轮写入、同一批 RENAME、同一个 TTL，因此一次请求读到的两者必然同源；
// 为 nil 时该 Window 的 highlights key 被清掉，而不是留着上一轮的内容配这一轮的榜。
type LeaderboardSnapshotWindow struct {
	Window      LeaderboardWindow
	WindowStart time.Time
	WindowEnd   time.Time
	UpdatedAt   time.Time
	Entries     []LeaderboardUserMetrics
	Highlights  *LeaderboardHighlights
}

// LeaderboardCache 是 Snapshot 在 Redis 上的派生结构读写口，由 repository 层实现。
//
// 刻意不提供 Clear：design D8 明确不设「禁用 / 删除 / 切换模式时清空 Snapshot」的钩子——
// Snapshot 不含身份，切换模式不会泄露；下架由渲染时剔除即时生效；而清空会让全站在
// 下一轮重建前的最长 5 分钟里只看到「正在计算」。
type LeaderboardCache interface {
	// ReplaceSnapshot 用 pipeline 把一个 Window 的新快照写入临时 key，再 RENAME 原子切换，
	// 读者不会在任何时刻读到半份榜。写入失败时正式 key 保持上一轮内容。
	ReplaceSnapshot(ctx context.Context, snapshot LeaderboardSnapshotWindow) error
	// TopEntries 按分数倒序取前 limit 个成员（ZREVRANGE）。
	TopEntries(ctx context.Context, window LeaderboardWindow, windowStart time.Time, metric LeaderboardMetric, limit int) ([]LeaderboardScoreEntry, error)
	// ParticipantCount 取该 Window ZSET 的成员数（ZCARD）。
	ParticipantCount(ctx context.Context, window LeaderboardWindow, windowStart time.Time, metric LeaderboardMetric) (int64, error)
	// RankOf 用 ZCOUNT 统计分数严格高于该用户的成员数再加一（并列同名次，其后跳号）。
	// 该用户不在快照里（窗口内零用量）时返回 found=false。
	RankOf(ctx context.Context, window LeaderboardWindow, windowStart time.Time, metric LeaderboardMetric, userID int64) (rank int64, found bool, err error)
	// MetricsOf 从该 Window 的 Hash 批量读取这些 user_id 的全部数值（三个 Metric 与派生指标的分量）。
	MetricsOf(ctx context.Context, window LeaderboardWindow, windowStart time.Time, userIDs []int64) (map[int64]LeaderboardUserMetrics, error)
	// UpdatedAt 返回该 Window 的 Snapshot 更新时间；key 不存在时 exists=false（即「正在计算」）。
	UpdatedAt(ctx context.Context, window LeaderboardWindow, windowStart time.Time) (updatedAt time.Time, exists bool, err error)
	// Highlights 读该 Window 的趣味卡 JSON 串；缺失或损坏时返回 nil, nil（响应里按 null 处理）。
	Highlights(ctx context.Context, window LeaderboardWindow, windowStart time.Time) (*LeaderboardHighlights, error)
	// ReplaceInsights 整体替换站点级洞察，key 带今日窗口起点以便跨零点自然作废；
	// TTL 与 Snapshot 同一条规则（min(60 分钟, 距今日窗口结束)）。
	ReplaceInsights(ctx context.Context, todayStart time.Time, insights *LeaderboardInsights, todayEnd time.Time) error
	// Insights 读站点级洞察；缺失或损坏时返回 nil, nil。
	Insights(ctx context.Context, todayStart time.Time) (*LeaderboardInsights, error)
	// ViewerModels 读本人模型偏好的 60 秒缓存（design D21 那条例外聚合的结果）。
	// 未命中或内容损坏时 found=false，由调用方回源并调 SetViewerModels 回填。
	ViewerModels(ctx context.Context, userID int64, window LeaderboardWindow, windowStart time.Time) (rows []usagestats.LeaderboardModelUsageRow, total int64, found bool, err error)
	// SetViewerModels 回填本人模型偏好缓存，TTL 取 min(60 秒, 距窗口结束)。
	SetViewerModels(ctx context.Context, userID int64, window LeaderboardWindow, windowStart, windowEnd time.Time, rows []usagestats.LeaderboardModelUsageRow, total int64) error
}

// LeaderboardAggregateRepository 是 Snapshot 与 Insights 重建所需的仓储能力。
// usageLogRepository 已实现这些方法，因此 UsageLogRepository 天然满足这个窄接口。
//
// 这几条全部只在后台作业里执行：请求路径只读 Redis，MUST NOT 回源查 usage_logs（design D8）。
type LeaderboardAggregateRepository interface {
	AggregateLeaderboardWindows(ctx context.Context, todayStart, weekStart, monthStart time.Time) ([]usagestats.LeaderboardAggregateRow, error)
	// LeaderboardTopModelsToday 返回今日 Top N 模型与今日全站成功请求总数（share_percent 的分母）。
	LeaderboardTopModelsToday(ctx context.Context, todayStart, todayEnd time.Time, limit int) ([]usagestats.LeaderboardModelUsageRow, int64, error)
	// LeaderboardDominantModel 返回某用户某窗口成功请求最多的模型，只对效率之星一个人查一次。
	LeaderboardDominantModel(ctx context.Context, userID int64, start, end time.Time) (string, error)
	// LeaderboardDailyBuckets 读 usage_dashboard_daily 的 [fromDate, toDate) 日桶，缺行时返回空切片。
	LeaderboardDailyBuckets(ctx context.Context, fromDate, toDate time.Time) ([]usagestats.LeaderboardDailyBucketRow, error)
	// LeaderboardHourlyBuckets 读 usage_dashboard_hourly 的 [from, to) 小时桶，缺行时返回空切片。
	LeaderboardHourlyBuckets(ctx context.Context, from, to time.Time) ([]usagestats.LeaderboardHourlyBucketRow, error)
	// LeaderboardTopStreak 读 usage_dashboard_daily_users 的近 90 天切片，返回连续活跃最长的那个人。
	LeaderboardTopStreak(ctx context.Context, fromDate, toDate time.Time) (usagestats.LeaderboardStreakRow, error)
	// LeaderboardUserModelBreakdown 一条 SQL 出指定这一小撮 user_id 在窗口内的全部模型用量（profiles 用）。
	LeaderboardUserModelBreakdown(ctx context.Context, userIDs []int64, start, end time.Time) ([]usagestats.LeaderboardUserModelUsageRow, error)
	// LeaderboardPlatformsToday 按账号平台聚合今日成功请求，另给今日全站成功请求总数（占比的分母）。
	LeaderboardPlatformsToday(ctx context.Context, todayStart, todayEnd time.Time) ([]usagestats.LeaderboardPlatformUsageRow, int64, error)
	// LeaderboardWeekdayHourBuckets 读 usage_dashboard_hourly 并按 (周几, 小时) 求平均，缺行时返回空切片。
	LeaderboardWeekdayHourBuckets(ctx context.Context, from, to time.Time) ([]usagestats.LeaderboardWeekdayHourRow, error)
	// UpsertLeaderboardRankHistory 覆盖写一轮的名次历史（同一天同一人只有一行）。
	UpsertLeaderboardRankHistory(ctx context.Context, rows []usagestats.LeaderboardRankHistoryRow) error
	// DeleteLeaderboardRankHistoryBefore 删除保留期之前的名次历史，返回删除行数。
	DeleteLeaderboardRankHistoryBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

// LeaderboardSnapshotService 负责每 5 分钟重建一次 Snapshot：
// 一条 SQL 出三个窗口，写 Redis 派生结构；请求路径只读，永不触发聚合。
type LeaderboardSnapshotService struct {
	repo        LeaderboardAggregateRepository
	cache       LeaderboardCache
	timingWheel *TimingWheelService

	running int32

	lockCache  LeaderLockCache
	db         *sql.DB
	instanceID string
}

// NewLeaderboardSnapshotService 创建 Snapshot 重建服务。
func NewLeaderboardSnapshotService(repo LeaderboardAggregateRepository, cache LeaderboardCache, timingWheel *TimingWheelService) *LeaderboardSnapshotService {
	return &LeaderboardSnapshotService{
		repo:        repo,
		cache:       cache,
		timingWheel: timingWheel,
		instanceID:  uuid.NewString(),
	}
}

// SetLeaderLock 注入选主用的缓存与数据库。两者都为 nil 时作业不做选主直接跑
// （单实例部署 / 测试行为），与 DashboardAggregationService 的语义一致。
func (s *LeaderboardSnapshotService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

// Start 注册每 5 分钟一轮的重建作业。
// 首轮在一个周期之后触发：首次开启 Leaderboard Mode 后最长等待 5 分钟 Snapshot 才就绪，
// 期间页面显示「正在计算」（见 design 的 Migration Plan 第 3 步）。
func (s *LeaderboardSnapshotService) Start() {
	if s == nil || s.repo == nil || s.cache == nil || s.timingWheel == nil {
		return
	}
	s.timingWheel.ScheduleRecurring(leaderboardSnapshotTimingWheelKey, LeaderboardSnapshotInterval, func() {
		s.runScheduledRebuild()
	})
	logger.LegacyPrintf("service.leaderboard_snapshot", "[LeaderboardSnapshot] 快照作业启动 (interval=%v)", LeaderboardSnapshotInterval)
}

// Stop 取消定时作业（进程退出时调用）。
func (s *LeaderboardSnapshotService) Stop() {
	if s == nil || s.timingWheel == nil {
		return
	}
	s.timingWheel.Cancel(leaderboardSnapshotTimingWheelKey)
}

func (s *LeaderboardSnapshotService) runScheduledRebuild() {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&s.running, 0)

	ctx, cancel := context.WithTimeout(context.Background(), defaultLeaderboardSnapshotTimeout)
	defer cancel()

	// 多实例护栏：只有 leader 执行聚合，其余实例直接跳过本轮，
	// 避免 N 倍的月窗口整表扫描与互相覆盖的 Redis 写入。
	release, ok := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, leaderboardSnapshotLeaderLockKey, s.instanceID, leaderboardSnapshotLeaderLockTTL)
	if !ok {
		return
	}
	defer release()

	if err := s.Rebuild(ctx, timezone.Now()); err != nil {
		logger.LegacyPrintf("service.leaderboard_snapshot", "[LeaderboardSnapshot] 快照重建失败: %v", err)
	}
}

// Rebuild 执行一轮重建：一条 SQL 出三个窗口，算好每个窗口的 Highlights（趣味卡）与一份
// 站点级 Insights（洞察），再整体写进 Redis。
// 导出是为了让运维脚本与测试可以直接驱动一轮，而不必等定时器。
//
// 顺序上 Insights 的**计算**排在三个窗口之前、**写入**排在三个窗口之后：全站概况卡的峰值
// 时段取自今日小时桶，三个窗口都要用它；而写入放到最后，前面任何一步失败时正式 key
// 都还是上一轮的完整内容。
func (s *LeaderboardSnapshotService) Rebuild(ctx context.Context, now time.Time) error {
	if s == nil || s.repo == nil || s.cache == nil {
		return errors.New("排行榜快照服务未初始化")
	}

	todayStart, todayEnd, _ := LeaderboardWindowBounds(LeaderboardWindowToday, now)
	weekStart, weekEnd, _ := LeaderboardWindowBounds(LeaderboardWindowWeek, now)
	monthStart, monthEnd, _ := LeaderboardWindowBounds(LeaderboardWindowMonth, now)

	rows, err := s.repo.AggregateLeaderboardWindows(ctx, todayStart, weekStart, monthStart)
	if err != nil {
		return err
	}

	// Insights 先算后写：全站概况卡的峰值时段取自今日小时桶，三个窗口的 Highlights 都要用它。
	// 这里返回 error 的只剩「今日模型热度」那一条（它与榜单同源，扫的是 usage_logs）：
	// 出错说明本轮的数据源本身不可信，整轮不切换——正式 key 保持上一轮的完整内容，
	// 更新时间也不推进。预聚合表读不动只降级成对应区块 nil，不牵连榜单本体（design D17）。
	insights, err := s.computeLeaderboardInsights(ctx, todayStart, todayEnd, monthStart, monthEnd)
	if err != nil {
		return err
	}
	peakHour := leaderboardPeakHour(insights.HourlyToday)

	// 连续活跃与 Window 无关，只查一次，三个窗口共用同一份结果（design D20）。
	// 读不动只降级成「没有这张卡」：它来自预聚合的日活表，与榜单本体无关。
	streak := s.readLeaderboardTopStreak(ctx, todayStart)

	updatedAt := now
	var firstErr error
	var todayEntries []LeaderboardUserMetrics
	for _, spec := range []struct {
		window LeaderboardWindow
		start  time.Time
		end    time.Time
		pick   func(usagestats.LeaderboardAggregateRow) LeaderboardUserMetrics
	}{
		{LeaderboardWindowToday, todayStart, todayEnd, func(r usagestats.LeaderboardAggregateRow) LeaderboardUserMetrics {
			return LeaderboardUserMetrics{
				UserID: r.UserID, TotalTokens: r.TodayTokens, SuccessfulRequests: r.TodayRequests,
				InputTokens: r.TodayInputTokens, CacheReadTokens: r.TodayCacheReadTokens,
				OutputTokens: r.TodayOutputTokens, CacheCreationTokens: r.TodayCacheCreationTokens,
				NightTokens: r.TodayNightTokens, DistinctModels: r.TodayDistinctModels,
				MaxSingleTokens: r.TodayMaxSingleTokens, MediaRequests: r.TodayMediaRequests,
				YesterdayTokens: r.YesterdayTokens, YesterdayRequests: r.YesterdayRequests,
				// 金额在 Snapshot 内部一律是定点 micros：ZSET 分数、Hash 段与名次都用它算，
				// 只在响应组装时才换回 USD。
				CostMicros: LeaderboardCostMicros(r.TodayCost),
			}
		}},
		{LeaderboardWindowWeek, weekStart, weekEnd, func(r usagestats.LeaderboardAggregateRow) LeaderboardUserMetrics {
			return LeaderboardUserMetrics{
				UserID: r.UserID, TotalTokens: r.WeekTokens, SuccessfulRequests: r.WeekRequests,
				InputTokens: r.WeekInputTokens, CacheReadTokens: r.WeekCacheReadTokens,
				OutputTokens: r.WeekOutputTokens, CacheCreationTokens: r.WeekCacheCreationTokens,
				NightTokens: r.WeekNightTokens, DistinctModels: r.WeekDistinctModels,
				MaxSingleTokens: r.WeekMaxSingleTokens, MediaRequests: r.WeekMediaRequests,
				YesterdayTokens: r.YesterdayTokens, YesterdayRequests: r.YesterdayRequests,
				CostMicros: LeaderboardCostMicros(r.WeekCost),
			}
		}},
		{LeaderboardWindowMonth, monthStart, monthEnd, func(r usagestats.LeaderboardAggregateRow) LeaderboardUserMetrics {
			return LeaderboardUserMetrics{
				UserID: r.UserID, TotalTokens: r.MonthTokens, SuccessfulRequests: r.MonthRequests,
				InputTokens: r.MonthInputTokens, CacheReadTokens: r.MonthCacheReadTokens,
				OutputTokens: r.MonthOutputTokens, CacheCreationTokens: r.MonthCacheCreationTokens,
				NightTokens: r.MonthNightTokens, DistinctModels: r.MonthDistinctModels,
				MaxSingleTokens: r.MonthMaxSingleTokens, MediaRequests: r.MonthMediaRequests,
				YesterdayTokens: r.YesterdayTokens, YesterdayRequests: r.YesterdayRequests,
				CostMicros: LeaderboardCostMicros(r.MonthCost),
			}
		}},
	} {
		entries := make([]LeaderboardUserMetrics, 0, len(rows))
		for _, row := range rows {
			m := spec.pick(row)
			// 窗口内没有任何用量的用户不进该窗口的 Snapshot，
			// 因此 ZCARD 与 Participant Count 的口径一致（spec: 今日窗口的条件聚合）。
			// 判定只看 Total Tokens 与 Successful Requests 两项：input / cache_read 已经含在
			// Total Tokens 里，不会出现「Total Tokens 为 0 却有缓存读取」的行；
			// Cost 同理——成功落账口径本身就是 actual_cost > 0，因此 CostMicros 为正的行
			// MUST 伴随至少一次成功请求，不必单独判一次。
			// 新增的十段同理：它们要么含在 Total Tokens 里，要么是「昨日」的基线，
			// 都不该让一个本窗口零用量的人进榜。
			if m.TotalTokens <= 0 && m.SuccessfulRequests <= 0 {
				continue
			}
			entries = append(entries, m)
		}
		if spec.window == LeaderboardWindowToday {
			// 名次历史只记今日窗口，留到三个窗口都写完之后再做，见下面的 upsert。
			todayEntries = entries
			insights.CompositionToday = leaderboardCompositionFromEntries(entries)
		}

		highlights := computeLeaderboardHighlights(entries, peakHour)
		if highlights != nil && highlights.CacheKing != nil {
			// 只对效率之星这一个用户查一次 dominant model；查不动就整段不切换，
			// 宁可让这个 Window 继续服务上一轮的完整快照，也不给出半份 Highlights。
			model, modelErr := s.repo.LeaderboardDominantModel(ctx, highlights.CacheKing.UserID, spec.start, spec.end)
			if modelErr != nil {
				if firstErr == nil {
					firstErr = modelErr
				}
				continue
			}
			highlights.CacheKing.DominantModel = model
		}
		if highlights != nil {
			// 之最与画像都是按 Window 算的，因此跟着这一份 Highlights 走同一批 RENAME。
			highlights.Extremes = computeLeaderboardExtremes(entries, spec.window, streak)
			highlights.Profiles = s.computeLeaderboardProfiles(ctx, spec.window, entries, spec.start, spec.end)
		}

		if err := s.cache.ReplaceSnapshot(ctx, LeaderboardSnapshotWindow{
			Window:      spec.window,
			WindowStart: spec.start,
			WindowEnd:   spec.end,
			UpdatedAt:   updatedAt,
			Entries:     entries,
			Highlights:  highlights,
		}); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if err := s.cache.ReplaceInsights(ctx, todayStart, insights, todayEnd); err != nil && firstErr == nil {
		firstErr = err
	}

	// 名次历史放在三个窗口都写完之后、同一把 leader lock 之内：
	// 它是 Postgres 上的旁路留痕，写不动 MUST NOT 让整轮快照重建失败（design D21）。
	s.maintainLeaderboardRankHistory(ctx, todayStart, todayEntries)
	return firstErr
}

// leaderboardCompositionFromEntries 由今日窗口的全部参与者汇总出 Token 构成。
// 四段来自同一轮聚合的站点合计，MUST NOT 另查一次（design D22）。
func leaderboardCompositionFromEntries(entries []LeaderboardUserMetrics) *LeaderboardCompositionInsight {
	var input, output, cacheCreation, cacheRead int64
	for _, entry := range entries {
		input += entry.InputTokens
		output += entry.OutputTokens
		cacheCreation += entry.CacheCreationTokens
		cacheRead += entry.CacheReadTokens
	}
	return buildLeaderboardCompositionInsight(input, output, cacheCreation, cacheRead)
}

// readLeaderboardTopStreak 读连续活跃之最；出错时记日志并返回 nil，由上层表现为「没有这张卡」。
func (s *LeaderboardSnapshotService) readLeaderboardTopStreak(ctx context.Context, todayStart time.Time) *usagestats.LeaderboardStreakRow {
	from, to := leaderboardStreakRange(todayStart)
	row, err := s.repo.LeaderboardTopStreak(ctx, from, to)
	if err != nil {
		logger.LegacyPrintf("service.leaderboard_snapshot",
			"[LeaderboardSnapshot] 连续活跃之最读取失败，该项降级为空 (from=%s, to=%s): %v",
			from.Format(time.RFC3339), to.Format(time.RFC3339), err)
		return nil
	}
	return &row
}

// computeLeaderboardProfiles 算该 Window 的模型偏好画像：Total Tokens 前 50 名各自用过的全部模型及占比。
// 一条限定这 50 个 user_id 的聚合出全部结果，MUST NOT 逐人各查一次（design D22）。
// 出错时记日志并返回 nil，该区块降级为不可用，本轮快照照常写入。
func (s *LeaderboardSnapshotService) computeLeaderboardProfiles(ctx context.Context, window LeaderboardWindow, entries []LeaderboardUserMetrics, start, end time.Time) []LeaderboardProfile {
	userIDs := leaderboardTopUserIDsByTokens(entries, leaderboardProfilesLimit)
	if len(userIDs) == 0 {
		return nil
	}
	rows, err := s.repo.LeaderboardUserModelBreakdown(ctx, userIDs, start, end)
	if err != nil {
		logger.LegacyPrintf("service.leaderboard_snapshot",
			"[LeaderboardSnapshot] 模型偏好画像聚合失败，该区块降级为空 (window=%s): %v", window, err)
		return nil
	}
	return buildLeaderboardProfiles(userIDs, rows)
}

// maintainLeaderboardRankHistory 对今日窗口的全部参与者写一轮名次历史，再清理保留期之前的行。
//
// 名次的算法与响应侧的 Rank 完全一致：严格高于自己的人数加一（并列同名次，其后跳号），
// 因此折线上的点与用户当天在页面上看到的名次对得上。两步任一失败只记日志——
// 名次历史是旁路留痕，MUST NOT 让整轮快照重建失败（design D21）。
func (s *LeaderboardSnapshotService) maintainLeaderboardRankHistory(ctx context.Context, todayStart time.Time, entries []LeaderboardUserMetrics) {
	if len(entries) > 0 {
		rows := buildLeaderboardRankHistoryRows(todayStart, entries)
		if err := s.repo.UpsertLeaderboardRankHistory(ctx, rows); err != nil {
			logger.LegacyPrintf("service.leaderboard_snapshot",
				"[LeaderboardSnapshot] 名次历史写入失败，本轮快照不受影响 (date=%s, rows=%d): %v",
				todayStart.Format(leaderboardInsightDateLayout), len(rows), err)
		}
	}

	cutoff := todayStart.AddDate(0, 0, -leaderboardRankHistoryRetentionDays)
	if _, err := s.repo.DeleteLeaderboardRankHistoryBefore(ctx, cutoff); err != nil {
		logger.LegacyPrintf("service.leaderboard_snapshot",
			"[LeaderboardSnapshot] 名次历史清理失败，本轮快照不受影响 (cutoff=%s): %v",
			cutoff.Format(leaderboardInsightDateLayout), err)
	}
}

// buildLeaderboardRankHistoryRows 把今日窗口的参与者按三个 Metric 各排一次，
// 组装成当天的名次历史行。名次是竞争排名：分数严格高于自己的人数加一。
// Cost 的名次按 micros 算——与 ZSET 分数同一个定点单位，两处因此永不互相矛盾。
func buildLeaderboardRankHistoryRows(todayStart time.Time, entries []LeaderboardUserMetrics) []usagestats.LeaderboardRankHistoryRow {
	tokenRanks := leaderboardCompetitiveRanks(entries, LeaderboardMetricTotalTokens)
	requestRanks := leaderboardCompetitiveRanks(entries, LeaderboardMetricSuccessfulRequests)
	costRanks := leaderboardCompetitiveRanks(entries, LeaderboardMetricCost)
	rows := make([]usagestats.LeaderboardRankHistoryRow, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, usagestats.LeaderboardRankHistoryRow{
			UserID:                 entry.UserID,
			SnapshotDate:           todayStart,
			RankTotalTokens:        tokenRanks[entry.UserID],
			RankSuccessfulRequests: requestRanks[entry.UserID],
			RankCost:               costRanks[entry.UserID],
		})
	}
	return rows
}

// leaderboardCompetitiveRanks 按某个 Metric 算出每个人的竞争名次：
// 同分的人共享同一个名次，其后跳号（1、1、3），与 LeaderboardCache.RankOf 的
// 「ZCOUNT 严格高于者数 + 1」逐条等价。
func leaderboardCompetitiveRanks(entries []LeaderboardUserMetrics, metric LeaderboardMetric) map[int64]int {
	ordered := make([]LeaderboardUserMetrics, len(entries))
	copy(ordered, entries)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Metric(metric) > ordered[j].Metric(metric)
	})
	ranks := make(map[int64]int, len(ordered))
	var previousScore int64
	previousRank := 0
	for i, entry := range ordered {
		score := entry.Metric(metric)
		if i == 0 || score != previousScore {
			previousRank = i + 1
			previousScore = score
		}
		ranks[entry.UserID] = previousRank
	}
	return ranks
}

// computeLeaderboardInsights 算一份与 Window 无关的站点级洞察。
//
// 本轮之后共八个区块（v1 的五块 + v2 的 platforms_today / weekly_rhythm / cache_trend_14；
// composition_today 由今日窗口的站点合计导出，在 Rebuild 里补上，不在这里读库）。
// 各区块有各自的来源，缺行、读不动与「数据源本身不可信」是三回事：
//   - 预聚合表缺行（作业未启用 / 保留期没覆盖）→ 对应区块为 nil，其余区块照常，
//     MUST NOT 用 0 填充，也 MUST NOT 回落到实时扫描 usage_logs；
//   - 预聚合表读取出错 → 与缺行同等对待：对应区块为 nil 并记一条日志。这两张表是
//     仪表盘的派生物，它们读不动不说明榜单数据有问题，没有理由把一整轮重建连坐掉——
//     页面上少一块洞察，好过全站在下一轮之前只能看着一份越来越旧的榜；
//   - models_today 出错 → 返回 error 中止整轮：它与榜单同源（直接扫 usage_logs），
//     扫不动意味着本轮的数据源本身不可信，此时宁可整轮不切换。
//
// models_today 不依赖预聚合表，因此关掉预聚合也照常有值。
func (s *LeaderboardSnapshotService) computeLeaderboardInsights(ctx context.Context, todayStart, todayEnd, monthStart, monthEnd time.Time) (*LeaderboardInsights, error) {
	insights := &LeaderboardInsights{}

	models, totalRequests, err := s.repo.LeaderboardTopModelsToday(ctx, todayStart, todayEnd, leaderboardTopModelsLimit)
	if err != nil {
		return nil, err
	}
	insights.ModelsToday = buildLeaderboardModelInsights(models, totalRequests)

	dailyFrom, dailyTo := leaderboardDailyInsightRange(todayStart)
	if daily, ok := s.readLeaderboardDailyBuckets(ctx, "daily_30", dailyFrom, dailyTo); ok {
		insights.Daily30 = buildLeaderboardDailyInsights(daily)
		// 近 14 天是近 30 天的子集，因此命中率趋势从同一批日桶里切出来，不另读一次。
		insights.CacheTrend14 = buildLeaderboardCacheTrendInsights(daily, leaderboardCacheTrendRange(todayStart))
	}

	if hourly, ok := s.readLeaderboardHourlyBuckets(ctx, todayStart, todayEnd); ok {
		insights.HourlyToday = buildLeaderboardHourlyInsights(hourly)
		insights.CacheToday = buildLeaderboardCacheInsight(hourly)
	}

	// 平台分布直接扫今日 usage_logs（JOIN accounts），不依赖预聚合表；
	// 读不动只降级本块：它与榜单本体不同源，没有理由把一整轮重建连坐掉。
	if platforms, total, err := s.repo.LeaderboardPlatformsToday(ctx, todayStart, todayEnd); err != nil {
		logger.LegacyPrintf("service.leaderboard_snapshot",
			"[LeaderboardSnapshot] 今日平台分布聚合失败，该区块降级为空: %v", err)
	} else {
		insights.PlatformsToday = buildLeaderboardPlatformInsights(platforms, total)
	}

	// 周内节奏读近 28 天（不含今日）的小时桶，缺行时整块为 null；
	// 个别 (周几, 小时) 缺行是「那个时段没有请求」，等级为 0，不是缺数据。
	rhythmFrom, rhythmTo := leaderboardRhythmRange(todayStart)
	if rhythm, err := s.repo.LeaderboardWeekdayHourBuckets(ctx, rhythmFrom, rhythmTo); err != nil {
		logger.LegacyPrintf("service.leaderboard_snapshot",
			"[LeaderboardSnapshot] 周内节奏读取失败，该区块降级为空 (from=%s, to=%s): %v",
			rhythmFrom.Format(time.RFC3339), rhythmTo.Format(time.RFC3339), err)
	} else {
		insights.WeeklyRhythm = buildLeaderboardWeeklyRhythm(rhythm)
	}

	// 本月与上月各求一次和：上月的绝对量只用来算「较上月」，不进下发结构。
	// 两段缺一不可——只有本月读得到时算不出「较上月」，整块一起降级为 nil。
	currentMonth, currentOK := s.readLeaderboardDailyBuckets(ctx, "month", monthStart, monthEnd)
	previousMonth, previousOK := s.readLeaderboardDailyBuckets(ctx, "previous_month", monthStart.AddDate(0, -1, 0), monthStart)
	if currentOK && previousOK {
		insights.Month = buildLeaderboardMonthInsight(currentMonth, previousMonth)
	}

	return insights, nil
}

// readLeaderboardDailyBuckets 读 usage_dashboard_daily 的一段日桶；出错时记日志并返回 ok=false，
// 由调用方把对应区块降级为 nil。block 只用于日志定位是哪一段区间。
func (s *LeaderboardSnapshotService) readLeaderboardDailyBuckets(ctx context.Context, block string, from, to time.Time) ([]usagestats.LeaderboardDailyBucketRow, bool) {
	rows, err := s.repo.LeaderboardDailyBuckets(ctx, from, to)
	if err != nil {
		logger.LegacyPrintf("service.leaderboard_snapshot",
			"[LeaderboardSnapshot] 预聚合日桶读取失败，该区块降级为空 (block=%s, from=%s, to=%s): %v",
			block, from.Format(time.RFC3339), to.Format(time.RFC3339), err)
		return nil, false
	}
	return rows, true
}

// readLeaderboardHourlyBuckets 读 usage_dashboard_hourly 的今日小时桶，降级规则同上。
func (s *LeaderboardSnapshotService) readLeaderboardHourlyBuckets(ctx context.Context, from, to time.Time) ([]usagestats.LeaderboardHourlyBucketRow, bool) {
	rows, err := s.repo.LeaderboardHourlyBuckets(ctx, from, to)
	if err != nil {
		logger.LegacyPrintf("service.leaderboard_snapshot",
			"[LeaderboardSnapshot] 预聚合小时桶读取失败，今日时段与缓存两块降级为空 (from=%s, to=%s): %v",
			from.Format(time.RFC3339), to.Format(time.RFC3339), err)
		return nil, false
	}
	return rows, true
}
