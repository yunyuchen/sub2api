package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Leaderboard（排行榜）Snapshot（榜单快照）在 Redis 上的 key 布局（design D7）：
//
//	<cfg.Dashboard.KeyPrefix>leaderboard:v1:<window>:<窗口起点>:z:total_tokens         ZSET  member=user_id score=Total Tokens
//	<cfg.Dashboard.KeyPrefix>leaderboard:v1:<window>:<窗口起点>:z:successful_requests  ZSET  member=user_id score=Successful Requests
//	<cfg.Dashboard.KeyPrefix>leaderboard:v1:<window>:<窗口起点>:z:cost                 ZSET  member=user_id score=Cost（micros，1 USD = 1e6）
//	<cfg.Dashboard.KeyPrefix>leaderboard:v1:<window>:<窗口起点>:h                      HASH  user_id -> "tokens,requests,input,cache_read,output,cache_creation,night_tokens,distinct_models,max_single,media_requests,yesterday_tokens,yesterday_requests,cost_micros"
//	<cfg.Dashboard.KeyPrefix>leaderboard:v1:<window>:<窗口起点>:updated_at             STR   快照更新时间（毫秒时间戳）
//	<cfg.Dashboard.KeyPrefix>leaderboard:v1:<window>:<窗口起点>:highlights             STR   该窗口的 Highlights（趣味卡）JSON
//	<cfg.Dashboard.KeyPrefix>leaderboard:v1:insights:<今日窗口起点>                     STR   站点级 Insights（洞察）JSON
//	<cfg.Dashboard.KeyPrefix>leaderboard:v1:viewer:models:<window>:<窗口起点>:<user_id> STR   本人模型偏好 JSON（60 秒）
//
// 三个 Metric（排名指标）各一个 ZSET，key 后缀就是 Metric 字符串本身（见 metricKey）。
// Cost 的分数与 Hash 第 13 段一律是 micros（1 USD = 1e6 的定点整数），只在响应组装时
// 换回 USD：ZSET 分数是 float64，直接放美元小数会让并列判定与「还差多少」出现尾差。
//
// key 必须带窗口起点：只按窗口名命名会在跨零点 / 周一 / 月初时读到上一窗口的残留。
// Insights 与 Window 无关，只存一份，key 带今日起点同样是为了跨零点自然作废。
// 整份榜单刻意不序列化成单个 JSON key——那样每个请求都要反序列化全量数据；
// Highlights 与 Insights 则本来就是「一小块算好的结论」，各自一个 JSON 串最省事。
// viewer:models 是唯一一类请求路径写入的 key：它缓存 design D21 那条「只查本人」的例外聚合，
// TTL 只有 60 秒，与快照的重建周期无关。
const (
	leaderboardKeyNamespace = "leaderboard:v1:"

	leaderboardZSetKeySuffix       = ":z:"
	leaderboardHashKeySuffix       = ":h"
	leaderboardUpdatedAtKeySuffix  = ":updated_at"
	leaderboardHighlightsKeySuffix = ":highlights"
	leaderboardInsightsKeyPrefix   = "insights:"
	leaderboardTempKeyInfix        = ":tmp:"

	// leaderboardViewerModelsKeyPrefix 是本人模型偏好缓存的 key 段，
	// 完整形状是 <prefix>leaderboard:v1:viewer:models:<window>:<窗口起点>:<user_id>。
	leaderboardViewerModelsKeyPrefix = "viewer:models:"
	// leaderboardViewerModelsTTL 是那条例外查询的缓存时长上限（design D21）。
	// 实际 TTL 取 min(它, 距窗口结束)，因此跨零点时今日窗口的缓存不会活过零点。
	leaderboardViewerModelsTTL = 60 * time.Second

	// leaderboardWriteChunkSize 限制单条 ZADD / HSET 的成员数，避免十万级用户下的巨包。
	leaderboardWriteChunkSize = 500
)

// leaderboardViewerModelsPayload 是 viewer:models 缓存的 JSON 形状。
// Total 是本人该窗口的成功请求总数（不止 Top N），即 share_percent 的分母，
// 必须与 Models 一起缓存——只缓存前几行会让占比在缓存命中时失去分母。
type leaderboardViewerModelsPayload struct {
	Models []usagestats.LeaderboardModelUsageRow `json:"models"`
	Total  int64                                 `json:"total"`
}

var errLeaderboardCacheUnavailable = errors.New("排行榜缓存不可用")

type leaderboardCache struct {
	rdb       *redis.Client
	keyPrefix string
}

// NewLeaderboardCache 创建 Leaderboard 的 Redis 派生结构读写口。
// keyPrefix 沿用 cfg.Dashboard.KeyPrefix 做环境隔离，写法与 NewDashboardCache 一致。
func NewLeaderboardCache(rdb *redis.Client, cfg *config.Config) service.LeaderboardCache {
	prefix := "sub2api:"
	if cfg != nil {
		prefix = strings.TrimSpace(cfg.Dashboard.KeyPrefix)
	}
	if prefix != "" && !strings.HasSuffix(prefix, ":") {
		prefix += ":"
	}
	return &leaderboardCache{
		rdb:       rdb,
		keyPrefix: prefix,
	}
}

// ReplaceSnapshot 用 pipeline 先把新数据写进临时 key，再在一个事务里 RENAME 到正式 key。
// 读者因此只会读到上一轮完整的 Snapshot 或本轮完整的 Snapshot，不会读到半份榜；
// 中途失败时正式 key 保持上一轮内容，更新时间也不会被推进。
func (c *leaderboardCache) ReplaceSnapshot(ctx context.Context, snapshot service.LeaderboardSnapshotWindow) error {
	if c == nil || c.rdb == nil {
		return errLeaderboardCacheUnavailable
	}
	base, err := c.windowBaseKey(snapshot.Window, snapshot.WindowStart)
	if err != nil {
		return err
	}

	tokensKey := base + leaderboardZSetKeySuffix + string(service.LeaderboardMetricTotalTokens)
	requestsKey := base + leaderboardZSetKeySuffix + string(service.LeaderboardMetricSuccessfulRequests)
	costKey := base + leaderboardZSetKeySuffix + string(service.LeaderboardMetricCost)
	hashKey := base + leaderboardHashKeySuffix
	updatedAtKey := base + leaderboardUpdatedAtKeySuffix
	highlightsKey := base + leaderboardHighlightsKeySuffix

	// Highlights 与榜单同生共死：序列化失败就整轮不切换，绝不写一半。
	var highlightsPayload []byte
	if snapshot.Highlights != nil {
		payload, err := json.Marshal(snapshot.Highlights)
		if err != nil {
			return err
		}
		highlightsPayload = payload
	}

	updatedAt := snapshot.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	ttl := leaderboardSnapshotTTL(snapshot.WindowEnd, time.Now())
	updatedAtValue := strconv.FormatInt(updatedAt.UnixMilli(), 10)

	// 该窗口没有任何合格用量：清掉上一轮的正式 key，但仍推进更新时间——
	// 空榜是一份就绪的快照，与「正在计算」是两回事。
	if len(snapshot.Entries) == 0 {
		tx := c.rdb.TxPipeline()
		tx.Del(ctx, tokensKey, requestsKey, costKey, hashKey, highlightsKey)
		if highlightsPayload != nil {
			tx.Set(ctx, highlightsKey, highlightsPayload, ttl)
		}
		tx.Set(ctx, updatedAtKey, updatedAtValue, ttl)
		_, err := tx.Exec(ctx)
		return err
	}

	nonce := uuid.NewString()
	tmpTokensKey := tokensKey + leaderboardTempKeyInfix + nonce
	tmpRequestsKey := requestsKey + leaderboardTempKeyInfix + nonce
	tmpCostKey := costKey + leaderboardTempKeyInfix + nonce
	tmpHashKey := hashKey + leaderboardTempKeyInfix + nonce
	tmpHighlightsKey := highlightsKey + leaderboardTempKeyInfix + nonce
	discard := func() {
		ctx2, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = c.rdb.Del(ctx2, tmpTokensKey, tmpRequestsKey, tmpCostKey, tmpHashKey, tmpHighlightsKey).Err()
	}

	pipe := c.rdb.Pipeline()
	for start := 0; start < len(snapshot.Entries); start += leaderboardWriteChunkSize {
		end := start + leaderboardWriteChunkSize
		if end > len(snapshot.Entries) {
			end = len(snapshot.Entries)
		}
		chunk := snapshot.Entries[start:end]

		tokenMembers := make([]redis.Z, 0, len(chunk))
		requestMembers := make([]redis.Z, 0, len(chunk))
		costMembers := make([]redis.Z, 0, len(chunk))
		hashFields := make([]any, 0, len(chunk)*2)
		for _, entry := range chunk {
			member := strconv.FormatInt(entry.UserID, 10)
			tokenMembers = append(tokenMembers, redis.Z{Score: float64(entry.TotalTokens), Member: member})
			requestMembers = append(requestMembers, redis.Z{Score: float64(entry.SuccessfulRequests), Member: member})
			// 分数是 micros 而不是 USD：整数分数下并列判定与 ZCOUNT 的边界才是精确的。
			costMembers = append(costMembers, redis.Z{Score: float64(entry.CostMicros), Member: member})
			hashFields = append(hashFields, member, encodeLeaderboardMetrics(entry))
		}
		pipe.ZAdd(ctx, tmpTokensKey, tokenMembers...)
		pipe.ZAdd(ctx, tmpRequestsKey, requestMembers...)
		pipe.ZAdd(ctx, tmpCostKey, costMembers...)
		pipe.HSet(ctx, tmpHashKey, hashFields...)
	}
	// RENAME 会把源 key 的 TTL 一并带过去，所以在切换前就把 TTL 设好。
	pipe.Expire(ctx, tmpTokensKey, ttl)
	pipe.Expire(ctx, tmpRequestsKey, ttl)
	pipe.Expire(ctx, tmpCostKey, ttl)
	pipe.Expire(ctx, tmpHashKey, ttl)
	if highlightsPayload != nil {
		pipe.Set(ctx, tmpHighlightsKey, highlightsPayload, ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		discard()
		return err
	}

	tx := c.rdb.TxPipeline()
	tx.Rename(ctx, tmpTokensKey, tokensKey)
	tx.Rename(ctx, tmpRequestsKey, requestsKey)
	tx.Rename(ctx, tmpCostKey, costKey)
	tx.Rename(ctx, tmpHashKey, hashKey)
	if highlightsPayload != nil {
		tx.Rename(ctx, tmpHighlightsKey, highlightsKey)
	} else {
		// 这一轮算不出 Highlights：连同榜单一起切换成「没有 Highlights」，
		// 而不是让上一轮的趣味卡配这一轮的榜。
		tx.Del(ctx, highlightsKey)
	}
	tx.Set(ctx, updatedAtKey, updatedAtValue, ttl)
	if _, err := tx.Exec(ctx); err != nil {
		discard()
		return err
	}
	return nil
}

// Highlights 读该 Window 的趣味卡。key 不存在（旧快照 / 本轮没算出来）或内容损坏时
// 返回 nil：上层按「没有 Highlights」渲染，页面隐藏四张卡，而不是报错。
func (c *leaderboardCache) Highlights(ctx context.Context, window service.LeaderboardWindow, windowStart time.Time) (*service.LeaderboardHighlights, error) {
	if c == nil || c.rdb == nil {
		return nil, errLeaderboardCacheUnavailable
	}
	base, err := c.windowBaseKey(window, windowStart)
	if err != nil {
		return nil, err
	}
	raw, err := c.rdb.Get(ctx, base+leaderboardHighlightsKeySuffix).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	var highlights service.LeaderboardHighlights
	if unmarshalErr := json.Unmarshal(raw, &highlights); unmarshalErr != nil {
		return nil, nil
	}
	return &highlights, nil
}

// ReplaceInsights 整体替换站点级洞察。一个 SET 就是原子的，不需要临时 key 与 RENAME；
// insights 为 nil 时清掉 key，避免昨天的今日热度配今天的榜。
func (c *leaderboardCache) ReplaceInsights(ctx context.Context, todayStart time.Time, insights *service.LeaderboardInsights, todayEnd time.Time) error {
	if c == nil || c.rdb == nil {
		return errLeaderboardCacheUnavailable
	}
	key, err := c.insightsKey(todayStart)
	if err != nil {
		return err
	}
	if insights == nil {
		return c.rdb.Del(ctx, key).Err()
	}
	payload, err := json.Marshal(insights)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, payload, leaderboardSnapshotTTL(todayEnd, time.Now())).Err()
}

// Insights 读站点级洞察；缺失或损坏时返回 nil，页面隐藏对应区块。
func (c *leaderboardCache) Insights(ctx context.Context, todayStart time.Time) (*service.LeaderboardInsights, error) {
	if c == nil || c.rdb == nil {
		return nil, errLeaderboardCacheUnavailable
	}
	key, err := c.insightsKey(todayStart)
	if err != nil {
		return nil, err
	}
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	var insights service.LeaderboardInsights
	if unmarshalErr := json.Unmarshal(raw, &insights); unmarshalErr != nil {
		return nil, nil
	}
	return &insights, nil
}

// TopEntries 按分数倒序取前 limit 个成员，不读取也不反序列化全体用户的数据。
func (c *leaderboardCache) TopEntries(ctx context.Context, window service.LeaderboardWindow, windowStart time.Time, metric service.LeaderboardMetric, limit int) ([]service.LeaderboardScoreEntry, error) {
	if c == nil || c.rdb == nil {
		return nil, errLeaderboardCacheUnavailable
	}
	if limit <= 0 {
		limit = service.LeaderboardTopEntryLimit
	}
	key, err := c.metricKey(window, windowStart, metric)
	if err != nil {
		return nil, err
	}
	items, err := c.rdb.ZRevRangeWithScores(ctx, key, 0, int64(limit-1)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return []service.LeaderboardScoreEntry{}, nil
		}
		return nil, err
	}
	entries := make([]service.LeaderboardScoreEntry, 0, len(items))
	for _, item := range items {
		member, ok := item.Member.(string)
		if !ok {
			continue
		}
		userID, parseErr := strconv.ParseInt(member, 10, 64)
		if parseErr != nil {
			continue
		}
		entries = append(entries, service.LeaderboardScoreEntry{UserID: userID, Score: int64(item.Score)})
	}
	return entries, nil
}

// ParticipantCount 取 ZCARD，即该 Window 内有过用量的合格用户总数。
func (c *leaderboardCache) ParticipantCount(ctx context.Context, window service.LeaderboardWindow, windowStart time.Time, metric service.LeaderboardMetric) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, errLeaderboardCacheUnavailable
	}
	key, err := c.metricKey(window, windowStart, metric)
	if err != nil {
		return 0, err
	}
	count, err := c.rdb.ZCard(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}

// RankOf 用 ZCOUNT 统计分数严格高于该用户的成员数再加一：
// 与查看者同分的用户不计入，因此并列同名次、其后跳号（1、1、3）。
func (c *leaderboardCache) RankOf(ctx context.Context, window service.LeaderboardWindow, windowStart time.Time, metric service.LeaderboardMetric, userID int64) (int64, bool, error) {
	if c == nil || c.rdb == nil {
		return 0, false, errLeaderboardCacheUnavailable
	}
	key, err := c.metricKey(window, windowStart, metric)
	if err != nil {
		return 0, false, err
	}
	score, err := c.rdb.ZScore(ctx, key, strconv.FormatInt(userID, 10)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// 该窗口零用量：没有 My Rank（我的名次），只提示「暂无用量」。
			return 0, false, nil
		}
		return 0, false, err
	}
	higher, err := c.rdb.ZCount(ctx, key, "("+strconv.FormatFloat(score, 'f', -1, 64), "+inf").Result()
	if err != nil {
		return 0, false, err
	}
	return higher + 1, true, nil
}

// MetricsOf 从该 Window 的 Hash 批量读取这些 user_id 的数值，
// 供「榜单按一个 Metric 排序、每行仍同时显示三个 Metric」使用（Cost 读出来是 micros）。
func (c *leaderboardCache) MetricsOf(ctx context.Context, window service.LeaderboardWindow, windowStart time.Time, userIDs []int64) (map[int64]service.LeaderboardUserMetrics, error) {
	if c == nil || c.rdb == nil {
		return nil, errLeaderboardCacheUnavailable
	}
	result := make(map[int64]service.LeaderboardUserMetrics, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	base, err := c.windowBaseKey(window, windowStart)
	if err != nil {
		return nil, err
	}
	fields := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		fields = append(fields, strconv.FormatInt(id, 10))
	}
	values, err := c.rdb.HMGet(ctx, base+leaderboardHashKeySuffix, fields...).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return result, nil
		}
		return nil, err
	}
	for i, raw := range values {
		if i >= len(userIDs) || raw == nil {
			continue
		}
		text, ok := raw.(string)
		if !ok {
			continue
		}
		metrics, ok := decodeLeaderboardMetrics(userIDs[i], text)
		if !ok {
			continue
		}
		result[userIDs[i]] = metrics
	}
	return result, nil
}

// UpdatedAt 返回该 Window 的 Snapshot 更新时间；key 不存在时 exists=false，
// 由上层渲染成「正在计算」而不是空榜或 500。
func (c *leaderboardCache) UpdatedAt(ctx context.Context, window service.LeaderboardWindow, windowStart time.Time) (time.Time, bool, error) {
	if c == nil || c.rdb == nil {
		return time.Time{}, false, errLeaderboardCacheUnavailable
	}
	base, err := c.windowBaseKey(window, windowStart)
	if err != nil {
		return time.Time{}, false, err
	}
	raw, err := c.rdb.Get(ctx, base+leaderboardUpdatedAtKeySuffix).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	millis, parseErr := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if parseErr != nil {
		// 值损坏时按缺失处理，避免把一个乱码时间当成新鲜快照。
		return time.Time{}, false, nil
	}
	return time.UnixMilli(millis), true, nil
}

func (c *leaderboardCache) metricKey(window service.LeaderboardWindow, windowStart time.Time, metric service.LeaderboardMetric) (string, error) {
	switch metric {
	case service.LeaderboardMetricTotalTokens, service.LeaderboardMetricSuccessfulRequests, service.LeaderboardMetricCost:
	default:
		return "", fmt.Errorf("未知的排行榜指标: %s", metric)
	}
	base, err := c.windowBaseKey(window, windowStart)
	if err != nil {
		return "", err
	}
	return base + leaderboardZSetKeySuffix + string(metric), nil
}

func (c *leaderboardCache) windowBaseKey(window service.LeaderboardWindow, windowStart time.Time) (string, error) {
	stamp, err := leaderboardWindowStamp(window, windowStart)
	if err != nil {
		return "", err
	}
	return c.keyPrefix + leaderboardKeyNamespace + string(window) + ":" + stamp, nil
}

// insightsKey 是站点级洞察的 key：与 Window 无关，只带今日窗口起点。
func (c *leaderboardCache) insightsKey(todayStart time.Time) (string, error) {
	stamp, err := leaderboardWindowStamp(service.LeaderboardWindowToday, todayStart)
	if err != nil {
		return "", err
	}
	return c.keyPrefix + leaderboardKeyNamespace + leaderboardInsightsKeyPrefix + stamp, nil
}

// leaderboardWindowStamp 把窗口起点压成 key 里的日期戳：
// today / week 用 YYYYMMDD（week 是该周周一），month 用 YYYYMM。
func leaderboardWindowStamp(window service.LeaderboardWindow, windowStart time.Time) (string, error) {
	if windowStart.IsZero() {
		return "", errors.New("排行榜窗口起点为空")
	}
	switch window {
	case service.LeaderboardWindowToday, service.LeaderboardWindowWeek:
		return windowStart.Format("20060102"), nil
	case service.LeaderboardWindowMonth:
		return windowStart.Format("200601"), nil
	default:
		return "", fmt.Errorf("未知的排行榜窗口: %s", window)
	}
}

// leaderboardSnapshotTTL 取 min(60 分钟, 距窗口结束)：
// 跨零点 / 周一 / 月初时旧 key 自然作废；60 分钟的硬上限保证作业停摆时
// 旧快照最多再服务一小时，之后退回「正在计算」而不是无限期展示旧数据。
func leaderboardSnapshotTTL(windowEnd, now time.Time) time.Duration {
	ttl := service.LeaderboardSnapshotMaxTTL
	if !windowEnd.IsZero() {
		if remaining := windowEnd.Sub(now); remaining < ttl {
			ttl = remaining
		}
	}
	if ttl < time.Second {
		ttl = time.Second
	}
	return ttl
}

// Hash value 是十三段逗号分隔的整数，顺序固定（design D20）：
//
//	tokens, requests, input, cache_read, output, cache_creation,
//	night_tokens, distinct_models, max_single, media_requests,
//	yesterday_tokens, yesterday_requests, cost_micros
//
// 第 1、2、13 段是三个 Metric，其余十段只喂 Cache Hit Rate（缓存命中率）、Extremes（之最）
// 与 Token 构成，MUST NOT 参与排名。media_requests 占住第 10 段但没有任何响应字段读它——
// 本轮只存不展示。cost_micros 追加在末尾而不是插在 Metric 旁边：段序是契约，
// 在中间插一段会让旧 value 整体错位，追加则让旧的十二段式自然解出 CostMicros = 0。
func encodeLeaderboardMetrics(entry service.LeaderboardUserMetrics) string {
	segments := []int64{
		entry.TotalTokens,
		entry.SuccessfulRequests,
		entry.InputTokens,
		entry.CacheReadTokens,
		entry.OutputTokens,
		entry.CacheCreationTokens,
		entry.NightTokens,
		int64(entry.DistinctModels),
		entry.MaxSingleTokens,
		entry.MediaRequests,
		entry.YesterdayTokens,
		entry.YesterdayRequests,
		entry.CostMicros,
	}
	var sb strings.Builder
	for i, value := range segments {
		if i > 0 {
			_ = sb.WriteByte(',')
		}
		_, _ = sb.WriteString(strconv.FormatInt(value, 10))
	}
	return sb.String()
}

// decodeLeaderboardMetrics 容忍段数不足的历史 value：缺的段一律按 0 处理，
// 因此上一版写入的两段式、四段式与十二段式 value 都仍能读出榜单本体，升级后无需清 Redis——
// 该窗口只是暂时「没有命中率 / 没有之最卡 / 金额为 0」，下一轮重建（最长 5 分钟）即补齐。
// 前两段解析不出来才算无效行（那不是旧值，是坏值）。
func decodeLeaderboardMetrics(userID int64, raw string) (service.LeaderboardUserMetrics, bool) {
	parts := strings.Split(raw, ",")
	tokens, ok := leaderboardMetricSegment(parts, 0)
	if !ok {
		return service.LeaderboardUserMetrics{}, false
	}
	requests, ok := leaderboardMetricSegment(parts, 1)
	if !ok {
		return service.LeaderboardUserMetrics{}, false
	}
	inputTokens, _ := leaderboardMetricSegment(parts, 2)
	cacheReadTokens, _ := leaderboardMetricSegment(parts, 3)
	outputTokens, _ := leaderboardMetricSegment(parts, 4)
	cacheCreationTokens, _ := leaderboardMetricSegment(parts, 5)
	nightTokens, _ := leaderboardMetricSegment(parts, 6)
	distinctModels, _ := leaderboardMetricSegment(parts, 7)
	maxSingleTokens, _ := leaderboardMetricSegment(parts, 8)
	mediaRequests, _ := leaderboardMetricSegment(parts, 9)
	yesterdayTokens, _ := leaderboardMetricSegment(parts, 10)
	yesterdayRequests, _ := leaderboardMetricSegment(parts, 11)
	costMicros, _ := leaderboardMetricSegment(parts, 12)
	return service.LeaderboardUserMetrics{
		UserID:              userID,
		TotalTokens:         tokens,
		SuccessfulRequests:  requests,
		InputTokens:         inputTokens,
		CacheReadTokens:     cacheReadTokens,
		OutputTokens:        outputTokens,
		CacheCreationTokens: cacheCreationTokens,
		NightTokens:         nightTokens,
		DistinctModels:      int(distinctModels),
		MaxSingleTokens:     maxSingleTokens,
		MediaRequests:       mediaRequests,
		YesterdayTokens:     yesterdayTokens,
		YesterdayRequests:   yesterdayRequests,
		CostMicros:          costMicros,
	}, true
}

// ViewerModels 读本人模型偏好的 60 秒缓存。key 不存在时 found=false，由上层回源并回填；
// 内容损坏时同样按未命中处理，宁可多查一次也不给出半份数据。
func (c *leaderboardCache) ViewerModels(ctx context.Context, userID int64, window service.LeaderboardWindow, windowStart time.Time) ([]usagestats.LeaderboardModelUsageRow, int64, bool, error) {
	if c == nil || c.rdb == nil {
		return nil, 0, false, errLeaderboardCacheUnavailable
	}
	key, err := c.viewerModelsKey(userID, window, windowStart)
	if err != nil {
		return nil, 0, false, err
	}
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, 0, false, nil
		}
		return nil, 0, false, err
	}
	var payload leaderboardViewerModelsPayload
	if unmarshalErr := json.Unmarshal(raw, &payload); unmarshalErr != nil {
		return nil, 0, false, nil
	}
	return payload.Models, payload.Total, true, nil
}

// SetViewerModels 回填本人模型偏好的缓存。TTL 取 min(60 秒, 距窗口结束)：
// 跨零点时今日窗口的缓存不会活过零点，因此 window=today 换天后必然重新回源（design D21）。
func (c *leaderboardCache) SetViewerModels(ctx context.Context, userID int64, window service.LeaderboardWindow, windowStart, windowEnd time.Time, rows []usagestats.LeaderboardModelUsageRow, total int64) error {
	if c == nil || c.rdb == nil {
		return errLeaderboardCacheUnavailable
	}
	key, err := c.viewerModelsKey(userID, window, windowStart)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(leaderboardViewerModelsPayload{Models: rows, Total: total})
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, payload, leaderboardViewerModelsCacheTTL(windowEnd, time.Now())).Err()
}

// viewerModelsKey 形如 <prefix>leaderboard:v1:viewer:models:<window>:<窗口起点>:<user_id>。
// 带窗口起点是为了跨零点 / 周一 / 月初自然作废，与快照 key 同一条规则。
func (c *leaderboardCache) viewerModelsKey(userID int64, window service.LeaderboardWindow, windowStart time.Time) (string, error) {
	stamp, err := leaderboardWindowStamp(window, windowStart)
	if err != nil {
		return "", err
	}
	return c.keyPrefix + leaderboardKeyNamespace + leaderboardViewerModelsKeyPrefix +
		string(window) + ":" + stamp + ":" + strconv.FormatInt(userID, 10), nil
}

// leaderboardViewerModelsCacheTTL 取 min(60 秒, 距窗口结束)，取小逻辑与 leaderboardSnapshotTTL 一致。
func leaderboardViewerModelsCacheTTL(windowEnd, now time.Time) time.Duration {
	ttl := leaderboardViewerModelsTTL
	if !windowEnd.IsZero() {
		if remaining := windowEnd.Sub(now); remaining < ttl {
			ttl = remaining
		}
	}
	if ttl < time.Second {
		ttl = time.Second
	}
	return ttl
}

// leaderboardMetricSegment 取第 index 段并解析；段缺失或解析失败时返回 0, false，
// 由调用方决定是「按 0 处理」（第 3 段起的十一段）还是「整行作废」（前两段）。
func leaderboardMetricSegment(parts []string, index int) (int64, bool) {
	if index >= len(parts) {
		return 0, false
	}
	value, err := strconv.ParseInt(strings.TrimSpace(parts[index]), 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}
