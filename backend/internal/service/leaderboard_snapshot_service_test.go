//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type stubLeaderboardAggregateRepo struct {
	rows []usagestats.LeaderboardAggregateRow
	err  error

	calls      int
	todayStart time.Time
	weekStart  time.Time
	monthStart time.Time

	// 今日模型热度
	models      []usagestats.LeaderboardModelUsageRow
	modelsTotal int64
	modelsErr   error
	modelsCalls []leaderboardRangeCall
	modelsLimit int

	// 效率之星的 dominant model：记录每一次调用，用于断言「只对一个用户查一次」
	dominantModel string
	dominantErr   error
	dominantCalls []leaderboardDominantCall

	// 预聚合表
	daily      []usagestats.LeaderboardDailyBucketRow
	dailyFn    func(from, to time.Time) []usagestats.LeaderboardDailyBucketRow
	dailyErr   error
	dailyCalls []leaderboardRangeCall

	hourly      []usagestats.LeaderboardHourlyBucketRow
	hourlyErr   error
	hourlyCalls []leaderboardRangeCall

	// v2：连续活跃、模型偏好画像、平台分布、周内节奏
	streak      usagestats.LeaderboardStreakRow
	streakErr   error
	streakCalls []leaderboardRangeCall

	userModels      []usagestats.LeaderboardUserModelUsageRow
	userModelsErr   error
	userModelCalls  []leaderboardUserModelCall
	platforms       []usagestats.LeaderboardPlatformUsageRow
	platformsTotal  int64
	platformsErr    error
	platformsCalls  []leaderboardRangeCall
	rhythm          []usagestats.LeaderboardWeekdayHourRow
	rhythmErr       error
	rhythmCalls     []leaderboardRangeCall
	rankHistory     [][]usagestats.LeaderboardRankHistoryRow
	rankHistoryErr  error
	rankCutoffs     []time.Time
	rankCleanupErr  error
	rankCleanupHits int
}

type leaderboardUserModelCall struct {
	userIDs []int64
	start   time.Time
	end     time.Time
}

func (r *stubLeaderboardAggregateRepo) LeaderboardTopStreak(_ context.Context, fromDate, toDate time.Time) (usagestats.LeaderboardStreakRow, error) {
	r.streakCalls = append(r.streakCalls, leaderboardRangeCall{fromDate, toDate})
	if r.streakErr != nil {
		return usagestats.LeaderboardStreakRow{}, r.streakErr
	}
	return r.streak, nil
}

func (r *stubLeaderboardAggregateRepo) LeaderboardUserModelBreakdown(_ context.Context, userIDs []int64, start, end time.Time) ([]usagestats.LeaderboardUserModelUsageRow, error) {
	r.userModelCalls = append(r.userModelCalls, leaderboardUserModelCall{append([]int64(nil), userIDs...), start, end})
	if r.userModelsErr != nil {
		return nil, r.userModelsErr
	}
	return r.userModels, nil
}

func (r *stubLeaderboardAggregateRepo) LeaderboardPlatformsToday(_ context.Context, todayStart, todayEnd time.Time) ([]usagestats.LeaderboardPlatformUsageRow, int64, error) {
	r.platformsCalls = append(r.platformsCalls, leaderboardRangeCall{todayStart, todayEnd})
	if r.platformsErr != nil {
		return nil, 0, r.platformsErr
	}
	return r.platforms, r.platformsTotal, nil
}

func (r *stubLeaderboardAggregateRepo) LeaderboardWeekdayHourBuckets(_ context.Context, from, to time.Time) ([]usagestats.LeaderboardWeekdayHourRow, error) {
	r.rhythmCalls = append(r.rhythmCalls, leaderboardRangeCall{from, to})
	if r.rhythmErr != nil {
		return nil, r.rhythmErr
	}
	return r.rhythm, nil
}

func (r *stubLeaderboardAggregateRepo) UpsertLeaderboardRankHistory(_ context.Context, rows []usagestats.LeaderboardRankHistoryRow) error {
	r.rankHistory = append(r.rankHistory, append([]usagestats.LeaderboardRankHistoryRow(nil), rows...))
	return r.rankHistoryErr
}

func (r *stubLeaderboardAggregateRepo) DeleteLeaderboardRankHistoryBefore(_ context.Context, cutoff time.Time) (int64, error) {
	r.rankCleanupHits++
	r.rankCutoffs = append(r.rankCutoffs, cutoff)
	return 0, r.rankCleanupErr
}

type leaderboardRangeCall struct {
	from time.Time
	to   time.Time
}

type leaderboardDominantCall struct {
	userID int64
	start  time.Time
	end    time.Time
}

func (r *stubLeaderboardAggregateRepo) AggregateLeaderboardWindows(_ context.Context, todayStart, weekStart, monthStart time.Time) ([]usagestats.LeaderboardAggregateRow, error) {
	r.calls++
	r.todayStart, r.weekStart, r.monthStart = todayStart, weekStart, monthStart
	if r.err != nil {
		return nil, r.err
	}
	return r.rows, nil
}

func (r *stubLeaderboardAggregateRepo) LeaderboardTopModelsToday(_ context.Context, todayStart, todayEnd time.Time, limit int) ([]usagestats.LeaderboardModelUsageRow, int64, error) {
	r.modelsCalls = append(r.modelsCalls, leaderboardRangeCall{todayStart, todayEnd})
	r.modelsLimit = limit
	if r.modelsErr != nil {
		return nil, 0, r.modelsErr
	}
	return r.models, r.modelsTotal, nil
}

func (r *stubLeaderboardAggregateRepo) LeaderboardDominantModel(_ context.Context, userID int64, start, end time.Time) (string, error) {
	r.dominantCalls = append(r.dominantCalls, leaderboardDominantCall{userID, start, end})
	if r.dominantErr != nil {
		return "", r.dominantErr
	}
	return r.dominantModel, nil
}

func (r *stubLeaderboardAggregateRepo) LeaderboardDailyBuckets(_ context.Context, fromDate, toDate time.Time) ([]usagestats.LeaderboardDailyBucketRow, error) {
	r.dailyCalls = append(r.dailyCalls, leaderboardRangeCall{fromDate, toDate})
	if r.dailyErr != nil {
		return nil, r.dailyErr
	}
	if r.dailyFn != nil {
		return r.dailyFn(fromDate, toDate), nil
	}
	return r.daily, nil
}

func (r *stubLeaderboardAggregateRepo) LeaderboardHourlyBuckets(_ context.Context, from, to time.Time) ([]usagestats.LeaderboardHourlyBucketRow, error) {
	r.hourlyCalls = append(r.hourlyCalls, leaderboardRangeCall{from, to})
	if r.hourlyErr != nil {
		return nil, r.hourlyErr
	}
	return r.hourly, nil
}

type stubLeaderboardCache struct {
	written []LeaderboardSnapshotWindow
	err     error

	insights      []*LeaderboardInsights
	insightsStart []time.Time
	insightsErr   error
}

func (c *stubLeaderboardCache) ReplaceSnapshot(_ context.Context, snapshot LeaderboardSnapshotWindow) error {
	c.written = append(c.written, snapshot)
	return c.err
}

func (c *stubLeaderboardCache) TopEntries(context.Context, LeaderboardWindow, time.Time, LeaderboardMetric, int) ([]LeaderboardScoreEntry, error) {
	return nil, nil
}

func (c *stubLeaderboardCache) ParticipantCount(context.Context, LeaderboardWindow, time.Time, LeaderboardMetric) (int64, error) {
	return 0, nil
}

func (c *stubLeaderboardCache) RankOf(context.Context, LeaderboardWindow, time.Time, LeaderboardMetric, int64) (int64, bool, error) {
	return 0, false, nil
}

func (c *stubLeaderboardCache) MetricsOf(context.Context, LeaderboardWindow, time.Time, []int64) (map[int64]LeaderboardUserMetrics, error) {
	return nil, nil
}

func (c *stubLeaderboardCache) UpdatedAt(context.Context, LeaderboardWindow, time.Time) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func (c *stubLeaderboardCache) Highlights(context.Context, LeaderboardWindow, time.Time) (*LeaderboardHighlights, error) {
	return nil, nil
}

func (c *stubLeaderboardCache) ReplaceInsights(_ context.Context, todayStart time.Time, insights *LeaderboardInsights, _ time.Time) error {
	c.insights = append(c.insights, insights)
	c.insightsStart = append(c.insightsStart, todayStart)
	return c.insightsErr
}

func (c *stubLeaderboardCache) Insights(context.Context, time.Time) (*LeaderboardInsights, error) {
	return nil, nil
}

func (c *stubLeaderboardCache) ViewerModels(context.Context, int64, LeaderboardWindow, time.Time) ([]usagestats.LeaderboardModelUsageRow, int64, bool, error) {
	return nil, 0, false, nil
}

func (c *stubLeaderboardCache) SetViewerModels(context.Context, int64, LeaderboardWindow, time.Time, time.Time, []usagestats.LeaderboardModelUsageRow, int64) error {
	return nil
}

func (c *stubLeaderboardCache) byWindow(t *testing.T, window LeaderboardWindow) LeaderboardSnapshotWindow {
	t.Helper()
	for _, snapshot := range c.written {
		if snapshot.Window == window {
			return snapshot
		}
	}
	t.Fatalf("窗口 %s 未被写入", window)
	return LeaderboardSnapshotWindow{}
}

func TestParseLeaderboardWindowAndMetric(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want LeaderboardWindow
		ok   bool
	}{
		{"today", LeaderboardWindowToday, true},
		{" WEEK ", LeaderboardWindowWeek, true},
		{"month", LeaderboardWindowMonth, true},
		{"", "", false},
		{"all", "", false},
		{"yesterday", "", false},
	} {
		got, ok := ParseLeaderboardWindow(tc.raw)
		require.Equal(t, tc.ok, ok, "raw=%q", tc.raw)
		require.Equal(t, tc.want, got, "raw=%q", tc.raw)
	}

	for _, tc := range []struct {
		raw  string
		want LeaderboardMetric
		ok   bool
	}{
		{"total_tokens", LeaderboardMetricTotalTokens, true},
		{" Successful_Requests ", LeaderboardMetricSuccessfulRequests, true},
		{"", "", false},
		// 金额连排序选项都不提供。
		{"actual_cost", "", false},
		{"cost", "", false},
	} {
		got, ok := ParseLeaderboardMetric(tc.raw)
		require.Equal(t, tc.ok, ok, "raw=%q", tc.raw)
		require.Equal(t, tc.want, got, "raw=%q", tc.raw)
	}
}

func TestLeaderboardWindowBounds(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	// 2026-03-18 是周三，所属周的周一是 2026-03-16。
	now := time.Date(2026, 3, 18, 15, 30, 45, 0, time.UTC)

	start, end, ok := LeaderboardWindowBounds(LeaderboardWindowToday, now)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC), start.UTC())
	require.Equal(t, time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC), end.UTC())

	start, end, ok = LeaderboardWindowBounds(LeaderboardWindowWeek, now)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC), start.UTC(), "周从周一开始")
	require.Equal(t, time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC), end.UTC())

	start, end, ok = LeaderboardWindowBounds(LeaderboardWindowMonth, now)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), start.UTC())
	require.Equal(t, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), end.UTC())

	_, _, ok = LeaderboardWindowBounds(LeaderboardWindow("all"), now)
	require.False(t, ok)
}

// 一轮重建只调一次聚合，却要产出三份快照——三处数字因此同源。
func TestLeaderboardSnapshotService_RebuildFansOutThreeWindows(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 45, 0, time.UTC)

	repo := &stubLeaderboardAggregateRepo{rows: []usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 30, TodayRequests: 3, WeekTokens: 70, WeekRequests: 7, MonthTokens: 100, MonthRequests: 10},
		// 今日没有任何用量：不进今日快照，但本周 / 本月照常。
		{UserID: 2, WeekTokens: 5, WeekRequests: 1, MonthTokens: 50, MonthRequests: 5},
		// 只有成功请求数（按次计费、token 为 0）：仍然是参与者。
		{UserID: 3, TodayRequests: 1, WeekRequests: 1, MonthRequests: 1},
		// 三个窗口全 0（只有失败占位行）：任何窗口都不进快照。
		{UserID: 4},
	}}
	cache := &stubLeaderboardCache{}
	svc := NewLeaderboardSnapshotService(repo, cache, nil)

	require.NoError(t, svc.Rebuild(context.Background(), now))
	require.Equal(t, 1, repo.calls, "一轮只扫一次，绝不是每个窗口各跑一次")
	require.Len(t, cache.written, 3)

	todayStart, todayEnd, _ := LeaderboardWindowBounds(LeaderboardWindowToday, now)
	weekStart, _, _ := LeaderboardWindowBounds(LeaderboardWindowWeek, now)
	monthStart, _, _ := LeaderboardWindowBounds(LeaderboardWindowMonth, now)
	require.True(t, repo.todayStart.Equal(todayStart))
	require.True(t, repo.weekStart.Equal(weekStart))
	require.True(t, repo.monthStart.Equal(monthStart))

	today := cache.byWindow(t, LeaderboardWindowToday)
	require.True(t, today.WindowStart.Equal(todayStart))
	require.True(t, today.WindowEnd.Equal(todayEnd))
	require.True(t, today.UpdatedAt.Equal(now))
	require.Equal(t, []LeaderboardUserMetrics{
		{UserID: 1, TotalTokens: 30, SuccessfulRequests: 3},
		{UserID: 3, TotalTokens: 0, SuccessfulRequests: 1},
	}, today.Entries)

	week := cache.byWindow(t, LeaderboardWindowWeek)
	require.Equal(t, []LeaderboardUserMetrics{
		{UserID: 1, TotalTokens: 70, SuccessfulRequests: 7},
		{UserID: 2, TotalTokens: 5, SuccessfulRequests: 1},
		{UserID: 3, TotalTokens: 0, SuccessfulRequests: 1},
	}, week.Entries)

	month := cache.byWindow(t, LeaderboardWindowMonth)
	require.Equal(t, []LeaderboardUserMetrics{
		{UserID: 1, TotalTokens: 100, SuccessfulRequests: 10},
		{UserID: 2, TotalTokens: 50, SuccessfulRequests: 5},
		{UserID: 3, TotalTokens: 0, SuccessfulRequests: 1},
	}, month.Entries)

	for _, snapshot := range cache.written {
		for _, entry := range snapshot.Entries {
			require.NotEqual(t, int64(4), entry.UserID, "两个数值都是 0 的用户不得进入任何窗口的 Snapshot")
		}
	}
}

// 聚合失败时不写任何一份快照：正式 key 保持上一轮内容，更新时间也不被推进。
func TestLeaderboardSnapshotService_RebuildAggregateFailureWritesNothing(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	repo := &stubLeaderboardAggregateRepo{err: errors.New("boom")}
	cache := &stubLeaderboardCache{}
	svc := NewLeaderboardSnapshotService(repo, cache, nil)

	require.Error(t, svc.Rebuild(context.Background(), time.Now()))
	require.Empty(t, cache.written)
}

// 未注入依赖时不 panic，只报错——Start() 同样直接返回。
func TestLeaderboardSnapshotService_RebuildWithoutDependencies(t *testing.T) {
	require.Error(t, (&LeaderboardSnapshotService{}).Rebuild(context.Background(), time.Now()))
	NewLeaderboardSnapshotService(nil, nil, nil).Start()
	NewLeaderboardSnapshotService(nil, nil, nil).Stop()
}

// 一轮重建里的 Insights（洞察）与 Highlights（趣味卡）默认桩：够让 Rebuild 走完全流程。
func leaderboardInsightsStubRepo(rows []usagestats.LeaderboardAggregateRow) *stubLeaderboardAggregateRepo {
	return &stubLeaderboardAggregateRepo{
		rows:          rows,
		models:        []usagestats.LeaderboardModelUsageRow{{Model: "claude-sonnet-5", SuccessfulRequests: 30}},
		modelsTotal:   50,
		dominantModel: "claude-sonnet-5",
	}
}

// 四个数值必须一路带到快照：后两个只用于算命中率，不进 ZSET（ZSET 由缓存实现负责）。
func TestLeaderboardSnapshotService_RebuildCarriesCacheMetrics(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardInsightsStubRepo([]usagestats.LeaderboardAggregateRow{
		{
			UserID:      1,
			TodayTokens: 30, TodayRequests: 3, TodayInputTokens: 10, TodayCacheReadTokens: 20,
			WeekTokens: 70, WeekRequests: 7, WeekInputTokens: 25, WeekCacheReadTokens: 45,
			MonthTokens: 100, MonthRequests: 10, MonthInputTokens: 40, MonthCacheReadTokens: 60,
		},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	require.Equal(t, []LeaderboardUserMetrics{
		{UserID: 1, TotalTokens: 30, SuccessfulRequests: 3, InputTokens: 10, CacheReadTokens: 20},
	}, cache.byWindow(t, LeaderboardWindowToday).Entries)
	require.Equal(t, []LeaderboardUserMetrics{
		{UserID: 1, TotalTokens: 70, SuccessfulRequests: 7, InputTokens: 25, CacheReadTokens: 45},
	}, cache.byWindow(t, LeaderboardWindowWeek).Entries)
	require.Equal(t, []LeaderboardUserMetrics{
		{UserID: 1, TotalTokens: 100, SuccessfulRequests: 10, InputTokens: 40, CacheReadTokens: 60},
	}, cache.byWindow(t, LeaderboardWindowMonth).Entries)
}

// Highlights 的四块与榜单同源：领先者、占比、领先第 2 名的百分比与全站汇总都来自同一次遍历。
func TestLeaderboardSnapshotService_HighlightsTopAndSite(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)
	todayStart, _, _ := LeaderboardWindowBounds(LeaderboardWindowToday, now)

	repo := leaderboardInsightsStubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 600, TodayRequests: 10, TodayInputTokens: 100, TodayCacheReadTokens: 300},
		{UserID: 2, TodayTokens: 300, TodayRequests: 40, TodayInputTokens: 100, TodayCacheReadTokens: 100},
		{UserID: 3, TodayTokens: 100, TodayRequests: 5},
	})
	repo.hourly = []usagestats.LeaderboardHourlyBucketRow{
		{BucketStart: todayStart.Add(9 * time.Hour), Requests: 100, InputTokens: 50, CacheReadTokens: 50},
		{BucketStart: todayStart.Add(14 * time.Hour), Requests: 400, InputTokens: 150, CacheReadTokens: 450},
	}
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	highlights := cache.byWindow(t, LeaderboardWindowToday).Highlights
	require.NotNil(t, highlights)

	require.Equal(t, &LeaderboardHighlightUser{
		UserID: 1, TotalTokens: 600, SuccessfulRequests: 10,
		SharePercent: 60,  // 600 / 1000
		LeadPercent:  100, // 比第 2 名的 300 多一倍
	}, highlights.TopTokens)
	require.Equal(t, &LeaderboardHighlightUser{
		UserID: 2, TotalTokens: 300, SuccessfulRequests: 40,
		SharePercent: 72,  // 40 / 55 向下取整
		LeadPercent:  300, // 比第 2 名的 10 多三倍
	}, highlights.TopRequests)

	require.Equal(t, int64(1000), highlights.Site.TotalTokens)
	require.Equal(t, int64(55), highlights.Site.SuccessfulRequests)
	require.Equal(t, int64(3), highlights.Site.ParticipantCount)
	require.NotNil(t, highlights.Site.CacheHitRate)
	require.InDelta(t, 400.0/600.0, *highlights.Site.CacheHitRate, 1e-9)
	require.NotNil(t, highlights.Site.PeakHour, "峰值时段取自今日小时桶")
	require.Equal(t, 14, *highlights.Site.PeakHour)

	// 效率之星在达标者里取命中率最高者：user 1 的 300/400 高于 user 2 的 100/200。
	require.NotNil(t, highlights.CacheKing)
	require.Equal(t, int64(1), highlights.CacheKing.UserID)
	require.InDelta(t, 0.75, highlights.CacheKing.CacheHitRate, 1e-9)
	require.Equal(t, "claude-sonnet-5", highlights.CacheKing.DominantModel)
}

// 效率之星的门槛是「成功请求数大于 5」：几次请求就能刷出 100% 命中率，不该上榜。
func TestLeaderboardSnapshotService_CacheKingRequiresMoreThanFiveRequests(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardInsightsStubRepo([]usagestats.LeaderboardAggregateRow{
		// 命中率 100%，但只有 5 次成功请求：不参与评选。
		{UserID: 1, TodayTokens: 100, TodayRequests: leaderboardCacheKingMinRequests, TodayInputTokens: 0, TodayCacheReadTokens: 900},
		// 命中率只有 10%，但请求数过线：它才是效率之星。
		{UserID: 2, TodayTokens: 100, TodayRequests: leaderboardCacheKingMinRequests + 1, TodayInputTokens: 900, TodayCacheReadTokens: 100},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	king := cache.byWindow(t, LeaderboardWindowToday).Highlights.CacheKing
	require.NotNil(t, king)
	require.Equal(t, int64(2), king.UserID, "成功请求数不超过门槛的用户不得当选")
	require.InDelta(t, 0.1, king.CacheHitRate, 1e-9)
}

// input + cache_read 为 0 的用户没有命中率（不是 0%），全窗口无人达标时整块为 nil。
func TestLeaderboardSnapshotService_CacheKingAbsentWithoutHitRate(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardInsightsStubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 50},
		{UserID: 2, TodayTokens: 80, TodayRequests: 40},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	highlights := cache.byWindow(t, LeaderboardWindowToday).Highlights
	require.NotNil(t, highlights)
	require.Nil(t, highlights.CacheKing, "没有命中率就没有效率之星，MUST NOT 记成 0%")
	require.Nil(t, highlights.Site.CacheHitRate, "全站分母为 0 时同样是字段缺席")
	require.Empty(t, repo.dominantCalls, "没有效率之星就不必查 dominant model")
}

// dominant model 只对效率之星一个用户查一次，且区间就是该窗口自己的区间。
func TestLeaderboardSnapshotService_DominantModelQueriedOncePerWindow(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardInsightsStubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 50, TodayInputTokens: 100, TodayCacheReadTokens: 300,
			WeekTokens: 200, WeekRequests: 80, WeekInputTokens: 200, WeekCacheReadTokens: 600,
			MonthTokens: 300, MonthRequests: 90, MonthInputTokens: 300, MonthCacheReadTokens: 900},
		{UserID: 2, TodayTokens: 90, TodayRequests: 40, TodayInputTokens: 900, TodayCacheReadTokens: 100,
			WeekTokens: 90, WeekRequests: 40, WeekInputTokens: 900, WeekCacheReadTokens: 100,
			MonthTokens: 90, MonthRequests: 40, MonthInputTokens: 900, MonthCacheReadTokens: 100},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	require.Len(t, repo.dominantCalls, 3, "每个窗口只查一次，绝不是每个用户查一次")
	for _, call := range repo.dominantCalls {
		require.Equal(t, int64(1), call.userID)
	}
	todayStart, todayEnd, _ := LeaderboardWindowBounds(LeaderboardWindowToday, now)
	require.True(t, repo.dominantCalls[0].start.Equal(todayStart))
	require.True(t, repo.dominantCalls[0].end.Equal(todayEnd))
}

// dominant model 查不动时该窗口整段不切换：宁可继续服务上一轮的完整快照。
func TestLeaderboardSnapshotService_DominantModelFailureSkipsWindow(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardInsightsStubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 50, TodayInputTokens: 100, TodayCacheReadTokens: 300,
			WeekTokens: 100, WeekRequests: 50, WeekInputTokens: 100, WeekCacheReadTokens: 300,
			MonthTokens: 100, MonthRequests: 50, MonthInputTokens: 100, MonthCacheReadTokens: 300},
	})
	repo.dominantErr = errors.New("boom")
	cache := &stubLeaderboardCache{}

	require.Error(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))
	require.Empty(t, cache.written, "算不出完整 Highlights 的窗口 MUST NOT 推进更新时间")
}

// 预聚合表缺行：依赖它的三块为 nil，不补 0；不依赖它的模型热度照常。
func TestLeaderboardSnapshotService_InsightsNilBlocksWhenPreAggregateMissing(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardInsightsStubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 50},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	require.Len(t, cache.insights, 1)
	insights := cache.insights[0]
	require.NotNil(t, insights)
	require.Equal(t, []LeaderboardModelInsight{
		{Model: "claude-sonnet-5", SuccessfulRequests: 30, SharePercent: 60},
	}, insights.ModelsToday, "模型热度直接扫 usage_logs，不受预聚合开关影响")
	require.Nil(t, insights.Daily30)
	require.Nil(t, insights.HourlyToday)
	require.Nil(t, insights.CacheToday)
	require.Nil(t, insights.Month)
	require.Equal(t, leaderboardTopModelsLimit, repo.modelsLimit)

	// 近 30 天、本月、上月各一次窄读取，全部落在预聚合表上。
	todayStart, _, _ := LeaderboardWindowBounds(LeaderboardWindowToday, now)
	monthStart, monthEnd, _ := LeaderboardWindowBounds(LeaderboardWindowMonth, now)
	require.Len(t, repo.dailyCalls, 3)
	require.True(t, repo.dailyCalls[0].from.Equal(todayStart.AddDate(0, 0, -29)))
	require.True(t, repo.dailyCalls[0].to.Equal(todayStart.AddDate(0, 0, 1)))
	require.True(t, repo.dailyCalls[1].from.Equal(monthStart))
	require.True(t, repo.dailyCalls[1].to.Equal(monthEnd))
	require.True(t, repo.dailyCalls[2].from.Equal(monthStart.AddDate(0, -1, 0)))
	require.True(t, repo.dailyCalls[2].to.Equal(monthStart))
}

// 「较上月」由两段区间各求一次和算出，上月的绝对量只参与计算、不进下发结构。
func TestLeaderboardSnapshotService_InsightsMonthChangePercent(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)
	monthStart, _, _ := LeaderboardWindowBounds(LeaderboardWindowMonth, now)

	repo := leaderboardInsightsStubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 50},
	})
	repo.dailyFn = func(from, _ time.Time) []usagestats.LeaderboardDailyBucketRow {
		if from.Equal(monthStart) {
			return []usagestats.LeaderboardDailyBucketRow{
				{Date: monthStart, Requests: 10, TotalTokens: 600},
				{Date: monthStart.AddDate(0, 0, 1), Requests: 20, TotalTokens: 400},
			}
		}
		if from.Equal(monthStart.AddDate(0, -1, 0)) {
			return []usagestats.LeaderboardDailyBucketRow{
				{Date: monthStart.AddDate(0, -1, 0), Requests: 30, TotalTokens: 800},
			}
		}
		// 近 30 天：最高一天是 1000，另一天 250 → 相对百分比 25。
		return []usagestats.LeaderboardDailyBucketRow{
			{Date: monthStart, Requests: 7, TotalTokens: 1000},
			{Date: monthStart.AddDate(0, 0, 1), Requests: 3, TotalTokens: 250},
		}
	}
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	insights := cache.insights[0]
	require.Equal(t, &LeaderboardMonthInsight{TotalTokens: 1000, ChangePercent: 25}, insights.Month)

	// 结构体里没有上月的位置，序列化后自然也找不到——这是「不下发」最硬的保证。
	payload, err := json.Marshal(insights.Month)
	require.NoError(t, err)
	require.JSONEq(t, `{"total_tokens":1000,"change_percent":25}`, string(payload))

	require.Equal(t, []LeaderboardDailyInsight{
		{Date: "2026-03-01", Requests: 7, TotalTokens: 1000, RelativePercent: 100},
		{Date: "2026-03-02", Requests: 3, TotalTokens: 250, RelativePercent: 25},
	}, insights.Daily30)
}

// 小时桶的边界本来就是站点时区：直接取本地小时，不做二次映射，也不多读一天。
func TestLeaderboardSnapshotService_InsightsHourlyUsesSiteTimezone(t *testing.T) {
	require.NoError(t, timezone.Init("Asia/Shanghai"))
	t.Cleanup(func() { require.NoError(t, timezone.Init("UTC")) })

	now := time.Date(2026, 3, 18, 15, 30, 0, 0, timezone.Location())
	todayStart, todayEnd, _ := LeaderboardWindowBounds(LeaderboardWindowToday, now)

	repo := leaderboardInsightsStubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 50},
	})
	repo.hourly = []usagestats.LeaderboardHourlyBucketRow{
		{BucketStart: todayStart.Add(9 * time.Hour), Requests: 100, InputTokens: 200, CacheReadTokens: 300},
		{BucketStart: todayStart.Add(14 * time.Hour), Requests: 400, InputTokens: 300, CacheReadTokens: 700},
	}
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	require.Len(t, repo.hourlyCalls, 1, "今日 24 桶只读今日这一段")
	require.True(t, repo.hourlyCalls[0].from.Equal(todayStart))
	require.True(t, repo.hourlyCalls[0].to.Equal(todayEnd))

	insights := cache.insights[0]
	require.Len(t, insights.HourlyToday, 24)
	require.Equal(t, LeaderboardHourlyInsight{Hour: 9, Requests: 100, RelativePercent: 25}, insights.HourlyToday[9])
	require.Equal(t, LeaderboardHourlyInsight{Hour: 14, Requests: 400, RelativePercent: 100}, insights.HourlyToday[14])
	require.Equal(t, LeaderboardHourlyInsight{Hour: 0, Requests: 0, RelativePercent: 0}, insights.HourlyToday[0],
		"没有请求的小时补 0，那是真实值；整块缺行才是 nil")

	// 今日缓存命中由同一批小时桶汇总，不另查一次。
	require.NotNil(t, insights.CacheToday)
	require.Equal(t, int64(1000), insights.CacheToday.CacheReadTokens)
	require.Equal(t, int64(500), insights.CacheToday.InputTokens)
	require.InDelta(t, 1000.0/1500.0, insights.CacheToday.CacheHitRate, 1e-9)
}

// 模型热度与榜单同源（直接扫 usage_logs）：它出错说明本轮数据源本身不可信，
// 整轮不切换——三个窗口与 insights key 都保持上一轮内容。
func TestLeaderboardSnapshotService_ModelsFailureWritesNothing(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardInsightsStubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 50},
	})
	repo.modelsErr = errors.New("boom")
	cache := &stubLeaderboardCache{}

	require.Error(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))
	require.Empty(t, cache.written)
	require.Empty(t, cache.insights)
}

// 预聚合表读不动只降级对应区块：窗口快照照常写入，榜单本体不被两张派生表连坐。
func TestLeaderboardSnapshotService_PreAggregateFailureDegradesBlocksOnly(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardInsightsStubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 50, TodayInputTokens: 40, TodayCacheReadTokens: 60},
	})
	repo.dailyErr = errors.New("daily boom")
	repo.hourlyErr = errors.New("hourly boom")
	cache := &stubLeaderboardCache{}

	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	// 三个窗口照常切换，榜单条目完好。
	require.Len(t, cache.written, 3)
	require.Equal(t, []LeaderboardUserMetrics{
		{UserID: 1, TotalTokens: 100, SuccessfulRequests: 50, InputTokens: 40, CacheReadTokens: 60},
	}, cache.byWindow(t, LeaderboardWindowToday).Entries)

	// 读不动的那几块降级为 nil，扫 usage_logs 的模型热度照常有值。
	require.Len(t, cache.insights, 1)
	insights := cache.insights[0]
	require.NotNil(t, insights)
	require.Equal(t, []LeaderboardModelInsight{
		{Model: "claude-sonnet-5", SuccessfulRequests: 30, SharePercent: 60},
	}, insights.ModelsToday)
	require.Nil(t, insights.Daily30)
	require.Nil(t, insights.HourlyToday)
	require.Nil(t, insights.CacheToday)
	require.Nil(t, insights.Month)

	// 没有小时桶就没有峰值时段，全站概况卡的 PeakHour 缺席而不是记成 0 点。
	highlights := cache.byWindow(t, LeaderboardWindowToday).Highlights
	require.NotNil(t, highlights)
	require.Nil(t, highlights.Site.PeakHour)
}

// 空窗口没有可言说的 Highlights：整块为 nil，而不是一份零值卡片。
func TestLeaderboardSnapshotService_EmptyWindowHasNoHighlights(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardInsightsStubRepo(nil)
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	for _, snapshot := range cache.written {
		require.Empty(t, snapshot.Entries)
		require.Nil(t, snapshot.Highlights)
	}
}

// 领先百分比的两个边界：并列时为 0；没有第 2 名时按「领先一倍」封顶成 100，
// 不给无穷大，也不留一个能反推绝对量的超大数。
func TestLeaderboardLeadPercent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		first  int64
		second int64
		want   int
	}{
		{"并列第一", 100, 100, 0},
		{"多一倍", 200, 100, 100},
		{"向下取整", 199, 100, 99},
		{"只有一个人", 100, 0, 100},
		{"第一名为 0", 0, 0, 0},
	} {
		require.Equal(t, tc.want, leaderboardLeadPercent(tc.first, tc.second), tc.name)
	}
}

// 命中率的分母为 0 时没有命中率（ok=false），而不是 0。
func TestLeaderboardCacheHitRate(t *testing.T) {
	rate, ok := leaderboardCacheHitRate(100, 300)
	require.True(t, ok)
	require.InDelta(t, 0.75, rate, 1e-9)

	_, ok = leaderboardCacheHitRate(0, 0)
	require.False(t, ok)
}

// ---- v2：Extremes（之最）、Profiles（画像）、新增 Insights 与名次历史 ----

// leaderboardV2StubRepo 在 leaderboardInsightsStubRepo 之上补齐 v2 的桩数据，
// 让 Rebuild 能一路走到之最、画像、平台分布、周内节奏与名次历史。
func leaderboardV2StubRepo(rows []usagestats.LeaderboardAggregateRow) *stubLeaderboardAggregateRepo {
	repo := leaderboardInsightsStubRepo(rows)
	repo.streak = usagestats.LeaderboardStreakRow{UserID: 9, Days: 23}
	return repo
}

// rising 只在 today 窗口产出：week / month 没有同口径的基线列，那张卡必然缺席。
func TestLeaderboardSnapshotService_ExtremesRisingOnlyToday(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{
			UserID: 1, TodayTokens: 300, TodayRequests: 10, WeekTokens: 900, WeekRequests: 30,
			MonthTokens: 1800, MonthRequests: 60, YesterdayTokens: 100, YesterdayRequests: 4,
		},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	today := cache.byWindow(t, LeaderboardWindowToday).Highlights
	require.NotNil(t, today.Extremes.Rising)
	require.Equal(t, int64(1), today.Extremes.Rising.UserID)
	require.Equal(t, 200, today.Extremes.Rising.ChangePercent, "300 比 100 多 200%")

	require.Nil(t, cache.byWindow(t, LeaderboardWindowWeek).Highlights.Extremes.Rising)
	require.Nil(t, cache.byWindow(t, LeaderboardWindowMonth).Highlights.Extremes.Rising)
}

// 昨日为 0 或今日没有超过昨日的用户都不参评：否则「昨天零用量」会算出无穷增幅。
func TestLeaderboardSnapshotService_ExtremesRisingEligibility(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		// 昨日零用量：不参评。
		{UserID: 1, TodayTokens: 900, TodayRequests: 9},
		// 今日反而少了：不参评。
		{UserID: 2, TodayTokens: 50, TodayRequests: 5, YesterdayTokens: 500, YesterdayRequests: 9},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))
	require.Nil(t, cache.byWindow(t, LeaderboardWindowToday).Highlights.Extremes.Rising)
}

// 话痨的参评门槛：成功请求少于 5 的用户不参与，哪怕他的 output 占比是 100%。
func TestLeaderboardSnapshotService_ExtremesTalkerThreshold(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		// 4 次请求、output 占满：差一次达不到门槛。
		{UserID: 1, TodayTokens: 100, TodayRequests: 4, TodayOutputTokens: 100},
		// 正好 5 次：含等于，参评。
		{UserID: 2, TodayTokens: 100, TodayRequests: 5, TodayOutputTokens: 40},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	talker := cache.byWindow(t, LeaderboardWindowToday).Highlights.Extremes.Talker
	require.NotNil(t, talker)
	require.Equal(t, int64(2), talker.UserID, "门槛之下的用户 MUST NOT 顶掉达标者")
	require.Equal(t, 40, talker.OutputSharePercent)
}

// max_single 的 ratio_to_median：分母是全体参与者「各自单次最大值」的中位数。
func TestLeaderboardSnapshotService_ExtremesMaxSingleRatioToMedian(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 1000, TodayRequests: 9, TodayMaxSingleTokens: 900},
		{UserID: 2, TodayTokens: 500, TodayRequests: 5, TodayMaxSingleTokens: 300},
		{UserID: 3, TodayTokens: 400, TodayRequests: 4, TodayMaxSingleTokens: 100},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	maxSingle := cache.byWindow(t, LeaderboardWindowToday).Highlights.Extremes.MaxSingle
	require.NotNil(t, maxSingle)
	require.Equal(t, int64(1), maxSingle.UserID)
	require.Equal(t, int64(900), maxSingle.MaxSingleTokens)
	require.InDelta(t, 3.0, maxSingle.RatioToMedian, 0.001, "900 / 中位数 300 = 3.0 倍")
}

// 中位数为 0（没有人有单次最大值）时整项缺席：anonymous 档只有 ratio_to_median 可用，
// 分母没了就没有可下发的相对量，给 0 倍反而是个假事实。
func TestLeaderboardSnapshotService_ExtremesMaxSingleAbsentWhenNoMedian(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 5},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))
	require.Nil(t, cache.byWindow(t, LeaderboardWindowToday).Highlights.Extremes.MaxSingle)
}

// 夜猫子与杂食者：取该窗口对应列的最大者，同值时固定取 user_id 较小者。
func TestLeaderboardSnapshotService_ExtremesNightOwlAndOmnivore(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 5, TodayTokens: 400, TodayRequests: 8, TodayNightTokens: 300, TodayDistinctModels: 2},
		{UserID: 2, TodayTokens: 900, TodayRequests: 9, TodayNightTokens: 100, TodayDistinctModels: 4},
		{UserID: 7, TodayTokens: 800, TodayRequests: 7, TodayNightTokens: 100, TodayDistinctModels: 4},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	extremes := cache.byWindow(t, LeaderboardWindowToday).Highlights.Extremes
	require.NotNil(t, extremes.NightOwl)
	require.Equal(t, int64(5), extremes.NightOwl.UserID)
	require.Equal(t, int64(300), extremes.NightOwl.NightTokens)
	require.Equal(t, 75, extremes.NightOwl.NightSharePercent, "夜间占其自身窗口 tokens 的比例")

	require.NotNil(t, extremes.Omnivore)
	require.Equal(t, int64(2), extremes.Omnivore.UserID, "并列时取 user_id 较小者，避免每轮换人")
	require.Equal(t, 4, extremes.Omnivore.DistinctModels)
}

// streak 与 Window 无关：只查一次，三个窗口的这一项内容完全相同。
func TestLeaderboardSnapshotService_ExtremesStreakSharedAcrossWindows(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)
	todayStart, _, _ := LeaderboardWindowBounds(LeaderboardWindowToday, now)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 5, WeekTokens: 300, WeekRequests: 9, MonthTokens: 900, MonthRequests: 30},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	require.Len(t, repo.streakCalls, 1, "连续活跃 MUST 只查一次，三个窗口共用")
	require.True(t, repo.streakCalls[0].from.Equal(todayStart.AddDate(0, 0, -(leaderboardStreakLookbackDays-1))))
	require.True(t, repo.streakCalls[0].to.Equal(todayStart.AddDate(0, 0, 1)))

	want := &LeaderboardExtremeStreak{UserID: 9, Days: 23}
	for _, window := range []LeaderboardWindow{LeaderboardWindowToday, LeaderboardWindowWeek, LeaderboardWindowMonth} {
		require.Equal(t, want, cache.byWindow(t, window).Highlights.Extremes.Streak, "窗口 %s", window)
	}
}

// streak 查不动只降级成「没有这张卡」：它来自预聚合的日活表，与榜单本体无关。
func TestLeaderboardSnapshotService_ExtremesStreakFailureDegradesOnly(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 5},
	})
	repo.streakErr = errors.New("daily users unavailable")
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	require.Len(t, cache.written, 3, "三个窗口照常写入")
	require.Nil(t, cache.byWindow(t, LeaderboardWindowToday).Highlights.Extremes.Streak)
}

// 画像：每个窗口一条限定 Top 50 的聚合，MUST NOT 为这 50 个人各发一条查询。
func TestLeaderboardSnapshotService_ProfilesSingleQueryPerWindow(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)
	todayStart, todayEnd, _ := LeaderboardWindowBounds(LeaderboardWindowToday, now)

	// 60 个人都有用量：画像只取前 50，第 51–60 名 MUST NOT 进入聚合的 user_id 集合。
	rows := make([]usagestats.LeaderboardAggregateRow, 0, 60)
	for i := 1; i <= 60; i++ {
		rows = append(rows, usagestats.LeaderboardAggregateRow{
			UserID:      int64(i),
			TodayTokens: int64(1000 - i*10), TodayRequests: 5,
			WeekTokens: int64(2000 - i*10), WeekRequests: 15,
			MonthTokens: int64(3000 - i*10), MonthRequests: 45,
		})
	}
	repo := leaderboardV2StubRepo(rows)
	repo.userModels = []usagestats.LeaderboardUserModelUsageRow{
		{UserID: 1, Model: "claude-sonnet-5", SuccessfulRequests: 6},
		{UserID: 1, Model: "claude-opus-5", SuccessfulRequests: 3},
		{UserID: 1, Model: "gpt-5.1", SuccessfulRequests: 2},
		{UserID: 1, Model: "gemini-3", SuccessfulRequests: 1},
		{UserID: 2, Model: "claude-opus-5", SuccessfulRequests: 4},
	}
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	require.Len(t, repo.userModelCalls, 3, "每个窗口一条聚合，三个窗口共三条")
	todayCall := repo.userModelCalls[0]
	require.Len(t, todayCall.userIDs, leaderboardProfilesLimit, "只取前 50 名")
	wantIDs := make([]int64, 0, leaderboardProfilesLimit)
	for i := 1; i <= leaderboardProfilesLimit; i++ {
		wantIDs = append(wantIDs, int64(i))
	}
	require.Equal(t, wantIDs, todayCall.userIDs)
	require.True(t, todayCall.start.Equal(todayStart))
	require.True(t, todayCall.end.Equal(todayEnd))

	profiles := cache.byWindow(t, LeaderboardWindowToday).Highlights.Profiles
	require.Len(t, profiles, 2, "聚合里没有行的用户不出现在画像里")
	require.Equal(t, int64(1), profiles[0].UserID)
	require.Len(t, profiles[0].Models, leaderboardProfileModelsLimit, "每人最多 Top 3")
	require.Equal(t, LeaderboardProfileModel{Model: "claude-sonnet-5", SharePercent: 50}, profiles[0].Models[0])
	require.Equal(t, LeaderboardProfileModel{Model: "claude-opus-5", SharePercent: 25}, profiles[0].Models[1])
}

// 画像聚合失败只降级本块：本轮三个窗口的快照照常写入。
func TestLeaderboardSnapshotService_ProfilesFailureDegradesOnly(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 5},
	})
	repo.userModelsErr = errors.New("breakdown unavailable")
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	require.Len(t, cache.written, 3)
	require.Nil(t, cache.byWindow(t, LeaderboardWindowToday).Highlights.Profiles)
}

// 周内节奏：近 28 天（不含今日）按 (周几, 小时) 的平均请求数分五档；
// 缺行的格子是「那个时段没有请求」，等级为 0，不是缺数据。
func TestLeaderboardSnapshotService_InsightsWeeklyRhythmLevels(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)
	todayStart, _, _ := LeaderboardWindowBounds(LeaderboardWindowToday, now)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 5},
	})
	repo.rhythm = []usagestats.LeaderboardWeekdayHourRow{
		{Weekday: 1, Hour: 0, Requests: 100}, // 最大值 → 4
		{Weekday: 1, Hour: 9, Requests: 75},  // 3
		{Weekday: 3, Hour: 14, Requests: 50}, // 2
		{Weekday: 7, Hour: 23, Requests: 25}, // 1
		{Weekday: 7, Hour: 22, Requests: 1},  // 非零必至少 1
	}
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	require.Len(t, repo.rhythmCalls, 1)
	require.True(t, repo.rhythmCalls[0].from.Equal(todayStart.AddDate(0, 0, -leaderboardRhythmLookbackDays)))
	require.True(t, repo.rhythmCalls[0].to.Equal(todayStart), "今天还没过完，不进平均值")

	rhythm := cache.insights[0].WeeklyRhythm
	require.Len(t, rhythm, 7, "行序周一起，共 7 行")
	require.Len(t, rhythm[0], 24)
	require.Equal(t, 4, rhythm[0][0])
	require.Equal(t, 3, rhythm[0][9])
	require.Equal(t, 2, rhythm[2][14])
	require.Equal(t, 1, rhythm[6][23])
	require.Equal(t, 1, rhythm[6][22], "非零的格子至少是 1 级")
	require.Equal(t, 0, rhythm[1][5], "表里没有这一格 = 那个时段没有请求")
}

// 预聚合整段缺行时周内节奏整块为 null，MUST NOT 补成 7 × 24 的 0。
func TestLeaderboardSnapshotService_InsightsWeeklyRhythmNilWhenNoRows(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 5},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))
	require.Nil(t, cache.insights[0].WeeklyRhythm)

	// 读取出错同样只降级本块，本轮快照照常写入。
	repo.rhythmErr = errors.New("hourly unavailable")
	cache2 := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache2, nil).Rebuild(context.Background(), now))
	require.Nil(t, cache2.insights[0].WeeklyRhythm)
	require.Len(t, cache2.written, 3)
}

// 平台分布与 Token 构成：前者一条 JOIN accounts 的聚合，后者由今日窗口的站点合计导出。
func TestLeaderboardSnapshotService_InsightsPlatformsAndComposition(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{
			UserID: 1, TodayTokens: 100, TodayRequests: 5,
			TodayInputTokens: 40, TodayCacheReadTokens: 30, TodayOutputTokens: 20, TodayCacheCreationTokens: 10,
		},
		{
			UserID: 2, TodayTokens: 100, TodayRequests: 5,
			TodayInputTokens: 10, TodayCacheReadTokens: 10, TodayOutputTokens: 60, TodayCacheCreationTokens: 20,
		},
	})
	repo.platforms = []usagestats.LeaderboardPlatformUsageRow{
		{Platform: "anthropic", SuccessfulRequests: 30},
		{Platform: "openai", SuccessfulRequests: 10},
	}
	repo.platformsTotal = 40
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	insights := cache.insights[0]
	require.Equal(t, []LeaderboardPlatformInsight{
		{Platform: "anthropic", SuccessfulRequests: 30, SharePercent: 75},
		{Platform: "openai", SuccessfulRequests: 10, SharePercent: 25},
	}, insights.PlatformsToday)

	require.NotNil(t, insights.CompositionToday)
	require.Equal(t, int64(50), insights.CompositionToday.InputTokens)
	require.Equal(t, int64(80), insights.CompositionToday.OutputTokens)
	require.Equal(t, int64(30), insights.CompositionToday.CacheCreationTokens)
	require.Equal(t, int64(40), insights.CompositionToday.CacheReadTokens)
	require.Equal(t, 25, insights.CompositionToday.InputPercent)
	require.Equal(t, 40, insights.CompositionToday.OutputPercent)
	require.Equal(t, 15, insights.CompositionToday.CacheCreationPercent)
	require.Equal(t, 20, insights.CompositionToday.CacheReadPercent)

	// 平台聚合失败只降级本块。
	repo.platformsErr = errors.New("accounts unavailable")
	cache2 := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache2, nil).Rebuild(context.Background(), now))
	require.Nil(t, cache2.insights[0].PlatformsToday)
	require.Len(t, cache2.written, 3)
}

// 缓存命中率趋势从近 30 天的日桶里切出近 14 天，不另读一次。
func TestLeaderboardSnapshotService_InsightsCacheTrend14(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)
	todayStart, _, _ := LeaderboardWindowBounds(LeaderboardWindowToday, now)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 5},
	})
	repo.dailyFn = func(from, to time.Time) []usagestats.LeaderboardDailyBucketRow {
		return []usagestats.LeaderboardDailyBucketRow{
			// 第 20 天前：在 30 天窗口内、但不在 14 天趋势里。
			{Date: todayStart.AddDate(0, 0, -20), Requests: 10, TotalTokens: 100, InputTokens: 50, CacheReadTokens: 50},
			{Date: todayStart.AddDate(0, 0, -2), Requests: 10, TotalTokens: 100, InputTokens: 25, CacheReadTokens: 75},
			// input + cache_read 为 0：那一天没有命中率，跳过而不是记 0%。
			{Date: todayStart.AddDate(0, 0, -1), Requests: 3, TotalTokens: 30},
			{Date: todayStart, Requests: 10, TotalTokens: 100, InputTokens: 80, CacheReadTokens: 20},
		}
	}
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	trend := cache.insights[0].CacheTrend14
	require.Len(t, trend, 2, "只保留近 14 天里算得出命中率的那些天")
	require.Equal(t, todayStart.AddDate(0, 0, -2).Format("2006-01-02"), trend[0].Date)
	require.InDelta(t, 0.75, trend[0].CacheHitRate, 0.0001)
	require.Equal(t, todayStart.Format("2006-01-02"), trend[1].Date)
	require.InDelta(t, 0.2, trend[1].CacheHitRate, 0.0001)

	// 日桶整段缺行时整块为 null。
	repo.dailyFn = nil
	cache2 := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache2, nil).Rebuild(context.Background(), now))
	require.Nil(t, cache2.insights[0].CacheTrend14)
}

// 全站概况增加的 avg_tokens_per_request：成功请求为 0 时字段缺席，MUST NOT 记成 0。
func TestLeaderboardSnapshotService_SiteAvgTokensPerRequest(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 300, TodayRequests: 3},
		{UserID: 2, TodayTokens: 100, TodayRequests: 1},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	site := cache.byWindow(t, LeaderboardWindowToday).Highlights.Site
	require.NotNil(t, site.AvgTokensPerRequest)
	require.InDelta(t, 100.0, *site.AvgTokensPerRequest, 0.0001)

	// 只有 token、没有成功请求（全是失败占位行）：该字段缺席。
	repo2 := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{{UserID: 1, TodayTokens: 300}})
	cache2 := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo2, cache2, nil).Rebuild(context.Background(), now))
	require.Nil(t, cache2.byWindow(t, LeaderboardWindowToday).Highlights.Site.AvgTokensPerRequest)
}

// 名次历史：对今日窗口的全部参与者写一行，名次与响应侧的竞争排名完全一致。
func TestLeaderboardSnapshotService_RankHistoryUpsertsTodayParticipants(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)
	todayStart, _, _ := LeaderboardWindowBounds(LeaderboardWindowToday, now)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 300, TodayRequests: 1, WeekTokens: 300, WeekRequests: 1},
		{UserID: 2, TodayTokens: 300, TodayRequests: 9, WeekTokens: 300, WeekRequests: 9},
		{UserID: 3, TodayTokens: 100, TodayRequests: 5, WeekTokens: 100, WeekRequests: 5},
		// 今日零用量：只进本周窗口，MUST NOT 出现在今日的名次历史里。
		{UserID: 4, WeekTokens: 900, WeekRequests: 30},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	require.Len(t, repo.rankHistory, 1, "一轮只写一批")
	rows := repo.rankHistory[0]
	require.Len(t, rows, 3, "只写今日窗口的参与者")

	byUser := make(map[int64]usagestats.LeaderboardRankHistoryRow, len(rows))
	for _, row := range rows {
		require.True(t, row.SnapshotDate.Equal(todayStart), "日期取今日窗口起点")
		byUser[row.UserID] = row
	}
	// tokens 并列：两人都是第 1，其后跳到第 3。
	require.Equal(t, 1, byUser[1].RankTotalTokens)
	require.Equal(t, 1, byUser[2].RankTotalTokens)
	require.Equal(t, 3, byUser[3].RankTotalTokens)
	// 成功请求数是另一套名次，两列都写。
	require.Equal(t, 1, byUser[2].RankSuccessfulRequests)
	require.Equal(t, 2, byUser[3].RankSuccessfulRequests)
	require.Equal(t, 3, byUser[1].RankSuccessfulRequests)

	for _, row := range rows {
		require.NotEqual(t, int64(4), row.UserID)
	}

	// 清理按 90 天保留期，cutoff 是「今日起点减 90 天」：第 90 天保留、第 91 天删除。
	require.Equal(t, 1, repo.rankCleanupHits)
	require.Len(t, repo.rankCutoffs, 1)
	require.True(t, repo.rankCutoffs[0].Equal(todayStart.AddDate(0, 0, -leaderboardRankHistoryRetentionDays)))
	require.True(t, repo.rankCutoffs[0].Before(todayStart.AddDate(0, 0, -(leaderboardRankHistoryRetentionDays-1))),
		"第 90 天（今日减 89）严格晚于 cutoff，因此保留")
}

// 名次历史写入或清理失败只记日志：整轮快照照常写入，正式 key MUST NOT 停留在上一轮。
func TestLeaderboardSnapshotService_RankHistoryFailureDoesNotAbortRebuild(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, TodayTokens: 100, TodayRequests: 5},
	})
	repo.rankHistoryErr = errors.New("upsert failed")
	repo.rankCleanupErr = errors.New("delete failed")
	cache := &stubLeaderboardCache{}

	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))
	require.Len(t, cache.written, 3)
	require.Len(t, cache.insights, 1)
	require.Equal(t, 1, repo.rankCleanupHits, "写失败 MUST NOT 跳过清理")
}

// 今日窗口没有任何参与者时不发空的 upsert，但保留期清理照常执行。
func TestLeaderboardSnapshotService_RankHistorySkipsEmptyUpsert(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{UserID: 1, WeekTokens: 100, WeekRequests: 5},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	require.Empty(t, repo.rankHistory)
	require.Equal(t, 1, repo.rankCleanupHits)
}

// 十四个新增数必须一路带进快照条目：Hash 的十二段与 Extremes 都靠它们。
func TestLeaderboardSnapshotService_RebuildCarriesExtremeMetrics(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC)

	repo := leaderboardV2StubRepo([]usagestats.LeaderboardAggregateRow{
		{
			UserID:      1,
			TodayTokens: 30, TodayRequests: 3, TodayInputTokens: 10, TodayCacheReadTokens: 20,
			TodayOutputTokens: 4, TodayCacheCreationTokens: 5, TodayNightTokens: 6,
			TodayDistinctModels: 7, TodayMaxSingleTokens: 8, TodayMediaRequests: 9,
			WeekTokens: 70, WeekRequests: 7, WeekInputTokens: 25, WeekCacheReadTokens: 45,
			WeekOutputTokens: 14, WeekCacheCreationTokens: 15, WeekNightTokens: 16,
			WeekDistinctModels: 17, WeekMaxSingleTokens: 18, WeekMediaRequests: 19,
			MonthTokens: 100, MonthRequests: 10, MonthInputTokens: 40, MonthCacheReadTokens: 60,
			MonthOutputTokens: 24, MonthCacheCreationTokens: 25, MonthNightTokens: 26,
			MonthDistinctModels: 27, MonthMaxSingleTokens: 28, MonthMediaRequests: 29,
			YesterdayTokens: 12, YesterdayRequests: 2,
		},
	})
	cache := &stubLeaderboardCache{}
	require.NoError(t, NewLeaderboardSnapshotService(repo, cache, nil).Rebuild(context.Background(), now))

	require.Equal(t, []LeaderboardUserMetrics{{
		UserID: 1, TotalTokens: 30, SuccessfulRequests: 3, InputTokens: 10, CacheReadTokens: 20,
		OutputTokens: 4, CacheCreationTokens: 5, NightTokens: 6, DistinctModels: 7,
		MaxSingleTokens: 8, MediaRequests: 9, YesterdayTokens: 12, YesterdayRequests: 2,
	}}, cache.byWindow(t, LeaderboardWindowToday).Entries)

	require.Equal(t, []LeaderboardUserMetrics{{
		UserID: 1, TotalTokens: 70, SuccessfulRequests: 7, InputTokens: 25, CacheReadTokens: 45,
		OutputTokens: 14, CacheCreationTokens: 15, NightTokens: 16, DistinctModels: 17,
		MaxSingleTokens: 18, MediaRequests: 19, YesterdayTokens: 12, YesterdayRequests: 2,
	}}, cache.byWindow(t, LeaderboardWindowWeek).Entries, "「昨日」与 Window 无关，三个窗口同一个值")

	require.Equal(t, []LeaderboardUserMetrics{{
		UserID: 1, TotalTokens: 100, SuccessfulRequests: 10, InputTokens: 40, CacheReadTokens: 60,
		OutputTokens: 24, CacheCreationTokens: 25, NightTokens: 26, DistinctModels: 27,
		MaxSingleTokens: 28, MediaRequests: 29, YesterdayTokens: 12, YesterdayRequests: 2,
	}}, cache.byWindow(t, LeaderboardWindowMonth).Entries)
}
