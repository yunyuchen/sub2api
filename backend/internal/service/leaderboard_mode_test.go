//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// TestNormalizeLeaderboardMode 锁定读取侧的 fail-closed 方向：空值与非法值一律落到
// off，只有三档之一（允许大小写与首尾空白变体）才会被接受。
// 对照 channel_monitor_probe_retirement_test.go 的 TestNormalizeChannelMonitorMode，
// 区别只在默认值方向：渠道监控落首档 v1，排行榜落最保守的 off。
func TestNormalizeLeaderboardMode(t *testing.T) {
	t.Run("合法档位原样返回", func(t *testing.T) {
		require.Equal(t, LeaderboardModeOff, normalizeLeaderboardMode("off"))
		require.Equal(t, LeaderboardModeAnonymous, normalizeLeaderboardMode("anonymous"))
		require.Equal(t, LeaderboardModeNamed, normalizeLeaderboardMode("named"))
	})

	t.Run("空值落到 off", func(t *testing.T) {
		require.Equal(t, LeaderboardModeOff, normalizeLeaderboardMode(""))
		require.Equal(t, LeaderboardModeOff, normalizeLeaderboardMode("   "))
		require.Equal(t, LeaderboardModeOff, normalizeLeaderboardMode("\t\n"))
	})

	t.Run("非法值落到 off", func(t *testing.T) {
		for _, raw := range []string{
			"public",
			"on",
			"true",
			"enabled",
			"v2",
			"NAME",
			" PUBLIC ",
			"anonymou",
			"named ranking",
		} {
			require.Equal(t, LeaderboardModeOff, normalizeLeaderboardMode(raw), "raw=%q", raw)
		}
	})

	t.Run("大小写与空格变体归一化到对应档位", func(t *testing.T) {
		require.Equal(t, LeaderboardModeOff, normalizeLeaderboardMode(" OFF "))
		require.Equal(t, LeaderboardModeAnonymous, normalizeLeaderboardMode(" Anonymous "))
		require.Equal(t, LeaderboardModeNamed, normalizeLeaderboardMode("\tNAMED\n"))
	})

	t.Run("默认档位常量即 off", func(t *testing.T) {
		require.Equal(t, LeaderboardModeOff, defaultLeaderboardMode)
	})
}

// LeaderboardMode 读取器是路由 guard 的唯一入口：它必须 fail-closed，且必须靠
// 进程内缓存把「每请求一次 settings 查询」收敛掉。

func TestLeaderboardMode_ReturnsStoredModeAndCaches(t *testing.T) {
	repo := &bmRepoStub{
		getValueFn: func(_ context.Context, key string) (string, error) {
			require.Equal(t, SettingKeyLeaderboardMode, key)
			return LeaderboardModeNamed, nil
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	require.Equal(t, LeaderboardModeNamed, svc.LeaderboardMode(context.Background()))
	require.Equal(t, LeaderboardModeNamed, svc.LeaderboardMode(context.Background()))
	require.Equal(t, 1, repo.calls, "TTL 内的连续请求 MUST NOT 各自查一次 settings 表")
}

func TestLeaderboardMode_InvalidStoredValueFallsBackToOff(t *testing.T) {
	for _, stored := range []string{"", "public", "  ", "enabled"} {
		repo := &bmRepoStub{
			getValueFn: func(_ context.Context, _ string) (string, error) {
				return stored, nil
			},
		}
		svc := NewSettingService(repo, &config.Config{})
		require.Equal(t, LeaderboardModeOff, svc.LeaderboardMode(context.Background()), "stored=%q", stored)
	}
}

func TestLeaderboardMode_DBErrorFailsClosed(t *testing.T) {
	repo := &bmRepoStub{
		getValueFn: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("database is down")
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	require.Equal(t, LeaderboardModeOff, svc.LeaderboardMode(context.Background()))
}

func TestLeaderboardMode_SettingNotFoundFallsBackToOff(t *testing.T) {
	repo := &bmRepoStub{
		getValueFn: func(_ context.Context, _ string) (string, error) {
			return "", ErrSettingNotFound
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	require.Equal(t, LeaderboardModeOff, svc.LeaderboardMode(context.Background()))
}

// 缓存挂在实例上而不是包级变量：否则一个测试（或一个部署里的一份 SettingService）
// 读到的档位会串到另一个上去。
func TestLeaderboardMode_CacheIsPerService(t *testing.T) {
	named := NewSettingService(&bmRepoStub{
		getValueFn: func(_ context.Context, _ string) (string, error) { return LeaderboardModeNamed, nil },
	}, &config.Config{})
	anonymous := NewSettingService(&bmRepoStub{
		getValueFn: func(_ context.Context, _ string) (string, error) { return LeaderboardModeAnonymous, nil },
	}, &config.Config{})

	require.Equal(t, LeaderboardModeNamed, named.LeaderboardMode(context.Background()))
	require.Equal(t, LeaderboardModeAnonymous, anonymous.LeaderboardMode(context.Background()))
	require.Equal(t, LeaderboardModeNamed, named.LeaderboardMode(context.Background()))
}

func TestLeaderboardMode_NilServiceFailsClosed(t *testing.T) {
	var svc *SettingService
	require.Equal(t, LeaderboardModeOff, svc.LeaderboardMode(context.Background()))
}
