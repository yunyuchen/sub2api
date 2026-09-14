package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserFromServiceAdmin_MapsActivityTimestamps(t *testing.T) {
	t.Parallel()

	lastLoginAt := time.Date(2026, time.April, 20, 10, 0, 0, 0, time.UTC)
	lastActiveAt := lastLoginAt.Add(15 * time.Minute)
	lastUsedAt := lastLoginAt.Add(45 * time.Minute)

	out := UserFromServiceAdmin(&service.User{
		ID:           42,
		Email:        "admin@example.com",
		Username:     "admin",
		Role:         service.RoleAdmin,
		Status:       service.StatusActive,
		LastActiveAt: &lastActiveAt,
		LastUsedAt:   &lastUsedAt,
	})

	require.NotNil(t, out)
	require.NotNil(t, out.LastActiveAt)
	require.NotNil(t, out.LastUsedAt)
	require.WithinDuration(t, lastActiveAt, *out.LastActiveAt, time.Second)
	require.WithinDuration(t, lastUsedAt, *out.LastUsedAt, time.Second)
}

func TestUserFromServiceAdmin_MapsAvatarURL(t *testing.T) {
	t.Parallel()

	out := UserFromServiceAdmin(&service.User{
		ID:           42,
		Email:        "admin@example.com",
		AvatarURL:    "data:image/webp;base64,QUJD",
		AvatarSource: "inline",
	})

	require.NotNil(t, out)
	require.Equal(t, "data:image/webp;base64,QUJD", out.AvatarURL)
}

// 没有头像时 avatar_url 必须被 omitempty 整个省掉，普通用户 DTO 形态也不受影响。
func TestUserFromServiceAdmin_OmitsEmptyAvatarURL(t *testing.T) {
	t.Parallel()

	out := UserFromServiceAdmin(&service.User{ID: 42, Email: "admin@example.com"})
	require.NotNil(t, out)
	require.Empty(t, out.AvatarURL)

	encoded, err := json.Marshal(out)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "avatar_url")
}
