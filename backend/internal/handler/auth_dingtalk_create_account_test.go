package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// dingTalkRegistrationClosedOptions 模拟线上配置：公开注册关闭、钉钉走 internal_only，企业豁免按 bypass 开或关。
func dingTalkRegistrationClosedOptions(bypass bool) oauthPendingFlowTestHandlerOptions {
	dingTalk := config.DingTalkConnectConfig{
		ClientID:            "test-client",
		ClientSecret:        "test-secret",
		AuthorizeURL:        "https://example.com/oauth2/auth",
		TokenURL:            "https://example.com/oauth2/token",
		UserInfoURL:         "https://example.com/oauth2/userinfo",
		RedirectURL:         "https://example.com/callback",
		FrontendRedirectURL: "https://example.com/auth/callback",
		DingTalkAppKind:     "internal_app",
		AppType:             "internal",
	}
	return oauthPendingFlowTestHandlerOptions{
		dingTalk: &dingTalk,
		settingValues: map[string]string{
			service.SettingKeyRegistrationEnabled:                  "false",
			service.SettingKeyDingTalkConnectEnabled:               "true",
			service.SettingKeyDingTalkConnectCorpRestrictionPolicy: "internal_only",
			service.SettingKeyDingTalkConnectBypassRegistration:    boolSettingValue(bypass),
		},
	}
}

// corpMemberClaims 是钉钉回调把用户解析成企业内 userid 后写入的 upstream claims。
func corpMemberClaims(subject string) map[string]any {
	return map[string]any{
		"corp_user_id": "corp-" + subject,
		"union_id":     subject,
	}
}

func createPendingOAuthTestSession(
	t *testing.T,
	client *dbent.Client,
	providerType string,
	subject string,
	claims map[string]any,
) *dbent.PendingAuthSession {
	t.Helper()
	session, err := client.PendingAuthSession.Create().
		SetSessionToken(subject + "-session-token").
		SetIntent(oauthIntentLogin).
		SetProviderType(providerType).
		SetProviderKey(providerType).
		SetProviderSubject(subject).
		SetBrowserSessionKey(subject + "-browser-key").
		SetUpstreamIdentityClaims(claims).
		SetRedirectTo("/dashboard").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(context.Background())
	require.NoError(t, err)
	return session
}

func postCreateAccount(t *testing.T, handle gin.HandlerFunc, path string, session *dbent.PendingAuthSession, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue(session.BrowserSessionKey)})
	ginCtx.Request = req
	handle(ginCtx)
	return recorder
}

func postPendingCreateAccount(t *testing.T, handler *AuthHandler, session *dbent.PendingAuthSession, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postCreateAccount(t, handler.CreatePendingOAuthAccount, "/api/v1/auth/oauth/pending/create-account", session, body)
}

func requireNoUserWithEmail(t *testing.T, client *dbent.Client, email string) {
	t.Helper()
	exists, err := client.User.Query().Where(dbuser.EmailEQ(email)).Exist(context.Background())
	require.NoError(t, err)
	require.False(t, exists)
}

func TestPendingOAuthSessionHasVerifiedCorpMembership(t *testing.T) {
	require.False(t, pendingOAuthSessionHasVerifiedCorpMembership(nil))
	require.False(t, pendingOAuthSessionHasVerifiedCorpMembership(&dbent.PendingAuthSession{ProviderType: "dingtalk"}))
	require.False(t, pendingOAuthSessionHasVerifiedCorpMembership(&dbent.PendingAuthSession{
		ProviderType:           "dingtalk",
		UpstreamIdentityClaims: map[string]any{"corp_user_id": "   "},
	}))
	require.False(t, pendingOAuthSessionHasVerifiedCorpMembership(&dbent.PendingAuthSession{
		ProviderType:           "wechat",
		UpstreamIdentityClaims: map[string]any{"corp_user_id": "u1"},
	}))
	require.True(t, pendingOAuthSessionHasVerifiedCorpMembership(&dbent.PendingAuthSession{
		ProviderType:           "DingTalk",
		UpstreamIdentityClaims: map[string]any{"corp_user_id": "u1"},
	}))
}

func TestCreatePendingOAuthAccountSkipsEmailCodeForDingTalkCorpMember(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	session := createPendingOAuthTestSession(t, client, "dingtalk", "dingtalk-corp-member-union", corpMemberClaims("dingtalk-corp-member-union"))

	recorder := postPendingCreateAccount(t, handler, session, `{"email":"any-address@example.org","password":"secret-123"}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.NotEmpty(t, payload["access_token"])

	createdUser, err := client.User.Query().Where(dbuser.EmailEQ("any-address@example.org")).Only(ctx)
	require.NoError(t, err)
	require.Equal(t, "dingtalk", createdUser.SignupSource)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("dingtalk"),
			authidentity.ProviderSubjectEQ("dingtalk-corp-member-union"),
		).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, createdUser.ID, identity.UserID)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, storedSession.ConsumedAt)
}

func TestCreateDingTalkOAuthAccountCorpMemberPassesClosedRegistrationViaBypass(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, dingTalkRegistrationClosedOptions(true))

	session := createPendingOAuthTestSession(t, client, "dingtalk", "dingtalk-bypass-union", corpMemberClaims("dingtalk-bypass-union"))

	// 钉钉专用路由；企业成员填错的 verify_code 会被忽略。
	recorder := postCreateAccount(t, handler.CreateDingTalkOAuthAccount, "/api/v1/auth/oauth/dingtalk/create-account", session,
		`{"email":"bypass-member@example.org","password":"secret-123","verify_code":"000000"}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	createdUser, err := client.User.Query().Where(dbuser.EmailEQ("bypass-member@example.org")).Only(context.Background())
	require.NoError(t, err)
	require.Equal(t, "dingtalk", createdUser.SignupSource)
}

func TestCreatePendingOAuthAccountCorpMemberStillBlockedWhenRegistrationClosedWithoutBypass(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, dingTalkRegistrationClosedOptions(false))

	session := createPendingOAuthTestSession(t, client, "dingtalk", "dingtalk-no-bypass-union", corpMemberClaims("dingtalk-no-bypass-union"))

	recorder := postPendingCreateAccount(t, handler, session, `{"email":"no-bypass@example.org","password":"secret-123"}`)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "REGISTRATION_DISABLED")
	requireNoUserWithEmail(t, client, "no-bypass@example.org")
}

func TestCreatePendingOAuthAccountRequiresEmailCodeForDingTalkWithoutCorpMembership(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)

	session := createPendingOAuthTestSession(t, client, "dingtalk", "dingtalk-cross-org-union", map[string]any{
		"corp_user_id": "",
		"union_id":     "dingtalk-cross-org-union",
	})

	recorder := postPendingCreateAccount(t, handler, session, `{"email":"outsider@example.org","password":"secret-123"}`)

	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "EMAIL_VERIFY_REQUIRED")
	requireNoUserWithEmail(t, client, "outsider@example.org")
}

func TestCreatePendingOAuthAccountIgnoresCorpUserIDClaimForOtherProviders(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)

	session := createPendingOAuthTestSession(t, client, "oidc", "oidc-with-corp-claim", map[string]any{
		"corp_user_id": "not-a-dingtalk-proof",
	})

	recorder := postPendingCreateAccount(t, handler, session, `{"email":"oidc-user@example.org","password":"secret-123"}`)

	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "EMAIL_VERIFY_REQUIRED")
	requireNoUserWithEmail(t, client, "oidc-user@example.org")
}

func TestCreatePendingOAuthAccountCorpMemberAliasCollisionReturnsEmailExists(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)

	_, err := client.User.Create().
		SetEmail("alice.alias@gmail.com").
		SetUsername("alice").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(context.Background())
	require.NoError(t, err)

	session := createPendingOAuthTestSession(t, client, "dingtalk", "dingtalk-alias-union", corpMemberClaims("dingtalk-alias-union"))

	recorder := postPendingCreateAccount(t, handler, session, `{"email":"alicealias+work@gmail.com","password":"secret-123"}`)

	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "EMAIL_EXISTS")
	requireNoUserWithEmail(t, client, "alicealias+work@gmail.com")
}

func TestCreatePendingOAuthAccountCorpMemberRejectedWhenIdentityOwnedByDisabledUser(t *testing.T) {
	createCalls := 0
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		userRepoOptions: oauthPendingFlowUserRepoOptions{createCalls: &createCalls},
	})
	ctx := context.Background()

	disabledOwner, err := client.User.Create().
		SetEmail("disabled-owner@example.org").
		SetUsername("disabled-owner").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusDisabled).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.AuthIdentity.Create().
		SetUserID(disabledOwner.ID).
		SetProviderType("dingtalk").
		SetProviderKey("dingtalk").
		SetProviderSubject("dingtalk-disabled-union").
		Save(ctx)
	require.NoError(t, err)

	session := createPendingOAuthTestSession(t, client, "dingtalk", "dingtalk-disabled-union", corpMemberClaims("dingtalk-disabled-union"))

	recorder := postPendingCreateAccount(t, handler, session, `{"email":"fresh-for-disabled@example.org","password":"secret-123"}`)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "USER_NOT_ACTIVE")
	require.Zero(t, createCalls, "身份预检必须在建号之前拒绝")
	requireNoUserWithEmail(t, client, "fresh-for-disabled@example.org")
}
