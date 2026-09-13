package repository

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/lib/pq"
)

// leaderboardTopModelsFallbackLimit 是调用方没给条数时的兜底：两处调用（今日模型热度 Top 8、
// 本人模型偏好 Top 5）都显式传入条数，这里只防 limit <= 0。
const leaderboardTopModelsFallbackLimit = 8

// leaderboardNightHourEnd 是夜猫子的时段上界（站点时区 0 点起、不含该点）。
// 夜间的划分只发生在下面 night_tokens 那一列的 SQL 上，所以这个数也只写在这里，
// 由 nightHourExpr 拼进 SQL——service 层拿到的是算好的 night_tokens，不需要再知道边界。
const leaderboardNightHourEnd = 6

// AggregateLeaderboardWindows 用一条 SQL、一次扫描 usage_logs 同时产出今日 / 本周 / 本月
// 三个 Window（榜单窗口）的按用户聚合，供 Snapshot（榜单快照）重建作业使用。
//
// 形状要点（design D6）：
//   - 扫描下界取 min(monthStart, weekStart)，三个窗口各自用条件聚合 FILTER 收敛，
//     绝不为每个窗口各跑一次聚合。
//   - INNER JOIN users 并过滤 status <> 'disabled' AND deleted_at IS NULL：
//     被禁用 / 已软删的用户与孤儿日志都不该进 Snapshot，也不该计入 Participant Count（参与人数）。
//     这与 GetUserBreakdownStats 的 LEFT JOIN 是有意的差异。
//   - Total Tokens（总 tokens）= input + output + cache_creation + cache_read，
//     与用户仪表盘、管理端 User Breakdown（用户用量明细）口径一致。
//   - Successful Requests（成功请求数）沿用 usageLogSuccessFilterUL（actual_cost > 0），
//     不是裸 COUNT(*)——否则「刷失败请求」就成了冲榜路径。
//   - 每个窗口另出 input_tokens 与 cache_read_tokens 两个 SUM，只用于 Cache Hit Rate
//     （缓存命中率），不参与排名；它们与前四个数来自同一次扫描，绝不另起一条查询。
//
// v2 在同一条 SQL 上扩容（design D20）：每个窗口再加六个数喂 Extremes（之最）与 Token 构成，
// 另加两个与窗口无关的「昨日」数当进步之星的基线，扫描下界相应放宽到
// min(月初, 周一, 昨日起点)。新增的十四个数一律不参与排名，ZSET 仍然只有两个 Metric。
//
// 新增列的口径分两类，刻意不统一（见 usagestats.LeaderboardAggregateRow 的注释）：
//   - output / cache_creation / night / yesterday_tokens 是 token 求和，沿用既有 token 列
//     「窗口内所有行」的口径，这样话痨的 output/total、Token 构成的四段占比与「较昨日」
//     的分子分母同源，也与 User Breakdown 的同名列对得上；
//   - distinct_models / max_single / media_requests / yesterday_requests 是按请求取值，
//     带 usageLogSuccessFilterUL，否则「刷失败请求」就能改写杂食者与单次最大。
//
// 返回行只含 user_id 与数值，不含身份与任何金额。
func (r *usageLogRepository) AggregateLeaderboardWindows(ctx context.Context, todayStart, weekStart, monthStart time.Time) (results []usagestats.LeaderboardAggregateRow, err error) {
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	scanStart := monthStart
	if weekStart.Before(scanStart) {
		scanStart = weekStart
	}
	if yesterdayStart.Before(scanStart) {
		scanStart = yesterdayStart
	}

	const totalTokensExpr = "ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens"
	// 夜间是站点时区的 0–6 点：created_at 是 timestamptz，先落到站点时区再取小时，
	// MUST NOT 直接对 UTC 取小时——那会让非 UTC 站点的夜猫子整整错位若干小时。
	nightHourExpr := "EXTRACT(HOUR FROM ul.created_at AT TIME ZONE $6::text) < " + strconv.Itoa(leaderboardNightHourEnd)
	const mediaExpr = "(COALESCE(ul.image_count, 0) > 0 OR COALESCE(ul.video_count, 0) > 0)"
	query := `
		SELECT
			ul.user_id,
			COALESCE(SUM(` + totalTokensExpr + `) FILTER (WHERE ul.created_at >= $1), 0) AS today_tokens,
			COUNT(*) FILTER (WHERE ul.created_at >= $1 AND ` + usageLogSuccessFilterUL + `) AS today_requests,
			COALESCE(SUM(ul.input_tokens) FILTER (WHERE ul.created_at >= $1), 0) AS today_input_tokens,
			COALESCE(SUM(ul.cache_read_tokens) FILTER (WHERE ul.created_at >= $1), 0) AS today_cache_read_tokens,
			COALESCE(SUM(` + totalTokensExpr + `) FILTER (WHERE ul.created_at >= $2), 0) AS week_tokens,
			COUNT(*) FILTER (WHERE ul.created_at >= $2 AND ` + usageLogSuccessFilterUL + `) AS week_requests,
			COALESCE(SUM(ul.input_tokens) FILTER (WHERE ul.created_at >= $2), 0) AS week_input_tokens,
			COALESCE(SUM(ul.cache_read_tokens) FILTER (WHERE ul.created_at >= $2), 0) AS week_cache_read_tokens,
			COALESCE(SUM(` + totalTokensExpr + `) FILTER (WHERE ul.created_at >= $3), 0) AS month_tokens,
			COUNT(*) FILTER (WHERE ul.created_at >= $3 AND ` + usageLogSuccessFilterUL + `) AS month_requests,
			COALESCE(SUM(ul.input_tokens) FILTER (WHERE ul.created_at >= $3), 0) AS month_input_tokens,
			COALESCE(SUM(ul.cache_read_tokens) FILTER (WHERE ul.created_at >= $3), 0) AS month_cache_read_tokens,

			COALESCE(SUM(ul.output_tokens) FILTER (WHERE ul.created_at >= $1), 0) AS today_output_tokens,
			COALESCE(SUM(ul.cache_creation_tokens) FILTER (WHERE ul.created_at >= $1), 0) AS today_cache_creation_tokens,
			COALESCE(SUM(` + totalTokensExpr + `) FILTER (WHERE ul.created_at >= $1 AND ` + nightHourExpr + `), 0) AS today_night_tokens,
			COUNT(DISTINCT ul.model) FILTER (WHERE ul.created_at >= $1 AND ` + usageLogSuccessFilterUL + `) AS today_distinct_models,
			COALESCE(MAX(` + totalTokensExpr + `) FILTER (WHERE ul.created_at >= $1 AND ` + usageLogSuccessFilterUL + `), 0) AS today_max_single_tokens,
			COUNT(*) FILTER (WHERE ul.created_at >= $1 AND ` + usageLogSuccessFilterUL + ` AND ` + mediaExpr + `) AS today_media_requests,

			COALESCE(SUM(ul.output_tokens) FILTER (WHERE ul.created_at >= $2), 0) AS week_output_tokens,
			COALESCE(SUM(ul.cache_creation_tokens) FILTER (WHERE ul.created_at >= $2), 0) AS week_cache_creation_tokens,
			COALESCE(SUM(` + totalTokensExpr + `) FILTER (WHERE ul.created_at >= $2 AND ` + nightHourExpr + `), 0) AS week_night_tokens,
			COUNT(DISTINCT ul.model) FILTER (WHERE ul.created_at >= $2 AND ` + usageLogSuccessFilterUL + `) AS week_distinct_models,
			COALESCE(MAX(` + totalTokensExpr + `) FILTER (WHERE ul.created_at >= $2 AND ` + usageLogSuccessFilterUL + `), 0) AS week_max_single_tokens,
			COUNT(*) FILTER (WHERE ul.created_at >= $2 AND ` + usageLogSuccessFilterUL + ` AND ` + mediaExpr + `) AS week_media_requests,

			COALESCE(SUM(ul.output_tokens) FILTER (WHERE ul.created_at >= $3), 0) AS month_output_tokens,
			COALESCE(SUM(ul.cache_creation_tokens) FILTER (WHERE ul.created_at >= $3), 0) AS month_cache_creation_tokens,
			COALESCE(SUM(` + totalTokensExpr + `) FILTER (WHERE ul.created_at >= $3 AND ` + nightHourExpr + `), 0) AS month_night_tokens,
			COUNT(DISTINCT ul.model) FILTER (WHERE ul.created_at >= $3 AND ` + usageLogSuccessFilterUL + `) AS month_distinct_models,
			COALESCE(MAX(` + totalTokensExpr + `) FILTER (WHERE ul.created_at >= $3 AND ` + usageLogSuccessFilterUL + `), 0) AS month_max_single_tokens,
			COUNT(*) FILTER (WHERE ul.created_at >= $3 AND ` + usageLogSuccessFilterUL + ` AND ` + mediaExpr + `) AS month_media_requests,

			COALESCE(SUM(` + totalTokensExpr + `) FILTER (WHERE ul.created_at >= $5 AND ul.created_at < $1), 0) AS yesterday_tokens,
			COUNT(*) FILTER (WHERE ul.created_at >= $5 AND ul.created_at < $1 AND ` + usageLogSuccessFilterUL + `) AS yesterday_requests
		FROM usage_logs ul
		INNER JOIN users u ON u.id = ul.user_id
		WHERE ul.created_at >= $4
		  AND u.status <> 'disabled'
		  AND u.deleted_at IS NULL
		GROUP BY ul.user_id
	`

	rows, err := r.sql.QueryContext(ctx, query, todayStart, weekStart, monthStart, scanStart, yesterdayStart, resolveUsageStatsTimezone())
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]usagestats.LeaderboardAggregateRow, 0)
	for rows.Next() {
		var row usagestats.LeaderboardAggregateRow
		if err := rows.Scan(
			&row.UserID,
			&row.TodayTokens,
			&row.TodayRequests,
			&row.TodayInputTokens,
			&row.TodayCacheReadTokens,
			&row.WeekTokens,
			&row.WeekRequests,
			&row.WeekInputTokens,
			&row.WeekCacheReadTokens,
			&row.MonthTokens,
			&row.MonthRequests,
			&row.MonthInputTokens,
			&row.MonthCacheReadTokens,
			&row.TodayOutputTokens,
			&row.TodayCacheCreationTokens,
			&row.TodayNightTokens,
			&row.TodayDistinctModels,
			&row.TodayMaxSingleTokens,
			&row.TodayMediaRequests,
			&row.WeekOutputTokens,
			&row.WeekCacheCreationTokens,
			&row.WeekNightTokens,
			&row.WeekDistinctModels,
			&row.WeekMaxSingleTokens,
			&row.WeekMediaRequests,
			&row.MonthOutputTokens,
			&row.MonthCacheCreationTokens,
			&row.MonthNightTokens,
			&row.MonthDistinctModels,
			&row.MonthMaxSingleTokens,
			&row.MonthMediaRequests,
			&row.YesterdayTokens,
			&row.YesterdayRequests,
		); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// LeaderboardTopModelsToday 按模型聚合今日的成功请求数，供 Insights（洞察）的模型热度使用。
//
// 计数带 usageLogSuccessFilterUL（actual_cost > 0），与榜单的 Successful Requests 同口径，
// 与两张预聚合表的裸 COUNT(*) 不同——两者在响应里也刻意不共用字段名。
// 第二个返回值是今日**全站**（不止 Top N）的成功请求总数，是 share_percent 的分母：
// 用窗口函数在同一条 SQL 里算出，避免为一个分母再扫一次今日日志。
func (r *usageLogRepository) LeaderboardTopModelsToday(ctx context.Context, todayStart, todayEnd time.Time, limit int) (rows []usagestats.LeaderboardModelUsageRow, total int64, err error) {
	if limit <= 0 {
		limit = leaderboardTopModelsFallbackLimit
	}
	query := `
		WITH per_model AS (
			SELECT
				ul.model AS model,
				COUNT(*) AS successful_requests
			FROM usage_logs ul
			WHERE ul.created_at >= $1
			  AND ul.created_at < $2
			  AND ` + usageLogSuccessFilterUL + `
			GROUP BY ul.model
		)
		SELECT
			model,
			successful_requests,
			SUM(successful_requests) OVER () AS total_requests
		FROM per_model
		ORDER BY successful_requests DESC, model ASC
		LIMIT $3
	`

	sqlRows, err := r.sql.QueryContext(ctx, query, todayStart, todayEnd, limit)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if closeErr := sqlRows.Close(); closeErr != nil && err == nil {
			err = closeErr
			rows, total = nil, 0
		}
	}()

	rows = make([]usagestats.LeaderboardModelUsageRow, 0, limit)
	for sqlRows.Next() {
		var row usagestats.LeaderboardModelUsageRow
		var rowTotal int64
		if err := sqlRows.Scan(&row.Model, &row.SuccessfulRequests, &rowTotal); err != nil {
			return nil, 0, err
		}
		total = rowTotal
		rows = append(rows, row)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// LeaderboardDominantModel 返回某个用户在 [start, end) 内成功请求最多的模型，
// 用于 Highlights（趣味卡）里效率之星的 dominant model。无成功请求时返回空串。
//
// 只在重建时对选中的那一个用户查一次：这条按 (user_id, model) 的聚合 MUST NOT 进请求路径。
func (r *usageLogRepository) LeaderboardDominantModel(ctx context.Context, userID int64, start, end time.Time) (string, error) {
	query := `
		SELECT ul.model
		FROM usage_logs ul
		WHERE ul.user_id = $1
		  AND ul.created_at >= $2
		  AND ul.created_at < $3
		  AND ` + usageLogSuccessFilterUL + `
		GROUP BY ul.model
		ORDER BY COUNT(*) DESC, ul.model ASC
		LIMIT 1
	`
	var model string
	if err := scanSingleRow(ctx, r.sql, query, []any{userID, start, end}, &model); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return model, nil
}

// LeaderboardDailyBuckets 读 usage_dashboard_daily 的 [fromDate, toDate) 日桶，按日期升序。
//
// 该表的桶边界本来就是站点时区（dashboard_aggregation_repo.go 的 upsertDailyAggregates 用
// (bucket_start AT TIME ZONE $5)::date 落桶），因此这里 MUST NOT 再做一次 UTC → 本地映射；
// 表注释里的「UTC dates」是过时的。
//
// 预聚合作业未启用、或保留期没覆盖到这段日期时返回空切片而不是报错——
// 缺的天由上层整块置 null，MUST NOT 补成 0。
func (r *usageLogRepository) LeaderboardDailyBuckets(ctx context.Context, fromDate, toDate time.Time) (results []usagestats.LeaderboardDailyBucketRow, err error) {
	query := `
		SELECT
			bucket_date,
			total_requests,
			(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) AS total_tokens,
			input_tokens,
			cache_read_tokens
		FROM usage_dashboard_daily
		WHERE bucket_date >= $1::date AND bucket_date < $2::date
		ORDER BY bucket_date ASC
	`
	rows, err := r.sql.QueryContext(ctx, query, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]usagestats.LeaderboardDailyBucketRow, 0)
	for rows.Next() {
		var row usagestats.LeaderboardDailyBucketRow
		if err := rows.Scan(&row.Date, &row.Requests, &row.TotalTokens, &row.InputTokens, &row.CacheReadTokens); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// LeaderboardHourlyBuckets 读 usage_dashboard_hourly 的 [from, to) 小时桶，按桶起点升序。
// 桶边界同样已是站点时区（upsertHourlyAggregates 的 date_trunc('hour', created_at AT TIME ZONE $3)），
// 因此今日 24 桶只读今日这一段，不必为跨 UTC 日多读一天。缺行时返回空切片。
func (r *usageLogRepository) LeaderboardHourlyBuckets(ctx context.Context, from, to time.Time) (results []usagestats.LeaderboardHourlyBucketRow, err error) {
	query := `
		SELECT
			bucket_start,
			total_requests,
			input_tokens,
			cache_read_tokens
		FROM usage_dashboard_hourly
		WHERE bucket_start >= $1 AND bucket_start < $2
		ORDER BY bucket_start ASC
	`
	rows, err := r.sql.QueryContext(ctx, query, from, to)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]usagestats.LeaderboardHourlyBucketRow, 0)
	for rows.Next() {
		var row usagestats.LeaderboardHourlyBucketRow
		if err := rows.Scan(&row.BucketStart, &row.Requests, &row.InputTokens, &row.CacheReadTokens); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// LeaderboardTopStreak 返回「连续活跃」之最：截至今日或昨日、连续有用量天数最长的那个人。
//
// 数据源是 usage_dashboard_daily_users（每天每人一行，主键即索引），MUST NOT 回去扫 usage_logs：
// 连续活跃天数本身跨窗口，按 Window 切反而算不出「连续 23 天」这种事实（design D20）。
// 参与资格与聚合 SQL 同一条：INNER JOIN users 过滤禁用与软删除，孤儿行自然被排除。
//
// 算法是 gaps-and-islands：同一个人按日期排序后，bucket_date 减去行号在一段连续日期上是常数，
// 因此这个差值就是「第几段连续」的分组键；每段的行数即天数，MAX(bucket_date) 即该段的末日。
//
// 区间是半开的 [fromDate, toDate)，与 LeaderboardDailyBuckets 一致：toDate 取今日的次日零点，
// 因此「截至今日或昨日」等价于末日不早于 toDate::date - 2。无人满足时返回零值（Days 为 0），
// 由上层渲染成「没有这张卡」。
func (r *usageLogRepository) LeaderboardTopStreak(ctx context.Context, fromDate, toDate time.Time) (usagestats.LeaderboardStreakRow, error) {
	query := `
		WITH active_days AS (
			SELECT du.user_id AS user_id, du.bucket_date AS bucket_date
			FROM usage_dashboard_daily_users du
			INNER JOIN users u ON u.id = du.user_id
			WHERE du.bucket_date >= $1::date
			  AND du.bucket_date < $2::date
			  AND u.status <> 'disabled'
			  AND u.deleted_at IS NULL
		),
		islands AS (
			SELECT
				user_id,
				bucket_date,
				bucket_date - (ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY bucket_date))::int AS island_key
			FROM active_days
		),
		runs AS (
			SELECT
				user_id,
				COUNT(*) AS streak_days,
				MAX(bucket_date) AS last_date
			FROM islands
			GROUP BY user_id, island_key
		)
		SELECT user_id, streak_days
		FROM runs
		WHERE last_date >= $2::date - 2
		ORDER BY streak_days DESC, user_id ASC
		LIMIT 1
	`
	var row usagestats.LeaderboardStreakRow
	if err := scanSingleRow(ctx, r.sql, query, []any{fromDate, toDate}, &row.UserID, &row.Days); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return usagestats.LeaderboardStreakRow{}, nil
		}
		return usagestats.LeaderboardStreakRow{}, err
	}
	return row, nil
}

// LeaderboardUserModelBreakdown 把指定的一小撮 user_id 在 [start, end) 内按 (user_id, model) 聚合，
// 供模型偏好画像使用：一条 SQL 出这几个人的全部模型，MUST NOT 逐人各查一次（design D22）。
//
// userIDs 只会是该 Window Total Tokens 的前 50 名（与榜单本体同一个数），因此 = ANY 的集合很小；
// 为空时直接返回空切片，连库都不查。排序保证同一个人的模型按成功请求数倒序、同数按模型名升序，
// 上层据此直接按序列出每人的全部模型，不必再排一次。
func (r *usageLogRepository) LeaderboardUserModelBreakdown(ctx context.Context, userIDs []int64, start, end time.Time) (results []usagestats.LeaderboardUserModelUsageRow, err error) {
	if len(userIDs) == 0 {
		return []usagestats.LeaderboardUserModelUsageRow{}, nil
	}
	query := `
		SELECT
			ul.user_id,
			ul.model,
			COUNT(*) AS successful_requests
		FROM usage_logs ul
		WHERE ul.user_id = ANY($1)
		  AND ul.created_at >= $2
		  AND ul.created_at < $3
		  AND ` + usageLogSuccessFilterUL + `
		GROUP BY ul.user_id, ul.model
		ORDER BY ul.user_id ASC, successful_requests DESC, ul.model ASC
	`
	rows, err := r.sql.QueryContext(ctx, query, pq.Array(userIDs), start, end)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]usagestats.LeaderboardUserModelUsageRow, 0, len(userIDs))
	for rows.Next() {
		var row usagestats.LeaderboardUserModelUsageRow
		if err := rows.Scan(&row.UserID, &row.Model, &row.SuccessfulRequests); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// LeaderboardPlatformsToday 按账号平台聚合今日的成功请求，供 Insights（洞察）的平台分布使用。
// 第二个返回值是今日全站（不止返回的这几行）的成功请求总数，即 share_percent 的分母，
// 用窗口函数在同一条 SQL 里算出。
//
// 用 INNER JOIN accounts 而不是渠道监控那样的 LEFT JOIN：账号已被删掉的日志没有平台可归，
// 不该凑出一个空平台分类（design D22），这与聚合 SQL 对孤儿日志的处理是同一条规则。
func (r *usageLogRepository) LeaderboardPlatformsToday(ctx context.Context, todayStart, todayEnd time.Time) (rows []usagestats.LeaderboardPlatformUsageRow, total int64, err error) {
	query := `
		WITH per_platform AS (
			SELECT
				a.platform AS platform,
				COUNT(*) AS successful_requests
			FROM usage_logs ul
			INNER JOIN accounts a ON a.id = ul.account_id
			WHERE ul.created_at >= $1
			  AND ul.created_at < $2
			  AND ` + usageLogSuccessFilterUL + `
			GROUP BY a.platform
		)
		SELECT
			platform,
			successful_requests,
			SUM(successful_requests) OVER () AS total_requests
		FROM per_platform
		ORDER BY successful_requests DESC, platform ASC
	`
	sqlRows, err := r.sql.QueryContext(ctx, query, todayStart, todayEnd)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if closeErr := sqlRows.Close(); closeErr != nil && err == nil {
			err = closeErr
			rows, total = nil, 0
		}
	}()

	rows = make([]usagestats.LeaderboardPlatformUsageRow, 0)
	for sqlRows.Next() {
		var row usagestats.LeaderboardPlatformUsageRow
		var rowTotal int64
		if err := sqlRows.Scan(&row.Platform, &row.SuccessfulRequests, &rowTotal); err != nil {
			return nil, 0, err
		}
		total = rowTotal
		rows = append(rows, row)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// LeaderboardWeekdayHourBuckets 把 [from, to) 的 usage_dashboard_hourly 按 (周几, 小时) 求平均请求数，
// 供周内节奏热力图使用。周几是 ISO 编号（1 = 周一 … 7 = 周日），与热力图「行序周一起」一致。
//
// bucket_start 是 timestamptz，它的**瞬间**对应站点时区某个整点的起点；要拿回那个整点的墙钟，
// 必须 AT TIME ZONE 站点时区还原一次——这与 LeaderboardHourlyBuckets 在 Go 侧做的
// .In(timezone.Location()).Hour() 是同一件事，不是「再做一次 UTC → 本地映射」。
// 直接对 timestamptz 取 EXTRACT 会跟着数据库会话时区跑偏，非 UTC 站点会整块错位。
//
// 表在这段区间里一行都没有时返回空切片而不是报错：由上层整块置 null，MUST NOT 补成 0。
func (r *usageLogRepository) LeaderboardWeekdayHourBuckets(ctx context.Context, from, to time.Time) (results []usagestats.LeaderboardWeekdayHourRow, err error) {
	query := `
		SELECT
			EXTRACT(ISODOW FROM (bucket_start AT TIME ZONE $3::text))::int AS weekday,
			EXTRACT(HOUR FROM (bucket_start AT TIME ZONE $3::text))::int AS hour,
			FLOOR(AVG(total_requests))::bigint AS requests
		FROM usage_dashboard_hourly
		WHERE bucket_start >= $1 AND bucket_start < $2
		GROUP BY 1, 2
		ORDER BY 1, 2
	`
	rows, err := r.sql.QueryContext(ctx, query, from, to, resolveUsageStatsTimezone())
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			results = nil
		}
	}()

	results = make([]usagestats.LeaderboardWeekdayHourRow, 0)
	for rows.Next() {
		var row usagestats.LeaderboardWeekdayHourRow
		if err := rows.Scan(&row.Weekday, &row.Hour, &row.Requests); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// LeaderboardViewerModels 按模型聚合**查看者本人**在 [start, end) 内的成功请求，
// 是 design D8「请求路径只读」的唯一例外（design D21）。例外的边界写死三条：
//   - SQL MUST 带 ul.user_id = $1，只扫这一个人的行（走 (user_id, created_at) 索引）；
//   - 只服务 viewer.models 这一个字段；
//   - 结果在 Redis 上按 (user_id, window, 窗口起点) 缓存 60 秒，见 leaderboard_cache.go。
//
// 第二个返回值是本人在该窗口的成功请求总数（不止 Top N），即 share_percent 的分母。
func (r *usageLogRepository) LeaderboardViewerModels(ctx context.Context, userID int64, start, end time.Time, limit int) (rows []usagestats.LeaderboardModelUsageRow, total int64, err error) {
	if limit <= 0 {
		limit = leaderboardTopModelsFallbackLimit
	}
	query := `
		WITH per_model AS (
			SELECT
				ul.model AS model,
				COUNT(*) AS successful_requests
			FROM usage_logs ul
			WHERE ul.user_id = $1
			  AND ul.created_at >= $2
			  AND ul.created_at < $3
			  AND ` + usageLogSuccessFilterUL + `
			GROUP BY ul.model
		)
		SELECT
			model,
			successful_requests,
			SUM(successful_requests) OVER () AS total_requests
		FROM per_model
		ORDER BY successful_requests DESC, model ASC
		LIMIT $4
	`
	sqlRows, err := r.sql.QueryContext(ctx, query, userID, start, end, limit)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if closeErr := sqlRows.Close(); closeErr != nil && err == nil {
			err = closeErr
			rows, total = nil, 0
		}
	}()

	rows = make([]usagestats.LeaderboardModelUsageRow, 0, limit)
	for sqlRows.Next() {
		var row usagestats.LeaderboardModelUsageRow
		var rowTotal int64
		if err := sqlRows.Scan(&row.Model, &row.SuccessfulRequests, &rowTotal); err != nil {
			return nil, 0, err
		}
		total = rowTotal
		rows = append(rows, row)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
