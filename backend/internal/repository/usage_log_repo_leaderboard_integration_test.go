//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 固定的三个窗口起点：monthStart < weekStart < todayStart，因此扫描下界取月初。
// 用固定时间而不是 time.Now()，避免「今天恰好是月初 / 周一」时三个窗口重合导致断言退化。
var (
	leaderboardMonthStart = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	leaderboardWeekStart  = time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC) // 周一
	leaderboardTodayStart = time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)
)

func leaderboardRowsByUser(t *testing.T, rows []usagestats.LeaderboardAggregateRow) map[int64]usagestats.LeaderboardAggregateRow {
	t.Helper()
	byUser := make(map[int64]usagestats.LeaderboardAggregateRow, len(rows))
	for _, row := range rows {
		byUser[row.UserID] = row
	}
	return byUser
}

// 一次调用同时返回三个窗口的九个数（tokens / requests / cost 各三份）：
// 条件聚合 FILTER 收敛，绝不是每个窗口各跑一次。
// 同时验证 Successful Requests 只计 actual_cost > 0 的成功落账行，
// 而 Total Tokens 对窗口内所有行求和（与 GetUserBreakdownStats 口径一致）。
func TestUsageLog_AggregateLeaderboardWindows_ThreeWindowsInOnePass(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	user := mustCreateUser(t, client, &service.User{Email: "leaderboard-windows@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-leaderboard-windows", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-leaderboard-windows"})

	// 只在本月窗口内：月初 +1h。
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardMonthStart.Add(time.Hour), 1, 2, 3, 4, 0.5)
	// 本周与本月窗口内：周一 +1h。
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardWeekStart.Add(time.Hour), 2, 2, 2, 2, 0.5)
	// 三个窗口都在内：今日 +1h。
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardTodayStart.Add(time.Hour), 5, 5, 5, 5, 0.5)
	// 失败占位行：actual_cost = 0，token 仍计入 Total Tokens，但不计入 Successful Requests。
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardTodayStart.Add(2*time.Hour), 7, 0, 0, 0, 0)

	rows, err := repo.AggregateLeaderboardWindows(ctx, leaderboardTodayStart, leaderboardWeekStart, leaderboardMonthStart)
	require.NoError(t, err)

	got, ok := leaderboardRowsByUser(t, rows)[user.ID]
	require.True(t, ok, "有用量的合格用户必须出现在聚合结果里")
	require.Equal(t, int64(27), got.TodayTokens, "今日 Total Tokens = 20 + 失败占位行的 7")
	require.Equal(t, int64(1), got.TodayRequests, "今日 Successful Requests 不含 actual_cost = 0 的占位行")
	require.Equal(t, int64(35), got.WeekTokens)
	require.Equal(t, int64(2), got.WeekRequests)
	require.Equal(t, int64(45), got.MonthTokens)
	require.Equal(t, int64(3), got.MonthRequests)
}

// Total Tokens 与管理端 User Breakdown（GetUserBreakdownStats）的口径必须一致，
// 否则用户会拿两个页面对账。
func TestUsageLog_AggregateLeaderboardWindows_TotalTokensMatchesUserBreakdown(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	user := mustCreateUser(t, client, &service.User{Email: "leaderboard-parity@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-leaderboard-parity", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-leaderboard-parity"})

	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardMonthStart.Add(time.Hour), 11, 13, 17, 19, 0.3)
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardTodayStart.Add(time.Hour), 23, 29, 31, 37, 0.7)
	// 失败占位行同样进入两侧的 token 求和。
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardTodayStart.Add(2*time.Hour), 3, 0, 0, 0, 0)

	rows, err := repo.AggregateLeaderboardWindows(ctx, leaderboardTodayStart, leaderboardWeekStart, leaderboardMonthStart)
	require.NoError(t, err)
	got, ok := leaderboardRowsByUser(t, rows)[user.ID]
	require.True(t, ok)

	breakdown, err := repo.GetUserBreakdownStats(ctx,
		leaderboardMonthStart,
		leaderboardTodayStart.Add(24*time.Hour),
		usagestats.UserBreakdownDimension{UserID: user.ID},
		0,
	)
	require.NoError(t, err)
	require.Len(t, breakdown, 1)
	require.Equal(t, breakdown[0].TotalTokens, got.MonthTokens, "本月 Total Tokens 必须与 User Breakdown 对得上")
	// 请求数口径是有意的差异：User Breakdown 是裸 COUNT(*)，Leaderboard 只计成功落账。
	require.Equal(t, int64(3), breakdown[0].Requests)
	require.Equal(t, int64(2), got.MonthRequests)
}

// INNER JOIN users + status <> 'disabled' + deleted_at IS NULL：
// 被禁用与已软删的用户都不进 Snapshot，也不计入 Participant Count。
func TestUsageLog_AggregateLeaderboardWindows_ExcludesDisabledAndDeletedUsers(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	account := mustCreateAccount(t, client, &service.Account{Name: "acc-leaderboard-eligibility"})

	active := mustCreateUser(t, client, &service.User{Email: "leaderboard-active@test.com"})
	activeKey := mustCreateApiKey(t, client, &service.APIKey{UserID: active.ID, Key: "sk-leaderboard-active", Name: "k"})
	mustCreateLeaderboardLog(t, repo, active.ID, activeKey.ID, account.ID, leaderboardTodayStart.Add(time.Hour), 1, 1, 1, 1, 0.5)

	disabled := mustCreateUser(t, client, &service.User{Email: "leaderboard-disabled@test.com", Status: service.StatusDisabled})
	disabledKey := mustCreateApiKey(t, client, &service.APIKey{UserID: disabled.ID, Key: "sk-leaderboard-disabled", Name: "k"})
	mustCreateLeaderboardLog(t, repo, disabled.ID, disabledKey.ID, account.ID, leaderboardTodayStart.Add(time.Hour), 100, 100, 100, 100, 9)

	softDeleted := mustCreateUser(t, client, &service.User{Email: "leaderboard-deleted@test.com"})
	deletedKey := mustCreateApiKey(t, client, &service.APIKey{UserID: softDeleted.ID, Key: "sk-leaderboard-deleted", Name: "k"})
	mustCreateLeaderboardLog(t, repo, softDeleted.ID, deletedKey.ID, account.ID, leaderboardTodayStart.Add(time.Hour), 100, 100, 100, 100, 9)
	// 触发 SoftDeleteMixin Hook → UPDATE deleted_at。
	require.NoError(t, client.User.DeleteOneID(softDeleted.ID).Exec(ctx))

	rows, err := repo.AggregateLeaderboardWindows(ctx, leaderboardTodayStart, leaderboardWeekStart, leaderboardMonthStart)
	require.NoError(t, err)
	byUser := leaderboardRowsByUser(t, rows)

	require.Contains(t, byUser, active.ID, "管理员之外的合格用户照常参与聚合")
	require.NotContains(t, byUser, disabled.ID, "status = disabled 的用户不得进入 Snapshot")
	require.NotContains(t, byUser, softDeleted.ID, "deleted_at 非空的用户不得进入 Snapshot")
}

// 孤儿日志（user_id 在 users 里已无对应行）必须被 INNER JOIN 排除。
// 线上 usage_logs.user_id 有 ON DELETE CASCADE 外键，孤儿行只可能来自历史数据修复，
// 因此这里在事务内临时摘掉外键再造一行，事务回滚后约束自动恢复。
func TestUsageLog_AggregateLeaderboardWindows_ExcludesOrphanLogs(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	owner := mustCreateUser(t, client, &service.User{Email: "leaderboard-orphan-owner@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: owner.ID, Key: "sk-leaderboard-orphan", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-leaderboard-orphan"})

	_, err := tx.ExecContext(ctx, "ALTER TABLE usage_logs DROP CONSTRAINT IF EXISTS usage_logs_user_id_fkey")
	require.NoError(t, err)

	const orphanUserID int64 = 9_000_000_001
	mustCreateLeaderboardLog(t, repo, orphanUserID, apiKey.ID, account.ID, leaderboardTodayStart.Add(time.Hour), 50, 50, 50, 50, 5)
	mustCreateLeaderboardLog(t, repo, owner.ID, apiKey.ID, account.ID, leaderboardTodayStart.Add(time.Hour), 1, 1, 1, 1, 0.5)

	rows, err := repo.AggregateLeaderboardWindows(ctx, leaderboardTodayStart, leaderboardWeekStart, leaderboardMonthStart)
	require.NoError(t, err)
	byUser := leaderboardRowsByUser(t, rows)

	require.Contains(t, byUser, owner.ID)
	require.NotContains(t, byUser, orphanUserID, "孤儿日志不得产生任何榜单条目")
}

// 扫描下界取 min(月初, 周一)：早于下界的日志不参与本轮聚合。
func TestUsageLog_AggregateLeaderboardWindows_ScanLowerBound(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	user := mustCreateUser(t, client, &service.User{Email: "leaderboard-lowerbound@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-leaderboard-lowerbound", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-leaderboard-lowerbound"})

	// 只有一条上个月的日志：整行都不应出现。
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardMonthStart.Add(-24*time.Hour), 9, 9, 9, 9, 1)

	rows, err := repo.AggregateLeaderboardWindows(ctx, leaderboardTodayStart, leaderboardWeekStart, leaderboardMonthStart)
	require.NoError(t, err)
	require.NotContains(t, leaderboardRowsByUser(t, rows), user.ID)

	// 本月窗口早于本周窗口时，扫描下界必须是月初——把窗口起点换成「本周早于本月」不可能发生，
	// 因此这里只再确认一次：把月初放到日志之前，同一条日志立刻可见。
	rows, err = repo.AggregateLeaderboardWindows(ctx,
		leaderboardTodayStart,
		leaderboardWeekStart,
		leaderboardMonthStart.AddDate(0, -1, 0),
	)
	require.NoError(t, err)
	got, ok := leaderboardRowsByUser(t, rows)[user.ID]
	require.True(t, ok)
	require.Equal(t, int64(36), got.MonthTokens)
	require.Zero(t, got.WeekTokens)
	require.Zero(t, got.TodayTokens)
}

func mustCreateLeaderboardLog(
	t *testing.T,
	repo *usageLogRepository,
	userID, apiKeyID, accountID int64,
	createdAt time.Time,
	input, output, cacheCreation, cacheRead int,
	actualCost float64,
) {
	t.Helper()
	_, err := repo.Create(context.Background(), &service.UsageLog{
		UserID:              userID,
		APIKeyID:            apiKeyID,
		AccountID:           accountID,
		Model:               "claude-3",
		InputTokens:         input,
		OutputTokens:        output,
		CacheCreationTokens: cacheCreation,
		CacheReadTokens:     cacheRead,
		TotalCost:           actualCost,
		ActualCost:          actualCost,
		CreatedAt:           createdAt,
	})
	require.NoError(t, err)
}

// input_tokens / cache_read_tokens 两列与管理端 User Breakdown（用户用量明细）同源：
// 命中率算出来才不会与其它页面互相矛盾。
func TestUsageLog_AggregateLeaderboardWindows_CacheColumnsMatchUserBreakdown(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	user := mustCreateUser(t, client, &service.User{Email: "leaderboard-cache-parity@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-leaderboard-cache-parity", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-leaderboard-cache-parity"})

	// input / output / cache_creation / cache_read
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardMonthStart.Add(time.Hour), 11, 13, 17, 19, 0.3)
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardTodayStart.Add(time.Hour), 23, 29, 31, 37, 0.7)
	// 失败占位行的 token 同样进两侧求和（它只影响请求数口径，不影响 token 口径）。
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardTodayStart.Add(2*time.Hour), 5, 0, 0, 7, 0)

	rows, err := repo.AggregateLeaderboardWindows(ctx, leaderboardTodayStart, leaderboardWeekStart, leaderboardMonthStart)
	require.NoError(t, err)
	got, ok := leaderboardRowsByUser(t, rows)[user.ID]
	require.True(t, ok)

	require.Equal(t, int64(23+5), got.TodayInputTokens)
	require.Equal(t, int64(37+7), got.TodayCacheReadTokens)
	require.Equal(t, int64(11+23+5), got.MonthInputTokens)
	require.Equal(t, int64(19+37+7), got.MonthCacheReadTokens)

	breakdown, err := repo.GetUserBreakdownStats(ctx,
		leaderboardMonthStart,
		leaderboardTodayStart.Add(24*time.Hour),
		usagestats.UserBreakdownDimension{UserID: user.ID},
		0,
	)
	require.NoError(t, err)
	require.Len(t, breakdown, 1)
	require.Equal(t, breakdown[0].InputTokens, got.MonthInputTokens, "输入 tokens 必须与 User Breakdown 对得上")
	// User Breakdown 的 CacheTokens 是「创建 + 读取」，榜单只取读取那一半。
	require.Equal(t, breakdown[0].CacheTokens, got.MonthCacheReadTokens+int64(17+31+0))
}

// 第三个 Metric（Cost）的三列：窗口内 actual_cost 之和（USD），
// 与管理端 User Breakdown（用户用量明细）的 actual_cost 合计对得上——
// 用户自己的用量页「花费」用的就是这一列，两处对账不该出现差额。
func TestUsageLog_AggregateLeaderboardWindows_CostColumnsMatchUserBreakdown(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	user := mustCreateUser(t, client, &service.User{Email: "leaderboard-cost-parity@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-leaderboard-cost-parity", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-leaderboard-cost-parity"})

	// 金额取二进制可精确表示的值，断言才不会被浮点尾差干扰。
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardMonthStart.Add(time.Hour), 1, 1, 1, 1, 0.25)
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardWeekStart.Add(time.Hour), 1, 1, 1, 1, 0.5)
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardTodayStart.Add(time.Hour), 1, 1, 1, 1, 1.125)
	// 失败占位行：actual_cost = 0，token 照常进求和，金额则原样不变。
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardTodayStart.Add(2*time.Hour), 9, 0, 0, 0, 0)
	// 唯一一条 total_cost != actual_cost 的行：求和 MUST 取 actual_cost，
	// 否则这三条断言与下面的 User Breakdown 对账都会失败（其余 fixture 两列相等，钉不住列的选择）。
	_, err := repo.Create(ctx, &service.UsageLog{
		UserID:      user.ID,
		APIKeyID:    apiKey.ID,
		AccountID:   account.ID,
		Model:       "claude-3",
		InputTokens: 1,
		TotalCost:   10,
		ActualCost:  2,
		CreatedAt:   leaderboardTodayStart.Add(3 * time.Hour),
	})
	require.NoError(t, err)

	rows, err := repo.AggregateLeaderboardWindows(ctx, leaderboardTodayStart, leaderboardWeekStart, leaderboardMonthStart)
	require.NoError(t, err)
	got, ok := leaderboardRowsByUser(t, rows)[user.ID]
	require.True(t, ok)

	require.InDelta(t, 3.125, got.TodayCost, 1e-9, "求和取 actual_cost，MUST NOT 取 total_cost")
	require.InDelta(t, 3.625, got.WeekCost, 1e-9)
	require.InDelta(t, 3.875, got.MonthCost, 1e-9)
	require.Equal(t, int64(4), got.MonthRequests, "失败占位行不计入成功请求，但金额本来就是 0")

	breakdown, err := repo.GetUserBreakdownStats(ctx,
		leaderboardMonthStart,
		leaderboardTodayStart.Add(24*time.Hour),
		usagestats.UserBreakdownDimension{UserID: user.ID},
		0,
	)
	require.NoError(t, err)
	require.Len(t, breakdown, 1)
	require.InDelta(t, breakdown[0].ActualCost, got.MonthCost, 1e-9,
		"本月 Cost 必须与 User Breakdown 的 actual_cost 合计对得上")
}

// 今日模型热度只计成功落账的请求，并给出今日全站总数作为占比分母。
func TestUsageLog_LeaderboardTopModelsToday(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	user := mustCreateUser(t, client, &service.User{Email: "leaderboard-models@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-leaderboard-models", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-leaderboard-models"})
	todayEnd := leaderboardTodayStart.Add(24 * time.Hour)

	mustCreateLeaderboardModelLog(t, repo, user.ID, apiKey.ID, account.ID, "claude-sonnet-5", leaderboardTodayStart.Add(time.Hour), 0.5)
	mustCreateLeaderboardModelLog(t, repo, user.ID, apiKey.ID, account.ID, "claude-sonnet-5", leaderboardTodayStart.Add(2*time.Hour), 0.5)
	mustCreateLeaderboardModelLog(t, repo, user.ID, apiKey.ID, account.ID, "claude-opus-5", leaderboardTodayStart.Add(3*time.Hour), 0.5)
	// 失败占位行：actual_cost = 0，既不进某个模型的计数，也不进分母。
	mustCreateLeaderboardModelLog(t, repo, user.ID, apiKey.ID, account.ID, "claude-opus-5", leaderboardTodayStart.Add(4*time.Hour), 0)
	// 昨天的成功请求不属于今日。
	mustCreateLeaderboardModelLog(t, repo, user.ID, apiKey.ID, account.ID, "gpt-5.1", leaderboardTodayStart.Add(-time.Hour), 0.5)

	rows, total, err := repo.LeaderboardTopModelsToday(ctx, leaderboardTodayStart, todayEnd, 8)
	require.NoError(t, err)
	require.Equal(t, []usagestats.LeaderboardModelUsageRow{
		{Model: "claude-sonnet-5", SuccessfulRequests: 2},
		{Model: "claude-opus-5", SuccessfulRequests: 1},
	}, rows)
	require.Equal(t, int64(3), total, "分母是今日全站成功请求数，不含失败占位行与昨天")

	// limit 生效：只要 Top 1，分母仍是全站总数。
	rows, total, err = repo.LeaderboardTopModelsToday(ctx, leaderboardTodayStart, todayEnd, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "claude-sonnet-5", rows[0].Model)
	require.Equal(t, int64(3), total)

	// 今日一条成功请求都没有：空切片 + 0，而不是报错。
	rows, total, err = repo.LeaderboardTopModelsToday(ctx, todayEnd, todayEnd.Add(24*time.Hour), 8)
	require.NoError(t, err)
	require.Empty(t, rows)
	require.Zero(t, total)
}

// dominant model 取该用户该窗口成功请求最多的模型；没有成功请求时是空串。
func TestUsageLog_LeaderboardDominantModel(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	user := mustCreateUser(t, client, &service.User{Email: "leaderboard-dominant@test.com"})
	other := mustCreateUser(t, client, &service.User{Email: "leaderboard-dominant-other@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-leaderboard-dominant", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-leaderboard-dominant"})
	todayEnd := leaderboardTodayStart.Add(24 * time.Hour)

	mustCreateLeaderboardModelLog(t, repo, user.ID, apiKey.ID, account.ID, "claude-opus-5", leaderboardTodayStart.Add(time.Hour), 0.5)
	mustCreateLeaderboardModelLog(t, repo, user.ID, apiKey.ID, account.ID, "claude-sonnet-5", leaderboardTodayStart.Add(2*time.Hour), 0.5)
	mustCreateLeaderboardModelLog(t, repo, user.ID, apiKey.ID, account.ID, "claude-sonnet-5", leaderboardTodayStart.Add(3*time.Hour), 0.5)
	// 失败占位行不参与评选，否则「刷失败请求」能改写别人的主力模型。
	for i := 0; i < 5; i++ {
		mustCreateLeaderboardModelLog(t, repo, user.ID, apiKey.ID, account.ID, "gpt-5.1", leaderboardTodayStart.Add(time.Duration(4+i)*time.Hour), 0)
	}

	model, err := repo.LeaderboardDominantModel(ctx, user.ID, leaderboardTodayStart, todayEnd)
	require.NoError(t, err)
	require.Equal(t, "claude-sonnet-5", model)

	model, err = repo.LeaderboardDominantModel(ctx, other.ID, leaderboardTodayStart, todayEnd)
	require.NoError(t, err)
	require.Empty(t, model, "没有成功请求时返回空串而不是报错")
}

// 两个预聚合表读取方法：表为空时返回空切片；有行时按站点时区的桶边界原样返回。
func TestUsageLog_LeaderboardPreAggregateBuckets(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	from := leaderboardTodayStart.AddDate(0, 0, -29)
	to := leaderboardTodayStart.AddDate(0, 0, 1)

	// 预聚合作业未启用（表为空）：空切片，不是错误，也不是 0 填充的 30 天。
	daily, err := repo.LeaderboardDailyBuckets(ctx, from, to)
	require.NoError(t, err)
	require.Empty(t, daily)

	hourly, err := repo.LeaderboardHourlyBuckets(ctx, leaderboardTodayStart, leaderboardTodayStart.Add(24*time.Hour))
	require.NoError(t, err)
	require.Empty(t, hourly)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO usage_dashboard_daily (bucket_date, total_requests, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens)
		VALUES ($1::date, 40, 1, 2, 3, 4), ($2::date, 10, 10, 20, 30, 40)
	`, leaderboardTodayStart, leaderboardTodayStart.AddDate(0, 0, -1))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO usage_dashboard_hourly (bucket_start, total_requests, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens)
		VALUES ($1, 7, 100, 0, 0, 300), ($2, 3, 50, 0, 0, 50)
	`, leaderboardTodayStart.Add(9*time.Hour), leaderboardTodayStart.Add(14*time.Hour))
	require.NoError(t, err)

	daily, err = repo.LeaderboardDailyBuckets(ctx, from, to)
	require.NoError(t, err)
	require.Len(t, daily, 2)
	require.Equal(t, leaderboardTodayStart.AddDate(0, 0, -1).Format("2006-01-02"), daily[0].Date.Format("2006-01-02"), "按日期升序")
	require.Equal(t, int64(10), daily[0].Requests, "requests 是预聚合表的裸计数，含失败占位记录")
	require.Equal(t, int64(100), daily[0].TotalTokens)
	require.Equal(t, int64(40), daily[1].Requests)
	require.Equal(t, int64(10), daily[1].TotalTokens)

	// 上界是开区间：今日那一天不在 [from, todayStart) 里。
	daily, err = repo.LeaderboardDailyBuckets(ctx, from, leaderboardTodayStart)
	require.NoError(t, err)
	require.Len(t, daily, 1)

	hourly, err = repo.LeaderboardHourlyBuckets(ctx, leaderboardTodayStart, leaderboardTodayStart.Add(24*time.Hour))
	require.NoError(t, err)
	require.Len(t, hourly, 2)
	require.True(t, hourly[0].BucketStart.Equal(leaderboardTodayStart.Add(9*time.Hour)))
	require.Equal(t, int64(7), hourly[0].Requests)
	require.Equal(t, int64(100), hourly[0].InputTokens)
	require.Equal(t, int64(300), hourly[0].CacheReadTokens)
	require.Equal(t, int64(50), hourly[1].InputTokens)
}

func mustCreateLeaderboardModelLog(
	t *testing.T,
	repo *usageLogRepository,
	userID, apiKeyID, accountID int64,
	model string,
	createdAt time.Time,
	actualCost float64,
) {
	t.Helper()
	_, err := repo.Create(context.Background(), &service.UsageLog{
		UserID:      userID,
		APIKeyID:    apiKeyID,
		AccountID:   accountID,
		Model:       model,
		InputTokens: 1,
		TotalCost:   actualCost,
		ActualCost:  actualCost,
		CreatedAt:   createdAt,
	})
	require.NoError(t, err)
}

// ---- v2：十四个新增聚合列、之最的数据源与名次历史 ----

// mustCreateLeaderboardRichLog 造一条能同时喂 token 口径与「按请求取值」口径的日志。
func mustCreateLeaderboardRichLog(
	t *testing.T,
	repo *usageLogRepository,
	userID, apiKeyID, accountID int64,
	model string,
	createdAt time.Time,
	input, output, cacheCreation, cacheRead int,
	actualCost float64,
	videoCount int,
) {
	t.Helper()
	_, err := repo.Create(context.Background(), &service.UsageLog{
		UserID:              userID,
		APIKeyID:            apiKeyID,
		AccountID:           accountID,
		Model:               model,
		InputTokens:         input,
		OutputTokens:        output,
		CacheCreationTokens: cacheCreation,
		CacheReadTokens:     cacheRead,
		TotalCost:           actualCost,
		ActualCost:          actualCost,
		VideoCount:          videoCount,
		CreatedAt:           createdAt,
	})
	require.NoError(t, err)
}

// output / cache_creation 两列与管理端 User Breakdown 的同名口径一致：
// 两个页面对账时不该出现差额。
func TestUsageLog_AggregateLeaderboardWindows_NewTokenColumnsMatchUserBreakdown(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	user := mustCreateUser(t, client, &service.User{Email: "leaderboard-v2-parity@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-leaderboard-v2-parity", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-leaderboard-v2-parity"})

	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardMonthStart.Add(9*time.Hour), 11, 13, 17, 19, 0.3)
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardTodayStart.Add(9*time.Hour), 23, 29, 31, 37, 0.7)

	rows, err := repo.AggregateLeaderboardWindows(ctx, leaderboardTodayStart, leaderboardWeekStart, leaderboardMonthStart)
	require.NoError(t, err)
	got, ok := leaderboardRowsByUser(t, rows)[user.ID]
	require.True(t, ok)

	require.Equal(t, int64(29), got.TodayOutputTokens)
	require.Equal(t, int64(31), got.TodayCacheCreationTokens)
	require.Equal(t, int64(13+29), got.MonthOutputTokens)
	require.Equal(t, int64(17+31), got.MonthCacheCreationTokens)

	breakdown, err := repo.GetUserBreakdownStats(ctx,
		leaderboardMonthStart,
		leaderboardTodayStart.Add(24*time.Hour),
		usagestats.UserBreakdownDimension{UserID: user.ID},
		0,
	)
	require.NoError(t, err)
	require.Len(t, breakdown, 1)
	require.Equal(t, breakdown[0].OutputTokens, got.MonthOutputTokens, "输出 tokens 必须与 User Breakdown 对得上")
	// User Breakdown 的 CacheTokens 是「创建 + 读取」，榜单把两半分开存。
	require.Equal(t, breakdown[0].CacheTokens, got.MonthCacheCreationTokens+got.MonthCacheReadTokens)
	// 四段之和必须等于 Total Tokens：Token 构成靠这条恒等式才不会出现「四段加起来不是 100%」。
	require.Equal(t, got.MonthTokens, got.MonthInputTokens+got.MonthOutputTokens+got.MonthCacheCreationTokens+got.MonthCacheReadTokens)
}

// 夜间列按站点时区 0–6 点划分；distinct_models / max_single / media_requests
// 沿用成功落账口径，失败占位行既不进计数也不进单次最大。
func TestUsageLog_AggregateLeaderboardWindows_NightDistinctMaxAndMedia(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	user := mustCreateUser(t, client, &service.User{Email: "leaderboard-v2-extremes@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-leaderboard-v2-extremes", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-leaderboard-v2-extremes"})

	// 集成测试的站点时区是 UTC（见 integration_harness_test.go 的 TestMain），
	// 因此「0–6 点」就是窗口起点后的前 6 小时。
	mustCreateLeaderboardRichLog(t, repo, user.ID, apiKey.ID, account.ID, "claude-opus-5", leaderboardTodayStart.Add(1*time.Hour), 10, 10, 0, 0, 0.5, 0)
	mustCreateLeaderboardRichLog(t, repo, user.ID, apiKey.ID, account.ID, "claude-opus-5", leaderboardTodayStart.Add(5*time.Hour+59*time.Minute), 5, 5, 0, 0, 0.5, 0)
	// 正好 6 点：不算夜间（EXTRACT(HOUR) < 6 是开区间上界）。
	mustCreateLeaderboardRichLog(t, repo, user.ID, apiKey.ID, account.ID, "claude-sonnet-5", leaderboardTodayStart.Add(6*time.Hour), 100, 100, 0, 0, 0.5, 0)
	// 视频行：进 media_requests。
	mustCreateLeaderboardRichLog(t, repo, user.ID, apiKey.ID, account.ID, "grok-video", leaderboardTodayStart.Add(8*time.Hour), 1, 1, 0, 0, 0.5, 1)
	// 失败占位行：模型是第四个，但既不进 distinct_models，也不进 max_single / media_requests。
	mustCreateLeaderboardRichLog(t, repo, user.ID, apiKey.ID, account.ID, "gpt-5.1", leaderboardTodayStart.Add(9*time.Hour), 9000, 0, 0, 0, 0, 1)

	rows, err := repo.AggregateLeaderboardWindows(ctx, leaderboardTodayStart, leaderboardWeekStart, leaderboardMonthStart)
	require.NoError(t, err)
	got, ok := leaderboardRowsByUser(t, rows)[user.ID]
	require.True(t, ok)

	require.Equal(t, int64(30), got.TodayNightTokens, "只有 0–6 点的两条进夜间列")
	require.Equal(t, 3, got.TodayDistinctModels, "失败占位行的模型不计入")
	require.Equal(t, int64(200), got.TodayMaxSingleTokens, "9000 那条是失败占位行，不参与单次最大")
	require.Equal(t, int64(1), got.TodayMediaRequests, "只有成功的视频行计入")
	// token 求和仍是「窗口内所有行」，失败占位行的 9000 照常进 Total Tokens。
	require.Equal(t, int64(9232), got.TodayTokens)
}

// 昨日两列的区间是 [昨日起点, 今日起点)：前一天之前与今天都不算。
func TestUsageLog_AggregateLeaderboardWindows_YesterdayColumns(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	user := mustCreateUser(t, client, &service.User{Email: "leaderboard-v2-yesterday@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-leaderboard-v2-yesterday", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-leaderboard-v2-yesterday"})
	yesterdayStart := leaderboardTodayStart.AddDate(0, 0, -1)

	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, yesterdayStart, 1, 1, 1, 1, 0.5)                 // 昨日起点这一刻算昨天
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, yesterdayStart.Add(23*time.Hour), 2, 2, 2, 2, 0) // 昨天的失败占位行
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, yesterdayStart.Add(-time.Second), 50, 50, 50, 50, 0.5)
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, leaderboardTodayStart, 3, 3, 3, 3, 0.5)

	rows, err := repo.AggregateLeaderboardWindows(ctx, leaderboardTodayStart, leaderboardWeekStart, leaderboardMonthStart)
	require.NoError(t, err)
	got, ok := leaderboardRowsByUser(t, rows)[user.ID]
	require.True(t, ok)

	require.Equal(t, int64(12), got.YesterdayTokens, "只有昨天那两条：4 + 8")
	require.Equal(t, int64(1), got.YesterdayRequests, "失败占位行不计入昨日成功请求")
	require.Equal(t, int64(12), got.TodayTokens)
}

// 扫描下界必须放宽到昨日起点：否则「今天是月初的第一天」时昨日两列永远是 0。
func TestUsageLog_AggregateLeaderboardWindows_ScanLowerBoundCoversYesterday(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	user := mustCreateUser(t, client, &service.User{Email: "leaderboard-v2-lowerbound@test.com"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-leaderboard-v2-lowerbound", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-leaderboard-v2-lowerbound"})

	// 今天就是月初与周一：三个窗口起点重合，昨天严格早于所有窗口。
	monthStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) // 周一
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, monthStart.AddDate(0, 0, -1).Add(time.Hour), 5, 5, 5, 5, 0.5)
	mustCreateLeaderboardLog(t, repo, user.ID, apiKey.ID, account.ID, monthStart.Add(time.Hour), 1, 1, 1, 1, 0.5)

	rows, err := repo.AggregateLeaderboardWindows(ctx, monthStart, monthStart, monthStart)
	require.NoError(t, err)
	got, ok := leaderboardRowsByUser(t, rows)[user.ID]
	require.True(t, ok)
	require.Equal(t, int64(4), got.TodayTokens)
	require.Equal(t, int64(20), got.YesterdayTokens, "昨日的行必须落在扫描区间内")
	require.Equal(t, int64(1), got.YesterdayRequests)
}

// 连续活跃：gaps-and-islands 取「截至今日或昨日」的那一段，断档的旧记录不参与。
func TestUsageLog_LeaderboardTopStreak(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	from := leaderboardTodayStart.AddDate(0, 0, -89)
	to := leaderboardTodayStart.AddDate(0, 0, 1)

	// 表为空：零值而不是报错。
	row, err := repo.LeaderboardTopStreak(ctx, from, to)
	require.NoError(t, err)
	require.Zero(t, row.Days)
	require.Zero(t, row.UserID)

	current := mustCreateUser(t, client, &service.User{Email: "leaderboard-streak-current@test.com"})
	yesterday := mustCreateUser(t, client, &service.User{Email: "leaderboard-streak-yesterday@test.com"})
	stale := mustCreateUser(t, client, &service.User{Email: "leaderboard-streak-stale@test.com"})
	disabled := mustCreateUser(t, client, &service.User{Email: "leaderboard-streak-disabled@test.com", Status: service.StatusDisabled})

	insertDay := func(userID int64, offset int) {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO usage_dashboard_daily_users (bucket_date, user_id) VALUES ($1::date, $2)`,
			leaderboardTodayStart.AddDate(0, 0, offset), userID)
		require.NoError(t, err)
	}

	// 截至今日连续 3 天。
	for _, offset := range []int{0, -1, -2} {
		insertDay(current.ID, offset)
	}
	// 截至昨日连续 4 天：仍然算「当前连续」。
	for _, offset := range []int{-1, -2, -3, -4} {
		insertDay(yesterday.ID, offset)
	}
	// 一段很长但早已断档的历史：MUST NOT 被选中。
	for offset := -20; offset >= -30; offset-- {
		insertDay(stale.ID, offset)
	}
	// 被禁用的用户即使连续 10 天也不参与。
	for offset := 0; offset >= -9; offset-- {
		insertDay(disabled.ID, offset)
	}

	row, err = repo.LeaderboardTopStreak(ctx, from, to)
	require.NoError(t, err)
	require.Equal(t, yesterday.ID, row.UserID, "截至昨日的 4 天长于截至今日的 3 天")
	require.Equal(t, 4, row.Days)

	// 断档：把「截至昨日」那位的中间一天删掉，他的当前连续段缩到 1 天。
	_, err = tx.ExecContext(ctx,
		`DELETE FROM usage_dashboard_daily_users WHERE user_id = $1 AND bucket_date = $2::date`,
		yesterday.ID, leaderboardTodayStart.AddDate(0, 0, -2))
	require.NoError(t, err)

	row, err = repo.LeaderboardTopStreak(ctx, from, to)
	require.NoError(t, err)
	require.Equal(t, current.ID, row.UserID, "断档之后换成截至今日的 3 天")
	require.Equal(t, 3, row.Days)
}

// 模型偏好画像的聚合：限定在给定的 user_id 集合内，按 (user_id, 成功请求数倒序) 排好。
func TestUsageLog_LeaderboardUserModelBreakdown(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	a := mustCreateUser(t, client, &service.User{Email: "leaderboard-profile-a@test.com"})
	b := mustCreateUser(t, client, &service.User{Email: "leaderboard-profile-b@test.com"})
	outsider := mustCreateUser(t, client, &service.User{Email: "leaderboard-profile-out@test.com"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: a.ID, Key: "sk-leaderboard-profile", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-leaderboard-profile"})
	todayEnd := leaderboardTodayStart.Add(24 * time.Hour)

	// 空集合不查库。
	rows, err := repo.LeaderboardUserModelBreakdown(ctx, nil, leaderboardTodayStart, todayEnd)
	require.NoError(t, err)
	require.Empty(t, rows)

	mustCreateLeaderboardModelLog(t, repo, a.ID, key.ID, account.ID, "claude-sonnet-5", leaderboardTodayStart.Add(time.Hour), 0.5)
	mustCreateLeaderboardModelLog(t, repo, a.ID, key.ID, account.ID, "claude-sonnet-5", leaderboardTodayStart.Add(2*time.Hour), 0.5)
	mustCreateLeaderboardModelLog(t, repo, a.ID, key.ID, account.ID, "claude-opus-5", leaderboardTodayStart.Add(3*time.Hour), 0.5)
	// 失败占位行不计入。
	mustCreateLeaderboardModelLog(t, repo, a.ID, key.ID, account.ID, "gpt-5.1", leaderboardTodayStart.Add(4*time.Hour), 0)
	mustCreateLeaderboardModelLog(t, repo, b.ID, key.ID, account.ID, "gemini-3", leaderboardTodayStart.Add(5*time.Hour), 0.5)
	mustCreateLeaderboardModelLog(t, repo, outsider.ID, key.ID, account.ID, "claude-opus-5", leaderboardTodayStart.Add(6*time.Hour), 0.5)

	rows, err = repo.LeaderboardUserModelBreakdown(ctx, []int64{a.ID, b.ID}, leaderboardTodayStart, todayEnd)
	require.NoError(t, err)
	require.Equal(t, []usagestats.LeaderboardUserModelUsageRow{
		{UserID: a.ID, Model: "claude-sonnet-5", SuccessfulRequests: 2},
		{UserID: a.ID, Model: "claude-opus-5", SuccessfulRequests: 1},
		{UserID: b.ID, Model: "gemini-3", SuccessfulRequests: 1},
	}, rows, "集合之外的用户 MUST NOT 出现")
}

// 平台分布用 INNER JOIN accounts：账号已被删掉的日志没有平台可归，不凑空分类。
func TestUsageLog_LeaderboardPlatformsToday(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	user := mustCreateUser(t, client, &service.User{Email: "leaderboard-platform@test.com"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-leaderboard-platform", Name: "k"})
	anthropic := mustCreateAccount(t, client, &service.Account{Name: "acc-lb-anthropic", Platform: service.PlatformAnthropic})
	openai := mustCreateAccount(t, client, &service.Account{Name: "acc-lb-openai", Platform: service.PlatformOpenAI})
	todayEnd := leaderboardTodayStart.Add(24 * time.Hour)

	mustCreateLeaderboardModelLog(t, repo, user.ID, key.ID, anthropic.ID, "claude-opus-5", leaderboardTodayStart.Add(time.Hour), 0.5)
	mustCreateLeaderboardModelLog(t, repo, user.ID, key.ID, anthropic.ID, "claude-opus-5", leaderboardTodayStart.Add(2*time.Hour), 0.5)
	mustCreateLeaderboardModelLog(t, repo, user.ID, key.ID, anthropic.ID, "claude-opus-5", leaderboardTodayStart.Add(3*time.Hour), 0.5)
	mustCreateLeaderboardModelLog(t, repo, user.ID, key.ID, openai.ID, "gpt-5.1", leaderboardTodayStart.Add(4*time.Hour), 0.5)
	// 失败占位行：既不进某个平台，也不进分母。
	mustCreateLeaderboardModelLog(t, repo, user.ID, key.ID, openai.ID, "gpt-5.1", leaderboardTodayStart.Add(5*time.Hour), 0)
	// 昨天的成功请求不属于今日。
	mustCreateLeaderboardModelLog(t, repo, user.ID, key.ID, openai.ID, "gpt-5.1", leaderboardTodayStart.Add(-time.Hour), 0.5)

	// 孤儿日志：account_id 在 accounts 里已无对应行，INNER JOIN 必须排除。
	_, err := tx.ExecContext(ctx, "ALTER TABLE usage_logs DROP CONSTRAINT IF EXISTS usage_logs_account_id_fkey")
	require.NoError(t, err)
	mustCreateLeaderboardModelLog(t, repo, user.ID, key.ID, 9_000_000_002, "ghost", leaderboardTodayStart.Add(6*time.Hour), 0.5)

	rows, total, err := repo.LeaderboardPlatformsToday(ctx, leaderboardTodayStart, todayEnd)
	require.NoError(t, err)
	require.Equal(t, []usagestats.LeaderboardPlatformUsageRow{
		{Platform: service.PlatformAnthropic, SuccessfulRequests: 3},
		{Platform: service.PlatformOpenAI, SuccessfulRequests: 1},
	}, rows)
	require.Equal(t, int64(4), total, "孤儿日志既不成一行，也不进分母")

	// 今日一条成功请求都没有：空切片 + 0，而不是报错。
	rows, total, err = repo.LeaderboardPlatformsToday(ctx, todayEnd, todayEnd.Add(24*time.Hour))
	require.NoError(t, err)
	require.Empty(t, rows)
	require.Zero(t, total)
}

// 周内节奏：按 (ISO 周几, 小时) 求平均；表为空时空切片而不是报错。
func TestUsageLog_LeaderboardWeekdayHourBuckets(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	from := leaderboardTodayStart.AddDate(0, 0, -28)
	to := leaderboardTodayStart

	rows, err := repo.LeaderboardWeekdayHourBuckets(ctx, from, to)
	require.NoError(t, err)
	require.Empty(t, rows, "预聚合未启用时返回空切片，由上层整块置 null")

	// 2026-03-16 与 2026-03-09 都是周一（ISODOW = 1），同一个 9 点桶求平均。
	monday := time.Date(2026, 3, 16, 9, 0, 0, 0, time.UTC)
	previousMonday := time.Date(2026, 3, 9, 9, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 3, 15, 23, 0, 0, 0, time.UTC)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO usage_dashboard_hourly (bucket_start, total_requests)
		VALUES ($1, 10), ($2, 30), ($3, 7)
	`, monday, previousMonday, sunday)
	require.NoError(t, err)

	rows, err = repo.LeaderboardWeekdayHourBuckets(ctx, from, to)
	require.NoError(t, err)
	require.Equal(t, []usagestats.LeaderboardWeekdayHourRow{
		{Weekday: 1, Hour: 9, Requests: 20},
		{Weekday: 7, Hour: 23, Requests: 7},
	}, rows, "周一 9 点是两周的平均，周日单独一格")
}

// viewer.models：请求路径唯一的回源例外，SQL 必须带 user_id 过滤，绝不返回他人的行。
func TestUsageLog_LeaderboardViewerModels(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	viewer := mustCreateUser(t, client, &service.User{Email: "leaderboard-viewer@test.com"})
	other := mustCreateUser(t, client, &service.User{Email: "leaderboard-viewer-other@test.com"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: viewer.ID, Key: "sk-leaderboard-viewer", Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-leaderboard-viewer"})
	todayEnd := leaderboardTodayStart.Add(24 * time.Hour)

	// 该窗口零用量：空切片 + 0，而不是报错。
	rows, total, err := repo.LeaderboardViewerModels(ctx, viewer.ID, leaderboardTodayStart, todayEnd, 5)
	require.NoError(t, err)
	require.Empty(t, rows)
	require.Zero(t, total)

	for i := 0; i < 3; i++ {
		mustCreateLeaderboardModelLog(t, repo, viewer.ID, key.ID, account.ID, "claude-sonnet-5", leaderboardTodayStart.Add(time.Duration(i+1)*time.Hour), 0.5)
	}
	mustCreateLeaderboardModelLog(t, repo, viewer.ID, key.ID, account.ID, "claude-opus-5", leaderboardTodayStart.Add(5*time.Hour), 0.5)
	// 失败占位行既不进某个模型，也不进分母。
	mustCreateLeaderboardModelLog(t, repo, viewer.ID, key.ID, account.ID, "gpt-5.1", leaderboardTodayStart.Add(6*time.Hour), 0)
	// 别人的用量：MUST NOT 出现在任何一行里，也不进分母。
	for i := 0; i < 10; i++ {
		mustCreateLeaderboardModelLog(t, repo, other.ID, key.ID, account.ID, "gemini-3", leaderboardTodayStart.Add(time.Duration(i+7)*time.Hour), 0.5)
	}

	rows, total, err = repo.LeaderboardViewerModels(ctx, viewer.ID, leaderboardTodayStart, todayEnd, 5)
	require.NoError(t, err)
	require.Equal(t, []usagestats.LeaderboardModelUsageRow{
		{Model: "claude-sonnet-5", SuccessfulRequests: 3},
		{Model: "claude-opus-5", SuccessfulRequests: 1},
	}, rows)
	require.Equal(t, int64(4), total, "分母只算本人的成功请求")

	// limit 生效，分母仍是本人的总数。
	rows, total, err = repo.LeaderboardViewerModels(ctx, viewer.ID, leaderboardTodayStart, todayEnd, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "claude-sonnet-5", rows[0].Model)
	require.Equal(t, int64(4), total)
}
