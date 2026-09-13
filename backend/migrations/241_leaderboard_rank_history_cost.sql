-- 241: Leaderboard（排行榜）名次历史表补上按 Cost（消费金额）的名次列。
--
-- Metric（排名指标）从两项扩成三项（total_tokens / successful_requests / cost），
-- 名次历史随之从两列变三列：三个名次本来就在同一份 Snapshot 里算好，写第三列不多一次
-- 计算，而不写就会让「按消费金额看走势」这种后续增量必须从头重新攒历史——这与 239
-- 当初一次写两列是同一条理由。
--
-- 默认 0 的含义是「那天没有这个数」：本次迁移之前的旧行不会凭空补出一个名次，
-- 上层把 0 当成缺数据，MUST NOT 渲染成「第 0 名」。
-- 表里仍然只有 user_id、日期与名次：没有身份、没有数值，金额本身也不落这张表。
--
-- 只有一次带常量默认值的 ADD COLUMN（PostgreSQL 11 起不重写整表），普通事务迁移即可，
-- 不需要 _notx.sql 后缀。
-- 注意：正文与注释里都 MUST NOT 出现并发建索引的那个关键字——迁移校验器
-- （internal/repository/migrations_runner.go）对非 _notx.sql 文件是整文件裸匹配，
-- 不剥离注释，写进注释同样会让整轮迁移中止。

ALTER TABLE leaderboard_rank_history
    ADD COLUMN IF NOT EXISTS rank_cost INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN leaderboard_rank_history.rank_cost IS
    'Competitive rank by Cost (sum of actual_cost) in the today window on that date (1-based; ties share a rank and the next rank skips); 0 marks rows written before this column existed';
