package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Leaderboard（排行榜）的 gin.Context 键：由用户侧路由的 leaderboardModeGuard 写入。
//
// guard 用带短 TTL 的进程内缓存读取器取一次档位，handler 直接复用，避免同一个请求里
// 读两次设置而在 TTL 边界上给出自相矛盾的响应（guard 放行了，响应却按另一档渲染）。
const (
	LeaderboardModeContextKey    = "leaderboard_mode"
	LeaderboardPreviewContextKey = "leaderboard_preview"
)

// leaderboardNotFoundBody 是 gin 对未注册路由的默认 404 响应体（gin 的 default404Body）。
// off 档下普通用户看到的 404 与它逐字节一致——纯文本、同一个 Content-Type——因此
// 无法和「这条路由根本不存在」区分开：响应 MUST NOT 透露该路由存在，带 reason 的
// JSON 错误信封（更别说 reason 里写着 LEADERBOARD）等于亲口承认功能存在。
const leaderboardNotFoundBody = "404 page not found"

// leaderboardNotFoundContentType 与 gin serveError 用的 mimePlain 一致：是裸的
// "text/plain"，不带 charset——c.String 会补上 "; charset=utf-8"，那一点点差异
// 同样足以把这条路由认出来，所以这里显式写 Content-Type。
const leaderboardNotFoundContentType = "text/plain"

// AbortLeaderboardNotFound 写出这个不可区分的 404 并中止该请求。
// 用户侧路由的 leaderboardModeGuard 与本文件的 Get 都走它，两处的字节输出因此一致；
// 它 MUST NOT 被换成 response.ErrorFrom / response.NotFound——那会把功能的存在暴露出去。
func AbortLeaderboardNotFound(c *gin.Context) {
	c.Data(http.StatusNotFound, leaderboardNotFoundContentType, []byte(leaderboardNotFoundBody))
	c.Abort()
}

// LeaderboardHandler 提供用户侧的 GET /api/v1/leaderboard。
// 管理员与普通用户共用这一个接口，不另开管理端路由（design D9）。
type LeaderboardHandler struct {
	service *service.LeaderboardService
}

func NewLeaderboardHandler(svc *service.LeaderboardService) *LeaderboardHandler {
	return &LeaderboardHandler{service: svc}
}

// Get 返回某个 Window（榜单窗口）× Metric（排名指标）的榜单。
//
// 参数都有默认值（window=today、metric=total_tokens），取值不在枚举内一律 400 且
// MUST NOT 回落到默认值；自定义起止日期之类的多余参数不会改变窗口边界——这里根本不读它们。
func (h *LeaderboardHandler) Get(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "user not found in context")
		return
	}

	window := service.LeaderboardWindowToday
	if raw := strings.TrimSpace(c.Query("window")); raw != "" {
		parsed, valid := service.ParseLeaderboardWindow(raw)
		if !valid {
			response.ErrorFrom(c, service.ErrLeaderboardInvalidWindow)
			return
		}
		window = parsed
	}

	metric := service.LeaderboardMetricTotalTokens
	if raw := strings.TrimSpace(c.Query("metric")); raw != "" {
		parsed, valid := service.ParseLeaderboardMetric(raw)
		if !valid {
			// Metric 只有 total_tokens / successful_requests / cost 三项；写别的名字
			// （money、actual_cost 之类）走的就是这条「非法 metric」分支。
			response.ErrorFrom(c, service.ErrLeaderboardInvalidMetric)
			return
		}
		metric = parsed
	}

	view, err := h.service.Query(
		c.Request.Context(),
		subject.UserID,
		window,
		metric,
		leaderboardModeFromContext(c),
		leaderboardViewerIsAdmin(c),
	)
	if err != nil {
		// off 档下的普通用户：服务端再强制一次，输出与未注册路由一致的 404。
		if errors.Is(err, service.ErrLeaderboardNotFound) {
			AbortLeaderboardNotFound(c)
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

// leaderboardModeFromContext 读取 guard 写入的档位；读不到时 fail-closed 到 off。
func leaderboardModeFromContext(c *gin.Context) string {
	value, exists := c.Get(LeaderboardModeContextKey)
	if !exists {
		return service.LeaderboardModeOff
	}
	mode, ok := value.(string)
	if !ok {
		return service.LeaderboardModeOff
	}
	return mode
}

// leaderboardViewerIsAdmin 只用于 off 档：普通用户 404，管理员进入 Preview（预览）。
// 模式开启后管理员与普通用户看到完全相同的数据，Leaderboard 不做角色分支。
func leaderboardViewerIsAdmin(c *gin.Context) bool {
	if preview, exists := c.Get(LeaderboardPreviewContextKey); exists {
		if flag, ok := preview.(bool); ok && flag {
			return true
		}
	}
	role, ok := middleware.GetUserRoleFromContext(c)
	return ok && role == service.RoleAdmin
}
