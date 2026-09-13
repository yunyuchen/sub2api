package repository

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// leaderboard_rank_history（名次历史，migrations/239，rank_cost 列见 migrations/241）的读写。
// 方法挂在既有的 usageLogRepository 上，与 usage_log_repo_leaderboard.go 同一个接收者：
// 它们同属 Leaderboard（排行榜）这一条链，由同一轮快照作业写、由同一个响应组装读。
//
// 表里只有 user_id、日期与三个 Metric 各自的名次：没有身份、没有数值，
// 金额本身也不落这张表——Cost 那一列存的是名次，不是钱（design D21）。

const (
	// leaderboardRankHistoryBatchSize 是一次 INSERT 的行数上限。
	// 每行五个占位符，1000 行即 5000 个参数，远低于 PostgreSQL 的 65535 上限，
	// 同时避免十万人站点把一整轮参与者塞进单条语句。
	leaderboardRankHistoryBatchSize = 1000

	// leaderboardRankHistoryDateLayout 是写入时把 snapshot_date 拍平成 DATE 字面量的格式。
	// 直接传 time.Time 会带上时刻与时区偏移，落到 DATE 列时是否跨日取决于会话时区；
	// 这里先按调用方给的时间（站点时区的窗口起点）取日期部分，语义就与 Window 完全一致。
	leaderboardRankHistoryDateLayout = "2006-01-02"
)

// UpsertLeaderboardRankHistory 写入一轮的名次历史：同一天同一人只有一行，多轮互相覆盖，
// 日终那一轮写下的就是当天的最终名次（design D21）。
//
// 用多行 INSERT ... ON CONFLICT DO UPDATE 而不是逐行写：一轮要覆盖今日窗口的**全部参与者**，
// 逐行会把一次批量写摊成几千次往返。rows 为空时直接返回，连库都不查。
func (r *usageLogRepository) UpsertLeaderboardRankHistory(ctx context.Context, rows []usagestats.LeaderboardRankHistoryRow) error {
	if len(rows) == 0 {
		return nil
	}
	for start := 0; start < len(rows); start += leaderboardRankHistoryBatchSize {
		end := start + leaderboardRankHistoryBatchSize
		if end > len(rows) {
			end = len(rows)
		}
		if err := r.upsertLeaderboardRankHistoryBatch(ctx, rows[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (r *usageLogRepository) upsertLeaderboardRankHistoryBatch(ctx context.Context, rows []usagestats.LeaderboardRankHistoryRow) error {
	values := make([]string, 0, len(rows))
	args := make([]any, 0, len(rows)*5)
	for _, row := range rows {
		base := len(args)
		values = append(values, "($"+strconv.Itoa(base+1)+", $"+strconv.Itoa(base+2)+"::date, $"+strconv.Itoa(base+3)+", $"+strconv.Itoa(base+4)+", $"+strconv.Itoa(base+5)+")")
		args = append(args,
			row.UserID,
			row.SnapshotDate.Format(leaderboardRankHistoryDateLayout),
			row.RankTotalTokens,
			row.RankSuccessfulRequests,
			row.RankCost,
		)
	}
	query := `
		INSERT INTO leaderboard_rank_history (user_id, snapshot_date, rank_total_tokens, rank_successful_requests, rank_cost)
		VALUES ` + strings.Join(values, ", ") + `
		ON CONFLICT (user_id, snapshot_date) DO UPDATE SET
			rank_total_tokens = EXCLUDED.rank_total_tokens,
			rank_successful_requests = EXCLUDED.rank_successful_requests,
			rank_cost = EXCLUDED.rank_cost
	`
	_, err := r.sql.ExecContext(ctx, query, args...)
	return err
}

// LeaderboardRankHistory 读某一个用户 [fromDate, toDate) 的名次历史，按日期升序。
//
// 只查一个 user_id：响应里的名次走势 MUST 只含查看者本人，MUST NOT 下发其他用户的任何名次历史
// （design D21）。查看者还没有历史时返回空切片，由上层渲染成「隐藏这一格」而不是一条空折线。
func (r *usageLogRepository) LeaderboardRankHistory(ctx context.Context, userID int64, fromDate, toDate time.Time) (results []usagestats.LeaderboardRankHistoryRow, err error) {
	query := `
		SELECT user_id, snapshot_date, rank_total_tokens, rank_successful_requests, rank_cost
		FROM leaderboard_rank_history
		WHERE user_id = $1
		  AND snapshot_date >= $2::date
		  AND snapshot_date < $3::date
		ORDER BY snapshot_date ASC
	`
	rows, err := r.sql.QueryContext(ctx, query, userID,
		fromDate.Format(leaderboardRankHistoryDateLayout),
		toDate.Format(leaderboardRankHistoryDateLayout),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]usagestats.LeaderboardRankHistoryRow, 0)
	for rows.Next() {
		var row usagestats.LeaderboardRankHistoryRow
		if err := rows.Scan(&row.UserID, &row.SnapshotDate, &row.RankTotalTokens, &row.RankSuccessfulRequests, &row.RankCost); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// DeleteLeaderboardRankHistoryBefore 删除 cutoff 那天之前（不含当天）的行，返回删除行数。
//
// 这张表按「人 × 天」增长，没有保留期就会无限长；清理写在同一轮作业里而不是靠人工（design D21）。
// 删除按 snapshot_date 走 idx_leaderboard_rank_history_snapshot_date。
func (r *usageLogRepository) DeleteLeaderboardRankHistoryBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	const query = `DELETE FROM leaderboard_rank_history WHERE snapshot_date < $1::date`
	result, err := r.sql.ExecContext(ctx, query, cutoff.Format(leaderboardRankHistoryDateLayout))
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}
