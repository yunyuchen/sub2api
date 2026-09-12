-- 238: Leaderboard（排行榜）的 Named Participation（实名参与）开关。
--
-- 用户自选是否同意在排行榜上以 username 实名展示，默认关闭；关闭的用户仍然参与
-- 排名，只是以匿名形态出现，因此这不是「退出排行」开关。
--
-- 新增带默认值的布尔列在 PostgreSQL 11+ 不重写整表，普通事务迁移即可，
-- 不需要并发建索引，也就不用 _notx.sql 后缀。
-- 注意：正文与注释里都 MUST NOT 出现并发建索引的那个关键字——迁移校验器
-- （internal/repository/migrations_runner.go）对非 _notx.sql 文件是整文件裸匹配，
-- 不剥离注释，写进注释同样会让整轮迁移中止。

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS leaderboard_named_participation BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN users.leaderboard_named_participation IS
    'Whether the user enabled Named Participation (shown by username on the leaderboard); false keeps them ranked anonymously';
