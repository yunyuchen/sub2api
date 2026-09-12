//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateLeaderboardUsername_Accepts(t *testing.T) {
	for _, name := range []string{
		"ab",
		"alice",
		"Alice Wang",
		"a_b-c.d",
		"张三",
		"用户 一",
		"n9",
		strings.Repeat("a", 32),
		"  trimmed  ", // 去首尾空白后仍合格
	} {
		require.NoError(t, validateLeaderboardUsername(name), "username=%q", name)
	}
}

func TestValidateLeaderboardUsername_Rejects(t *testing.T) {
	cases := []struct {
		name     string
		username string
	}{
		{"空值", ""},
		{"纯空白", "   "},
		{"去空白后只剩一个字符", " a "},
		{"单字符", "a"},
		{"超长", strings.Repeat("a", 33)},
		{"邮箱形态", "someone@example.com"},
		{"仅含 @", "a@b"},
		{"非法字符斜杠", "alice/bob"},
		{"非法字符井号", "alice#1"},
		{"非法字符尖括号", "<script>"},
		{"词间连续空格", "alice  bob"},
		{"含制表符", "alice\tbob"},
		{"含换行", "alice\nbob"},
		{"保留词 admin", "admin"},
		{"保留词大小写变体", "AdMiN"},
		{"保留词子串", "superadmin"},
		{"保留词 root", "root"},
		{"保留词 system", "my system"},
		{"保留词 official", "Official Team"},
		{"保留词 support", "support"},
		{"保留词 sub2api", "sub2api"},
		{"保留词 官方", "官方账号"},
		{"保留词 管理员", "超级管理员"},
		{"保留词 客服", "客服小王"},
		{"保留词 系统", "系统通知"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLeaderboardUsername(tc.username)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrLeaderboardUsernameInvalid)
		})
	}
}

func TestIsLeaderboardNamedEligible(t *testing.T) {
	t.Run("开关开启且 username 合格", func(t *testing.T) {
		require.True(t, isLeaderboardNamedEligible(&User{
			Username:                      "alice",
			LeaderboardNamedParticipation: true,
		}))
	})

	t.Run("开关关闭一律不合格", func(t *testing.T) {
		require.False(t, isLeaderboardNamedEligible(&User{
			Username:                      "alice",
			LeaderboardNamedParticipation: false,
		}))
	})

	t.Run("开关开启但 username 不合格", func(t *testing.T) {
		for _, username := range []string{
			"",
			"   ",
			"someone@example.com",
			"admin",
			strings.Repeat("a", 33),
		} {
			require.False(t, isLeaderboardNamedEligible(&User{
				Username:                      username,
				LeaderboardNamedParticipation: true,
			}), "username=%q", username)
		}
	})

	t.Run("nil 用户不合格", func(t *testing.T) {
		require.False(t, isLeaderboardNamedEligible(nil))
	})
}
