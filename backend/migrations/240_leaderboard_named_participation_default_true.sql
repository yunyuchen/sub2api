-- 240: Leaderboard（排行榜）的昵称展示开关默认改为开启，并回填全部现有用户。
--
-- 238 建这一列时默认 false（「实名参与」需用户自选）。产品口径已改为「默认显示昵称、
-- 用户可自行关掉」，所以这里做两件事：把列默认值改成 true，并把现有行回填成 true。
--
-- 回填是一次性语义：迁移按文件名记录在 schema_migrations 里，只应用一次，此后用户
-- 自己关掉写下的 false 不会被再翻回来。语句本身不依赖列的当前状态，重复执行这份文件
-- 得到的结果相同，因此是幂等的。
--
-- 关掉开关的用户仍然照常参与排名，只是以「第 N 位」出现；N 是榜单序号（ordinal），
-- 不是 user_id。
--
-- 只有一次改默认值与一次整表 UPDATE，普通事务迁移即可，不需要 _notx.sql 后缀。
-- 注意：正文与注释里都 MUST NOT 出现并发建索引的那个关键字——迁移校验器
-- （internal/repository/migrations_runner.go）对非 _notx.sql 文件是整文件裸匹配，
-- 不剥离注释，写进注释同样会让整轮迁移中止。

ALTER TABLE users
    ALTER COLUMN leaderboard_named_participation SET DEFAULT true;

UPDATE users
SET leaderboard_named_participation = true
WHERE leaderboard_named_participation = false;

COMMENT ON COLUMN users.leaderboard_named_participation IS
    'Whether the leaderboard shows this user by username (default true); false keeps them ranked under the anonymous ordinal pseudonym';
