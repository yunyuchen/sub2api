package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newLeaderboardTestCache(t *testing.T, keyPrefix string) (*leaderboardCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cache := NewLeaderboardCache(rdb, &config.Config{
		Dashboard: config.DashboardCacheConfig{KeyPrefix: keyPrefix},
	})
	impl, ok := cache.(*leaderboardCache)
	require.True(t, ok)
	return impl, mr
}

func TestNewLeaderboardCacheKeyPrefix(t *testing.T) {
	cache, _ := newLeaderboardTestCache(t, "prod")
	require.Equal(t, "prod:", cache.keyPrefix)

	cache, _ = newLeaderboardTestCache(t, "staging:")
	require.Equal(t, "staging:", cache.keyPrefix)

	// 未配置时兜底 sub2api:，与 dashboard_cache.go 的行为一致。
	fallback, ok := NewLeaderboardCache(nil, nil).(*leaderboardCache)
	require.True(t, ok)
	require.Equal(t, "sub2api:", fallback.keyPrefix)
}

// key 必须带 cfg.Dashboard.KeyPrefix 与窗口起点，否则跨窗口会串味、跨环境会互相污染。
func TestLeaderboardCache_ReplaceSnapshot_KeyNaming(t *testing.T) {
	ctx := context.Background()
	cache, mr := newLeaderboardTestCache(t, "prod")

	cases := []struct {
		window   service.LeaderboardWindow
		start    time.Time
		end      time.Time
		wantBase string
	}{
		{
			window:   service.LeaderboardWindowToday,
			start:    time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC),
			end:      time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
			wantBase: "prod:leaderboard:v1:today:20260911",
		},
		{
			window:   service.LeaderboardWindowWeek,
			start:    time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
			end:      time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
			wantBase: "prod:leaderboard:v1:week:20260907",
		},
		{
			window:   service.LeaderboardWindowMonth,
			start:    time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			end:      time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
			wantBase: "prod:leaderboard:v1:month:202609",
		},
	}

	for _, tc := range cases {
		require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
			Window:      tc.window,
			WindowStart: tc.start,
			WindowEnd:   tc.end,
			UpdatedAt:   time.Now(),
			Entries: []service.LeaderboardUserMetrics{
				{UserID: 1, TotalTokens: 100, SuccessfulRequests: 3},
			},
		}))
	}

	keys := mr.Keys()
	for _, tc := range cases {
		require.Contains(t, keys, tc.wantBase+":z:total_tokens")
		require.Contains(t, keys, tc.wantBase+":z:successful_requests")
		require.Contains(t, keys, tc.wantBase+":h")
		require.Contains(t, keys, tc.wantBase+":updated_at")
	}
	// RENAME 之后不应残留任何临时 key。
	for _, k := range keys {
		require.NotContains(t, k, ":tmp:", "临时 key 未被 RENAME 消费: %s", k)
	}
}

// 跨零点 / 周一 / 月初时，读新窗口起点的 key 必须读不到上一窗口的残留。
func TestLeaderboardCache_WindowStartIsolatesSnapshots(t *testing.T) {
	ctx := context.Background()
	cache, _ := newLeaderboardTestCache(t, "prod")

	cases := []struct {
		name     string
		window   service.LeaderboardWindow
		oldStart time.Time
		oldEnd   time.Time
		newStart time.Time
	}{
		{"跨零点", service.LeaderboardWindowToday,
			time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)},
		{"跨周一", service.LeaderboardWindowWeek,
			time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)},
		{"跨月初", service.LeaderboardWindowMonth,
			time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
				Window:      tc.window,
				WindowStart: tc.oldStart,
				WindowEnd:   tc.oldEnd,
				UpdatedAt:   time.Now(),
				Entries: []service.LeaderboardUserMetrics{
					{UserID: 7, TotalTokens: 999, SuccessfulRequests: 9},
				},
			}))

			// 上一窗口有数据，新窗口起点应当是「正在计算」。
			_, exists, err := cache.UpdatedAt(ctx, tc.window, tc.newStart)
			require.NoError(t, err)
			require.False(t, exists)

			count, err := cache.ParticipantCount(ctx, tc.window, tc.newStart, service.LeaderboardMetricTotalTokens)
			require.NoError(t, err)
			require.Zero(t, count)

			entries, err := cache.TopEntries(ctx, tc.window, tc.newStart, service.LeaderboardMetricTotalTokens, service.LeaderboardTopEntryLimit)
			require.NoError(t, err)
			require.Empty(t, entries)

			// 旧窗口起点仍能读到完整的上一份快照。
			oldCount, err := cache.ParticipantCount(ctx, tc.window, tc.oldStart, service.LeaderboardMetricTotalTokens)
			require.NoError(t, err)
			require.Equal(t, int64(1), oldCount)
		})
	}
}

// TTL 取 min(60 分钟, 距窗口结束)：窗口快结束时按剩余时长，窗口还很长时封顶 60 分钟。
func TestLeaderboardCache_SnapshotTTL(t *testing.T) {
	ctx := context.Background()
	cache, mr := newLeaderboardTestCache(t, "prod")
	now := time.Now()

	t.Run("距窗口结束不足 60 分钟时取剩余时长", func(t *testing.T) {
		start := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
		require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
			Window:      service.LeaderboardWindowToday,
			WindowStart: start,
			WindowEnd:   now.Add(30 * time.Minute),
			UpdatedAt:   now,
			Entries:     []service.LeaderboardUserMetrics{{UserID: 1, TotalTokens: 10, SuccessfulRequests: 1}},
		}))
		base := "prod:leaderboard:v1:today:20260911"
		for _, suffix := range []string{":z:total_tokens", ":z:successful_requests", ":h", ":updated_at"} {
			ttl := mr.TTL(base + suffix)
			require.Greater(t, ttl, 29*time.Minute, suffix)
			require.LessOrEqual(t, ttl, 30*time.Minute, suffix)
		}
	})

	t.Run("窗口还很长时封顶 60 分钟", func(t *testing.T) {
		start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
		require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
			Window:      service.LeaderboardWindowMonth,
			WindowStart: start,
			WindowEnd:   now.Add(72 * time.Hour),
			UpdatedAt:   now,
			Entries:     []service.LeaderboardUserMetrics{{UserID: 1, TotalTokens: 10, SuccessfulRequests: 1}},
		}))
		base := "prod:leaderboard:v1:month:202609"
		for _, suffix := range []string{":z:total_tokens", ":z:successful_requests", ":h", ":updated_at"} {
			require.Equal(t, service.LeaderboardSnapshotMaxTTL, mr.TTL(base+suffix), suffix)
		}
	})
}

// RENAME 原子切换：第二轮写入后不得留下上一轮的成员，也不得留下半份榜。
func TestLeaderboardCache_ReplaceSnapshotIsFullSwap(t *testing.T) {
	ctx := context.Background()
	cache, mr := newLeaderboardTestCache(t, "prod")
	start := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)

	write := func(entries []service.LeaderboardUserMetrics) {
		require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
			Window:      service.LeaderboardWindowToday,
			WindowStart: start,
			WindowEnd:   end,
			UpdatedAt:   time.Now(),
			Entries:     entries,
		}))
	}

	write([]service.LeaderboardUserMetrics{
		{UserID: 1, TotalTokens: 300, SuccessfulRequests: 9},
		{UserID: 2, TotalTokens: 200, SuccessfulRequests: 5},
		{UserID: 3, TotalTokens: 100, SuccessfulRequests: 1},
	})
	// 第二轮只剩两个人：旧成员必须整体消失，而不是与新数据合并。
	write([]service.LeaderboardUserMetrics{
		{UserID: 2, TotalTokens: 500, SuccessfulRequests: 7},
		{UserID: 9, TotalTokens: 400, SuccessfulRequests: 6},
	})

	count, err := cache.ParticipantCount(ctx, service.LeaderboardWindowToday, start, service.LeaderboardMetricTotalTokens)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)

	entries, err := cache.TopEntries(ctx, service.LeaderboardWindowToday, start, service.LeaderboardMetricTotalTokens, service.LeaderboardTopEntryLimit)
	require.NoError(t, err)
	require.Equal(t, []service.LeaderboardScoreEntry{
		{UserID: 2, Score: 500},
		{UserID: 9, Score: 400},
	}, entries)

	metrics, err := cache.MetricsOf(ctx, service.LeaderboardWindowToday, start, []int64{1, 2, 9})
	require.NoError(t, err)
	require.NotContains(t, metrics, int64(1), "上一轮的成员不得残留在 Hash 里")
	require.Equal(t, service.LeaderboardUserMetrics{UserID: 2, TotalTokens: 500, SuccessfulRequests: 7}, metrics[2])
	require.Equal(t, service.LeaderboardUserMetrics{UserID: 9, TotalTokens: 400, SuccessfulRequests: 6}, metrics[9])

	for _, k := range mr.Keys() {
		require.False(t, strings.Contains(k, ":tmp:"), "临时 key 未被清理: %s", k)
	}
}

// 空快照（该窗口没有任何合格用量）仍要推进更新时间，并把上一轮的正式 key 清掉，
// 否则页面会一直读到上一轮的旧榜。
func TestLeaderboardCache_ReplaceSnapshotWithNoEntries(t *testing.T) {
	ctx := context.Background()
	cache, _ := newLeaderboardTestCache(t, "prod")
	start := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)

	require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
		Window: service.LeaderboardWindowToday, WindowStart: start, WindowEnd: end,
		UpdatedAt: time.Now(),
		Entries:   []service.LeaderboardUserMetrics{{UserID: 1, TotalTokens: 10, SuccessfulRequests: 1}},
	}))
	require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
		Window: service.LeaderboardWindowToday, WindowStart: start, WindowEnd: end,
		UpdatedAt: time.Now(),
	}))

	_, exists, err := cache.UpdatedAt(ctx, service.LeaderboardWindowToday, start)
	require.NoError(t, err)
	require.True(t, exists, "空快照仍然是一份就绪的快照，不是「正在计算」")

	count, err := cache.ParticipantCount(ctx, service.LeaderboardWindowToday, start, service.LeaderboardMetricTotalTokens)
	require.NoError(t, err)
	require.Zero(t, count)

	metrics, err := cache.MetricsOf(ctx, service.LeaderboardWindowToday, start, []int64{1})
	require.NoError(t, err)
	require.Empty(t, metrics)
}

// Rank（名次）= 分数严格高于自己的人数 + 1：并列同名次，其后跳号（1、1、3）。
func TestLeaderboardCache_RankOfIsCompetitiveRank(t *testing.T) {
	ctx := context.Background()
	cache, _ := newLeaderboardTestCache(t, "prod")
	start := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

	require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
		Window:      service.LeaderboardWindowToday,
		WindowStart: start,
		WindowEnd:   time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Now(),
		Entries: []service.LeaderboardUserMetrics{
			{UserID: 1, TotalTokens: 500, SuccessfulRequests: 5},
			{UserID: 2, TotalTokens: 500, SuccessfulRequests: 4},
			{UserID: 3, TotalTokens: 100, SuccessfulRequests: 3},
		},
	}))

	for userID, wantRank := range map[int64]int64{1: 1, 2: 1, 3: 3} {
		rank, found, err := cache.RankOf(ctx, service.LeaderboardWindowToday, start, service.LeaderboardMetricTotalTokens, userID)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, wantRank, rank, "user %d", userID)
	}

	// 窗口内零用量的查看者不在快照里：没有名次，只提示「暂无用量」。
	_, found, err := cache.RankOf(ctx, service.LeaderboardWindowToday, start, service.LeaderboardMetricTotalTokens, 42)
	require.NoError(t, err)
	require.False(t, found)
}

// 另一个 Metric 的 ZSET 有自己的排序，Hash 则同时给出两个数值。
func TestLeaderboardCache_SuccessfulRequestsMetric(t *testing.T) {
	ctx := context.Background()
	cache, _ := newLeaderboardTestCache(t, "prod")
	start := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

	require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
		Window:      service.LeaderboardWindowToday,
		WindowStart: start,
		WindowEnd:   time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Now(),
		Entries: []service.LeaderboardUserMetrics{
			{UserID: 1, TotalTokens: 500, SuccessfulRequests: 1},
			{UserID: 2, TotalTokens: 100, SuccessfulRequests: 9},
		},
	}))

	entries, err := cache.TopEntries(ctx, service.LeaderboardWindowToday, start, service.LeaderboardMetricSuccessfulRequests, service.LeaderboardTopEntryLimit)
	require.NoError(t, err)
	require.Equal(t, []service.LeaderboardScoreEntry{
		{UserID: 2, Score: 9},
		{UserID: 1, Score: 1},
	}, entries)

	rank, found, err := cache.RankOf(ctx, service.LeaderboardWindowToday, start, service.LeaderboardMetricSuccessfulRequests, 1)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, int64(2), rank)
}

// 快照更新时间按毫秒往返，供响应判定陈旧（超过 15 分钟未推进）。
func TestLeaderboardCache_UpdatedAtRoundTrip(t *testing.T) {
	ctx := context.Background()
	cache, _ := newLeaderboardTestCache(t, "prod")
	start := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Now().Add(-16 * time.Minute).Truncate(time.Millisecond)

	require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
		Window:      service.LeaderboardWindowToday,
		WindowStart: start,
		WindowEnd:   time.Now().Add(2 * time.Hour),
		UpdatedAt:   updatedAt,
		Entries:     []service.LeaderboardUserMetrics{{UserID: 1, TotalTokens: 10, SuccessfulRequests: 1}},
	}))

	got, exists, err := cache.UpdatedAt(ctx, service.LeaderboardWindowToday, start)
	require.NoError(t, err)
	require.True(t, exists)
	require.True(t, got.Equal(updatedAt), "got %s want %s", got, updatedAt)
}

// TopEntries 受 limit 约束：榜单条目固定至多 50 条。
func TestLeaderboardCache_TopEntriesRespectsLimit(t *testing.T) {
	ctx := context.Background()
	cache, _ := newLeaderboardTestCache(t, "prod")
	start := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

	entries := make([]service.LeaderboardUserMetrics, 0, 60)
	for i := 1; i <= 60; i++ {
		entries = append(entries, service.LeaderboardUserMetrics{
			UserID: int64(i), TotalTokens: int64(i * 10), SuccessfulRequests: int64(i),
		})
	}
	require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
		Window:      service.LeaderboardWindowToday,
		WindowStart: start,
		WindowEnd:   time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Now(),
		Entries:     entries,
	}))

	top, err := cache.TopEntries(ctx, service.LeaderboardWindowToday, start, service.LeaderboardMetricTotalTokens, service.LeaderboardTopEntryLimit)
	require.NoError(t, err)
	require.Len(t, top, service.LeaderboardTopEntryLimit)
	require.Equal(t, int64(60), top[0].UserID)
	require.Equal(t, int64(600), top[0].Score)

	count, err := cache.ParticipantCount(ctx, service.LeaderboardWindowToday, start, service.LeaderboardMetricTotalTokens)
	require.NoError(t, err)
	require.Equal(t, int64(60), count, "Participant Count 取 ZCARD，不受榜单长度限制")
}

// Hash value 是十二段：前两段是 Metric，其余十段只喂 Cache Hit Rate（缓存命中率）、Extremes（之最）
// 与 Token 构成，MUST NOT 参与排名（顺序见 leaderboard_cache.go 的 encodeLeaderboardMetrics）。
func TestLeaderboardCache_MetricsRoundTripTwelveFields(t *testing.T) {
	ctx := context.Background()
	cache, mr := newLeaderboardTestCache(t, "prod")
	start := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

	entry := service.LeaderboardUserMetrics{
		UserID: 1, TotalTokens: 500, SuccessfulRequests: 7, InputTokens: 120, CacheReadTokens: 380,
		OutputTokens: 60, CacheCreationTokens: 40, NightTokens: 220, DistinctModels: 3,
		MaxSingleTokens: 190, MediaRequests: 2, YesterdayTokens: 300, YesterdayRequests: 5,
	}
	require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
		Window:      service.LeaderboardWindowToday,
		WindowStart: start,
		WindowEnd:   time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Now(),
		Entries:     []service.LeaderboardUserMetrics{entry},
	}))

	metrics, err := cache.MetricsOf(ctx, service.LeaderboardWindowToday, start, []int64{1})
	require.NoError(t, err)
	require.Equal(t, entry, metrics[1], "十二段编解码必须往返无损")

	// 段序是契约的一部分：tokens, requests, input, cache_read, output, cache_creation,
	// night_tokens, distinct_models, max_single, media_requests, yesterday_tokens, yesterday_requests。
	require.Equal(t, "500,7,120,380,60,40,220,3,190,2,300,5",
		mr.HGet("prod:leaderboard:v1:today:20260911:h", "1"),
		"Hash 里仍然只有数值，没有身份也没有金额")

	// 新增的数只进 Hash，MUST NOT 另建排序结构：ZSET 仍然只有两个 Metric。
	for _, k := range mr.Keys() {
		for _, forbidden := range []string{
			":z:input_tokens", ":z:cache_read_tokens", ":z:output_tokens",
			":z:cache_creation_tokens", ":z:night_tokens", ":z:distinct_models",
			":z:max_single_tokens", ":z:media_requests", ":z:yesterday_tokens",
		} {
			require.NotContains(t, k, forbidden)
		}
	}
}

// 上一版写入的两段式与四段式 value 仍能读：缺的段一律按 0 处理，
// 表现为「没有命中率 / 没有之最卡」而不是 0，榜单本体照常渲染。
func TestLeaderboardCache_DecodeLegacyHashValue(t *testing.T) {
	ctx := context.Background()
	cache, mr := newLeaderboardTestCache(t, "prod")
	start := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	hashKey := "prod:leaderboard:v1:today:20260911:h"

	mr.HSet(hashKey, "1", "500,7")             // v1 之前的两段式
	mr.HSet(hashKey, "2", "500,7,120")         // 中途截断
	mr.HSet(hashKey, "3", "abc,7,1,2")         // 前两段坏了：整行作废
	mr.HSet(hashKey, "4", "500,x,1,2")         // 同上
	mr.HSet(hashKey, "5", "500,7,120,380")     // v1 的四段式
	mr.HSet(hashKey, "6", "500,7,120,380,60")  // 扩容中途的五段
	mr.HSet(hashKey, "7", "500,7,1,2,3,4,5,z") // 新增段坏了：该段按 0，整行仍有效

	metrics, err := cache.MetricsOf(ctx, service.LeaderboardWindowToday, start, []int64{1, 2, 3, 4, 5, 6, 7})
	require.NoError(t, err)
	require.Equal(t, service.LeaderboardUserMetrics{UserID: 1, TotalTokens: 500, SuccessfulRequests: 7}, metrics[1])
	require.Zero(t, metrics[1].InputTokens)
	require.Zero(t, metrics[1].CacheReadTokens)
	require.Equal(t, service.LeaderboardUserMetrics{UserID: 2, TotalTokens: 500, SuccessfulRequests: 7, InputTokens: 120}, metrics[2])
	require.NotContains(t, metrics, int64(3), "前两段解析失败仍视为无效行")
	require.NotContains(t, metrics, int64(4))

	// 四段式：榜单本体与命中率照常，八个新增段一律 0 —— 之最卡因此缺席而不是显示 0。
	require.Equal(t, service.LeaderboardUserMetrics{
		UserID: 5, TotalTokens: 500, SuccessfulRequests: 7, InputTokens: 120, CacheReadTokens: 380,
	}, metrics[5])
	require.Zero(t, metrics[5].OutputTokens)
	require.Zero(t, metrics[5].NightTokens)
	require.Zero(t, metrics[5].DistinctModels)
	require.Zero(t, metrics[5].MaxSingleTokens)
	require.Zero(t, metrics[5].YesterdayTokens)

	require.Equal(t, int64(60), metrics[6].OutputTokens)
	require.Zero(t, metrics[6].CacheCreationTokens)

	require.Equal(t, int64(500), metrics[7].TotalTokens)
	require.Zero(t, metrics[7].DistinctModels, "坏掉的新增段按 0，MUST NOT 让整行作废")
}

// viewer:models 的 key 必须带 cfg.Dashboard.KeyPrefix、window、窗口起点与 user_id：
// 少任何一段都会在跨环境 / 跨窗口 / 跨用户时串味。
func TestLeaderboardCache_ViewerModelsKeyNamingAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	cache, mr := newLeaderboardTestCache(t, "prod")
	start := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)

	rows := []usagestats.LeaderboardModelUsageRow{
		{Model: "claude-sonnet-5", SuccessfulRequests: 9},
		{Model: "claude-opus-5", SuccessfulRequests: 3},
	}
	require.NoError(t, cache.SetViewerModels(ctx, 42, service.LeaderboardWindowToday, start, end, rows, 14))

	const wantKey = "prod:leaderboard:v1:viewer:models:today:20260911:42"
	require.Contains(t, mr.Keys(), wantKey)

	got, total, found, err := cache.ViewerModels(ctx, 42, service.LeaderboardWindowToday, start)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, rows, got)
	require.Equal(t, int64(14), total, "分母必须与 Top N 一起缓存，否则命中缓存时算不出占比")

	// 另一个用户、另一个窗口都是另一个 key，MUST NOT 读到别人的结果。
	_, _, found, err = cache.ViewerModels(ctx, 43, service.LeaderboardWindowToday, start)
	require.NoError(t, err)
	require.False(t, found)
	_, _, found, err = cache.ViewerModels(ctx, 42, service.LeaderboardWindowWeek, start)
	require.NoError(t, err)
	require.False(t, found)

	// 跨零点：今日窗口换了起点就是另一个 key，读不到昨天的残留。
	_, _, found, err = cache.ViewerModels(ctx, 42, service.LeaderboardWindowToday, start.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.False(t, found, "跨零点 MUST NOT 读到昨天的 viewer.models 缓存")

	// 内容损坏按未命中处理：宁可多查一次，也不给半份数据。
	require.NoError(t, mr.Set(wantKey, "{not json"))
	_, _, found, err = cache.ViewerModels(ctx, 42, service.LeaderboardWindowToday, start)
	require.NoError(t, err)
	require.False(t, found)
}

// TTL 取 min(60 秒, 距窗口结束)：既不长过一分钟，也不活过窗口本身。
func TestLeaderboardCache_ViewerModelsTTL(t *testing.T) {
	ctx := context.Background()
	cache, mr := newLeaderboardTestCache(t, "prod")
	start := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	const key = "prod:leaderboard:v1:viewer:models:today:20260911:42"

	// 距窗口结束还很久：封顶 60 秒。
	require.NoError(t, cache.SetViewerModels(ctx, 42, service.LeaderboardWindowToday, start,
		time.Now().Add(6*time.Hour), nil, 0))
	ttl := mr.TTL(key)
	require.Greater(t, ttl, 55*time.Second)
	require.LessOrEqual(t, ttl, leaderboardViewerModelsTTL)

	// 窗口马上就结束：TTL 跟着缩短，MUST NOT 让缓存活过零点。
	require.NoError(t, cache.SetViewerModels(ctx, 42, service.LeaderboardWindowToday, start,
		time.Now().Add(8*time.Second), nil, 0))
	ttl = mr.TTL(key)
	require.LessOrEqual(t, ttl, 8*time.Second)
	require.Greater(t, ttl, time.Second)
}

func leaderboardTestHighlights(userID int64) *service.LeaderboardHighlights {
	rate := 0.8
	peak := 14
	return &service.LeaderboardHighlights{
		TopTokens:   &service.LeaderboardHighlightUser{UserID: userID, TotalTokens: 500, SuccessfulRequests: 7, SharePercent: 60, LeadPercent: 40},
		TopRequests: &service.LeaderboardHighlightUser{UserID: userID, TotalTokens: 500, SuccessfulRequests: 7, SharePercent: 55, LeadPercent: 20},
		CacheKing:   &service.LeaderboardCacheKing{UserID: userID, CacheHitRate: rate, DominantModel: "claude-sonnet-5"},
		Site: service.LeaderboardSiteSummary{
			TotalTokens: 900, SuccessfulRequests: 12, ParticipantCount: 3, CacheHitRate: &rate, PeakHour: &peak,
		},
	}
}

// Highlights 与榜单同一批 RENAME 切换：key 带前缀与窗口起点，切换后不留临时 key。
func TestLeaderboardCache_HighlightsSwitchWithSnapshot(t *testing.T) {
	ctx := context.Background()
	cache, mr := newLeaderboardTestCache(t, "prod")
	// 窗口起点固定（key 里的日期戳要可断言），但窗口结束必须相对 time.Now()：
	// 下面断言 TTL 是 60 分钟的硬上限，写死一个日期会在真实时间走过那一天之后
	// 变成「距窗口结束不足 1 秒」而必然失败。
	start := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	end := time.Now().Add(72 * time.Hour)

	write := func(userID int64, highlights *service.LeaderboardHighlights) {
		require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
			Window:      service.LeaderboardWindowToday,
			WindowStart: start,
			WindowEnd:   end,
			UpdatedAt:   time.Now(),
			Entries: []service.LeaderboardUserMetrics{
				{UserID: userID, TotalTokens: 500, SuccessfulRequests: 7, InputTokens: 100, CacheReadTokens: 400},
			},
			Highlights: highlights,
		}))
	}

	write(1, leaderboardTestHighlights(1))
	require.Contains(t, mr.Keys(), "prod:leaderboard:v1:today:20260911:highlights")

	got, err := cache.Highlights(ctx, service.LeaderboardWindowToday, start)
	require.NoError(t, err)
	require.Equal(t, leaderboardTestHighlights(1), got)
	require.Equal(t, service.LeaderboardSnapshotMaxTTL, mr.TTL("prod:leaderboard:v1:today:20260911:highlights"))

	// 第二轮换人：Highlights 必须整体换掉，不得留上一轮的领先者。
	write(2, leaderboardTestHighlights(2))
	got, err = cache.Highlights(ctx, service.LeaderboardWindowToday, start)
	require.NoError(t, err)
	require.Equal(t, int64(2), got.TopTokens.UserID)
	for _, k := range mr.Keys() {
		require.NotContains(t, k, ":tmp:", "临时 key 未被 RENAME 消费: %s", k)
	}

	// 这一轮算不出 Highlights：连同榜单一起切换成「没有 Highlights」。
	write(3, nil)
	got, err = cache.Highlights(ctx, service.LeaderboardWindowToday, start)
	require.NoError(t, err)
	require.Nil(t, got, "MUST NOT 让上一轮的趣味卡配这一轮的榜")
	require.NotContains(t, mr.Keys(), "prod:leaderboard:v1:today:20260911:highlights")
}

// 跨零点 / 周一 / 月初：新窗口起点读不到上一窗口的 Highlights。
func TestLeaderboardCache_HighlightsWindowStartIsolation(t *testing.T) {
	ctx := context.Background()
	cache, _ := newLeaderboardTestCache(t, "prod")
	oldStart := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	newStart := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)

	require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
		Window:      service.LeaderboardWindowToday,
		WindowStart: oldStart,
		WindowEnd:   newStart,
		UpdatedAt:   time.Now(),
		Entries:     []service.LeaderboardUserMetrics{{UserID: 1, TotalTokens: 10, SuccessfulRequests: 1}},
		Highlights:  leaderboardTestHighlights(1),
	}))

	got, err := cache.Highlights(ctx, service.LeaderboardWindowToday, newStart)
	require.NoError(t, err)
	require.Nil(t, got)

	got, err = cache.Highlights(ctx, service.LeaderboardWindowToday, oldStart)
	require.NoError(t, err)
	require.NotNil(t, got)
}

// 空快照（窗口内没有任何合格用量）同样要清掉上一轮的 Highlights。
func TestLeaderboardCache_EmptySnapshotClearsHighlights(t *testing.T) {
	ctx := context.Background()
	cache, _ := newLeaderboardTestCache(t, "prod")
	start := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)

	require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
		Window: service.LeaderboardWindowToday, WindowStart: start, WindowEnd: end,
		UpdatedAt:  time.Now(),
		Entries:    []service.LeaderboardUserMetrics{{UserID: 1, TotalTokens: 10, SuccessfulRequests: 1}},
		Highlights: leaderboardTestHighlights(1),
	}))
	require.NoError(t, cache.ReplaceSnapshot(ctx, service.LeaderboardSnapshotWindow{
		Window: service.LeaderboardWindowToday, WindowStart: start, WindowEnd: end,
		UpdatedAt: time.Now(),
	}))

	got, err := cache.Highlights(ctx, service.LeaderboardWindowToday, start)
	require.NoError(t, err)
	require.Nil(t, got)
}

// 站点级 Insights 与 Window 无关，只有一份，key 带今日窗口起点。
func TestLeaderboardCache_InsightsKeyNamingAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	cache, mr := newLeaderboardTestCache(t, "prod")
	todayStart := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	todayEnd := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)

	insights := &service.LeaderboardInsights{
		ModelsToday: []service.LeaderboardModelInsight{{Model: "claude-sonnet-5", SuccessfulRequests: 30, SharePercent: 60}},
		Daily30:     []service.LeaderboardDailyInsight{{Date: "2026-09-11", Requests: 10, TotalTokens: 100, RelativePercent: 100}},
		HourlyToday: []service.LeaderboardHourlyInsight{{Hour: 14, Requests: 400, RelativePercent: 100}},
		CacheToday:  &service.LeaderboardCacheInsight{CacheHitRate: 0.712, CacheReadTokens: 8600, InputTokens: 3400},
		Month:       &service.LeaderboardMonthInsight{TotalTokens: 165_000_000, ChangePercent: 18},
	}
	require.NoError(t, cache.ReplaceInsights(ctx, todayStart, insights, todayEnd))

	require.Contains(t, mr.Keys(), "prod:leaderboard:v1:insights:20260911")
	got, err := cache.Insights(ctx, todayStart)
	require.NoError(t, err)
	require.Equal(t, insights, got)

	// TTL 与 Snapshot 同一条规则：min(60 分钟, 距今日窗口结束)。
	require.NoError(t, cache.ReplaceInsights(ctx, todayStart, insights, time.Now().Add(30*time.Minute)))
	ttl := mr.TTL("prod:leaderboard:v1:insights:20260911")
	require.Greater(t, ttl, 29*time.Minute)
	require.LessOrEqual(t, ttl, 30*time.Minute)

	// 跨零点：新一天的 key 读不到昨天的今日热度。
	got, err = cache.Insights(ctx, todayEnd)
	require.NoError(t, err)
	require.Nil(t, got)

	// nil 表示这一轮没有洞察：清掉 key，而不是留着昨天的。
	require.NoError(t, cache.ReplaceInsights(ctx, todayStart, nil, todayEnd))
	got, err = cache.Insights(ctx, todayStart)
	require.NoError(t, err)
	require.Nil(t, got)
}

// 内容损坏时按缺失处理：页面隐藏对应区块，而不是 500。
func TestLeaderboardCache_CorruptJSONReadsAsMissing(t *testing.T) {
	ctx := context.Background()
	cache, mr := newLeaderboardTestCache(t, "prod")
	start := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

	require.NoError(t, mr.Set("prod:leaderboard:v1:insights:20260911", "{not json"))
	require.NoError(t, mr.Set("prod:leaderboard:v1:today:20260911:highlights", "{not json"))

	insights, err := cache.Insights(ctx, start)
	require.NoError(t, err)
	require.Nil(t, insights)

	highlights, err := cache.Highlights(ctx, service.LeaderboardWindowToday, start)
	require.NoError(t, err)
	require.Nil(t, highlights)
}
