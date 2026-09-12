//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

// v2 新增常量之间的关系是契约的一部分，改动任何一个都要连带想清楚另一个：
// 这条测试把它们钉住，免得日后单独调一个值让某块数据静默失真。
//
// 夜猫子的 0–6 点边界不在这里：它只影响聚合 SQL 的 night_tokens 那一列，
// 所以那个数写在 repository 的 leaderboardNightHourEnd 上、由 SQL 直接消费，
// service 拿到的已经是算好的 night_tokens。
func TestLeaderboardV2ConstantsHoldTogether(t *testing.T) {
	require.Equal(t, leaderboardCacheKingMinRequests, leaderboardTalkerMinRequests,
		"话痨与效率之星是同一类参评门槛")
	require.Equal(t, leaderboardStreakLookbackDays, leaderboardRankHistoryRetentionDays,
		"连续活跃的回溯窗口与名次历史的保留期是同一个数，一个数管两处")
	require.Greater(t, leaderboardRankHistoryRetentionDays, leaderboardRankHistoryDays,
		"保留期必须覆盖走势天数，否则折线会在保留期边界上断掉")
	require.Equal(t, 5, leaderboardViewerModelsLimit, "「你的模型偏好」取本人 Top 5")
	require.Equal(t, LeaderboardTopEntryLimit, leaderboardProfilesLimit,
		"模型偏好画像的人数与榜单本体的 Top 50 是同一个数，榜上有名的人在画像里都能找到自己")
	require.Equal(t, 5, leaderboardRhythmLevels, "周内节奏是 0–4 共五档")
}

// 名次走势的区间含今日、共 14 天，且上界是开区间。
func TestLeaderboardRankHistoryRange(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	todayStart := time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)

	from, to := leaderboardRankHistoryRange(todayStart)
	require.True(t, from.Equal(time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)))
	require.True(t, to.Equal(time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC)))
	require.Equal(t, leaderboardRankHistoryDays, int(to.Sub(from).Hours()/24))
}
