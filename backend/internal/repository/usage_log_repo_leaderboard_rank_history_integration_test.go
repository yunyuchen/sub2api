//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

// 名次历史的日期一律取窗口起点这一天；这里固定一个日期，避免「今天恰好是月初」之类的巧合。
var leaderboardRankHistoryToday = time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)

func leaderboardRankHistoryRow(userID int64, offsetDays, rankTokens, rankRequests, rankCost int) usagestats.LeaderboardRankHistoryRow {
	return usagestats.LeaderboardRankHistoryRow{
		UserID:                 userID,
		SnapshotDate:           leaderboardRankHistoryToday.AddDate(0, 0, offsetDays),
		RankTotalTokens:        rankTokens,
		RankSuccessfulRequests: rankRequests,
		RankCost:               rankCost,
	}
}

// 同一天同一人只有一行：多轮之间互相覆盖，日终那一轮写下的就是当天的最终名次。
func TestUsageLog_LeaderboardRankHistory_UpsertOverwritesSameDay(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newUsageLogRepositoryWithSQL(tx.Client(), tx)

	require.NoError(t, repo.UpsertLeaderboardRankHistory(ctx, []usagestats.LeaderboardRankHistoryRow{
		leaderboardRankHistoryRow(1, 0, 18, 20, 7),
		leaderboardRankHistoryRow(2, 0, 5, 3, 4),
	}))
	// 同一天的下一轮：名次变了，行数不变。
	require.NoError(t, repo.UpsertLeaderboardRankHistory(ctx, []usagestats.LeaderboardRankHistoryRow{
		leaderboardRankHistoryRow(1, 0, 12, 9, 6),
	}))

	rows, err := repo.LeaderboardRankHistory(ctx, 1,
		leaderboardRankHistoryToday.AddDate(0, 0, -13),
		leaderboardRankHistoryToday.AddDate(0, 0, 1),
	)
	require.NoError(t, err)
	require.Len(t, rows, 1, "同一天同一人 MUST NOT 追加多行")
	require.Equal(t, 12, rows[0].RankTotalTokens, "取最近一轮写下的值")
	require.Equal(t, 9, rows[0].RankSuccessfulRequests)
	require.Equal(t, 6, rows[0].RankCost, "三个 Metric 的名次同轮覆盖，MUST NOT 只更新其中两列")
	require.Equal(t, leaderboardRankHistoryToday.Format("2006-01-02"), rows[0].SnapshotDate.Format("2006-01-02"))

	// 另一个人的行不受影响，且只读本人 MUST NOT 带出别人。
	rows, err = repo.LeaderboardRankHistory(ctx, 2,
		leaderboardRankHistoryToday.AddDate(0, 0, -13),
		leaderboardRankHistoryToday.AddDate(0, 0, 1),
	)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(2), rows[0].UserID)
	require.Equal(t, 5, rows[0].RankTotalTokens)
	require.Equal(t, 4, rows[0].RankCost)
}

// 迁移 241 之前写下的行没有 rank_cost：列的默认值 0 就是「那天没有这个数」，
// 读出来仍是一行完整历史，MUST NOT 因为缺这一列而整行读不出来。
func TestUsageLog_LeaderboardRankHistory_LegacyRowsReadCostRankAsZero(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newUsageLogRepositoryWithSQL(tx.Client(), tx)

	_, err := tx.ExecContext(ctx, `
		INSERT INTO leaderboard_rank_history (user_id, snapshot_date, rank_total_tokens, rank_successful_requests)
		VALUES ($1, $2::date, 3, 4)
	`, int64(1), leaderboardRankHistoryToday.Format("2006-01-02"))
	require.NoError(t, err)

	rows, err := repo.LeaderboardRankHistory(ctx, 1,
		leaderboardRankHistoryToday.AddDate(0, 0, -13),
		leaderboardRankHistoryToday.AddDate(0, 0, 1),
	)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 3, rows[0].RankTotalTokens)
	require.Equal(t, 4, rows[0].RankSuccessfulRequests)
	require.Zero(t, rows[0].RankCost, "旧行的 rank_cost 是列默认值 0，不是凭空补出的名次")

	// 下一轮作业照常覆盖，三列一起写上去。
	require.NoError(t, repo.UpsertLeaderboardRankHistory(ctx, []usagestats.LeaderboardRankHistoryRow{
		leaderboardRankHistoryRow(1, 0, 3, 4, 5),
	}))
	rows, err = repo.LeaderboardRankHistory(ctx, 1,
		leaderboardRankHistoryToday.AddDate(0, 0, -13),
		leaderboardRankHistoryToday.AddDate(0, 0, 1),
	)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 5, rows[0].RankCost)
}

// 空切片不查库；超过一批（1000 行）时分批写入，行数与内容都不能丢。
func TestUsageLog_LeaderboardRankHistory_UpsertBatchBoundary(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newUsageLogRepositoryWithSQL(tx.Client(), tx)

	require.NoError(t, repo.UpsertLeaderboardRankHistory(ctx, nil))

	const total = leaderboardRankHistoryBatchSize + 7
	rows := make([]usagestats.LeaderboardRankHistoryRow, 0, total)
	for i := 1; i <= total; i++ {
		rows = append(rows, leaderboardRankHistoryRow(int64(i), 0, i, total-i+1, i))
	}
	require.NoError(t, repo.UpsertLeaderboardRankHistory(ctx, rows))

	countRows, err := tx.QueryContext(ctx,
		`SELECT COUNT(*) FROM leaderboard_rank_history WHERE snapshot_date = $1::date`,
		leaderboardRankHistoryToday.Format("2006-01-02"),
	)
	require.NoError(t, err)
	var count int64
	require.True(t, countRows.Next())
	require.NoError(t, countRows.Scan(&count))
	require.NoError(t, countRows.Close())
	require.Equal(t, int64(total), count, "跨批次的行必须全部落库")

	// 最后一批里的那几行同样完整。
	last, err := repo.LeaderboardRankHistory(ctx, int64(total),
		leaderboardRankHistoryToday.AddDate(0, 0, -1),
		leaderboardRankHistoryToday.AddDate(0, 0, 1),
	)
	require.NoError(t, err)
	require.Len(t, last, 1)
	require.Equal(t, total, last[0].RankTotalTokens)
	require.Equal(t, 1, last[0].RankSuccessfulRequests)
	require.Equal(t, total, last[0].RankCost, "一行五个占位符，跨批次时第五列同样不能丢")
}

// 按日期升序只读区间内的行：上界是开区间，下界含当天。
func TestUsageLog_LeaderboardRankHistory_ReadRangeAscending(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newUsageLogRepositoryWithSQL(tx.Client(), tx)

	require.NoError(t, repo.UpsertLeaderboardRankHistory(ctx, []usagestats.LeaderboardRankHistoryRow{
		leaderboardRankHistoryRow(1, -20, 40, 40, 40), // 14 天窗口之外
		leaderboardRankHistoryRow(1, -13, 30, 31, 32), // 窗口的第一天
		leaderboardRankHistoryRow(1, -1, 20, 21, 22),
		leaderboardRankHistoryRow(1, 0, 10, 11, 12),
	}))

	from := leaderboardRankHistoryToday.AddDate(0, 0, -13)
	to := leaderboardRankHistoryToday.AddDate(0, 0, 1)
	rows, err := repo.LeaderboardRankHistory(ctx, 1, from, to)
	require.NoError(t, err)
	require.Len(t, rows, 3, "14 天之外的那一行不在区间内")
	require.Equal(t, []int{30, 20, 10}, []int{rows[0].RankTotalTokens, rows[1].RankTotalTokens, rows[2].RankTotalTokens},
		"按日期升序返回")

	// 上界是开区间：今日那一天不在 [from, today) 里。
	rows, err = repo.LeaderboardRankHistory(ctx, 1, from, leaderboardRankHistoryToday)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	// 没有历史的用户是空切片，不是报错。
	rows, err = repo.LeaderboardRankHistory(ctx, 999, from, to)
	require.NoError(t, err)
	require.Empty(t, rows)
}

// 保留期清理：cutoff 那天保留，更早的删除；返回删除行数。
func TestUsageLog_LeaderboardRankHistory_DeleteBeforeRetention(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newUsageLogRepositoryWithSQL(tx.Client(), tx)

	require.NoError(t, repo.UpsertLeaderboardRankHistory(ctx, []usagestats.LeaderboardRankHistoryRow{
		leaderboardRankHistoryRow(1, -91, 1, 1, 1), // 第 91 天：删
		leaderboardRankHistoryRow(1, -90, 2, 2, 2), // cutoff 当天：保留
		leaderboardRankHistoryRow(2, -91, 3, 3, 3), // 另一个人的第 91 天：删
		leaderboardRankHistoryRow(1, -89, 4, 4, 4), // 第 89 天：保留
		leaderboardRankHistoryRow(1, 0, 5, 5, 5),
	}))

	cutoff := leaderboardRankHistoryToday.AddDate(0, 0, -90)
	deleted, err := repo.DeleteLeaderboardRankHistoryBefore(ctx, cutoff)
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted, "只删 cutoff 之前的两行")

	rows, err := repo.LeaderboardRankHistory(ctx, 1,
		leaderboardRankHistoryToday.AddDate(0, 0, -365),
		leaderboardRankHistoryToday.AddDate(0, 0, 1),
	)
	require.NoError(t, err)
	require.Len(t, rows, 3, "cutoff 当天与之后的行全部保留")
	require.Equal(t, cutoff.Format("2006-01-02"), rows[0].SnapshotDate.Format("2006-01-02"))

	// 再删一次是幂等的空操作。
	deleted, err = repo.DeleteLeaderboardRankHistoryBefore(ctx, cutoff)
	require.NoError(t, err)
	require.Zero(t, deleted)
}
