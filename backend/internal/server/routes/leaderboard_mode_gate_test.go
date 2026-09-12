package routes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// leaderboardRouteSettingRepoStub 是给 guard 用的最小 SettingRepository。
// err 非空时模拟「设置读取失败」，guard MUST 按 off 处理（fail-closed）。
type leaderboardRouteSettingRepoStub struct {
	values map[string]string
	err    error
}

func (s *leaderboardRouteSettingRepoStub) Get(context.Context, string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *leaderboardRouteSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.values[key], nil
}

func (s *leaderboardRouteSettingRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *leaderboardRouteSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *leaderboardRouteSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *leaderboardRouteSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *leaderboardRouteSettingRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

// newLeaderboardModeSettings 每个用例新建一份 SettingService：档位缓存挂在实例上，
// 共用实例会让 5 秒 TTL 内的后一个用例读到前一个用例的档位。
func newLeaderboardModeSettings(mode string) *service.SettingService {
	return service.NewSettingService(&leaderboardRouteSettingRepoStub{
		values: map[string]string{service.SettingKeyLeaderboardMode: mode},
	}, &config.Config{})
}

func newFailingLeaderboardModeSettings() *service.SettingService {
	return service.NewSettingService(&leaderboardRouteSettingRepoStub{
		err: errors.New("settings unavailable"),
	}, &config.Config{})
}

func TestLeaderboardModeGuard(t *testing.T) {
	tests := []struct {
		name         string
		svc          *service.SettingService
		role         string
		wantStatus   int
		wantNotFound bool
		wantMode     string
		wantPreview  bool
	}{
		{
			name:         "off 档普通用户返回 404",
			svc:          newLeaderboardModeSettings(service.LeaderboardModeOff),
			role:         service.RoleUser,
			wantStatus:   http.StatusNotFound,
			wantNotFound: true,
		},
		{
			name:        "off 档管理员放行并带 preview 标记",
			svc:         newLeaderboardModeSettings(service.LeaderboardModeOff),
			role:        service.RoleAdmin,
			wantStatus:  http.StatusOK,
			wantMode:    service.LeaderboardModeOff,
			wantPreview: true,
		},
		{
			name:         "未写入过该设置时按 off 处理",
			svc:          newLeaderboardModeSettings(""),
			role:         service.RoleUser,
			wantStatus:   http.StatusNotFound,
			wantNotFound: true,
		},
		{
			name:         "库里存量非法值归一化为 off",
			svc:          newLeaderboardModeSettings("public"),
			role:         service.RoleUser,
			wantStatus:   http.StatusNotFound,
			wantNotFound: true,
		},
		{
			name:         "设置读取失败时 fail-closed 到 off",
			svc:          newFailingLeaderboardModeSettings(),
			role:         service.RoleUser,
			wantStatus:   http.StatusNotFound,
			wantNotFound: true,
		},
		{
			name:        "设置读取失败时管理员仍进入 Preview",
			svc:         newFailingLeaderboardModeSettings(),
			role:        service.RoleAdmin,
			wantStatus:  http.StatusOK,
			wantMode:    service.LeaderboardModeOff,
			wantPreview: true,
		},
		{
			name:         "settingService 缺失时按 off 处理",
			svc:          nil,
			role:         service.RoleUser,
			wantStatus:   http.StatusNotFound,
			wantNotFound: true,
		},
		{
			name:       "anonymous 档对普通用户放行",
			svc:        newLeaderboardModeSettings(service.LeaderboardModeAnonymous),
			role:       service.RoleUser,
			wantStatus: http.StatusOK,
			wantMode:   service.LeaderboardModeAnonymous,
		},
		{
			name:       "anonymous 档对管理员放行且不带 preview",
			svc:        newLeaderboardModeSettings(service.LeaderboardModeAnonymous),
			role:       service.RoleAdmin,
			wantStatus: http.StatusOK,
			wantMode:   service.LeaderboardModeAnonymous,
		},
		{
			name:       "named 档对普通用户放行",
			svc:        newLeaderboardModeSettings(service.LeaderboardModeNamed),
			role:       service.RoleUser,
			wantStatus: http.StatusOK,
			wantMode:   service.LeaderboardModeNamed,
		},
		{
			name:       "named 档对管理员放行且不带 preview",
			svc:        newLeaderboardModeSettings(service.LeaderboardModeNamed),
			role:       service.RoleAdmin,
			wantStatus: http.StatusOK,
			wantMode:   service.LeaderboardModeNamed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)

			router := gin.New()
			router.Use(func(c *gin.Context) {
				if tt.role != "" {
					c.Set(string(middleware.ContextKeyUserRole), tt.role)
				}
				c.Next()
			})
			router.Use(leaderboardModeGuard(tt.svc))
			router.GET("/test", func(c *gin.Context) {
				mode, _ := c.Get(handler.LeaderboardModeContextKey)
				preview, _ := c.Get(handler.LeaderboardPreviewContextKey)
				c.JSON(http.StatusOK, gin.H{"mode": mode, "preview": preview == true})
			})

			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
			router.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantNotFound {
				// 404 MUST NOT 透露该路由存在，更不能退化成 403：响应体与 gin 未注册
				// 路由的默认 404 逐字节一致，任何档位/功能名都不得出现在里面。
				require.Equal(t, "404 page not found", rec.Body.String())
				require.NotContains(t, strings.ToLower(rec.Body.String()), "leaderboard")
				return
			}
			require.Contains(t, rec.Body.String(), `"mode":"`+tt.wantMode+`"`)
			if tt.wantPreview {
				require.Contains(t, rec.Body.String(), `"preview":true`)
			} else {
				require.Contains(t, rec.Body.String(), `"preview":false`)
			}
		})
	}
}

// guard MUST 先于参数校验执行：off 档下带非法参数的请求同样是 404，不是 400。
func TestLeaderboardModeGuardRunsBeforeParameterValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUser)
		c.Next()
	})
	router.Use(leaderboardModeGuard(newLeaderboardModeSettings(service.LeaderboardModeOff)))
	router.GET("/test", func(c *gin.Context) {
		t.Fatal("off 档下普通用户的请求 MUST NOT 触达任何查询逻辑")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test?window=year&metric=cost", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "404 page not found", rec.Body.String())
}

// off 档下普通用户的 404 MUST 与「这条路由根本不存在」无法区分：
// 状态码、响应体与 Content-Type 都要和 gin 未注册路由的默认响应一致，
// 否则光凭响应形状就能确认功能存在（只是被关掉了）。
func TestLeaderboardModeGuardNotFoundIsIndistinguishableFromUnregisteredRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	guarded := gin.New()
	guarded.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUserRole), service.RoleUser)
		c.Next()
	})
	guarded.Use(leaderboardModeGuard(newLeaderboardModeSettings(service.LeaderboardModeOff)))
	guarded.GET("/leaderboard", func(c *gin.Context) {
		t.Fatal("off 档下普通用户的请求 MUST NOT 触达 handler")
	})

	guardedRec := httptest.NewRecorder()
	guarded.ServeHTTP(guardedRec, httptest.NewRequestWithContext(
		context.Background(), http.MethodGet, "/leaderboard", nil))

	// 对照组：一个没有注册任何路由的 gin engine。
	bare := gin.New()
	bareRec := httptest.NewRecorder()
	bare.ServeHTTP(bareRec, httptest.NewRequestWithContext(
		context.Background(), http.MethodGet, "/leaderboard", nil))

	require.Equal(t, bareRec.Code, guardedRec.Code)
	require.Equal(t, bareRec.Body.String(), guardedRec.Body.String())
	require.Equal(t, bareRec.Header().Get("Content-Type"), guardedRec.Header().Get("Content-Type"))
}
