//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// leaderboardCacheStub 是一份最小 Snapshot（榜单快照）：默认只有查看者一个人、一条记录，
// 足以让响应走到 ready 分支，从而断言参数解析与档位透传的结果。
// 需要一整页数据（档位裁剪的 JSON 断言）时给 entries 赋值。
type leaderboardCacheStub struct {
	window service.LeaderboardWindow
	metric service.LeaderboardMetric

	entries    []service.LeaderboardUserMetrics
	missing    bool
	highlights *service.LeaderboardHighlights
	insights   *service.LeaderboardInsights
}

func (c *leaderboardCacheStub) ReplaceSnapshot(context.Context, service.LeaderboardSnapshotWindow) error {
	return nil
}

// rows 按当前 Metric（排名指标）倒序返回快照条目，语义与 Redis 上的 ZSET 一致。
func (c *leaderboardCacheStub) rows(metric service.LeaderboardMetric) []service.LeaderboardUserMetrics {
	rows := c.entries
	if len(rows) == 0 {
		rows = []service.LeaderboardUserMetrics{{UserID: 7, TotalTokens: 100, SuccessfulRequests: 5}}
	}
	out := append([]service.LeaderboardUserMetrics(nil), rows...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Metric(metric) > out[j].Metric(metric) })
	return out
}

func (c *leaderboardCacheStub) TopEntries(_ context.Context, window service.LeaderboardWindow, _ time.Time, metric service.LeaderboardMetric, _ int) ([]service.LeaderboardScoreEntry, error) {
	c.window, c.metric = window, metric
	rows := c.rows(metric)
	out := make([]service.LeaderboardScoreEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, service.LeaderboardScoreEntry{UserID: row.UserID, Score: row.Metric(metric)})
	}
	return out, nil
}

func (c *leaderboardCacheStub) ParticipantCount(_ context.Context, _ service.LeaderboardWindow, _ time.Time, metric service.LeaderboardMetric) (int64, error) {
	return int64(len(c.rows(metric))), nil
}

func (c *leaderboardCacheStub) RankOf(_ context.Context, _ service.LeaderboardWindow, _ time.Time, metric service.LeaderboardMetric, userID int64) (int64, bool, error) {
	rows := c.rows(metric)
	for i, row := range rows {
		if row.UserID == userID {
			return int64(i + 1), true, nil
		}
	}
	return 0, false, nil
}

func (c *leaderboardCacheStub) MetricsOf(_ context.Context, _ service.LeaderboardWindow, _ time.Time, userIDs []int64) (map[int64]service.LeaderboardUserMetrics, error) {
	out := make(map[int64]service.LeaderboardUserMetrics, len(userIDs))
	for _, id := range userIDs {
		for _, row := range c.rows(service.LeaderboardMetricTotalTokens) {
			if row.UserID == id {
				out[id] = row
			}
		}
	}
	return out, nil
}

func (c *leaderboardCacheStub) UpdatedAt(context.Context, service.LeaderboardWindow, time.Time) (time.Time, bool, error) {
	if c.missing {
		return time.Time{}, false, nil
	}
	return time.Now(), true, nil
}

func (c *leaderboardCacheStub) Highlights(context.Context, service.LeaderboardWindow, time.Time) (*service.LeaderboardHighlights, error) {
	return c.highlights, nil
}

func (c *leaderboardCacheStub) ReplaceInsights(context.Context, time.Time, *service.LeaderboardInsights, time.Time) error {
	return nil
}

func (c *leaderboardCacheStub) Insights(context.Context, time.Time) (*service.LeaderboardInsights, error) {
	return c.insights, nil
}

func (c *leaderboardCacheStub) ViewerModels(context.Context, int64, service.LeaderboardWindow, time.Time) ([]usagestats.LeaderboardModelUsageRow, int64, bool, error) {
	return nil, 0, false, nil
}

func (c *leaderboardCacheStub) SetViewerModels(context.Context, int64, service.LeaderboardWindow, time.Time, time.Time, []usagestats.LeaderboardModelUsageRow, int64) error {
	return nil
}

// leaderboardViewerRepoStub 是 viewer（「你的统计」）那两条只查本人的查询：
// 名次走势与本人模型偏好都给固定数据，用来断言它们在任何档位下都原样下发。
type leaderboardViewerRepoStub struct{}

func (leaderboardViewerRepoStub) LeaderboardRankHistory(_ context.Context, userID int64, _, _ time.Time) ([]usagestats.LeaderboardRankHistoryRow, error) {
	return []usagestats.LeaderboardRankHistoryRow{
		{UserID: userID, SnapshotDate: time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC), RankTotalTokens: 9, RankSuccessfulRequests: 11},
		{UserID: userID, SnapshotDate: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), RankTotalTokens: 6, RankSuccessfulRequests: 7},
	}, nil
}

func (leaderboardViewerRepoStub) LeaderboardViewerModels(context.Context, int64, time.Time, time.Time, int) ([]usagestats.LeaderboardModelUsageRow, int64, error) {
	return []usagestats.LeaderboardModelUsageRow{
		{Model: "claude-sonnet-5", SuccessfulRequests: 3},
		{Model: "claude-opus-5", SuccessfulRequests: 1},
	}, 4, nil
}

type leaderboardUserRepoStub struct{}

func (leaderboardUserRepoStub) GetByIDs(_ context.Context, ids []int64) ([]service.User, error) {
	out := make([]service.User, 0, len(ids))
	for _, id := range ids {
		out = append(out, service.User{ID: id, Username: "alice", Status: service.StatusActive})
	}
	return out, nil
}

func newLeaderboardTestRouter(cache service.LeaderboardCache, mode string, role string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewLeaderboardHandler(service.NewLeaderboardService(cache, leaderboardUserRepoStub{}, leaderboardViewerRepoStub{}))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Set(string(middleware.ContextKeyUserRole), role)
		// 路由层的 leaderboardModeGuard 会写入档位，这里照样透传。
		c.Set(LeaderboardModeContextKey, mode)
		if mode == service.LeaderboardModeOff && role == service.RoleAdmin {
			c.Set(LeaderboardPreviewContextKey, true)
		}
		c.Next()
	})
	router.GET("/leaderboard", h.Get)
	return router
}

func TestLeaderboardHandlerGetParams(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantWindow service.LeaderboardWindow
		wantMetric service.LeaderboardMetric
		wantReason string
	}{
		{
			name:       "不带参数时用默认的 today + total_tokens",
			query:      "",
			wantStatus: http.StatusOK,
			wantWindow: service.LeaderboardWindowToday,
			wantMetric: service.LeaderboardMetricTotalTokens,
		},
		{
			name:       "显式指定 window 与 metric",
			query:      "?window=month&metric=successful_requests",
			wantStatus: http.StatusOK,
			wantWindow: service.LeaderboardWindowMonth,
			wantMetric: service.LeaderboardMetricSuccessfulRequests,
		},
		{
			name:       "空串参数回落到默认值",
			query:      "?window=&metric=",
			wantStatus: http.StatusOK,
			wantWindow: service.LeaderboardWindowToday,
			wantMetric: service.LeaderboardMetricTotalTokens,
		},
		{
			name:       "非法 window 返回 400 且不回落默认值",
			query:      "?window=year",
			wantStatus: http.StatusBadRequest,
			wantReason: "LEADERBOARD_INVALID_WINDOW",
		},
		{
			name:       "非法 metric 返回 400",
			query:      "?metric=cost",
			wantStatus: http.StatusBadRequest,
			wantReason: "LEADERBOARD_INVALID_METRIC",
		},
		{
			name:       "自定义日期参数不改变窗口边界",
			query:      "?start_date=2026-01-01&end_date=2026-01-31",
			wantStatus: http.StatusOK,
			wantWindow: service.LeaderboardWindowToday,
			wantMetric: service.LeaderboardMetricTotalTokens,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := &leaderboardCacheStub{}
			router := newLeaderboardTestRouter(cache, service.LeaderboardModeNamed, service.RoleUser)

			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/leaderboard"+tt.query, nil)
			router.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantReason != "" {
				require.Contains(t, rec.Body.String(), tt.wantReason)
				require.Empty(t, cache.window, "非法参数 MUST NOT 触达 Snapshot 读取")
				return
			}

			var body struct {
				Data service.LeaderboardView `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, string(tt.wantWindow), body.Data.Window)
			require.Equal(t, string(tt.wantMetric), body.Data.Metric)
			require.Equal(t, tt.wantWindow, cache.window)
			require.Equal(t, tt.wantMetric, cache.metric)
		})
	}
}

func TestLeaderboardHandlerGetMode(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		role        string
		wantStatus  int
		wantMode    string
		wantPreview bool
	}{
		{
			name:        "off 档管理员进入 Preview",
			mode:        service.LeaderboardModeOff,
			role:        service.RoleAdmin,
			wantStatus:  http.StatusOK,
			wantMode:    service.LeaderboardModeOff,
			wantPreview: true,
		},
		{
			name:       "off 档普通用户 404（服务端再强制一次）",
			mode:       service.LeaderboardModeOff,
			role:       service.RoleUser,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "anonymous 档不带 preview",
			mode:       service.LeaderboardModeAnonymous,
			role:       service.RoleAdmin,
			wantStatus: http.StatusOK,
			wantMode:   service.LeaderboardModeAnonymous,
		},
		{
			name:       "named 档不带 preview",
			mode:       service.LeaderboardModeNamed,
			role:       service.RoleUser,
			wantStatus: http.StatusOK,
			wantMode:   service.LeaderboardModeNamed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newLeaderboardTestRouter(&leaderboardCacheStub{}, tt.mode, tt.role)

			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/leaderboard", nil)
			router.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantStatus != http.StatusOK {
				// 404 MUST NOT 透露该路由存在：响应体与 gin 未注册路由的默认 404
				// 逐字节一致，不出现任何功能名或档位信息。
				require.Equal(t, "404 page not found", rec.Body.String())
				require.NotContains(t, strings.ToLower(rec.Body.String()), "leaderboard")
				return
			}
			var body struct {
				Data service.LeaderboardView `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, tt.wantMode, body.Data.Mode)
			require.Equal(t, tt.wantPreview, body.Data.Preview)
			require.NotEmpty(t, body.Data.Timezone)
			require.NotContains(t, rec.Body.String(), "user_id")
		})
	}
}

// 缺少档位标记（路由未挂 guard）时 fail-closed 到 off：普通用户 404。
func TestLeaderboardHandlerGetWithoutModeMarkerFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewLeaderboardHandler(service.NewLeaderboardService(&leaderboardCacheStub{}, leaderboardUserRepoStub{}, leaderboardViewerRepoStub{}))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUser)
		c.Next()
	})
	router.GET("/leaderboard", h.Get)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/leaderboard", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "404 page not found", rec.Body.String())
}

// 未认证（上下文里没有 AuthSubject）时不进入任何查询逻辑。
func TestLeaderboardHandlerGetRequiresAuthSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewLeaderboardHandler(service.NewLeaderboardService(&leaderboardCacheStub{}, leaderboardUserRepoStub{}, leaderboardViewerRepoStub{}))
	router := gin.New()
	router.GET("/leaderboard", h.Get)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/leaderboard", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// leaderboardAbsoluteFieldNames 是「绝对量」字段名的全集（含 v2 新增的 Extremes（之最）
// 与 Token 构成那几个）：anonymous 档下它们只允许出现在查看者本人的条目、my_rank 与顶层
// viewer 里，其余任何位置（他人条目、Highlights、Insights、全站概况）出现一次都是泄露。
// user_id 任何档位都不该出现。
var leaderboardAbsoluteFieldNames = []string{
	"total_tokens",
	"successful_requests",
	"requests",
	"input_tokens",
	"output_tokens",
	"cache_creation_tokens",
	"cache_read_tokens",
	"night_tokens",
	"max_single_tokens",
	"user_id",
}

// leaderboardForeignAbsolutePaths 深度遍历响应体，返回所有「他人绝对量」字段的路径。
//
// 三处豁免与 design D14 / D18 / D21 一致，也只有这三处：my_rank 与顶层 viewer 整块都是
// 查看者自己的真实数值，is_self 为 true 的条目同理。豁免按结构判定而不是按字段名，
// 因此新增字段不会悄悄漏网。
func leaderboardForeignAbsolutePaths(node any, path string) []string {
	switch value := node.(type) {
	case map[string]any:
		if isSelf, ok := value["is_self"].(bool); ok && isSelf {
			return nil
		}
		found := make([]string, 0)
		for key, child := range value {
			childPath := path + "." + key
			if path == "data" && (key == "my_rank" || key == "viewer") {
				continue
			}
			if slices.Contains(leaderboardAbsoluteFieldNames, key) {
				found = append(found, childPath)
			}
			found = append(found, leaderboardForeignAbsolutePaths(child, childPath)...)
		}
		return found
	case []any:
		found := make([]string, 0)
		for i, item := range value {
			found = append(found, leaderboardForeignAbsolutePaths(item, fmt.Sprintf("%s[%d]", path, i))...)
		}
		return found
	default:
		return nil
	}
}

// leaderboardFullSnapshotStub 是一份六人快照 + 完整的 Highlights 与 Insights：
// 人数压过 anonymous 档的门槛，因此条目不会被抑制，整页数据都能走到裁剪逻辑。
func leaderboardFullSnapshotStub() *leaderboardCacheStub {
	siteRate, peakHour, siteAvg := 0.712, 14, 36.4
	rhythm := make([][]int, 7)
	for weekday := range rhythm {
		rhythm[weekday] = make([]int, 24)
	}
	rhythm[0][9] = 4
	return &leaderboardCacheStub{
		entries: []service.LeaderboardUserMetrics{
			{UserID: 1, TotalTokens: 1000, SuccessfulRequests: 20, InputTokens: 400, CacheReadTokens: 600},
			{UserID: 2, TotalTokens: 800, SuccessfulRequests: 18},
			{UserID: 3, TotalTokens: 600, SuccessfulRequests: 16},
			{UserID: 4, TotalTokens: 400, SuccessfulRequests: 14},
			{UserID: 5, TotalTokens: 200, SuccessfulRequests: 12},
			{UserID: 7, TotalTokens: 100, SuccessfulRequests: 5, InputTokens: 40, CacheReadTokens: 60},
		},
		highlights: &service.LeaderboardHighlights{
			TopTokens:   &service.LeaderboardHighlightUser{UserID: 1, TotalTokens: 1000, SuccessfulRequests: 20, SharePercent: 32, LeadPercent: 25},
			TopRequests: &service.LeaderboardHighlightUser{UserID: 2, TotalTokens: 800, SuccessfulRequests: 18, SharePercent: 21, LeadPercent: 12},
			CacheKing:   &service.LeaderboardCacheKing{UserID: 99, CacheHitRate: 0.873, DominantModel: "claude-sonnet-5"},
			Site: service.LeaderboardSiteSummary{
				TotalTokens: 3100, SuccessfulRequests: 85, ParticipantCount: 137,
				CacheHitRate: &siteRate, PeakHour: &peakHour, AvgTokensPerRequest: &siteAvg,
			},
			Extremes: &service.LeaderboardExtremes{
				NightOwl:  &service.LeaderboardExtremeNightOwl{UserID: 1, NightTokens: 640, NightSharePercent: 64},
				Rising:    &service.LeaderboardExtremeRising{UserID: 2, ChangePercent: 210},
				Omnivore:  &service.LeaderboardExtremeOmnivore{UserID: 3, DistinctModels: 7},
				Talker:    &service.LeaderboardExtremeTalker{UserID: 4, OutputSharePercent: 38},
				MaxSingle: &service.LeaderboardExtremeMaxSingle{UserID: 99, MaxSingleTokens: 1280, RatioToMedian: 4.2},
				Streak:    &service.LeaderboardExtremeStreak{UserID: 5, Days: 23},
			},
			Profiles: []service.LeaderboardProfile{
				{UserID: 1, Models: []service.LeaderboardProfileModel{{Model: "claude-sonnet-5", SharePercent: 62}}},
			},
		},
		insights: &service.LeaderboardInsights{
			ModelsToday: []service.LeaderboardModelInsight{{Model: "claude-sonnet-5", SuccessfulRequests: 587, SharePercent: 47}},
			Daily30:     []service.LeaderboardDailyInsight{{Date: "2026-09-11", Requests: 260, TotalTokens: 2000, RelativePercent: 100}},
			HourlyToday: []service.LeaderboardHourlyInsight{{Hour: 14, Requests: 88, RelativePercent: 100}},
			CacheToday:  &service.LeaderboardCacheInsight{CacheHitRate: 0.712, CacheReadTokens: 8600, InputTokens: 3400},
			Month:       &service.LeaderboardMonthInsight{TotalTokens: 165000, ChangePercent: 18},

			PlatformsToday: []service.LeaderboardPlatformInsight{{Platform: "anthropic", SuccessfulRequests: 812, SharePercent: 61}},
			WeeklyRhythm:   rhythm,
			CompositionToday: &service.LeaderboardCompositionInsight{
				InputTokens: 3400, OutputTokens: 1200, CacheCreationTokens: 900, CacheReadTokens: 8600,
				InputPercent: 24, OutputPercent: 8, CacheCreationPercent: 6, CacheReadPercent: 61,
			},
			CacheTrend14: []service.LeaderboardCacheTrendInsight{{Date: "2026-09-11", CacheHitRate: 0.712}},
		},
	}
}

func leaderboardResponseData(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(body, &envelope))
	data, ok := envelope["data"].(map[string]any)
	require.True(t, ok, "响应信封里必须有 data 对象")
	return data
}

func leaderboardGet(t *testing.T, cache service.LeaderboardCache, mode, role string) map[string]any {
	t.Helper()
	router := newLeaderboardTestRouter(cache, mode, role)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/leaderboard", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	return leaderboardResponseData(t, rec.Body.Bytes())
}

// 10.24 anonymous 档的整段响应里不得出现任何他人或站点级的绝对量字段名：
// total_tokens / successful_requests 只允许出现在本人条目与 my_rank 里。
func TestLeaderboardHandlerAnonymousResponseHasNoForeignAbsolutes(t *testing.T) {
	data := leaderboardGet(t, leaderboardFullSnapshotStub(), service.LeaderboardModeAnonymous, service.RoleUser)

	require.Equal(t, false, data["entries_suppressed"])
	require.Empty(t, leaderboardForeignAbsolutePaths(data, "data"))

	// 豁免的两处仍然是真实数值。
	myRank, ok := data["my_rank"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, myRank, "total_tokens")
	require.Contains(t, myRank, "successful_requests")

	entries, ok := data["entries"].([]any)
	require.True(t, ok)
	selfSeen := false
	for _, item := range entries {
		entry, ok := item.(map[string]any)
		require.True(t, ok)
		if isSelf, _ := entry["is_self"].(bool); isSelf {
			selfSeen = true
			require.Contains(t, entry, "total_tokens")
			continue
		}
		require.NotContains(t, entry, "total_tokens")
		require.Contains(t, entry, "total_tokens_relative_percent")
	}
	require.True(t, selfSeen, "查看者应当出现在榜单里，否则这条断言是空跑")

	// 相对量与比率照常下发，Highlights 的领先者用 Ordinal 假名。
	highlights, ok := data["highlights"].(map[string]any)
	require.True(t, ok)
	topTokens, ok := highlights["top_tokens"].(map[string]any)
	require.True(t, ok)
	require.EqualValues(t, 32, topTokens["share_percent"])
	require.EqualValues(t, 1, topTokens["ordinal"])
	identity, ok := topTokens["identity"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, service.LeaderboardIdentityAnonymous, identity["kind"])
	require.NotContains(t, identity, "username")

	cacheKing, ok := highlights["cache_king"].(map[string]any)
	require.True(t, ok)
	require.Nil(t, cacheKing["ordinal"], "榜外领先者的 ordinal 是 null")
	require.Equal(t, "claude-sonnet-5", cacheKing["dominant_model"])

	site, ok := highlights["site"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "100+", site["participant_count"])
	require.Contains(t, site, "cache_hit_rate")
	require.Contains(t, site, "peak_hour")

	insights, ok := data["insights"].(map[string]any)
	require.True(t, ok)
	models, ok := insights["models_today"].([]any)
	require.True(t, ok)
	require.Len(t, models, 1)
	require.Contains(t, models[0].(map[string]any), "share_percent")
}

// 10.24 named 档给出绝对量：这同时证明上一条断言不是因为遍历函数看不见字段才通过的。
func TestLeaderboardHandlerNamedResponseKeepsAbsolutes(t *testing.T) {
	data := leaderboardGet(t, leaderboardFullSnapshotStub(), service.LeaderboardModeNamed, service.RoleUser)

	paths := leaderboardForeignAbsolutePaths(data, "data")
	require.NotEmpty(t, paths, "named 档下他人与站点级的绝对量本来就该出现")
	require.Contains(t, paths, "data.highlights.top_tokens.total_tokens")
	require.Contains(t, paths, "data.highlights.site.total_tokens")
	require.Contains(t, paths, "data.insights.month.total_tokens")
	require.NotContains(t, strings.Join(paths, ","), "user_id")

	highlights := data["highlights"].(map[string]any)
	require.EqualValues(t, 137, highlights["site"].(map[string]any)["participant_count"])
}

// 10.24 响应体带 highlights 与 insights 两个顶层字段；区块缺失时序列化成 null，
// MUST NOT 被 omitempty 吞掉。
func TestLeaderboardHandlerHighlightsAndInsightsAreTopLevelFields(t *testing.T) {
	cache := leaderboardFullSnapshotStub()
	data := leaderboardGet(t, cache, service.LeaderboardModeNamed, service.RoleUser)
	require.Contains(t, data, "highlights")
	require.Contains(t, data, "insights")
	require.NotNil(t, data["highlights"])
	require.NotNil(t, data["insights"])

	// 快照尚未生成：highlights 为 null（页面渲染骨架），insights 不随快照状态变化。
	computing := leaderboardFullSnapshotStub()
	computing.missing = true
	data = leaderboardGet(t, computing, service.LeaderboardModeNamed, service.RoleUser)
	require.Equal(t, service.LeaderboardStatusComputing, data["status"])
	require.Contains(t, data, "highlights")
	require.Nil(t, data["highlights"])
	require.NotNil(t, data["insights"])

	// 旧快照里根本没有这两类 key：两个字段都是 null，榜单本体照常。
	legacy := leaderboardFullSnapshotStub()
	legacy.highlights, legacy.insights = nil, nil
	data = leaderboardGet(t, legacy, service.LeaderboardModeNamed, service.RoleUser)
	require.Contains(t, data, "highlights")
	require.Nil(t, data["highlights"])
	require.Contains(t, data, "insights")
	require.Nil(t, data["insights"])
	require.NotEmpty(t, data["entries"])
}

// 11.38 anonymous 档：v2 新增的三块（之最、画像、平台 / 构成）同样只留相对量与比率，
// 这条与上面那条共用同一个深度遍历，因此新增字段不会绕过校验。
func TestLeaderboardHandlerAnonymousResponseCropsExtremesAndNewInsights(t *testing.T) {
	data := leaderboardGet(t, leaderboardFullSnapshotStub(), service.LeaderboardModeAnonymous, service.RoleUser)
	require.Empty(t, leaderboardForeignAbsolutePaths(data, "data"))

	highlights, ok := data["highlights"].(map[string]any)
	require.True(t, ok)
	extremes, ok := highlights["extremes"].(map[string]any)
	require.True(t, ok)

	nightOwl, ok := extremes["night_owl"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, nightOwl, "night_tokens", "夜猫子的绝对 tokens MUST 缺席")
	require.EqualValues(t, 64, nightOwl["night_share_percent"])
	require.EqualValues(t, 1, nightOwl["ordinal"])

	maxSingle, ok := extremes["max_single"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, maxSingle, "max_single_tokens")
	require.EqualValues(t, 4.2, maxSingle["ratio_to_median"])
	require.Nil(t, maxSingle["ordinal"], "榜外之最的 ordinal 是 null")

	require.EqualValues(t, 210, extremes["rising"].(map[string]any)["change_percent"])
	require.EqualValues(t, 7, extremes["omnivore"].(map[string]any)["distinct_models"])
	require.EqualValues(t, 38, extremes["talker"].(map[string]any)["output_share_percent"])
	require.EqualValues(t, 23, extremes["streak"].(map[string]any)["days"])

	site, ok := highlights["site"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, site, "avg_tokens_per_request", "比率两档都下发")

	insights, ok := data["insights"].(map[string]any)
	require.True(t, ok)

	platforms, ok := insights["platforms_today"].([]any)
	require.True(t, ok)
	require.NotContains(t, platforms[0].(map[string]any), "successful_requests")
	require.EqualValues(t, 61, platforms[0].(map[string]any)["share_percent"])

	composition, ok := insights["composition_today"].(map[string]any)
	require.True(t, ok)
	for _, field := range []string{"input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens"} {
		require.NotContainsf(t, composition, field, "Token 构成的绝对量 %s MUST 缺席", field)
	}
	require.EqualValues(t, 24, composition["input_percent"])

	rhythm, ok := insights["weekly_rhythm"].([]any)
	require.True(t, ok)
	require.Len(t, rhythm, 7)
	require.Len(t, rhythm[0].([]any), 24)

	trend, ok := insights["cache_trend_14"].([]any)
	require.True(t, ok)
	require.Contains(t, trend[0].(map[string]any), "cache_hit_rate")

	profiles, ok := insights["profiles"].([]any)
	require.True(t, ok)
	profile := profiles[0].(map[string]any)
	require.NotContains(t, profile["identity"].(map[string]any), "username")
	require.EqualValues(t, 62, profile["models"].([]any)[0].(map[string]any)["share_percent"])
}

// 11.38 named 档下 v2 的绝对量照常出现：这同时证明上一条断言不是因为字段根本没下发才通过的。
func TestLeaderboardHandlerNamedResponseKeepsNewAbsolutes(t *testing.T) {
	data := leaderboardGet(t, leaderboardFullSnapshotStub(), service.LeaderboardModeNamed, service.RoleUser)
	paths := leaderboardForeignAbsolutePaths(data, "data")

	require.Contains(t, paths, "data.highlights.extremes.night_owl.night_tokens")
	require.Contains(t, paths, "data.highlights.extremes.max_single.max_single_tokens")
	require.Contains(t, paths, "data.insights.platforms_today[0].successful_requests")
	require.Contains(t, paths, "data.insights.composition_today.input_tokens")
	require.Contains(t, paths, "data.insights.composition_today.output_tokens")
	require.NotContains(t, strings.Join(paths, ","), "user_id")
}

// 11.38 响应带顶层 viewer：任何档位都是查看者本人的真实数据，且不随 status 变化。
func TestLeaderboardHandlerViewerIsTopLevelAndUncropped(t *testing.T) {
	for _, mode := range []string{service.LeaderboardModeNamed, service.LeaderboardModeAnonymous} {
		t.Run(mode, func(t *testing.T) {
			data := leaderboardGet(t, leaderboardFullSnapshotStub(), mode, service.RoleUser)
			viewer, ok := data["viewer"].(map[string]any)
			require.True(t, ok, "viewer MUST 是顶层字段且 MUST NOT 为 null")

			history, ok := viewer["rank_history"].([]any)
			require.True(t, ok)
			require.Len(t, history, 2)
			require.Equal(t, "2026-09-12", history[1].(map[string]any)["date"])
			require.EqualValues(t, 6, history[1].(map[string]any)["rank"])

			models, ok := viewer["models"].([]any)
			require.True(t, ok)
			require.Len(t, models, 2)
			require.EqualValues(t, 3, models[0].(map[string]any)["successful_requests"], "本人的绝对量在任何档位都真实")
			require.EqualValues(t, 75, models[0].(map[string]any)["share_percent"])

			require.Contains(t, viewer, "cache_hit_rate")
			require.Contains(t, viewer, "avg_tokens_per_request")
		})
	}

	// 快照尚未生成：highlights 为 null，viewer 照常返回。
	computing := leaderboardFullSnapshotStub()
	computing.missing = true
	data := leaderboardGet(t, computing, service.LeaderboardModeNamed, service.RoleUser)
	require.Equal(t, service.LeaderboardStatusComputing, data["status"])
	require.Nil(t, data["highlights"])
	viewer, ok := data["viewer"].(map[string]any)
	require.True(t, ok)
	require.Len(t, viewer["rank_history"], 2)
	require.Len(t, viewer["models"], 2)
	require.NotContains(t, viewer, "cache_hit_rate", "本窗口没有快照行时比率缺席，MUST NOT 记成 0")
}

// 11.38 区块为 null 时序列化成 null，MUST NOT 被 omitempty 吞掉；旧快照没有这些块也不报错。
func TestLeaderboardHandlerNewBlocksSerializeAsNull(t *testing.T) {
	legacy := leaderboardFullSnapshotStub()
	legacy.highlights.Extremes, legacy.highlights.Profiles = nil, nil
	legacy.insights.PlatformsToday = nil
	legacy.insights.WeeklyRhythm = nil
	legacy.insights.CompositionToday = nil
	legacy.insights.CacheTrend14 = nil

	router := newLeaderboardTestRouter(legacy, service.LeaderboardModeNamed, service.RoleUser)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/leaderboard", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	for _, field := range []string{
		`"extremes":null`, `"profiles":null`, `"platforms_today":null`,
		`"weekly_rhythm":null`, `"composition_today":null`, `"cache_trend_14":null`,
	} {
		require.Containsf(t, body, field, "%s MUST 序列化成 null", field)
	}

	data := leaderboardResponseData(t, rec.Body.Bytes())
	require.NotEmpty(t, data["entries"], "榜单本体照常")
	require.NotNil(t, data["viewer"])
}
