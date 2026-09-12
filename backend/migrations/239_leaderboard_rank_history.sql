-- 239: Leaderboard（排行榜）的名次历史表。
--
-- 快照作业每一轮对今日窗口的全部参与者写一行「今天的名次」，同一天的多轮互相覆盖，
-- 日终那一轮写下的就是当天的最终名次；响应里的 viewer.rank_history 只读本人近 14 天。
-- 两个 Metric（排名指标）的名次都写：它们本来就在同一份 Snapshot 里，写第二列不多一次
-- 计算，而只写一列会让「按成功请求数看走势」这种后续增量必须重新攒历史。
--
-- 表里只有 user_id、日期与两个名次：没有身份、没有数值、没有任何金额。
-- 保留期 90 天，由同一轮作业按 snapshot_date 删除，因此需要下面那条按日期的索引。
--
-- 新建表没有锁表风险，普通事务迁移即可，不需要 _notx.sql 后缀。
-- 注意：正文与注释里都 MUST NOT 出现并发建索引的那个关键字——迁移校验器
-- （internal/repository/migrations_runner.go）对非 _notx.sql 文件是整文件裸匹配，
-- 不剥离注释，写进注释同样会让整轮迁移中止。

CREATE TABLE IF NOT EXISTS leaderboard_rank_history (
    user_id                  BIGINT NOT NULL,
    snapshot_date            DATE   NOT NULL,
    rank_total_tokens        INT    NOT NULL,
    rank_successful_requests INT    NOT NULL,
    PRIMARY KEY (user_id, snapshot_date)
);

CREATE INDEX IF NOT EXISTS idx_leaderboard_rank_history_snapshot_date
    ON leaderboard_rank_history (snapshot_date);

COMMENT ON COLUMN leaderboard_rank_history.rank_total_tokens IS
    'Competitive rank by Total Tokens in the today window on that date (1-based; ties share a rank and the next rank skips)';
COMMENT ON COLUMN leaderboard_rank_history.rank_successful_requests IS
    'Competitive rank by Successful Requests in the today window on that date (1-based; ties share a rank and the next rank skips)';
