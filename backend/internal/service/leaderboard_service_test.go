//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

// fakeLeaderboardSnapshotCache 是一份内存 Snapshot（榜单快照），按与 Redis 实现相同的
// 语义回答读请求：ZREVRANGE 取前 N、ZCARD 数人、ZCOUNT 严格大于者加一。
// 它让「三处数字同源」这条约束在单测里也成立：entries、participant_count 与 my_rank
// 都从同一份 entries 推导。
type fakeLeaderboardSnapshotCache struct {
	entries   []LeaderboardUserMetrics
	updatedAt time.Time
	missing   bool  // key 不存在：Snapshot 尚未生成
	err       error // Redis 不可用

	topLimit int // 记录服务层请求的条数上限，用于断言固定 50

	// Highlights（趣味卡）与 Insights（洞察）同样只从 Redis 读：为 nil 即「这一块没有数据」。
	highlights *LeaderboardHighlights
	insights   *LeaderboardInsights

	// viewer:models 的 60 秒缓存：viewerModelsFound 为真即命中，此时服务层 MUST NOT 回源。
	viewerModels      []usagestats.LeaderboardModelUsageRow
	viewerModelsTotal int64
	viewerModelsFound bool
	viewerModelsErr   error

	viewerModelsReads int
	viewerModelsSets  int
	viewerModelsSetTo []usagestats.LeaderboardModelUsageRow
}

func (c *fakeLeaderboardSnapshotCache) ReplaceSnapshot(context.Context, LeaderboardSnapshotWindow) error {
	return nil
}

func (c *fakeLeaderboardSnapshotCache) sorted(metric LeaderboardMetric) []LeaderboardUserMetrics {
	out := append([]LeaderboardUserMetrics(nil), c.entries...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Metric(metric) != out[j].Metric(metric) {
			return out[i].Metric(metric) > out[j].Metric(metric)
		}
		return out[i].UserID < out[j].UserID
	})
	return out
}

func (c *fakeLeaderboardSnapshotCache) TopEntries(_ context.Context, _ LeaderboardWindow, _ time.Time, metric LeaderboardMetric, limit int) ([]LeaderboardScoreEntry, error) {
	c.topLimit = limit
	if c.err != nil {
		return nil, c.err
	}
	rows := c.sorted(metric)
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]LeaderboardScoreEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, LeaderboardScoreEntry{UserID: row.UserID, Score: row.Metric(metric)})
	}
	return out, nil
}

func (c *fakeLeaderboardSnapshotCache) ParticipantCount(context.Context, LeaderboardWindow, time.Time, LeaderboardMetric) (int64, error) {
	if c.err != nil {
		return 0, c.err
	}
	return int64(len(c.entries)), nil
}

func (c *fakeLeaderboardSnapshotCache) RankOf(_ context.Context, _ LeaderboardWindow, _ time.Time, metric LeaderboardMetric, userID int64) (int64, bool, error) {
	if c.err != nil {
		return 0, false, c.err
	}
	var (
		self  int64
		found bool
	)
	for _, row := range c.entries {
		if row.UserID == userID {
			self, found = row.Metric(metric), true
		}
	}
	if !found {
		return 0, false, nil
	}
	rank := int64(1)
	for _, row := range c.entries {
		if row.Metric(metric) > self {
			rank++
		}
	}
	return rank, true, nil
}

func (c *fakeLeaderboardSnapshotCache) MetricsOf(_ context.Context, _ LeaderboardWindow, _ time.Time, userIDs []int64) (map[int64]LeaderboardUserMetrics, error) {
	if c.err != nil {
		return nil, c.err
	}
	out := make(map[int64]LeaderboardUserMetrics, len(userIDs))
	for _, id := range userIDs {
		for _, row := range c.entries {
			if row.UserID == id {
				out[id] = row
			}
		}
	}
	return out, nil
}

func (c *fakeLeaderboardSnapshotCache) UpdatedAt(context.Context, LeaderboardWindow, time.Time) (time.Time, bool, error) {
	if c.err != nil {
		return time.Time{}, false, c.err
	}
	if c.missing {
		return time.Time{}, false, nil
	}
	updated := c.updatedAt
	if updated.IsZero() {
		updated = time.Now()
	}
	return updated, true, nil
}

func (c *fakeLeaderboardSnapshotCache) Highlights(context.Context, LeaderboardWindow, time.Time) (*LeaderboardHighlights, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.highlights, nil
}

func (c *fakeLeaderboardSnapshotCache) ReplaceInsights(context.Context, time.Time, *LeaderboardInsights, time.Time) error {
	return nil
}

func (c *fakeLeaderboardSnapshotCache) Insights(context.Context, time.Time) (*LeaderboardInsights, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.insights, nil
}

func (c *fakeLeaderboardSnapshotCache) ViewerModels(context.Context, int64, LeaderboardWindow, time.Time) ([]usagestats.LeaderboardModelUsageRow, int64, bool, error) {
	c.viewerModelsReads++
	if c.viewerModelsErr != nil {
		return nil, 0, false, c.viewerModelsErr
	}
	if !c.viewerModelsFound {
		return nil, 0, false, nil
	}
	return c.viewerModels, c.viewerModelsTotal, true, nil
}

func (c *fakeLeaderboardSnapshotCache) SetViewerModels(_ context.Context, _ int64, _ LeaderboardWindow, _, _ time.Time, rows []usagestats.LeaderboardModelUsageRow, _ int64) error {
	c.viewerModelsSets++
	c.viewerModelsSetTo = rows
	return nil
}

// fakeLeaderboardUserRepo 刻意把 DeletedAt 非空的用户也原样返回：真实仓储由
// SoftDeleteMixin 自动过滤，这里要验证服务层自己也会再剔除一次。
type fakeLeaderboardUserRepo struct {
	users map[int64]User
	err   error

	calls      int
	lastIDs    []int64
	idsCount   int
	avatars    map[int64]*UserAvatar
	avatarsErr error

	// avatarCalls / lastAvatarIDs 钉住 design D25 的两条：anonymous 档整批不查头像，
	// named 档也只查一次、且与 GetByIDs 同一批 id。
	avatarCalls   int
	lastAvatarIDs []int64
}

func (r *fakeLeaderboardUserRepo) GetByIDs(_ context.Context, ids []int64) ([]User, error) {
	r.calls++
	r.lastIDs = append([]int64(nil), ids...)
	r.idsCount = len(ids)
	if r.err != nil {
		return nil, r.err
	}
	out := make([]User, 0, len(ids))
	for _, id := range ids {
		if u, ok := r.users[id]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}

func (r *fakeLeaderboardUserRepo) GetUserAvatarThumbsByUserIDs(_ context.Context, userIDs []int64) (map[int64]string, error) {
	r.avatarCalls++
	r.lastAvatarIDs = append([]int64(nil), userIDs...)
	if r.avatarsErr != nil {
		return nil, r.avatarsErr
	}
	out := make(map[int64]string, len(userIDs))
	for _, id := range userIDs {
		// 与真实仓储同口径：只有带小图的行才出现（remote_url / 未回填的行 ThumbURL 为空）。
		if avatar, ok := r.avatars[id]; ok && avatar != nil && avatar.ThumbURL != "" {
			out[id] = avatar.ThumbURL
		}
	}
	return out, nil
}

func activeLeaderboardUser(id int64, username string) User {
	return User{ID: id, Username: username, Email: "user@example.com", Status: StatusActive}
}

func leaderboardUsers(users ...User) *fakeLeaderboardUserRepo {
	m := make(map[int64]User, len(users))
	for _, u := range users {
		m[u.ID] = u
	}
	return &fakeLeaderboardUserRepo{users: m}
}

func metricsRow(id, tokens, requests int64) LeaderboardUserMetrics {
	return LeaderboardUserMetrics{UserID: id, TotalTokens: tokens, SuccessfulRequests: requests}
}

// costMetricsRow 是带消费金额的快照条目。金额在 Snapshot 内部一律是定点 micros，
// 因此这里走 LeaderboardCostMicros 换算，与作业写 ZSET 分数的那一步同一条路径。
func costMetricsRow(id, tokens, requests int64, costUSD float64) LeaderboardUserMetrics {
	row := metricsRow(id, tokens, requests)
	row.CostMicros = LeaderboardCostMicros(costUSD)
	return row
}

// fakeLeaderboardViewerRepo 是 viewer（「你的统计」）那两条只查本人的查询。
// 它记录每次调用收到的 user_id 与区间，用来钉住「只查自己」与「缓存命中时不再查库」两条约束。
type fakeLeaderboardViewerRepo struct {
	history []usagestats.LeaderboardRankHistoryRow
	models  []usagestats.LeaderboardModelUsageRow
	total   int64

	historyErr error
	modelsErr  error

	historyCalls  int
	modelCalls    int
	historyUserID int64
	modelUserID   int64
	modelStart    time.Time
	modelEnd      time.Time
	modelLimit    int
}

func (r *fakeLeaderboardViewerRepo) LeaderboardRankHistory(_ context.Context, userID int64, _, _ time.Time) ([]usagestats.LeaderboardRankHistoryRow, error) {
	r.historyCalls++
	r.historyUserID = userID
	if r.historyErr != nil {
		return nil, r.historyErr
	}
	return r.history, nil
}

func (r *fakeLeaderboardViewerRepo) LeaderboardViewerModels(_ context.Context, userID int64, start, end time.Time, limit int) ([]usagestats.LeaderboardModelUsageRow, int64, error) {
	r.modelCalls++
	r.modelUserID, r.modelStart, r.modelEnd, r.modelLimit = userID, start, end, limit
	if r.modelsErr != nil {
		return nil, 0, r.modelsErr
	}
	return r.models, r.total, nil
}

// newTestLeaderboardService 是绝大多数用例用的构造：viewer 的窄仓储为 nil，
// 因此「你的统计」两块降级为空数组——顺带覆盖了仓储缺席时 viewer 仍不为 null 这条约束。
func newTestLeaderboardService(cache LeaderboardCache, userRepo LeaderboardUserRepository) *LeaderboardService {
	return NewLeaderboardService(cache, userRepo, nil)
}

// 榜内 Rank 由已排序的 Top 50 前缀就地推出（省掉 50 次 ZCOUNT 往返），这只有在它与
// LeaderboardCache.RankOf 的「严格高于者数 + 1」逐条等价时才成立——任何分数严格更高的
// 用户必然也在前缀里。这条是两者的对拍，并让并列正好跨过第 50/51 名的边界情况过一遍。
func TestLeaderboardCompetitionRanksMatchRankOf(t *testing.T) {
	ctx := context.Background()
	windowStart := time.Time{}

	rows := make([]LeaderboardUserMetrics, 0, 60)
	for i := 1; i <= 60; i++ {
		// 第 1 名独占，其后两两并列：并列的一对正好被 Top 50 截断（user 50 在榜内、
		// 同分的 user 51 在榜外），这正是「就地推名次」最容易出错的地方。
		score := int64((61-i)/2+1) * 10
		rows = append(rows, metricsRow(int64(i), score, score))
	}
	cache := &fakeLeaderboardSnapshotCache{entries: rows}

	top, err := cache.TopEntries(ctx, LeaderboardWindowToday, windowStart, LeaderboardMetricTotalTokens, LeaderboardTopEntryLimit)
	require.NoError(t, err)
	require.Len(t, top, LeaderboardTopEntryLimit)
	require.Equal(t, top[len(top)-1].Score, rows[50].TotalTokens, "第 50 名与榜外第 51 名同分")

	ranks := leaderboardCompetitionRanks(top)
	for i := range top {
		want, found, err := cache.RankOf(ctx, LeaderboardWindowToday, windowStart, LeaderboardMetricTotalTokens, top[i].UserID)
		require.NoError(t, err)
		require.True(t, found)
		require.Equalf(t, want, ranks[i], "第 %d 行（user %d）的名次与 ZCOUNT 语义不一致", i+1, top[i].UserID)
	}

	// 榜内查看者的 My Rank 复用同一个值，因此也要与 ZCOUNT 对得上。
	users := make([]User, 0, len(rows))
	for i := range rows {
		users = append(users, activeLeaderboardUser(rows[i].UserID, "user"))
	}
	svc := newTestLeaderboardService(cache, leaderboardUsers(users...))
	viewerID := top[len(top)-1].UserID
	view, err := svc.Query(ctx, viewerID, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.NotNil(t, view.MyRank)
	wantMyRank, _, err := cache.RankOf(ctx, LeaderboardWindowToday, windowStart, LeaderboardMetricTotalTokens, viewerID)
	require.NoError(t, err)
	require.Equal(t, wantMyRank, view.MyRank.Rank)
}

// 7.1 竞争排名：并列同名次、其后跳号，且 My Rank 与榜内条目同规则。
func TestLeaderboardService_QueryRankIsCompetitionRank(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{entries: []LeaderboardUserMetrics{
		metricsRow(1, 100, 10),
		metricsRow(2, 100, 8),
		metricsRow(3, 50, 4),
	}}
	repo := leaderboardUsers(
		activeLeaderboardUser(1, "alice"),
		activeLeaderboardUser(2, "bob"),
		activeLeaderboardUser(3, "carol"),
	)
	svc := newTestLeaderboardService(cache, repo)

	view, err := svc.Query(context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)

	require.Equal(t, LeaderboardStatusReady, view.Status)
	require.False(t, view.Stale)
	require.False(t, view.Preview)
	require.False(t, view.EntriesSuppressed)
	require.Equal(t, LeaderboardModeNamed, view.Mode)
	require.Equal(t, string(LeaderboardWindowToday), view.Window)
	require.Equal(t, string(LeaderboardMetricTotalTokens), view.Metric)
	require.NotEmpty(t, view.Timezone)
	require.Equal(t, int64(3), view.ParticipantCount)
	require.Equal(t, LeaderboardTopEntryLimit, cache.topLimit)

	require.Len(t, view.Entries, 3)
	require.Equal(t, []int64{1, 1, 3}, []int64{view.Entries[0].Rank, view.Entries[1].Rank, view.Entries[2].Rank})
	require.Equal(t, []int{1, 2, 3}, []int{view.Entries[0].Ordinal, view.Entries[1].Ordinal, view.Entries[2].Ordinal})

	// 本人行：is_self + 真实数值，且 My Rank 与榜内条目完全一致。
	require.True(t, view.Entries[2].IsSelf)
	require.Equal(t, LeaderboardIdentitySelf, view.Entries[2].Identity.Kind)
	require.Empty(t, view.Entries[2].Identity.Username)
	require.NotNil(t, view.Entries[2].TotalTokens)
	require.Equal(t, int64(50), *view.Entries[2].TotalTokens)
	require.NotNil(t, view.MyRank)
	require.Equal(t, view.Entries[2].Rank, view.MyRank.Rank)
	require.Equal(t, int64(50), view.MyRank.TotalTokens)
	require.Equal(t, int64(4), view.MyRank.SuccessfulRequests)
}

// 7.1 切换 Metric 后按新指标重新排名，不沿用另一个 Metric 的名次。
func TestLeaderboardService_QueryRanksPerMetric(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{entries: []LeaderboardUserMetrics{
		metricsRow(1, 1000, 1),
		metricsRow(2, 10, 99),
	}}
	repo := leaderboardUsers(activeLeaderboardUser(1, "alice"), activeLeaderboardUser(2, "bob"))
	svc := newTestLeaderboardService(cache, repo)

	byTokens, err := svc.Query(context.Background(), 2, LeaderboardWindowWeek, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.Equal(t, int64(2), byTokens.MyRank.Rank)

	byRequests, err := svc.Query(context.Background(), 2, LeaderboardWindowWeek, LeaderboardMetricSuccessfulRequests, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.Equal(t, int64(1), byRequests.MyRank.Rank)
	require.Equal(t, int64(1), byRequests.Entries[0].Rank)
	require.True(t, byRequests.Entries[0].IsSelf)
	// 每行仍同时给出两个 Metric 的值。
	require.NotNil(t, byRequests.Entries[1].TotalTokens)
	require.Equal(t, int64(1000), *byRequests.Entries[1].TotalTokens)
	require.NotNil(t, byRequests.Entries[1].SuccessfulRequests)
	require.Equal(t, int64(1), *byRequests.Entries[1].SuccessfulRequests)
}

// 7.1 查看者在该 Window 零用量时没有 My Rank。
func TestLeaderboardService_QueryZeroUsageViewerHasNilMyRank(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{entries: []LeaderboardUserMetrics{metricsRow(1, 100, 10)}}
	svc := newTestLeaderboardService(cache, leaderboardUsers(activeLeaderboardUser(1, "alice")))

	view, err := svc.Query(context.Background(), 42, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.Nil(t, view.MyRank)
	require.Len(t, view.Entries, 1)
}

// 7.1 渲染时剔除不合格用户：Rank 不重算、Ordinal 重排、participant_count 不扣减。
func TestLeaderboardService_QueryDropsIneligibleUsers(t *testing.T) {
	deletedAt := time.Now().Add(-time.Hour)
	cache := &fakeLeaderboardSnapshotCache{entries: []LeaderboardUserMetrics{
		metricsRow(1, 100, 10),
		metricsRow(2, 90, 9),
		metricsRow(3, 80, 8),
		metricsRow(4, 70, 7),
	}}
	disabled := activeLeaderboardUser(2, "bob")
	disabled.Status = StatusDisabled
	softDeleted := activeLeaderboardUser(3, "carol")
	softDeleted.DeletedAt = &deletedAt
	repo := leaderboardUsers(activeLeaderboardUser(1, "alice"), disabled, softDeleted, activeLeaderboardUser(4, "dave"))
	svc := newTestLeaderboardService(cache, repo)

	view, err := svc.Query(context.Background(), 1, LeaderboardWindowMonth, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)

	require.Len(t, view.Entries, 2)
	require.Equal(t, int64(1), view.Entries[0].Rank)
	require.Equal(t, int64(4), view.Entries[1].Rank, "剔除 MUST NOT 让剩余条目的 Rank 重算")
	require.Equal(t, []int{1, 2}, []int{view.Entries[0].Ordinal, view.Entries[1].Ordinal})
	require.Equal(t, int64(4), view.ParticipantCount, "participant_count 取 Snapshot 的 ZCARD，渲染剔除不扣减")

	// 被剔除者的任何身份信息都不下发。
	payload, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "bob")
	require.NotContains(t, string(payload), "carol")
}

// 7.1 快照里有、users 表里已查不到（软删除被仓储过滤）的条目同样被剔除。
func TestLeaderboardService_QueryDropsUsersMissingFromRepository(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{entries: []LeaderboardUserMetrics{
		metricsRow(1, 100, 10),
		metricsRow(2, 90, 9),
	}}
	svc := newTestLeaderboardService(cache, leaderboardUsers(activeLeaderboardUser(1, "alice")))

	view, err := svc.Query(context.Background(), 1, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.Len(t, view.Entries, 1)
	require.Equal(t, int64(2), view.ParticipantCount)
}

// 7.2 anonymous 档：他人条目只给相对百分比，本人条目与 my_rank 给真实数值。
func TestLeaderboardService_QueryAnonymousUsesRelativePercent(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{entries: []LeaderboardUserMetrics{
		metricsRow(1, 1000, 20),
		metricsRow(2, 500, 15),
		metricsRow(3, 250, 10),
		metricsRow(4, 100, 5),
		metricsRow(5, 40, 1),
	}}
	named := activeLeaderboardUser(1, "alice")
	named.LeaderboardNamedParticipation = true
	repo := leaderboardUsers(
		named,
		activeLeaderboardUser(2, "bob"),
		activeLeaderboardUser(3, "carol"),
		activeLeaderboardUser(4, "dave"),
		activeLeaderboardUser(5, "erin"),
	)
	svc := newTestLeaderboardService(cache, repo)

	view, err := svc.Query(context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeAnonymous, false)
	require.NoError(t, err)

	require.False(t, view.EntriesSuppressed)
	require.Equal(t, "5+", view.ParticipantCount)
	require.Len(t, view.Entries, 5)

	first := view.Entries[0]
	require.Equal(t, LeaderboardIdentityAnonymous, first.Identity.Kind, "anonymous 档下已开启实名的用户也保持匿名")
	require.Empty(t, first.Identity.Username)
	require.Nil(t, first.TotalTokens)
	require.Nil(t, first.SuccessfulRequests)
	require.NotNil(t, first.TotalTokensRelativePercent)
	require.Equal(t, 100, *first.TotalTokensRelativePercent)
	require.NotNil(t, first.SuccessfulRequestsRelativePercent)
	require.Equal(t, 100, *first.SuccessfulRequestsRelativePercent)

	second := view.Entries[1]
	require.Equal(t, 50, *second.TotalTokensRelativePercent)
	require.Equal(t, 75, *second.SuccessfulRequestsRelativePercent)

	self := view.Entries[2]
	require.True(t, self.IsSelf)
	require.Equal(t, LeaderboardIdentitySelf, self.Identity.Kind)
	require.NotNil(t, self.TotalTokens)
	require.Equal(t, int64(250), *self.TotalTokens)
	require.NotNil(t, self.SuccessfulRequests)
	require.Equal(t, int64(10), *self.SuccessfulRequests)
	require.Nil(t, self.TotalTokensRelativePercent)
	require.Nil(t, self.SuccessfulRequestsRelativePercent)

	require.NotNil(t, view.MyRank)
	require.Equal(t, int64(3), view.MyRank.Rank)
	require.Equal(t, int64(250), view.MyRank.TotalTokens)
	require.Equal(t, int64(10), view.MyRank.SuccessfulRequests)

	// 字段是否存在由档位决定，而不是把字段清零——客户端不能把缺席误读成 0。
	payload, err := json.Marshal(view.Entries[0])
	require.NoError(t, err)
	require.NotContains(t, string(payload), `"total_tokens"`)
	require.NotContains(t, string(payload), `"successful_requests"`)
	require.Contains(t, string(payload), `"total_tokens_relative_percent"`)
	require.Contains(t, string(payload), `"successful_requests_relative_percent"`)
}

// 7.2 anonymous 档参与人数少于 5 时抑制榜单条目，只保留本人行信息。
func TestLeaderboardService_QueryAnonymousSuppressesSmallWindow(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{entries: []LeaderboardUserMetrics{
		metricsRow(1, 100, 10),
		metricsRow(2, 90, 9),
		metricsRow(3, 80, 8),
		metricsRow(4, 70, 7),
	}}
	repo := leaderboardUsers(
		activeLeaderboardUser(1, "alice"),
		activeLeaderboardUser(2, "bob"),
		activeLeaderboardUser(3, "carol"),
		activeLeaderboardUser(4, "dave"),
	)
	svc := newTestLeaderboardService(cache, repo)

	view, err := svc.Query(context.Background(), 4, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeAnonymous, false)
	require.NoError(t, err)

	require.Equal(t, LeaderboardStatusReady, view.Status)
	require.True(t, view.EntriesSuppressed)
	require.NotNil(t, view.Entries)
	require.Empty(t, view.Entries)
	require.Equal(t, "<5", view.ParticipantCount)
	require.NotNil(t, view.MyRank)
	require.Equal(t, int64(4), view.MyRank.Rank)
	require.Equal(t, int64(70), view.MyRank.TotalTokens)
	require.Zero(t, repo.calls, "抑制态不需要身份，MUST NOT 再查 users")
}

// 7.2 named 档不套用人数门槛。
func TestLeaderboardService_QueryNamedIgnoresParticipantThreshold(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{entries: []LeaderboardUserMetrics{
		metricsRow(1, 100, 10),
		metricsRow(2, 90, 9),
		metricsRow(3, 80, 8),
	}}
	repo := leaderboardUsers(
		activeLeaderboardUser(1, "alice"),
		activeLeaderboardUser(2, "bob"),
		activeLeaderboardUser(3, "carol"),
	)
	view, err := newTestLeaderboardService(cache, repo).Query(context.Background(), 1, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.False(t, view.EntriesSuppressed)
	require.Len(t, view.Entries, 3)
	require.Equal(t, int64(3), view.ParticipantCount)
}

// 7.2 anonymous 档的 participant_count 走下界序列分档。
func TestLeaderboardParticipantCountBucket(t *testing.T) {
	for _, tc := range []struct {
		count int64
		want  string
	}{
		{0, "<5"},
		{4, "<5"},
		{5, "5+"},
		{9, "5+"},
		{10, "10+"},
		{19, "10+"},
		{20, "20+"},
		{137, "100+"},
		{999, "500+"},
		{1000, "1000+"},
		{99999, "10000+"},
	} {
		require.Equalf(t, tc.want, leaderboardParticipantBucket(tc.count), "count=%d", tc.count)
	}
}

// 7.3 named 档的身份形态：只有「已开启实名 + username 合格」才实名。
func TestLeaderboardService_QueryNamedIdentityFallsBackToAnonymous(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{entries: []LeaderboardUserMetrics{
		metricsRow(1, 100, 10),
		metricsRow(2, 90, 9),
		metricsRow(3, 80, 8),
		metricsRow(4, 70, 7),
	}}
	eligible := activeLeaderboardUser(1, "alice")
	eligible.LeaderboardNamedParticipation = true
	emailShaped := activeLeaderboardUser(2, "someone@example.com")
	emailShaped.LeaderboardNamedParticipation = true
	notNamed := activeLeaderboardUser(3, "bob")
	repo := leaderboardUsers(eligible, emailShaped, notNamed, activeLeaderboardUser(4, "viewer"))
	svc := newTestLeaderboardService(cache, repo)

	view, err := svc.Query(context.Background(), 4, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.Len(t, view.Entries, 4)

	require.Equal(t, LeaderboardIdentityNamed, view.Entries[0].Identity.Kind)
	require.Equal(t, "alice", view.Entries[0].Identity.Username)

	require.Equal(t, LeaderboardIdentityAnonymous, view.Entries[1].Identity.Kind, "username 是邮箱形态时回退匿名")
	require.Empty(t, view.Entries[1].Identity.Username)

	require.Equal(t, LeaderboardIdentityAnonymous, view.Entries[2].Identity.Kind, "未开启实名参与的用户回退匿名")
	require.Empty(t, view.Entries[2].Identity.Username)

	require.Equal(t, LeaderboardIdentitySelf, view.Entries[3].Identity.Kind)
	require.Empty(t, view.Entries[3].Identity.Username)

	// 未开启实名的用户数值仍然精确。
	require.NotNil(t, view.Entries[2].TotalTokens)
	require.Equal(t, int64(80), *view.Entries[2].TotalTokens)

	payload, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "user_id")
	require.NotContains(t, string(payload), "email")
	require.NotContains(t, string(payload), "someone@example.com")
	// 金额是第三个 Metric，named 档下与 tokens 一样精确下发（design §1）。
	require.Contains(t, string(payload), `"cost":`)
}

// 7.4 Snapshot 缺失（key 不存在）时是「正在计算」，不是空榜也不是 500。
func TestLeaderboardService_QueryComputingWhenSnapshotMissing(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{missing: true}
	repo := leaderboardUsers(activeLeaderboardUser(1, "alice"))
	view, err := newTestLeaderboardService(cache, repo).Query(context.Background(), 1, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)

	require.Equal(t, LeaderboardStatusComputing, view.Status)
	require.Nil(t, view.SnapshotUpdatedAt)
	require.NotNil(t, view.Entries)
	require.Empty(t, view.Entries)
	require.False(t, view.EntriesSuppressed)
	require.False(t, view.Stale)
	require.Nil(t, view.MyRank)
	require.Zero(t, repo.calls)

	payload, err := json.Marshal(view)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"snapshot_updated_at":null`)
	require.Contains(t, string(payload), `"entries":[]`)
}

// 7.4 Redis 不可用时同样是「正在计算」，不回源查 usage_logs、不 500。
func TestLeaderboardService_QueryComputingWhenCacheUnavailable(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{err: errors.New("redis down")}
	repo := leaderboardUsers(activeLeaderboardUser(1, "alice"))
	view, err := newTestLeaderboardService(cache, repo).Query(context.Background(), 1, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeAnonymous, false)
	require.NoError(t, err)
	require.Equal(t, LeaderboardStatusComputing, view.Status)
	require.Nil(t, view.SnapshotUpdatedAt)
	require.Empty(t, view.Entries)
	require.Nil(t, view.MyRank)
	require.Equal(t, "<5", view.ParticipantCount)
	require.Zero(t, repo.calls)
}

// 7.4 超过 15 分钟未推进时仍返回数据，但带陈旧标记。
func TestLeaderboardService_QueryStaleSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name      string
		age       time.Duration
		wantStale bool
	}{
		{"六分钟前不算陈旧", 6 * time.Minute, false},
		{"十六分钟前算陈旧", 16 * time.Minute, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := &fakeLeaderboardSnapshotCache{
				entries:   []LeaderboardUserMetrics{metricsRow(1, 100, 10)},
				updatedAt: time.Now().Add(-tc.age),
			}
			repo := leaderboardUsers(activeLeaderboardUser(1, "alice"))
			view, err := newTestLeaderboardService(cache, repo).Query(context.Background(), 1, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
			require.NoError(t, err)
			require.Equal(t, LeaderboardStatusReady, view.Status)
			require.Equal(t, tc.wantStale, view.Stale)
			require.NotNil(t, view.SnapshotUpdatedAt)
			require.Len(t, view.Entries, 1, "陈旧态 MUST 仍然返回数据")
		})
	}
}

// 7.4 Snapshot 已就绪但窗口内无人有用量：空态而不是「正在计算」。
func TestLeaderboardService_QueryReadyEmptyWindow(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{}
	repo := leaderboardUsers()
	view, err := newTestLeaderboardService(cache, repo).Query(context.Background(), 1, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.Equal(t, LeaderboardStatusReady, view.Status)
	require.False(t, view.EntriesSuppressed)
	require.Empty(t, view.Entries)
	require.Equal(t, int64(0), view.ParticipantCount)
	require.Nil(t, view.MyRank)
}

// off 档：普通用户 404（服务端也强制一次），管理员进入 Preview 并按 named 档渲染。
func TestLeaderboardService_QueryOffMode(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{entries: []LeaderboardUserMetrics{
		metricsRow(1, 100, 10),
		metricsRow(2, 90, 9),
	}}
	named := activeLeaderboardUser(1, "alice")
	named.LeaderboardNamedParticipation = true
	repo := leaderboardUsers(named, activeLeaderboardUser(2, "viewer"))
	svc := newTestLeaderboardService(cache, repo)

	_, err := svc.Query(context.Background(), 2, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeOff, false)
	require.ErrorIs(t, err, ErrLeaderboardNotFound)

	view, err := svc.Query(context.Background(), 2, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeOff, true)
	require.NoError(t, err)
	require.Equal(t, LeaderboardModeOff, view.Mode)
	require.True(t, view.Preview)
	require.Equal(t, int64(2), view.ParticipantCount, "Preview 的 participant_count 是精确整数")
	require.Equal(t, LeaderboardIdentityNamed, view.Entries[0].Identity.Kind, "Preview 按 named 档渲染")
	require.Equal(t, "alice", view.Entries[0].Identity.Username)
	require.NotNil(t, view.Entries[0].TotalTokens)
	require.Equal(t, int64(100), *view.Entries[0].TotalTokens)
}

// 非法档位一律 fail-closed 到 off。
func TestLeaderboardService_QueryUnknownModeFailsClosed(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{entries: []LeaderboardUserMetrics{metricsRow(1, 100, 10)}}
	svc := newTestLeaderboardService(cache, leaderboardUsers(activeLeaderboardUser(1, "alice")))
	_, err := svc.Query(context.Background(), 1, LeaderboardWindowToday, LeaderboardMetricTotalTokens, "public", false)
	require.ErrorIs(t, err, ErrLeaderboardNotFound)
}

// 非法 Window 在服务层同样被拒（handler 已先校验，这里是第二道）。
func TestLeaderboardService_QueryInvalidWindow(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{}
	svc := newTestLeaderboardService(cache, leaderboardUsers())
	_, err := svc.Query(context.Background(), 1, LeaderboardWindow("year"), LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.ErrorIs(t, err, ErrLeaderboardInvalidWindow)
}

// 身份查询只发生一次，且至多 51 行（Top 50 + 查看者）。
func TestLeaderboardService_QueryUsesSingleUserLookup(t *testing.T) {
	entries := make([]LeaderboardUserMetrics, 0, 60)
	users := make([]User, 0, 60)
	for i := int64(1); i <= 60; i++ {
		entries = append(entries, metricsRow(i, 1000-i, 100-i))
		users = append(users, activeLeaderboardUser(i, "user"))
	}
	cache := &fakeLeaderboardSnapshotCache{entries: entries}
	repo := leaderboardUsers(users...)
	view, err := newTestLeaderboardService(cache, repo).Query(context.Background(), 60, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)

	require.Len(t, view.Entries, LeaderboardTopEntryLimit)
	require.Equal(t, 1, repo.calls)
	require.LessOrEqual(t, repo.idsCount, LeaderboardTopEntryLimit+1)
	require.Equal(t, int64(60), view.ParticipantCount)
	// 第 51 名及之后不出现在 entries 里，但仍能从 my_rank 拿到自己的名次。
	require.NotNil(t, view.MyRank)
	require.Equal(t, int64(60), view.MyRank.Rank)
}

// ---------------------------------------------------------------------------
// 10.23 Highlights（趣味卡）与 Insights（洞察）的档位裁剪
// ---------------------------------------------------------------------------

// leaderboardHighlightsFixture 是一份固定的 Highlights：三张人物卡各指向不同的 user_id，
// 其中效率之星（user 77）刻意不在榜单里，用来验证「榜外用户」形态。
//
// 六张之最与模型偏好画像同样在这里：它们与四张卡存在同一个 highlights JSON 里（design D20 / D22）。
// 单次最大（user 77）与画像的第二行（user 88）也刻意落在榜外，用来验证 ordinal 为 null。
func leaderboardHighlightsFixture() *LeaderboardHighlights {
	siteRate, peakHour, siteAvg := 0.712, 14, 17_341.5
	return &LeaderboardHighlights{
		TopTokens:   &LeaderboardHighlightUser{UserID: 1, TotalTokens: 1000, SuccessfulRequests: 20, SharePercent: 19, LeadPercent: 45},
		TopRequests: &LeaderboardHighlightUser{UserID: 4, TotalTokens: 100, SuccessfulRequests: 30, SharePercent: 23, LeadPercent: 23},
		CacheKing:   &LeaderboardCacheKing{UserID: 77, CacheHitRate: 0.873, DominantModel: "claude-sonnet-5"},
		Site: LeaderboardSiteSummary{
			// 站点合计的金额是 micros（7.8 USD），下发时才换回 USD。
			TotalTokens: 21_400_000, SuccessfulRequests: 1234, CostMicros: 7_800_000, ParticipantCount: 137,
			CacheHitRate: &siteRate, PeakHour: &peakHour, AvgTokensPerRequest: &siteAvg,
		},
		Extremes: &LeaderboardExtremes{
			NightOwl:  &LeaderboardExtremeNightOwl{UserID: 1, NightTokens: 640_000, NightSharePercent: 64},
			Rising:    &LeaderboardExtremeRising{UserID: 2, ChangePercent: 210},
			Omnivore:  &LeaderboardExtremeOmnivore{UserID: 3, DistinctModels: 7},
			Talker:    &LeaderboardExtremeTalker{UserID: 4, OutputSharePercent: 38},
			MaxSingle: &LeaderboardExtremeMaxSingle{UserID: 77, MaxSingleTokens: 128_000, RatioToMedian: 4.2},
			Streak:    &LeaderboardExtremeStreak{UserID: 3, Days: 23},
		},
		Profiles: []LeaderboardProfile{
			{UserID: 1, Models: []LeaderboardProfileModel{
				{Model: "claude-sonnet-5", SharePercent: 62},
				{Model: "claude-opus-5", SharePercent: 28},
			}},
			{UserID: 88, Models: []LeaderboardProfileModel{{Model: "gemini-3-pro", SharePercent: 100}}},
		},
	}
}

// leaderboardWeeklyRhythmFixture 是一张 7 × 24 的等级表：只有周一 9 点是满格，
// 其余为 0——足够验证「两档完全相同」与「按行拷贝不共享底层数组」。
func leaderboardWeeklyRhythmFixture() [][]int {
	rhythm := make([][]int, 7)
	for weekday := range rhythm {
		rhythm[weekday] = make([]int, 24)
	}
	rhythm[0][9] = 4
	rhythm[2][21] = 2
	return rhythm
}

func leaderboardInsightsFixture() *LeaderboardInsights {
	return &LeaderboardInsights{
		ModelsToday: []LeaderboardModelInsight{
			{Model: "claude-sonnet-5", SuccessfulRequests: 587, SharePercent: 47},
			{Model: "claude-opus-5", SuccessfulRequests: 213, SharePercent: 17},
		},
		Daily30: []LeaderboardDailyInsight{
			{Date: "2026-09-10", Requests: 120, TotalTokens: 900, RelativePercent: 45},
			{Date: "2026-09-11", Requests: 260, TotalTokens: 2000, RelativePercent: 100},
		},
		HourlyToday: []LeaderboardHourlyInsight{
			{Hour: 14, Requests: 88, RelativePercent: 100},
			{Hour: 15, Requests: 44, RelativePercent: 50},
		},
		CacheToday: &LeaderboardCacheInsight{CacheHitRate: 0.712, CacheReadTokens: 8_600_000, InputTokens: 3_400_000},
		Month:      &LeaderboardMonthInsight{TotalTokens: 165_000_000, ChangePercent: 18},

		PlatformsToday: []LeaderboardPlatformInsight{
			{Platform: "anthropic", SuccessfulRequests: 812, SharePercent: 61},
			{Platform: "openai", SuccessfulRequests: 331, SharePercent: 25},
		},
		WeeklyRhythm: leaderboardWeeklyRhythmFixture(),
		CompositionToday: &LeaderboardCompositionInsight{
			InputTokens: 3_400_000, OutputTokens: 1_200_000,
			CacheCreationTokens: 900_000, CacheReadTokens: 8_600_000,
			InputPercent: 24, OutputPercent: 8, CacheCreationPercent: 6, CacheReadPercent: 61,
		},
		CacheTrend14: []LeaderboardCacheTrendInsight{
			{Date: "2026-09-10", CacheHitRate: 0.62},
			{Date: "2026-09-11", CacheHitRate: 0.712},
		},
	}
}

// leaderboardHighlightsCache 是一份带 Highlights 与 Insights 的五人快照：
// 五个人正好压在 anonymous 档的人数门槛之上，因此条目不会被抑制。
func leaderboardHighlightsCache() *fakeLeaderboardSnapshotCache {
	return &fakeLeaderboardSnapshotCache{
		entries: []LeaderboardUserMetrics{
			metricsRow(1, 1000, 20),
			metricsRow(2, 500, 15),
			metricsRow(3, 250, 10),
			metricsRow(4, 100, 30),
			metricsRow(5, 40, 1),
		},
		highlights: leaderboardHighlightsFixture(),
		insights:   leaderboardInsightsFixture(),
	}
}

func leaderboardHighlightsUsers() *fakeLeaderboardUserRepo {
	named := activeLeaderboardUser(1, "alice")
	named.LeaderboardNamedParticipation = true
	return leaderboardUsers(
		named,
		activeLeaderboardUser(2, "bob"),
		activeLeaderboardUser(3, "carol"),
		activeLeaderboardUser(4, "dave"),
		activeLeaderboardUser(5, "erin"),
	)
}

// 10.23 named 档：Highlights 与 Insights 的绝对量齐备，相对量同样下发。
func TestLeaderboardService_QueryNamedHighlightsAndInsights(t *testing.T) {
	cache := leaderboardHighlightsCache()
	repo := leaderboardHighlightsUsers()
	svc := newTestLeaderboardService(cache, repo)

	view, err := svc.Query(context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.Equal(t, 1, repo.calls, "Highlights 的身份复用榜单那一次查询，MUST NOT 再查一次 users")

	require.NotNil(t, view.Highlights)
	top := view.Highlights.TopTokens
	require.NotNil(t, top)
	require.Equal(t, LeaderboardIdentityNamed, top.Identity.Kind)
	require.Equal(t, "alice", top.Identity.Username)
	require.NotNil(t, top.Ordinal)
	require.Equal(t, 1, *top.Ordinal)
	require.NotNil(t, top.TotalTokens)
	require.Equal(t, int64(1000), *top.TotalTokens)
	require.NotNil(t, top.SuccessfulRequests)
	require.Equal(t, int64(20), *top.SuccessfulRequests)
	require.Equal(t, 19, top.SharePercent)
	require.Equal(t, 45, top.LeadPercent)

	// 最勤快是 user 4：Ordinal 取当前 Window + Metric 榜单上的行序号（第 4 行），
	// 与它在「成功请求数」上的名次无关。
	requests := view.Highlights.TopRequests
	require.NotNil(t, requests)
	require.NotNil(t, requests.Ordinal)
	require.Equal(t, 4, *requests.Ordinal)
	require.Equal(t, LeaderboardIdentityAnonymous, requests.Identity.Kind, "未开启实名参与的用户在 named 档仍是匿名形态")

	king := view.Highlights.CacheKing
	require.NotNil(t, king)
	require.Nil(t, king.Ordinal, "榜外用户的 ordinal MUST 为 null")
	require.Equal(t, LeaderboardIdentityAnonymous, king.Identity.Kind)
	require.Empty(t, king.Identity.Username)
	require.InDelta(t, 0.873, king.CacheHitRate, 1e-9)
	require.Equal(t, "claude-sonnet-5", king.DominantModel)

	site := view.Highlights.Site
	require.NotNil(t, site.TotalTokens)
	require.Equal(t, int64(21_400_000), *site.TotalTokens)
	require.NotNil(t, site.SuccessfulRequests)
	require.Equal(t, int64(1234), *site.SuccessfulRequests)
	require.Equal(t, int64(137), site.ParticipantCount)
	require.NotNil(t, site.CacheHitRate)
	require.InDelta(t, 0.712, *site.CacheHitRate, 1e-9)
	require.NotNil(t, site.PeakHour)
	require.Equal(t, 14, *site.PeakHour)

	require.NotNil(t, view.Insights)
	require.Len(t, view.Insights.ModelsToday, 2)
	require.NotNil(t, view.Insights.ModelsToday[0].SuccessfulRequests)
	require.Equal(t, int64(587), *view.Insights.ModelsToday[0].SuccessfulRequests)
	require.Equal(t, 47, view.Insights.ModelsToday[0].SharePercent)

	require.Len(t, view.Insights.Daily30, 2)
	require.NotNil(t, view.Insights.Daily30[1].TotalTokens)
	require.Equal(t, int64(2000), *view.Insights.Daily30[1].TotalTokens)
	require.NotNil(t, view.Insights.Daily30[1].Requests)
	require.Equal(t, int64(260), *view.Insights.Daily30[1].Requests)
	require.Equal(t, 100, view.Insights.Daily30[1].RelativePercent)
	require.Equal(t, "2026-09-11", view.Insights.Daily30[1].Date)

	require.Len(t, view.Insights.HourlyToday, 2)
	require.Equal(t, 14, view.Insights.HourlyToday[0].Hour)
	require.NotNil(t, view.Insights.HourlyToday[0].Requests)
	require.Equal(t, int64(88), *view.Insights.HourlyToday[0].Requests)

	require.NotNil(t, view.Insights.CacheToday)
	require.InDelta(t, 0.712, view.Insights.CacheToday.CacheHitRate, 1e-9)
	require.NotNil(t, view.Insights.CacheToday.CacheReadTokens)
	require.Equal(t, int64(8_600_000), *view.Insights.CacheToday.CacheReadTokens)
	require.NotNil(t, view.Insights.CacheToday.InputTokens)

	require.NotNil(t, view.Insights.Month)
	require.NotNil(t, view.Insights.Month.TotalTokens)
	require.Equal(t, int64(165_000_000), *view.Insights.Month.TotalTokens)
	require.Equal(t, 18, view.Insights.Month.ChangePercent)
}

// 10.23 anonymous 档：他人与站点级的绝对量一个都不留，只剩相对量与比率。
func TestLeaderboardService_QueryAnonymousCropsHighlightsAndInsights(t *testing.T) {
	cache := leaderboardHighlightsCache()
	svc := newTestLeaderboardService(cache, leaderboardHighlightsUsers())

	view, err := svc.Query(context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeAnonymous, false)
	require.NoError(t, err)
	require.False(t, view.EntriesSuppressed)

	require.NotNil(t, view.Highlights)
	top := view.Highlights.TopTokens
	require.NotNil(t, top)
	require.Equal(t, LeaderboardIdentityAnonymous, top.Identity.Kind, "anonymous 档下已开启实名的领先者也保持匿名")
	require.Empty(t, top.Identity.Username)
	require.NotNil(t, top.Ordinal)
	require.Equal(t, 1, *top.Ordinal, "身份用榜单 Ordinal 假名，MUST NOT 用 Rank 代替")
	require.Nil(t, top.TotalTokens)
	require.Nil(t, top.SuccessfulRequests)
	require.Equal(t, 19, top.SharePercent)
	require.Equal(t, 45, top.LeadPercent)

	king := view.Highlights.CacheKing
	require.NotNil(t, king, "命中率与模型名两档都给")
	require.Nil(t, king.Ordinal)
	require.InDelta(t, 0.873, king.CacheHitRate, 1e-9)
	require.Equal(t, "claude-sonnet-5", king.DominantModel)

	site := view.Highlights.Site
	require.Nil(t, site.TotalTokens)
	require.Nil(t, site.SuccessfulRequests)
	require.Equal(t, "100+", site.ParticipantCount, "站点级人数只给分档字符串")
	require.NotNil(t, site.CacheHitRate)
	require.NotNil(t, site.PeakHour)

	require.NotNil(t, view.Insights)
	require.Nil(t, view.Insights.ModelsToday[0].SuccessfulRequests)
	require.Equal(t, 47, view.Insights.ModelsToday[0].SharePercent)
	require.Nil(t, view.Insights.Daily30[0].Requests)
	require.Nil(t, view.Insights.Daily30[0].TotalTokens)
	require.Equal(t, 100, view.Insights.Daily30[1].RelativePercent)
	require.Nil(t, view.Insights.HourlyToday[0].Requests)
	require.Equal(t, 14, view.Insights.HourlyToday[0].Hour)
	require.Equal(t, 100, view.Insights.HourlyToday[0].RelativePercent)
	require.Nil(t, view.Insights.CacheToday.CacheReadTokens)
	require.Nil(t, view.Insights.CacheToday.InputTokens)
	require.InDelta(t, 0.712, view.Insights.CacheToday.CacheHitRate, 1e-9)
	require.Nil(t, view.Insights.Month.TotalTokens)
	require.Equal(t, 18, view.Insights.Month.ChangePercent)

	// 档位决定字段是否存在，而不是把字段清零：整段 highlights + insights 里
	// 不得出现任何他人或站点级的绝对量字段名。
	payload, err := json.Marshal(struct {
		Highlights *LeaderboardHighlightsView `json:"highlights"`
		Insights   *LeaderboardInsightsView   `json:"insights"`
	}{view.Highlights, view.Insights})
	require.NoError(t, err)
	blob := string(payload)
	for _, field := range leaderboardAbsoluteFieldNames {
		require.NotContainsf(t, blob, field, "anonymous 档下 %s MUST 缺席", field)
	}
	for _, field := range []string{`"share_percent"`, `"lead_percent"`, `"relative_percent"`, `"cache_hit_rate"`, `"change_percent"`, `"peak_hour"`, `"ordinal"`} {
		require.Containsf(t, blob, field, "相对量与比率 %s 两档都下发", field)
	}
}

// leaderboardAbsoluteFieldNames 是「他人的或站点级的绝对量」字段名全集（含 v2 新增的六个）：
// anonymous 档下它们在 highlights + insights 里出现一次都是泄露。user_id 任何档位都不该出现。
// 顶层 viewer 与 my_rank 不在这份名单的适用范围内——那两处是查看者自己的数据。
var leaderboardAbsoluteFieldNames = []string{
	`"total_tokens"`,
	`"successful_requests"`,
	`"cost"`,
	`"requests"`,
	`"input_tokens"`,
	`"output_tokens"`,
	`"cache_creation_tokens"`,
	`"cache_read_tokens"`,
	`"night_tokens"`,
	`"max_single_tokens"`,
	`"user_id"`,
}

// 10.23 查看者本人是领先者：identity.kind 为 self，但数值仍按档位裁剪。
func TestLeaderboardService_QueryAnonymousSelfLeaderStillCropped(t *testing.T) {
	cache := leaderboardHighlightsCache()
	svc := newTestLeaderboardService(cache, leaderboardHighlightsUsers())

	view, err := svc.Query(context.Background(), 1, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeAnonymous, false)
	require.NoError(t, err)

	top := view.Highlights.TopTokens
	require.NotNil(t, top)
	require.Equal(t, LeaderboardIdentitySelf, top.Identity.Kind)
	require.NotNil(t, top.Ordinal)
	require.Equal(t, 1, *top.Ordinal)
	require.Nil(t, top.TotalTokens, "这张卡是给所有人看的同一份数据，MUST NOT 因为是本人而放行绝对值")
	require.Nil(t, top.SuccessfulRequests)
	require.Equal(t, 19, top.SharePercent)

	// 同一份响应里，本人的榜单条目与 my_rank 仍是真实数值。
	require.True(t, view.Entries[0].IsSelf)
	require.NotNil(t, view.Entries[0].TotalTokens)
	require.Equal(t, int64(1000), *view.Entries[0].TotalTokens)
	require.NotNil(t, view.MyRank)
	require.Equal(t, int64(1000), view.MyRank.TotalTokens)
}

// 10.23 领先者被剔除（禁用 / 软删除）时同样按榜外用户处理，不下发其身份。
func TestLeaderboardService_QueryHighlightLeaderIneligible(t *testing.T) {
	cache := leaderboardHighlightsCache()
	disabled := activeLeaderboardUser(1, "alice")
	disabled.LeaderboardNamedParticipation = true
	disabled.Status = StatusDisabled
	repo := leaderboardUsers(
		disabled,
		activeLeaderboardUser(2, "bob"),
		activeLeaderboardUser(3, "carol"),
		activeLeaderboardUser(4, "dave"),
		activeLeaderboardUser(5, "erin"),
	)
	view, err := newTestLeaderboardService(cache, repo).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)

	top := view.Highlights.TopTokens
	require.NotNil(t, top)
	require.Nil(t, top.Ordinal, "已被剔除的用户不在 entries 里，ordinal MUST 为 null")
	require.Equal(t, LeaderboardIdentityAnonymous, top.Identity.Kind)
	require.Empty(t, top.Identity.Username)

	payload, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "alice")
}

// 10.23 Preview 按 named 档渲染 Highlights 与 Insights。
func TestLeaderboardService_QueryPreviewHighlightsFollowNamed(t *testing.T) {
	cache := leaderboardHighlightsCache()
	view, err := newTestLeaderboardService(cache, leaderboardHighlightsUsers()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeOff, true)
	require.NoError(t, err)

	require.True(t, view.Preview)
	require.NotNil(t, view.Highlights.TopTokens.TotalTokens)
	require.Equal(t, int64(1000), *view.Highlights.TopTokens.TotalTokens)
	require.Equal(t, LeaderboardIdentityNamed, view.Highlights.TopTokens.Identity.Kind)
	require.Equal(t, int64(137), view.Highlights.Site.ParticipantCount)
	require.NotNil(t, view.Insights.Month.TotalTokens)
}

// 10.23 status = computing 时 highlights 为 null；insights 不随快照状态变化。
func TestLeaderboardService_QueryComputingDropsHighlightsKeepsInsights(t *testing.T) {
	cache := leaderboardHighlightsCache()
	cache.missing = true
	view, err := newTestLeaderboardService(cache, leaderboardHighlightsUsers()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)

	require.Equal(t, LeaderboardStatusComputing, view.Status)
	require.Nil(t, view.Highlights)
	require.NotNil(t, view.Insights, "Insights 是站点级洞察，与本 Window 的快照状态无关")

	payload, err := json.Marshal(view)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"highlights":null`, "区块为 null 时 MUST 序列化成 null，MUST NOT 被 omitempty 吞掉")
}

// 10.23 抑制态（anonymous 档人数过少）连 Highlights 一起不下发：
// 四张卡指名道姓地描述某个人，人数过少时与榜单条目一样接近可辨认。
func TestLeaderboardService_QuerySuppressedDropsHighlights(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{
		entries: []LeaderboardUserMetrics{
			metricsRow(1, 100, 10),
			metricsRow(2, 90, 9),
			metricsRow(3, 80, 8),
		},
		highlights: leaderboardHighlightsFixture(),
		insights:   leaderboardInsightsFixture(),
	}
	view, err := newTestLeaderboardService(cache, leaderboardHighlightsUsers()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeAnonymous, false)
	require.NoError(t, err)

	require.True(t, view.EntriesSuppressed)
	require.Nil(t, view.Highlights)
	require.NotNil(t, view.MyRank)
}

// 10.23 Highlights / Insights 的各区块独立降级：单块缺失只让那一块为 null。
func TestLeaderboardService_QueryPartialInsightsBlocks(t *testing.T) {
	cache := leaderboardHighlightsCache()
	cache.insights = &LeaderboardInsights{
		ModelsToday: leaderboardInsightsFixture().ModelsToday,
	}
	cache.highlights.CacheKing = nil
	view, err := newTestLeaderboardService(cache, leaderboardHighlightsUsers()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)

	require.NotNil(t, view.Highlights)
	require.Nil(t, view.Highlights.CacheKing, "无人达标时该卡 MUST 为 null，MUST NOT 用零值对象顶替")
	require.NotNil(t, view.Highlights.TopTokens)

	require.NotNil(t, view.Insights)
	require.NotEmpty(t, view.Insights.ModelsToday)
	require.Nil(t, view.Insights.Daily30)
	require.Nil(t, view.Insights.HourlyToday)
	require.Nil(t, view.Insights.CacheToday)
	require.Nil(t, view.Insights.Month)

	payload, err := json.Marshal(view.Insights)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"daily_30":null`)
	require.Contains(t, string(payload), `"cache_today":null`)
	require.Contains(t, string(payload), `"month":null`)
}

// 10.23 Highlights 与 Insights 都缺失（旧快照 / 作业未跑）时两个字段都是 null，
// 榜单本体照常返回。
func TestLeaderboardService_QueryWithoutHighlightsAndInsights(t *testing.T) {
	cache := leaderboardHighlightsCache()
	cache.highlights, cache.insights = nil, nil
	view, err := newTestLeaderboardService(cache, leaderboardHighlightsUsers()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)

	require.Nil(t, view.Highlights)
	require.Nil(t, view.Insights)
	require.Len(t, view.Entries, 5)

	payload, err := json.Marshal(view)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"highlights":null`)
	require.Contains(t, string(payload), `"insights":null`)
}

// leaderboardHintCache 是一份十二人快照：查看者（user 12）稳定排在第 12 名，
// 因此「你的位置」总有一句「进前 10 还差多少」的提示。
func leaderboardHintCache() (*fakeLeaderboardSnapshotCache, *fakeLeaderboardUserRepo) {
	entries := make([]LeaderboardUserMetrics, 0, 12)
	users := make([]User, 0, 12)
	for i := int64(1); i <= 12; i++ {
		entries = append(entries, metricsRow(i, (13-i)*100, 13-i))
		users = append(users, activeLeaderboardUser(i, "user"))
	}
	return &fakeLeaderboardSnapshotCache{entries: entries}, leaderboardUsers(users...)
}

// 「你的位置」的提示：named 档给绝对差额，单位随当前 Metric 而变。
func TestLeaderboardService_MyRankHintNamedGivesAbsoluteGap(t *testing.T) {
	cache, repo := leaderboardHintCache()
	svc := newTestLeaderboardService(cache, repo)

	byTokens, err := svc.Query(context.Background(), 12, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.NotNil(t, byTokens.MyRank)
	require.Equal(t, int64(12), byTokens.MyRank.Rank)
	require.NotNil(t, byTokens.MyRank.Hint)
	require.Equal(t, LeaderboardHintTokensToTop10, byTokens.MyRank.Hint.Kind)
	require.NotNil(t, byTokens.MyRank.Hint.Value)
	// 第 10 名是 300，本人是 100：再多 200 就够得着第 10 名。
	require.InDelta(t, 200.0, *byTokens.MyRank.Hint.Value, 1e-9)
	require.Nil(t, byTokens.MyRank.Hint.Self)
	require.Nil(t, byTokens.MyRank.Hint.Tenth)

	byRequests, err := svc.Query(context.Background(), 12, LeaderboardWindowToday, LeaderboardMetricSuccessfulRequests, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.NotNil(t, byRequests.MyRank.Hint)
	require.Equal(t, LeaderboardHintRequestsToTop10, byRequests.MyRank.Hint.Kind)
	require.InDelta(t, 2.0, *byRequests.MyRank.Hint.Value, 1e-9)
}

// 「你的位置」的提示：anonymous 档只给相对第一名的百分比，MUST NOT 含他人绝对量。
func TestLeaderboardService_MyRankHintAnonymousGivesRelativePercent(t *testing.T) {
	cache, repo := leaderboardHintCache()
	view, err := newTestLeaderboardService(cache, repo).Query(
		context.Background(), 12, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeAnonymous, false)
	require.NoError(t, err)

	hint := view.MyRank.Hint
	require.NotNil(t, hint)
	require.Equal(t, LeaderboardHintRelativePercent, hint.Kind)
	require.Nil(t, hint.Value, "绝对差额 MUST NOT 出现在 anonymous 档的提示里")
	require.NotNil(t, hint.Self)
	require.Equal(t, 8, *hint.Self) // 100 / 1200
	require.NotNil(t, hint.Tenth)
	require.Equal(t, 25, *hint.Tenth) // 300 / 1200

	payload, err := json.Marshal(hint)
	require.NoError(t, err)
	require.NotContains(t, string(payload), `"value"`)
}

// 已经在前 10 名之内（或人数不足 10 人）时没有可提示的差距，hint 缺席。
func TestLeaderboardService_MyRankHintAbsentInsideTopTen(t *testing.T) {
	cache, repo := leaderboardHintCache()
	svc := newTestLeaderboardService(cache, repo)

	inside, err := svc.Query(context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.NotNil(t, inside.MyRank)
	require.Equal(t, int64(3), inside.MyRank.Rank)
	require.Nil(t, inside.MyRank.Hint)

	small := &fakeLeaderboardSnapshotCache{entries: []LeaderboardUserMetrics{
		metricsRow(1, 100, 10),
		metricsRow(2, 90, 9),
	}}
	view, err := newTestLeaderboardService(small, leaderboardUsers(
		activeLeaderboardUser(1, "alice"), activeLeaderboardUser(2, "bob"))).Query(
		context.Background(), 2, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.NotNil(t, view.MyRank)
	require.Nil(t, view.MyRank.Hint, "不足 10 人时人人都在前 10，没有可提示的差距")

	payload, err := json.Marshal(view.MyRank)
	require.NoError(t, err)
	require.NotContains(t, string(payload), `"hint"`)
}

// ---------------------------------------------------------------------------
// Cost（消费金额）：第三个 Metric 在 named / anonymous / Preview 三档下的形态
// ---------------------------------------------------------------------------

// leaderboardCostCache 是一份带金额的五人快照：金额顺序与 tokens 顺序刻意不同
// （tokens 第 1 的 user 1 金额只排第 3），因此「按 cost 排」必须给出另一套名次，
// 而不是沿用 tokens 的。查看者是 user 3：金额第 2、tokens 第 3。
func leaderboardCostCache() *fakeLeaderboardSnapshotCache {
	cache := leaderboardHighlightsCache()
	cache.entries = []LeaderboardUserMetrics{
		costMetricsRow(1, 1000, 20, 1),
		costMetricsRow(2, 500, 15, 4),
		costMetricsRow(3, 250, 10, 2.5),
		costMetricsRow(4, 100, 30, 0.25),
		costMetricsRow(5, 40, 1, 0.05),
	}
	return cache
}

// named 档：按金额重新排名，entries / my_rank / site 的金额都是精确 USD。
func TestLeaderboardService_QueryCostMetricNamed(t *testing.T) {
	cache := leaderboardCostCache()
	view, err := newTestLeaderboardService(cache, leaderboardHighlightsUsers()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricCost, LeaderboardModeNamed, false)
	require.NoError(t, err)

	require.Equal(t, string(LeaderboardMetricCost), view.Metric)
	require.Len(t, view.Entries, 5)
	require.Equal(t, []int64{1, 2, 3, 4, 5},
		[]int64{view.Entries[0].Rank, view.Entries[1].Rank, view.Entries[2].Rank, view.Entries[3].Rank, view.Entries[4].Rank})

	// 金额最高的 user 2 排第 1，tokens 最高的 user 1 只排第 3：名次跟着 Metric 走。
	require.NotNil(t, view.Entries[0].Cost)
	require.InDelta(t, 4.0, *view.Entries[0].Cost, 1e-9)
	require.NotNil(t, view.Entries[0].TotalTokens)
	require.Equal(t, int64(500), *view.Entries[0].TotalTokens, "另外两个 Metric 的值照常同时下发")
	require.NotNil(t, view.Entries[2].Cost)
	require.InDelta(t, 1.0, *view.Entries[2].Cost, 1e-9)
	require.Nil(t, view.Entries[0].CostRelativePercent, "named 档没有相对百分比")

	self := view.Entries[1]
	require.True(t, self.IsSelf)
	require.NotNil(t, self.Cost)
	require.InDelta(t, 2.5, *self.Cost, 1e-9)

	require.NotNil(t, view.MyRank)
	require.Equal(t, int64(2), view.MyRank.Rank)
	require.InDelta(t, 2.5, view.MyRank.Cost, 1e-9)
	require.Equal(t, int64(250), view.MyRank.TotalTokens)

	require.NotNil(t, view.Highlights)
	require.NotNil(t, view.Highlights.Site.Cost)
	require.InDelta(t, 7.8, *view.Highlights.Site.Cost, 1e-9, "站点合计由 micros 换回 USD")

	payload, err := json.Marshal(view.MyRank)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"cost":2.5`, "本人的金额始终下发，MUST NOT 用指针表达缺席")
}

// anonymous 档：他人条目只给相对第一名的百分比，本人行与 my_rank 仍是真实金额，
// 站点合计的金额一并缺席。
func TestLeaderboardService_QueryCostMetricAnonymous(t *testing.T) {
	cache := leaderboardCostCache()
	view, err := newTestLeaderboardService(cache, leaderboardHighlightsUsers()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricCost, LeaderboardModeAnonymous, false)
	require.NoError(t, err)

	require.False(t, view.EntriesSuppressed)
	require.Len(t, view.Entries, 5)

	first := view.Entries[0]
	require.Nil(t, first.Cost, "他人的绝对金额 MUST 缺席而不是清零")
	require.NotNil(t, first.CostRelativePercent)
	require.Equal(t, 100, *first.CostRelativePercent)
	// 分母是该 Window 的 CostMicros 最大值（4 USD）：1 / 4 = 25%，0.25 / 4 向下取整是 6%。
	require.Equal(t, 25, *view.Entries[2].CostRelativePercent)
	require.Equal(t, 6, *view.Entries[3].CostRelativePercent)

	self := view.Entries[1]
	require.True(t, self.IsSelf)
	require.NotNil(t, self.Cost)
	require.InDelta(t, 2.5, *self.Cost, 1e-9)
	require.Nil(t, self.CostRelativePercent)

	require.NotNil(t, view.MyRank)
	require.InDelta(t, 2.5, view.MyRank.Cost, 1e-9)
	require.Nil(t, view.Highlights.Site.Cost, "站点级金额与 tokens 同一条裁剪规则")

	payload, err := json.Marshal(first)
	require.NoError(t, err)
	require.NotContains(t, string(payload), `"cost"`)
	require.Contains(t, string(payload), `"cost_relative_percent"`)
}

// Preview（off 档的管理员）按 named 档渲染金额。
func TestLeaderboardService_QueryCostMetricPreview(t *testing.T) {
	cache := leaderboardCostCache()
	view, err := newTestLeaderboardService(cache, leaderboardHighlightsUsers()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricCost, LeaderboardModeOff, true)
	require.NoError(t, err)

	require.True(t, view.Preview)
	require.NotNil(t, view.Entries[0].Cost)
	require.InDelta(t, 4.0, *view.Entries[0].Cost, 1e-9)
	require.Nil(t, view.Entries[0].CostRelativePercent)
	require.InDelta(t, 2.5, view.MyRank.Cost, 1e-9)
	require.NotNil(t, view.Highlights.Site.Cost)
	require.InDelta(t, 7.8, *view.Highlights.Site.Cost, 1e-9)
}

// leaderboardCostHintCache 是一份带金额的十二人快照：查看者（user 12）稳定排在第 12 名，
// 因此「你的位置」总有一句「再消费多少进前 10」的提示。
func leaderboardCostHintCache() (*fakeLeaderboardSnapshotCache, *fakeLeaderboardUserRepo) {
	entries := make([]LeaderboardUserMetrics, 0, 12)
	users := make([]User, 0, 12)
	for i := int64(1); i <= 12; i++ {
		entries = append(entries, costMetricsRow(i, (13-i)*100, 13-i, float64(13-i)*0.5))
		users = append(users, activeLeaderboardUser(i, "user"))
	}
	return &fakeLeaderboardSnapshotCache{entries: entries}, leaderboardUsers(users...)
}

// 「你的位置」在 cost 下是第三个 kind：差额按 micros 算完再换成 USD。
func TestLeaderboardService_MyRankHintCostToTop10(t *testing.T) {
	cache, repo := leaderboardCostHintCache()
	svc := newTestLeaderboardService(cache, repo)

	view, err := svc.Query(context.Background(), 12, LeaderboardWindowToday, LeaderboardMetricCost, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.NotNil(t, view.MyRank)
	require.Equal(t, int64(12), view.MyRank.Rank)

	hint := view.MyRank.Hint
	require.NotNil(t, hint)
	require.Equal(t, LeaderboardHintCostToTop10, hint.Kind)
	require.NotNil(t, hint.Value)
	// 第 10 名是 1.5 USD，本人是 0.5 USD：再消费 1 USD 就够得着。
	require.InDelta(t, 1.0, *hint.Value, 1e-9)
	require.Nil(t, hint.Self)
	require.Nil(t, hint.Tenth)

	// anonymous 档仍然只给相对第一名的百分比：他人的金额连差额形式都不出现。
	anonymous, err := svc.Query(context.Background(), 12, LeaderboardWindowToday, LeaderboardMetricCost, LeaderboardModeAnonymous, false)
	require.NoError(t, err)
	require.Equal(t, LeaderboardHintRelativePercent, anonymous.MyRank.Hint.Kind)
	require.Nil(t, anonymous.MyRank.Hint.Value)
	require.Equal(t, 8, *anonymous.MyRank.Hint.Self)   // 0.5 / 6
	require.Equal(t, 25, *anonymous.MyRank.Hint.Tenth) // 1.5 / 6

	// tokens / requests 的差额仍是整数形态：Value 改成 float64 不改变 JSON。
	byTokens, err := svc.Query(context.Background(), 12, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	payload, err := json.Marshal(byTokens.MyRank.Hint)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"value":200`)
}

// ---------------------------------------------------------------------------
// 11.37 v2：Extremes（之最）、新增 Insights 区块与顶层 viewer（「你的统计」）
// ---------------------------------------------------------------------------

// leaderboardViewerFixture 是一份「你的统计」：近三天的名次与本人 Top 2 模型。
func leaderboardViewerFixture() *fakeLeaderboardViewerRepo {
	day := func(offset int) time.Time {
		return time.Date(2026, 9, 12+offset, 0, 0, 0, 0, time.UTC)
	}
	return &fakeLeaderboardViewerRepo{
		history: []usagestats.LeaderboardRankHistoryRow{
			{UserID: 3, SnapshotDate: day(-2), RankTotalTokens: 18, RankSuccessfulRequests: 21},
			{UserID: 3, SnapshotDate: day(-1), RankTotalTokens: 12, RankSuccessfulRequests: 14},
			{UserID: 3, SnapshotDate: day(0), RankTotalTokens: 3, RankSuccessfulRequests: 5},
		},
		models: []usagestats.LeaderboardModelUsageRow{
			{Model: "claude-sonnet-5", SuccessfulRequests: 6},
			{Model: "claude-opus-5", SuccessfulRequests: 4},
		},
		total: 10,
	}
}

// 11.37 named 档：六张之最与五块新 Insights 的绝对量齐备，相对量与比率同样下发。
func TestLeaderboardService_QueryNamedExtremesAndNewInsights(t *testing.T) {
	cache := leaderboardHighlightsCache()
	repo := leaderboardHighlightsUsers()
	svc := newTestLeaderboardService(cache, repo)

	view, err := svc.Query(context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.Equal(t, 1, repo.calls, "之最与画像的身份复用榜单那一次查询，MUST NOT 再查一次 users")

	require.NotNil(t, view.Highlights)
	require.NotNil(t, view.Highlights.Site.AvgTokensPerRequest)
	require.InDelta(t, 17_341.5, *view.Highlights.Site.AvgTokensPerRequest, 1e-9)

	extremes := view.Highlights.Extremes
	require.NotNil(t, extremes)

	owl := extremes.NightOwl
	require.NotNil(t, owl)
	require.Equal(t, LeaderboardIdentityNamed, owl.Identity.Kind)
	require.Equal(t, "alice", owl.Identity.Username)
	require.NotNil(t, owl.Ordinal)
	require.Equal(t, 1, *owl.Ordinal)
	require.NotNil(t, owl.NightTokens)
	require.Equal(t, int64(640_000), *owl.NightTokens)
	require.Equal(t, 64, owl.NightSharePercent)

	require.NotNil(t, extremes.Rising)
	require.Equal(t, 210, extremes.Rising.ChangePercent)
	require.Equal(t, 2, *extremes.Rising.Ordinal)

	require.NotNil(t, extremes.Omnivore)
	require.Equal(t, 7, extremes.Omnivore.DistinctModels)

	require.NotNil(t, extremes.Talker)
	require.Equal(t, 38, extremes.Talker.OutputSharePercent)

	maxSingle := extremes.MaxSingle
	require.NotNil(t, maxSingle)
	require.Nil(t, maxSingle.Ordinal, "单次最大之最在榜外，ordinal MUST 为 null")
	require.Equal(t, LeaderboardIdentityAnonymous, maxSingle.Identity.Kind)
	require.NotNil(t, maxSingle.MaxSingleTokens)
	require.Equal(t, int64(128_000), *maxSingle.MaxSingleTokens)
	require.InDelta(t, 4.2, maxSingle.RatioToMedian, 1e-9)

	require.NotNil(t, extremes.Streak)
	require.Equal(t, 23, extremes.Streak.Days)
	require.Equal(t, LeaderboardIdentitySelf, extremes.Streak.Identity.Kind, "查看者自己是连续活跃之最")

	insights := view.Insights
	require.NotNil(t, insights)

	require.Len(t, insights.Profiles, 2)
	require.Equal(t, LeaderboardIdentityNamed, insights.Profiles[0].Identity.Kind)
	require.Equal(t, 1, *insights.Profiles[0].Ordinal)
	require.Len(t, insights.Profiles[0].Models, 2)
	require.Equal(t, "claude-sonnet-5", insights.Profiles[0].Models[0].Model)
	require.Equal(t, 62, insights.Profiles[0].Models[0].SharePercent)
	require.Nil(t, insights.Profiles[1].Ordinal, "榜外用户的画像同样只有身份形态，没有序号")

	require.Len(t, insights.PlatformsToday, 2)
	require.Equal(t, "anthropic", insights.PlatformsToday[0].Platform)
	require.NotNil(t, insights.PlatformsToday[0].SuccessfulRequests)
	require.Equal(t, int64(812), *insights.PlatformsToday[0].SuccessfulRequests)
	require.Equal(t, 61, insights.PlatformsToday[0].SharePercent)

	require.Len(t, insights.WeeklyRhythm, 7)
	require.Len(t, insights.WeeklyRhythm[0], 24)
	require.Equal(t, 4, insights.WeeklyRhythm[0][9])
	require.Equal(t, 0, insights.WeeklyRhythm[0][10], "没有请求的时段等级是真实的 0，不是缺数据")

	composition := insights.CompositionToday
	require.NotNil(t, composition)
	require.NotNil(t, composition.InputTokens)
	require.Equal(t, int64(3_400_000), *composition.InputTokens)
	require.NotNil(t, composition.OutputTokens)
	require.NotNil(t, composition.CacheCreationTokens)
	require.NotNil(t, composition.CacheReadTokens)
	require.Equal(t, 61, composition.CacheReadPercent)

	require.Len(t, insights.CacheTrend14, 2)
	require.Equal(t, "2026-09-11", insights.CacheTrend14[1].Date)
	require.InDelta(t, 0.712, insights.CacheTrend14[1].CacheHitRate, 1e-9)
}

// 11.37 anonymous 档：之最与新增区块里的绝对量一个都不留，只剩相对量、比率与等级。
func TestLeaderboardService_QueryAnonymousCropsExtremesAndNewInsights(t *testing.T) {
	cache := leaderboardHighlightsCache()
	svc := newTestLeaderboardService(cache, leaderboardHighlightsUsers())

	view, err := svc.Query(context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeAnonymous, false)
	require.NoError(t, err)

	extremes := view.Highlights.Extremes
	require.NotNil(t, extremes)
	require.Nil(t, extremes.NightOwl.NightTokens, "夜猫子的绝对 tokens MUST 缺席")
	require.Equal(t, 64, extremes.NightOwl.NightSharePercent)
	require.Equal(t, LeaderboardIdentityAnonymous, extremes.NightOwl.Identity.Kind, "anonymous 档下已开启实名的之最也保持匿名")
	require.Empty(t, extremes.NightOwl.Identity.Username)
	require.Equal(t, 1, *extremes.NightOwl.Ordinal, "身份用榜单 Ordinal 假名")
	require.Nil(t, extremes.MaxSingle.MaxSingleTokens)
	require.InDelta(t, 4.2, extremes.MaxSingle.RatioToMedian, 1e-9)
	require.Equal(t, 210, extremes.Rising.ChangePercent)
	require.Equal(t, 7, extremes.Omnivore.DistinctModels)
	require.Equal(t, 38, extremes.Talker.OutputSharePercent)
	require.Equal(t, 23, extremes.Streak.Days)

	// 全站平均每请求 tokens 是比率，anonymous 档同样下发。
	require.NotNil(t, view.Highlights.Site.AvgTokensPerRequest)

	insights := view.Insights
	require.Nil(t, insights.PlatformsToday[0].SuccessfulRequests)
	require.Equal(t, 61, insights.PlatformsToday[0].SharePercent)
	require.Nil(t, insights.CompositionToday.InputTokens)
	require.Nil(t, insights.CompositionToday.OutputTokens)
	require.Nil(t, insights.CompositionToday.CacheCreationTokens)
	require.Nil(t, insights.CompositionToday.CacheReadTokens)
	require.Equal(t, 24, insights.CompositionToday.InputPercent)
	require.Len(t, insights.Profiles, 2)
	require.Equal(t, 62, insights.Profiles[0].Models[0].SharePercent)
	require.Empty(t, insights.Profiles[0].Identity.Username)

	// 周内节奏两档完全相同。
	named, err := svc.Query(context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.Equal(t, named.Insights.WeeklyRhythm, insights.WeeklyRhythm)

	payload, err := json.Marshal(struct {
		Highlights *LeaderboardHighlightsView `json:"highlights"`
		Insights   *LeaderboardInsightsView   `json:"insights"`
	}{view.Highlights, view.Insights})
	require.NoError(t, err)
	blob := string(payload)
	for _, field := range leaderboardAbsoluteFieldNames {
		require.NotContainsf(t, blob, field, "anonymous 档下 %s MUST 缺席", field)
	}
	for _, field := range []string{
		`"night_share_percent"`, `"change_percent"`, `"distinct_models"`, `"output_share_percent"`,
		`"ratio_to_median"`, `"days"`, `"weekly_rhythm"`, `"cache_trend_14"`, `"avg_tokens_per_request"`,
	} {
		require.Containsf(t, blob, field, "相对量、比率与等级 %s 两档都下发", field)
	}
}

// 11.37 Preview 按 named 档渲染之最与新增区块。
func TestLeaderboardService_QueryPreviewExtremesFollowNamed(t *testing.T) {
	view, err := newTestLeaderboardService(leaderboardHighlightsCache(), leaderboardHighlightsUsers()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeOff, true)
	require.NoError(t, err)

	require.True(t, view.Preview)
	require.NotNil(t, view.Highlights.Extremes.NightOwl.NightTokens)
	require.NotNil(t, view.Highlights.Extremes.MaxSingle.MaxSingleTokens)
	require.NotNil(t, view.Insights.PlatformsToday[0].SuccessfulRequests)
	require.NotNil(t, view.Insights.CompositionToday.InputTokens)
	require.Equal(t, LeaderboardIdentityNamed, view.Highlights.Extremes.NightOwl.Identity.Kind)
}

// 11.37 rising 只在 today 窗口存在：week / month 下该项为 null，页面不渲染这张卡。
// 之最由同一个包里的 computeLeaderboardExtremes 算出，因此这条同时钉住计算与下发两侧。
func TestLeaderboardService_QueryRisingOnlyInTodayWindow(t *testing.T) {
	entries := []LeaderboardUserMetrics{
		{UserID: 1, TotalTokens: 1000, SuccessfulRequests: 20, YesterdayTokens: 100},
		{UserID: 2, TotalTokens: 500, SuccessfulRequests: 15, YesterdayTokens: 400},
		{UserID: 3, TotalTokens: 250, SuccessfulRequests: 10},
		{UserID: 4, TotalTokens: 100, SuccessfulRequests: 30},
		{UserID: 5, TotalTokens: 40, SuccessfulRequests: 1},
	}
	for _, tc := range []struct {
		window     LeaderboardWindow
		wantRising bool
	}{
		{LeaderboardWindowToday, true},
		{LeaderboardWindowWeek, false},
		{LeaderboardWindowMonth, false},
	} {
		t.Run(string(tc.window), func(t *testing.T) {
			highlights := leaderboardHighlightsFixture()
			highlights.Extremes = computeLeaderboardExtremes(entries, tc.window, nil)
			cache := leaderboardHighlightsCache()
			cache.entries, cache.highlights = entries, highlights

			view, err := newTestLeaderboardService(cache, leaderboardHighlightsUsers()).Query(
				context.Background(), 3, tc.window, LeaderboardMetricTotalTokens, LeaderboardModeAnonymous, false)
			require.NoError(t, err)

			rising := view.Highlights.Extremes.Rising
			if !tc.wantRising {
				require.Nil(t, rising)
				payload, err := json.Marshal(view.Highlights.Extremes)
				require.NoError(t, err)
				require.Contains(t, string(payload), `"rising":null`, "缺席的之最 MUST 序列化成 null")
				return
			}
			require.NotNil(t, rising)
			require.Equal(t, 900, rising.ChangePercent)
		})
	}
}

// 11.37 之最与画像整块缺失（旧快照写的是没有 extremes 的 JSON）时两个字段都是 null。
func TestLeaderboardService_QueryWithoutExtremesAndProfiles(t *testing.T) {
	cache := leaderboardHighlightsCache()
	cache.highlights.Extremes, cache.highlights.Profiles = nil, nil
	view, err := newTestLeaderboardService(cache, leaderboardHighlightsUsers()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)

	require.NotNil(t, view.Highlights, "四张卡照常")
	require.Nil(t, view.Highlights.Extremes)
	require.Nil(t, view.Insights.Profiles)

	payload, err := json.Marshal(view)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"extremes":null`)
	require.Contains(t, string(payload), `"profiles":null`)
}

// 画像存在该 Window 的 highlights 里、下发在 insights 下（design D22）：它的可用性只随
// highlights 走。站点级 insights 那一个 key 读不到时，已经读到的画像 MUST NOT 跟着被丢掉。
func TestLeaderboardService_QueryProfilesSurviveMissingInsights(t *testing.T) {
	cache := leaderboardHighlightsCache()
	cache.insights = nil
	view, err := newTestLeaderboardService(cache, leaderboardHighlightsUsers()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)

	require.NotNil(t, view.Insights, "只为挂上画像补一个空壳，而不是整块丢掉")
	require.NotEmpty(t, view.Insights.Profiles)
	require.Nil(t, view.Insights.ModelsToday, "其余区块仍然缺席，不凭空造数")
	require.Nil(t, view.Insights.CacheToday)

	payload, err := json.Marshal(view.Insights)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"models_today":null`)
}

// 11.37 新增 Insights 区块各自独立降级：来源缺行时该块为 null，MUST NOT 用 0 填充。
func TestLeaderboardService_QueryPartialNewInsightBlocks(t *testing.T) {
	cache := leaderboardHighlightsCache()
	cache.insights = &LeaderboardInsights{
		PlatformsToday: leaderboardInsightsFixture().PlatformsToday,
	}
	view, err := newTestLeaderboardService(cache, leaderboardHighlightsUsers()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)

	require.NotEmpty(t, view.Insights.PlatformsToday)
	require.Nil(t, view.Insights.WeeklyRhythm, "近 28 天小时桶一行都没有时整块为 null")
	require.Nil(t, view.Insights.CompositionToday)
	require.Nil(t, view.Insights.CacheTrend14)
	require.NotEmpty(t, view.Insights.Profiles, "画像不依赖预聚合表，照常返回")

	payload, err := json.Marshal(view.Insights)
	require.NoError(t, err)
	for _, field := range []string{`"weekly_rhythm":null`, `"composition_today":null`, `"cache_trend_14":null`} {
		require.Contains(t, string(payload), field)
	}
}

// 11.37 viewer 在任何档位下都是查看者本人的真实数据，两档逐字节相同。
func TestLeaderboardService_QueryViewerIsIdenticalAcrossModes(t *testing.T) {
	viewerRepo := leaderboardViewerFixture()
	svc := NewLeaderboardService(leaderboardHighlightsCache(), leaderboardHighlightsUsers(), viewerRepo)

	namedView, err := svc.Query(context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	anonymousView, err := svc.Query(context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeAnonymous, false)
	require.NoError(t, err)

	require.Equal(t, int64(3), viewerRepo.historyUserID, "名次走势只查查看者本人")
	require.Equal(t, int64(3), viewerRepo.modelUserID, "模型偏好只查查看者本人")
	require.Equal(t, leaderboardViewerModelsLimit, viewerRepo.modelLimit)

	viewer := namedView.Viewer
	require.NotNil(t, viewer)
	require.Len(t, viewer.RankHistory, 3)
	require.Equal(t, "2026-09-10", viewer.RankHistory[0].Date)
	require.Equal(t, 18, viewer.RankHistory[0].Rank)
	require.Equal(t, 3, viewer.RankHistory[2].Rank)

	require.Len(t, viewer.Models, 2)
	require.Equal(t, "claude-sonnet-5", viewer.Models[0].Model)
	require.Equal(t, int64(6), viewer.Models[0].SuccessfulRequests)
	require.Equal(t, 60, viewer.Models[0].SharePercent)
	require.Equal(t, 40, viewer.Models[1].SharePercent)

	require.Equal(t, namedView.Viewer, anonymousView.Viewer, "档位 MUST NOT 对 viewer 做任何裁剪")

	payload, err := json.Marshal(anonymousView.Viewer)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"successful_requests":6`, "本人的绝对量在任何档位都真实")
	require.NotContains(t, string(payload), `"user_id"`)
}

// 11.37 viewer 的两个比率来自本人在该 Window Hash 里的那一行，与全站值并排对比。
func TestLeaderboardService_QueryViewerRatios(t *testing.T) {
	cache := leaderboardHighlightsCache()
	cache.entries[2] = LeaderboardUserMetrics{
		UserID: 3, TotalTokens: 250, SuccessfulRequests: 10,
		InputTokens: 40, CacheReadTokens: 60,
	}
	view, err := NewLeaderboardService(cache, leaderboardHighlightsUsers(), leaderboardViewerFixture()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeAnonymous, false)
	require.NoError(t, err)

	require.NotNil(t, view.Viewer.CacheHitRate)
	require.InDelta(t, 0.6, *view.Viewer.CacheHitRate, 1e-9)
	require.NotNil(t, view.Viewer.AvgTokensPerRequest)
	require.InDelta(t, 25.0, *view.Viewer.AvgTokensPerRequest, 1e-9)
	require.NotNil(t, view.Highlights.Site.AvgTokensPerRequest, "全站的同名比率照常下发，页面才对得起来")
}

// 11.37 查看者本窗口零用量：models 是空数组、两个比率缺席，rank_history 照常，viewer 不为 null。
func TestLeaderboardService_QueryViewerZeroUsage(t *testing.T) {
	viewerRepo := leaderboardViewerFixture()
	viewerRepo.models, viewerRepo.total = nil, 0
	view, err := NewLeaderboardService(leaderboardHighlightsCache(), leaderboardHighlightsUsers(), viewerRepo).Query(
		context.Background(), 42, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)

	require.Nil(t, view.MyRank)
	require.NotNil(t, view.Viewer)
	require.NotNil(t, view.Viewer.Models)
	require.Empty(t, view.Viewer.Models)
	require.Nil(t, view.Viewer.CacheHitRate)
	require.Nil(t, view.Viewer.AvgTokensPerRequest)
	require.Len(t, view.Viewer.RankHistory, 3)

	payload, err := json.Marshal(view.Viewer)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"models":[]`, "空数组 MUST NOT 序列化成 null")
	require.NotContains(t, string(payload), `"cache_hit_rate"`)
	require.NotContains(t, string(payload), `"avg_tokens_per_request"`)
}

// 11.37 status = computing 时 highlights（连同 extremes 与 insights.profiles）不可用，
// 而 viewer 照常返回：它不出自 Snapshot。
func TestLeaderboardService_QueryViewerSurvivesComputing(t *testing.T) {
	cache := leaderboardHighlightsCache()
	cache.missing = true
	view, err := NewLeaderboardService(cache, leaderboardHighlightsUsers(), leaderboardViewerFixture()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)

	require.Equal(t, LeaderboardStatusComputing, view.Status)
	require.Nil(t, view.Highlights)
	require.NotNil(t, view.Insights)
	require.Nil(t, view.Insights.Profiles, "画像随 highlights 一起不可用")
	require.NotNil(t, view.Viewer)
	require.Len(t, view.Viewer.RankHistory, 3)
	require.Len(t, view.Viewer.Models, 2)

	payload, err := json.Marshal(view)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"highlights":null`)
	require.NotContains(t, string(payload), `"viewer":null`)
}

// 11.37 anonymous 档的抑制态同样保留 viewer：抑制的是他人条目，不是查看者自己的统计。
func TestLeaderboardService_QuerySuppressedKeepsViewer(t *testing.T) {
	cache := &fakeLeaderboardSnapshotCache{
		entries: []LeaderboardUserMetrics{
			metricsRow(1, 100, 10),
			metricsRow(2, 90, 9),
			metricsRow(3, 80, 8),
		},
		highlights: leaderboardHighlightsFixture(),
		insights:   leaderboardInsightsFixture(),
	}
	view, err := NewLeaderboardService(cache, leaderboardHighlightsUsers(), leaderboardViewerFixture()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeAnonymous, false)
	require.NoError(t, err)

	require.True(t, view.EntriesSuppressed)
	require.Nil(t, view.Highlights)
	require.NotNil(t, view.Viewer)
	require.Len(t, view.Viewer.Models, 2)
}

// 11.37 viewer.models 先读 60 秒缓存：命中时 MUST NOT 再查一次库，未命中时回源并回填。
func TestLeaderboardService_QueryViewerModelsUsesCache(t *testing.T) {
	t.Run("命中缓存不回源", func(t *testing.T) {
		cache := leaderboardHighlightsCache()
		cache.viewerModelsFound = true
		cache.viewerModels = []usagestats.LeaderboardModelUsageRow{{Model: "gemini-3-pro", SuccessfulRequests: 3}}
		cache.viewerModelsTotal = 4
		viewerRepo := leaderboardViewerFixture()

		view, err := NewLeaderboardService(cache, leaderboardHighlightsUsers(), viewerRepo).Query(
			context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
		require.NoError(t, err)

		require.Equal(t, 1, cache.viewerModelsReads)
		require.Zero(t, viewerRepo.modelCalls, "缓存命中时 MUST NOT 触达数据库")
		require.Zero(t, cache.viewerModelsSets)
		require.Len(t, view.Viewer.Models, 1)
		require.Equal(t, "gemini-3-pro", view.Viewer.Models[0].Model)
		require.Equal(t, 75, view.Viewer.Models[0].SharePercent)
	})

	t.Run("未命中时回源并回填", func(t *testing.T) {
		cache := leaderboardHighlightsCache()
		viewerRepo := leaderboardViewerFixture()

		view, err := NewLeaderboardService(cache, leaderboardHighlightsUsers(), viewerRepo).Query(
			context.Background(), 3, LeaderboardWindowWeek, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
		require.NoError(t, err)

		require.Equal(t, 1, viewerRepo.modelCalls)
		require.Equal(t, 1, cache.viewerModelsSets)
		require.Len(t, cache.viewerModelsSetTo, 2)
		require.Len(t, view.Viewer.Models, 2)

		// 回源区间就是当前 Window 的边界，与榜单用的是同一对起止。
		weekStart, weekEnd, ok := LeaderboardWindowBounds(LeaderboardWindowWeek, timezone.Now())
		require.True(t, ok)
		require.True(t, viewerRepo.modelStart.Equal(weekStart))
		require.True(t, viewerRepo.modelEnd.Equal(weekEnd))
	})

	t.Run("缓存读不动照样回源", func(t *testing.T) {
		cache := leaderboardHighlightsCache()
		cache.viewerModelsErr = errors.New("redis down")
		viewerRepo := leaderboardViewerFixture()

		view, err := NewLeaderboardService(cache, leaderboardHighlightsUsers(), viewerRepo).Query(
			context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
		require.NoError(t, err)
		require.Equal(t, 1, viewerRepo.modelCalls)
		require.Len(t, view.Viewer.Models, 2)
	})
}

// 11.37 viewer 的两条查询各自独立降级为空数组，MUST NOT 让整个响应失败。
func TestLeaderboardService_QueryViewerDegradesOnRepositoryError(t *testing.T) {
	viewerRepo := leaderboardViewerFixture()
	viewerRepo.historyErr = errors.New("db down")
	viewerRepo.modelsErr = errors.New("query timeout")

	view, err := NewLeaderboardService(leaderboardHighlightsCache(), leaderboardHighlightsUsers(), viewerRepo).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)

	require.NotNil(t, view.Viewer)
	require.Empty(t, view.Viewer.RankHistory)
	require.Empty(t, view.Viewer.Models)
	require.Len(t, view.Entries, 5, "榜单本体不受影响")

	payload, err := json.Marshal(view.Viewer)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"rank_history":[]`)
	require.Contains(t, string(payload), `"models":[]`)
}

// 11.37 窄仓储缺席（未注入）时 viewer 仍不是 null，只是两块为空数组。
func TestLeaderboardService_QueryViewerWithoutRepository(t *testing.T) {
	view, err := newTestLeaderboardService(leaderboardHighlightsCache(), leaderboardHighlightsUsers()).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)

	require.NotNil(t, view.Viewer)
	require.Empty(t, view.Viewer.RankHistory)
	require.Empty(t, view.Viewer.Models)
}

// leaderboardAvatarUsers 是头像用例共用的五人（压在 anonymous 档的人数门槛之上，
// 两档都不会被抑制）：1 实名且有小图、2 实名但只有外链头像（ThumbURL 为空）、
// 3 未开启实名、4 是查看者自己且也有小图（本人行 MUST NOT 带头像）、5 没有头像。
func leaderboardAvatarUsers() *fakeLeaderboardUserRepo {
	named := activeLeaderboardUser(1, "alice")
	named.LeaderboardNamedParticipation = true
	remoteOnly := activeLeaderboardUser(2, "bob")
	remoteOnly.LeaderboardNamedParticipation = true
	notNamed := activeLeaderboardUser(3, "carol")
	notNamed.LeaderboardNamedParticipation = true
	viewer := activeLeaderboardUser(4, "dave")
	viewer.LeaderboardNamedParticipation = true
	plain := activeLeaderboardUser(5, "erin")
	plain.LeaderboardNamedParticipation = true

	repo := leaderboardUsers(named, remoteOnly, notNamed, viewer, plain)
	repo.avatars = map[int64]*UserAvatar{
		1: {StorageProvider: "inline", URL: "data:image/png;base64,AAA", ThumbURL: leaderboardTestThumb},
		2: {StorageProvider: "remote_url", URL: "https://cdn.example.com/bob.png"},
		4: {StorageProvider: "inline", URL: "data:image/png;base64,DDD", ThumbURL: "data:image/jpeg;base64,SELF"},
	}
	return repo
}

const leaderboardTestThumb = "data:image/jpeg;base64,QUxJQ0U="

func leaderboardAvatarCache() *fakeLeaderboardSnapshotCache {
	return &fakeLeaderboardSnapshotCache{entries: []LeaderboardUserMetrics{
		metricsRow(1, 100, 10),
		metricsRow(2, 90, 9),
		metricsRow(3, 80, 8),
		metricsRow(4, 70, 7),
		metricsRow(5, 60, 6),
	}}
}

// 12.1 named 档：只有 named 形态的他人行带 avatar_url，且头像查询与 users 同一批 id、只发一次。
func TestLeaderboardService_QueryNamedAttachesAvatarThumb(t *testing.T) {
	repo := leaderboardAvatarUsers()
	// carol（user 3）开了实名但 username 合格，这里改成邮箱形态以外的另一条回退路径：
	// 直接关掉实名参与，身份因此落回 anonymous。
	user := repo.users[3]
	user.LeaderboardNamedParticipation = false
	repo.users[3] = user

	view, err := newTestLeaderboardService(leaderboardAvatarCache(), repo).Query(
		context.Background(), 4, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.Len(t, view.Entries, 5)

	require.Equal(t, LeaderboardIdentityNamed, view.Entries[0].Identity.Kind)
	require.Equal(t, leaderboardTestThumb, view.Entries[0].Identity.AvatarURL)

	require.Equal(t, LeaderboardIdentityNamed, view.Entries[1].Identity.Kind)
	require.Empty(t, view.Entries[1].Identity.AvatarURL, "只有外链头像（没有小图）的行 MUST NOT 带 avatar_url")

	require.Equal(t, LeaderboardIdentityAnonymous, view.Entries[2].Identity.Kind)
	require.Empty(t, view.Entries[2].Identity.AvatarURL, "anonymous 行带头像等于去匿名")

	require.Equal(t, LeaderboardIdentitySelf, view.Entries[3].Identity.Kind)
	require.Empty(t, view.Entries[3].Identity.AvatarURL, "本人头像由前端从个人资料取，后端 MUST NOT 下发")

	require.Equal(t, LeaderboardIdentityNamed, view.Entries[4].Identity.Kind)
	require.Empty(t, view.Entries[4].Identity.AvatarURL, "没有头像的 named 行不带这个字段，前端回退首字母")

	require.Equal(t, 1, repo.avatarCalls, "头像只查一次")
	require.Equal(t, repo.lastIDs, repo.lastAvatarIDs, "与 GetByIDs 同一批 id")

	payload, err := json.Marshal(view)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(payload), `"avatar_url"`), "omitempty：只有那一行带这个字段")
	require.NotContains(t, string(payload), "data:image/jpeg;base64,SELF")
}

// 12.2 anonymous 档 MUST NOT 发起头像查询：他人行本来就不带头像，多查一次既无用又是去匿名的入口。
func TestLeaderboardService_QueryAnonymousSkipsAvatarLookup(t *testing.T) {
	repo := leaderboardAvatarUsers()

	view, err := newTestLeaderboardService(leaderboardAvatarCache(), repo).Query(
		context.Background(), 4, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeAnonymous, false)
	require.NoError(t, err)
	require.Len(t, view.Entries, 5)
	require.Equal(t, 0, repo.avatarCalls, "anonymous 档整批不查头像")

	for i := range view.Entries {
		require.Emptyf(t, view.Entries[i].Identity.AvatarURL, "entries[%d]", i)
	}
	payload, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "avatar_url")
}

// 12.3 头像查询失败只降级这一个字段：榜单照常下发。
func TestLeaderboardService_QueryAvatarLookupErrorDegrades(t *testing.T) {
	repo := leaderboardAvatarUsers()
	repo.avatarsErr = errors.New("boom")

	view, err := newTestLeaderboardService(leaderboardAvatarCache(), repo).Query(
		context.Background(), 4, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.Len(t, view.Entries, 5)
	require.Equal(t, LeaderboardIdentityNamed, view.Entries[0].Identity.Kind)
	require.Equal(t, "alice", view.Entries[0].Identity.Username)
	require.NotNil(t, view.Entries[0].TotalTokens)

	for i := range view.Entries {
		require.Emptyf(t, view.Entries[i].Identity.AvatarURL, "entries[%d]", i)
	}
}

// 12.4 Highlights 与 Profiles 的身份从 slots 取，因此自动带上同一个 avatar_url，
// 且 MUST NOT 再查一次头像。
func TestLeaderboardService_QueryAvatarFlowsIntoHighlightsAndProfiles(t *testing.T) {
	repo := leaderboardHighlightsUsers()
	repo.avatars = map[int64]*UserAvatar{
		1: {StorageProvider: "inline", URL: "data:image/png;base64,AAA", ThumbURL: leaderboardTestThumb},
	}

	view, err := newTestLeaderboardService(leaderboardHighlightsCache(), repo).Query(
		context.Background(), 3, LeaderboardWindowToday, LeaderboardMetricTotalTokens, LeaderboardModeNamed, false)
	require.NoError(t, err)
	require.Equal(t, 1, repo.calls, "身份复用榜单那一次 users 查询")
	require.Equal(t, 1, repo.avatarCalls, "头像同样只查一次")

	require.Equal(t, leaderboardTestThumb, view.Entries[0].Identity.AvatarURL)

	require.NotNil(t, view.Highlights)
	require.NotNil(t, view.Highlights.TopTokens)
	require.Equal(t, LeaderboardIdentityNamed, view.Highlights.TopTokens.Identity.Kind)
	require.Equal(t, leaderboardTestThumb, view.Highlights.TopTokens.Identity.AvatarURL)

	require.NotNil(t, view.Insights)
	require.NotEmpty(t, view.Insights.Profiles)
	require.Equal(t, LeaderboardIdentityNamed, view.Insights.Profiles[0].Identity.Kind)
	require.Equal(t, leaderboardTestThumb, view.Insights.Profiles[0].Identity.AvatarURL)

	// 榜外用户（Profiles[1] 是 user 88）没有 slot，身份仍是匿名、也没有头像。
	require.Equal(t, LeaderboardIdentityAnonymous, view.Insights.Profiles[1].Identity.Kind)
	require.Empty(t, view.Insights.Profiles[1].Identity.AvatarURL)
}
