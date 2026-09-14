-- 242: user_avatars 补上榜单用的小图列 thumb_url。
--
-- Leaderboard（排行榜）一页要渲染几十个头像。inline 头像的原图上限是 20 KB，
-- 几十张塞进同一份榜单 JSON 就是几百 KB；所以另存一份 64px 正方形 JPEG 的 data URL
-- （约 1–3 KB），上传 inline 头像时从原图派生，榜单只下发这一列。
--
-- 默认空串的含义是「这行没有小图」，有两种情况：
--   1. remote_url（外链）头像——服务端不抓外链，因此永远没有小图，榜单不展示它们；
--   2. 本次迁移之前存下的 inline 头像——由服务启动时的一次性回填补上
--      （UserService.BackfillAvatarThumbs）。
-- 上层把空串当成「没有头像可展示」，MUST NOT 渲染成空图片。
--
-- 只有一次带常量默认值的 ADD COLUMN（PostgreSQL 11 起不重写整表），普通事务迁移即可，
-- 不需要 _notx.sql 后缀。
-- 注意：正文与注释里都 MUST NOT 出现并发建索引的那个关键字——迁移校验器
-- （internal/repository/migrations_runner.go）对非 _notx.sql 文件是整文件裸匹配，
-- 不剥离注释，写进注释同样会让整轮迁移中止。

ALTER TABLE user_avatars
    ADD COLUMN IF NOT EXISTS thumb_url TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN user_avatars.thumb_url IS
    'Square 64px JPEG data URL derived from an inline avatar, for list views such as the leaderboard; empty means no thumbnail (remote_url avatars never have one, and rows predating this column are filled in by the startup backfill)';
