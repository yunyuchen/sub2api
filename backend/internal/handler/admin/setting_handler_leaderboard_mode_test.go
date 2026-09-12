//go:build unit

package admin

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

// leaderboard_mode 是三档递增的暴露开关，写入侧必须严格白名单：一个拼错的档位
// 若被静默归一化落库，管理员会以为自己开的是这一档、实际生效的是另一档。
// 同时它必须是指针语义——只发别的字段的部分载荷不能把它刷回 off。

func TestUpdateSettingsRejectsInvalidLeaderboardMode(t *testing.T) {
	for _, raw := range []any{"public", "on", "enabled", "", "  ", "nameed"} {
		h, repo := newStepUpSwitchTestHandler(t, map[string]string{
			service.SettingKeyLeaderboardMode: service.LeaderboardModeNamed,
		})

		rec := doUpdateSettings(t, h, map[string]any{"leaderboard_mode": raw}, nil)

		require.Equal(t, http.StatusBadRequest, rec.Code, "raw=%v", raw)
		require.Contains(t, rec.Body.String(), "Leaderboard mode must be off, anonymous or named")
		require.Equal(t, service.LeaderboardModeNamed, repo.values[service.SettingKeyLeaderboardMode],
			"非法值 MUST NOT 落库，原值必须保持不变")
	}
}

func TestUpdateSettingsAcceptsEachLeaderboardMode(t *testing.T) {
	for _, mode := range []string{
		service.LeaderboardModeOff,
		service.LeaderboardModeAnonymous,
		service.LeaderboardModeNamed,
	} {
		h, repo := newStepUpSwitchTestHandler(t, map[string]string{})

		rec := doUpdateSettings(t, h, map[string]any{"leaderboard_mode": mode}, nil)

		require.Equal(t, http.StatusOK, rec.Code, "mode=%s", mode)
		require.Equal(t, mode, repo.values[service.SettingKeyLeaderboardMode])
	}
}

func TestUpdateSettingsPartialPayloadKeepsLeaderboardMode(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyLeaderboardMode: service.LeaderboardModeAnonymous,
	})

	rec := doUpdateSettings(t, h, map[string]any{"site_name": "Example Gateway"}, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, service.LeaderboardModeAnonymous, repo.values[service.SettingKeyLeaderboardMode],
		"未提交该字段的更新 MUST NOT 把已有档位刷掉")
}

// 库里出现空值 / 历史遗留非法值时，读取侧归一化为 off，写回的也必须是 off，
// 而不是把非法值原样带回去。
func TestUpdateSettingsNormalizesStoredInvalidLeaderboardMode(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyLeaderboardMode: "public",
	})

	rec := doUpdateSettings(t, h, map[string]any{"site_name": "Example Gateway"}, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, service.LeaderboardModeOff, repo.values[service.SettingKeyLeaderboardMode])
}

func TestDiffSettingsReportsLeaderboardMode(t *testing.T) {
	before := &service.SystemSettings{LeaderboardMode: service.LeaderboardModeOff}
	after := &service.SystemSettings{LeaderboardMode: service.LeaderboardModeNamed}

	changed := diffSettings(before, after, nil, nil, UpdateSettingsRequest{})
	require.Contains(t, changed, "leaderboard_mode")

	unchanged := diffSettings(before, before, nil, nil, UpdateSettingsRequest{})
	require.NotContains(t, unchanged, "leaderboard_mode")
}
