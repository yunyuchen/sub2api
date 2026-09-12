package service

import (
	"strings"
	"unicode"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ErrLeaderboardUsernameInvalid 表示 username 不足以在 Leaderboard（排行榜）上实名展示。
// 用户开启 Named Participation（昵称展示，默认开）时校验不通过就返回它（400，不落库）；
// 它 MUST NOT 用于个人资料现有的 username 通用校验——那条路径还承载注册与 OAuth 回填。
var ErrLeaderboardUsernameInvalid = infraerrors.BadRequest(
	"LEADERBOARD_USERNAME_INVALID",
	"username is not eligible for named participation on the leaderboard",
)

// Leaderboard 实名展示名的规则常量（design D2 的初始值，可调）。
const (
	// leaderboardUsernameMinLength / MaxLength 按 rune 计，长度在去首尾空白之后统计。
	leaderboardUsernameMinLength = 2
	leaderboardUsernameMaxLength = 32
	// leaderboardUsernameExtraRunes 是字母数字之外额外允许的字符（空格单独处理：
	// 只允许出现在词间，且不得连续）。
	leaderboardUsernameExtraRunes = "_-."
)

// leaderboardUsernameReservedWords 是保留词清单，按大小写不敏感的子串匹配。
// 命中即拒绝：这些词会让匿名的其他用户误以为该条目来自站点官方。
var leaderboardUsernameReservedWords = []string{
	"admin",
	"root",
	"system",
	"official",
	"support",
	"sub2api",
	"官方",
	"管理员",
	"客服",
	"系统",
}

// validateLeaderboardUsername 校验 username 是否可以在 Leaderboard 上实名展示。
//
// 规则（design D2）：去首尾空白后 2–32 个字符；只允许 Unicode 字母、数字、
// `_`、`-`、`.` 以及词间的单个空格；不得含 `@`（邮箱形态）；不得命中保留词。
// 两处调用它：用户开启 Named Participation 时（拒绝开启），以及每次渲染榜单时
// （不合格则回退匿名形态，因为用户可能在开启之后把 username 改坏）。
func validateLeaderboardUsername(username string) error {
	name := strings.TrimSpace(username)
	if name == "" {
		return ErrLeaderboardUsernameInvalid
	}

	runes := []rune(name)
	if len(runes) < leaderboardUsernameMinLength || len(runes) > leaderboardUsernameMaxLength {
		return ErrLeaderboardUsernameInvalid
	}

	// 邮箱形态单独挡一次：字符集检查已覆盖 `@`，但这条规则是显式写进设计的，
	// 保留它让「为什么拒绝」在代码里一眼可读。
	if strings.ContainsRune(name, '@') {
		return ErrLeaderboardUsernameInvalid
	}

	prevSpace := false
	for _, r := range runes {
		switch {
		case r == ' ':
			// 首尾空白已 Trim，这里只可能是词间空格；不允许连续空格。
			if prevSpace {
				return ErrLeaderboardUsernameInvalid
			}
			prevSpace = true
		case unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(leaderboardUsernameExtraRunes, r):
			prevSpace = false
		default:
			// 其余一律拒绝：控制字符、其他空白（\t \n 等）、标点与符号。
			return ErrLeaderboardUsernameInvalid
		}
	}

	lowered := strings.ToLower(name)
	for _, word := range leaderboardUsernameReservedWords {
		if strings.Contains(lowered, strings.ToLower(word)) {
			return ErrLeaderboardUsernameInvalid
		}
	}

	return nil
}

// isLeaderboardNamedEligible 报告某个用户是否应当以实名形态出现在 Leaderboard 上。
// 充分必要条件是「已开启 Named Participation」且「username 通过校验」；两者缺一
// 都回退匿名形态，而不是报错或把该用户整行丢弃——关闭开关的用户仍然参与排名。
//
// 注意这里 MUST NOT 兜底成「User #id」：用户 id 是自增的，可用来推算注册规模。
func isLeaderboardNamedEligible(u *User) bool {
	if u == nil || !u.LeaderboardNamedParticipation {
		return false
	}
	return validateLeaderboardUsername(u.Username) == nil
}
