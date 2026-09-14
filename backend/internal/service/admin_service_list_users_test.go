//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type userRepoStubForListUsers struct {
	userRepoStub
	users                 []User
	err                   error
	listWithFiltersParams pagination.PaginationParams
	lastUsedByUserID      map[int64]*time.Time
	lastUsedErr           error
	avatarsByUserID       map[int64]*UserAvatar
	avatarBatchErr        error
	avatarBatchIDs        []int64
}

func (s *userRepoStubForListUsers) ListWithFilters(_ context.Context, params pagination.PaginationParams, _ UserListFilters) ([]User, *pagination.PaginationResult, error) {
	s.listWithFiltersParams = params
	if s.err != nil {
		return nil, nil, s.err
	}
	out := make([]User, len(s.users))
	copy(out, s.users)
	return out, &pagination.PaginationResult{
		Total:    int64(len(out)),
		Page:     params.Page,
		PageSize: params.PageSize,
	}, nil
}

func (s *userRepoStubForListUsers) GetByIDs(context.Context, []int64) ([]User, error) {
	return nil, nil
}

func (s *userRepoStubForListUsers) GetLatestUsedAtByUserIDs(_ context.Context, userIDs []int64) (map[int64]*time.Time, error) {
	if s.lastUsedErr != nil {
		return nil, s.lastUsedErr
	}
	result := make(map[int64]*time.Time, len(userIDs))
	for _, userID := range userIDs {
		if ts, ok := s.lastUsedByUserID[userID]; ok {
			result[userID] = ts
		}
	}
	return result, nil
}

func (s *userRepoStubForListUsers) GetLatestUsedAtByUserID(_ context.Context, userID int64) (*time.Time, error) {
	if s.lastUsedErr != nil {
		return nil, s.lastUsedErr
	}
	return s.lastUsedByUserID[userID], nil
}

func (s *userRepoStubForListUsers) GetUserAvatar(_ context.Context, userID int64) (*UserAvatar, error) {
	if s.avatarBatchErr != nil {
		return nil, s.avatarBatchErr
	}
	return s.avatarsByUserID[userID], nil
}

func (s *userRepoStubForListUsers) GetUserAvatarsByUserIDs(_ context.Context, userIDs []int64) (map[int64]*UserAvatar, error) {
	s.avatarBatchIDs = append([]int64(nil), userIDs...)
	if s.avatarBatchErr != nil {
		return nil, s.avatarBatchErr
	}
	result := make(map[int64]*UserAvatar, len(userIDs))
	for _, userID := range userIDs {
		if avatar, ok := s.avatarsByUserID[userID]; ok {
			result[userID] = avatar
		}
	}
	return result, nil
}

type userGroupRateRepoStubForListUsers struct {
	batchCalls int
	singleCall []int64

	batchErr  error
	batchData map[int64]map[int64]float64

	singleErr  map[int64]error
	singleData map[int64]map[int64]float64
}

func (s *userGroupRateRepoStubForListUsers) GetByUserIDs(_ context.Context, _ []int64) (map[int64]map[int64]float64, error) {
	s.batchCalls++
	if s.batchErr != nil {
		return nil, s.batchErr
	}
	return s.batchData, nil
}

func (s *userGroupRateRepoStubForListUsers) GetByUserID(_ context.Context, userID int64) (map[int64]float64, error) {
	s.singleCall = append(s.singleCall, userID)
	if err, ok := s.singleErr[userID]; ok {
		return nil, err
	}
	if rates, ok := s.singleData[userID]; ok {
		return rates, nil
	}
	return map[int64]float64{}, nil
}

func (s *userGroupRateRepoStubForListUsers) GetByUserAndGroup(_ context.Context, userID, groupID int64) (*float64, error) {
	panic("unexpected GetByUserAndGroup call")
}

func (s *userGroupRateRepoStubForListUsers) GetRPMOverrideByUserAndGroup(_ context.Context, _, _ int64) (*int, error) {
	panic("unexpected GetRPMOverrideByUserAndGroup call")
}

func (s *userGroupRateRepoStubForListUsers) SyncUserGroupRates(_ context.Context, userID int64, rates map[int64]*float64) error {
	panic("unexpected SyncUserGroupRates call")
}

func (s *userGroupRateRepoStubForListUsers) GetByGroupID(_ context.Context, _ int64) ([]UserGroupRateEntry, error) {
	panic("unexpected GetByGroupID call")
}

func (s *userGroupRateRepoStubForListUsers) SyncGroupRateMultipliers(_ context.Context, _ int64, _ []GroupRateMultiplierInput) error {
	panic("unexpected SyncGroupRateMultipliers call")
}

func (s *userGroupRateRepoStubForListUsers) SyncGroupRPMOverrides(_ context.Context, _ int64, _ []GroupRPMOverrideInput) error {
	panic("unexpected SyncGroupRPMOverrides call")
}

func (s *userGroupRateRepoStubForListUsers) ClearGroupRPMOverrides(_ context.Context, _ int64) error {
	panic("unexpected ClearGroupRPMOverrides call")
}

func (s *userGroupRateRepoStubForListUsers) DeleteByGroupID(_ context.Context, _ int64) error {
	panic("unexpected DeleteByGroupID call")
}

func (s *userGroupRateRepoStubForListUsers) DeleteByUserID(_ context.Context, userID int64) error {
	panic("unexpected DeleteByUserID call")
}

func TestAdminService_ListUsers_BatchRateFallbackToSingle(t *testing.T) {
	userRepo := &userRepoStubForListUsers{
		users: []User{
			{ID: 101, Username: "u1"},
			{ID: 202, Username: "u2"},
		},
	}
	rateRepo := &userGroupRateRepoStubForListUsers{
		batchErr: errors.New("batch unavailable"),
		singleData: map[int64]map[int64]float64{
			101: {11: 1.1},
			202: {22: 2.2},
		},
	}
	svc := &adminServiceImpl{
		userRepo:          userRepo,
		userGroupRateRepo: rateRepo,
	}

	users, total, err := svc.ListUsers(context.Background(), 1, 20, UserListFilters{}, "", "")
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, users, 2)
	require.Equal(t, 1, rateRepo.batchCalls)
	require.ElementsMatch(t, []int64{101, 202}, rateRepo.singleCall)
	require.Equal(t, 1.1, users[0].GroupRates[11])
	require.Equal(t, 2.2, users[1].GroupRates[22])
}

func TestAdminService_ListUsers_PassesSortParams(t *testing.T) {
	userRepo := &userRepoStubForListUsers{
		users: []User{{ID: 1, Email: "a@example.com"}},
	}
	svc := &adminServiceImpl{userRepo: userRepo}

	_, _, err := svc.ListUsers(context.Background(), 2, 50, UserListFilters{}, "email", "ASC")
	require.NoError(t, err)
	require.Equal(t, pagination.PaginationParams{
		Page:      2,
		PageSize:  50,
		SortBy:    "email",
		SortOrder: "ASC",
	}, userRepo.listWithFiltersParams)
}

func TestAdminService_ListUsers_PopulatesLastUsedAt(t *testing.T) {
	lastUsed := time.Now().UTC().Add(-30 * time.Minute).Truncate(time.Second)
	userRepo := &userRepoStubForListUsers{
		users: []User{{ID: 101, Email: "u@example.com"}},
		lastUsedByUserID: map[int64]*time.Time{
			101: &lastUsed,
		},
	}
	svc := &adminServiceImpl{userRepo: userRepo}

	users, total, err := svc.ListUsers(context.Background(), 1, 20, UserListFilters{}, "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, users, 1)
	require.NotNil(t, users[0].LastUsedAt)
	require.WithinDuration(t, lastUsed, *users[0].LastUsedAt, time.Second)
}

func TestAdminService_ListUsers_PopulatesAvatarURL(t *testing.T) {
	userRepo := &userRepoStubForListUsers{
		users: []User{
			{ID: 101, Email: "inline@example.com"},
			{ID: 202, Email: "remote@example.com"},
			{ID: 303, Email: "none@example.com"},
		},
		avatarsByUserID: map[int64]*UserAvatar{
			101: {StorageProvider: "inline", URL: "data:image/webp;base64,QUJD", ContentType: "image/webp"},
			202: {StorageProvider: "remote_url", URL: "https://cdn.example.com/a.png"},
		},
	}
	svc := &adminServiceImpl{userRepo: userRepo}

	users, total, err := svc.ListUsers(context.Background(), 1, 20, UserListFilters{IncludeAvatars: true}, "", "")
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, users, 3)
	require.ElementsMatch(t, []int64{101, 202, 303}, userRepo.avatarBatchIDs)

	require.Equal(t, "data:image/webp;base64,QUJD", users[0].AvatarURL)
	require.Equal(t, "inline", users[0].AvatarSource)
	require.Equal(t, "https://cdn.example.com/a.png", users[1].AvatarURL)
	require.Equal(t, "remote_url", users[1].AvatarSource)
	require.Empty(t, users[2].AvatarURL)
}

func TestAdminService_ListUsers_AvatarBatchErrorKeepsUsers(t *testing.T) {
	userRepo := &userRepoStubForListUsers{
		users: []User{
			{ID: 101, Email: "a@example.com"},
			{ID: 202, Email: "b@example.com"},
		},
		avatarBatchErr: errors.New("avatar batch unavailable"),
	}
	svc := &adminServiceImpl{userRepo: userRepo}

	users, total, err := svc.ListUsers(context.Background(), 1, 20, UserListFilters{IncludeAvatars: true}, "", "")
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, users, 2)
	require.Equal(t, int64(101), users[0].ID)
	require.Equal(t, int64(202), users[1].ID)
	require.Empty(t, users[0].AvatarURL)
	require.Empty(t, users[1].AvatarURL)
}

func TestAdminService_ListUsers_SkipsAvatarsUnlessRequested(t *testing.T) {
	userRepo := &userRepoStubForListUsers{
		users: []User{{ID: 101, Email: "inline@example.com"}},
		avatarsByUserID: map[int64]*UserAvatar{
			101: {StorageProvider: "inline", URL: "data:image/webp;base64,QUJD", ContentType: "image/webp"},
		},
	}
	svc := &adminServiceImpl{userRepo: userRepo}

	// 默认（搜索联想、批量取 ID 等调用方）不查头像、不下发头像。
	users, _, err := svc.ListUsers(context.Background(), 1, 20, UserListFilters{}, "", "")
	require.NoError(t, err)
	require.Len(t, users, 1)
	require.Nil(t, userRepo.avatarBatchIDs, "avatar batch must not run unless IncludeAvatars is set")
	require.Empty(t, users[0].AvatarURL)
}

func TestAdminService_GetUser_PopulatesAvatarURL(t *testing.T) {
	userRepo := &userRepoStubForListUsers{
		avatarsByUserID: map[int64]*UserAvatar{
			7: {StorageProvider: "inline", URL: "data:image/webp;base64,QUJD", ContentType: "image/webp", ByteSize: 3},
		},
	}
	userRepo.user = &User{ID: 7, Email: "u@example.com"}
	svc := &adminServiceImpl{userRepo: userRepo}

	user, err := svc.GetUser(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "data:image/webp;base64,QUJD", user.AvatarURL)
	require.Equal(t, "inline", user.AvatarSource)
	require.Equal(t, "image/webp", user.AvatarMIME)
}
