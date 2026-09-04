package server

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jiguangliandong/superapp-embed-demo/partner-backend-go/internal/config"
	embedsdk "github.com/jiguangliandong/superapp-embed-go-sdk"
)

const testClientID = "embcli_test_partner"
const testKeyID = "partner-test-key"

type superappProbe struct {
	mu                sync.Mutex
	expectedChallenge string
	tokenCalls        int
	refreshCalls      int
	lastTokenForm     url.Values
	rejectUserInfo    bool
	publicKey         *ecdsa.PublicKey
	tokenEndpoint     string
}

type testEnvironment struct {
	partner     *httptest.Server
	superapp    *httptest.Server
	probe       *superappProbe
	application *Server
}

func TestSSOCompletesThroughGoSDK(t *testing.T) {
	environment := newTestEnvironment(t)
	client := newBrowserClient(t)

	bootstrap := beginTransaction(t, client, environment.partner.URL, []string{"profile.name"})
	if bootstrap.ClientID != testClientID {
		t.Fatalf("client_id = %q, want %q", bootstrap.ClientID, testClientID)
	}
	if strings.Join(bootstrap.Scopes, " ") != "auth_base profile.name" {
		t.Fatalf("scopes = %v, want auth_base and profile.name", bootstrap.Scopes)
	}
	environment.probe.mu.Lock()
	environment.probe.expectedChallenge = bootstrap.CodeChallenge
	environment.probe.mu.Unlock()

	partnerURL, _ := url.Parse(environment.partner.URL)
	beforeCookies := client.Jar.Cookies(partnerURL)
	if len(beforeCookies) != 1 {
		t.Fatalf("bootstrap cookies = %d, want 1", len(beforeCookies))
	}
	beforeSessionID := beforeCookies[0].Value

	status, body := postJSON(t, client, environment.partner.URL+"/api/sso/complete", map[string]any{
		"transaction_id": bootstrap.TransactionID,
		"code":           "embcode_valid",
		"state":          bootstrap.State,
	})
	if status != http.StatusOK {
		t.Fatalf("complete status = %d, body = %s", status, body)
	}
	var result publicAuthentication
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode complete response: %v", err)
	}
	if !result.Authenticated || result.User.OpenID != "open_test_123" {
		t.Fatalf("unexpected authentication response: %+v", result)
	}
	if result.User.DisplayName == nil || *result.User.DisplayName != "Demo Customer" {
		t.Fatalf("display_name = %v, want Demo Customer", result.User.DisplayName)
	}
	if bytes.Contains(body, []byte("access-token-secret")) ||
		bytes.Contains(body, []byte("refresh-token-secret")) {
		t.Fatal("Partner token leaked into browser response")
	}

	afterCookies := client.Jar.Cookies(partnerURL)
	if len(afterCookies) != 1 || afterCookies[0].Value == beforeSessionID {
		t.Fatal("Partner session ID was not rotated after authentication")
	}

	environment.probe.mu.Lock()
	defer environment.probe.mu.Unlock()
	if environment.probe.tokenCalls != 1 {
		t.Fatalf("token endpoint calls = %d, want 1", environment.probe.tokenCalls)
	}
	if environment.probe.lastTokenForm.Get("code_verifier") == "" {
		t.Fatal("Go SDK did not send the server-side PKCE verifier")
	}
	if environment.probe.lastTokenForm.Get("client_assertion") == "" {
		t.Fatal("Go SDK did not send private_key_jwt client authentication")
	}
}

func TestAuthenticatedPartnerSessionIsRestoredWithoutAnotherAuthorizationCode(t *testing.T) {
	environment := newTestEnvironment(t)
	client := newBrowserClient(t)
	completeLogin(t, environment, client)

	status, body := getJSON(t, client, environment.partner.URL+"/api/sso/session")
	if status != http.StatusOK {
		t.Fatalf("session status = %d, body = %s", status, body)
	}
	var result publicAuthentication
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if !result.Authenticated || result.User.OpenID != "open_test_123" ||
		!result.SessionExpires.After(time.Now()) {
		t.Fatalf("unexpected restored session: %+v", result)
	}
	environment.probe.mu.Lock()
	defer environment.probe.mu.Unlock()
	if environment.probe.tokenCalls != 1 || environment.probe.refreshCalls != 0 {
		t.Fatalf("token calls = %d, refresh calls = %d", environment.probe.tokenCalls, environment.probe.refreshCalls)
	}
}

func TestExpiredAccessTokenIsRefreshedWhileRestoringPartnerSession(t *testing.T) {
	environment := newTestEnvironment(t)
	client := newBrowserClient(t)
	completeLogin(t, environment, client)

	partnerURL, _ := url.Parse(environment.partner.URL)
	sessionID := client.Jar.Cookies(partnerURL)[0].Value
	if err := environment.application.sessions.ReplaceAuthentication(sessionID, storedAuthentication{
		ClientID:         testClientID,
		OpenID:           "open_test_123",
		AccessToken:      "expired-access-token",
		AccessExpiresAt:  time.Now().Add(-time.Minute),
		RefreshToken:     "refresh-token-secret",
		RefreshExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("expire access token: %v", err)
	}

	status, body := getJSON(t, client, environment.partner.URL+"/api/sso/session")
	if status != http.StatusOK {
		t.Fatalf("session status = %d, body = %s", status, body)
	}
	environment.probe.mu.Lock()
	defer environment.probe.mu.Unlock()
	if environment.probe.refreshCalls != 1 {
		t.Fatalf("refresh calls = %d, want 1", environment.probe.refreshCalls)
	}
}

func TestRevokedSuperappAuthorizationInvalidatesPartnerSession(t *testing.T) {
	environment := newTestEnvironment(t)
	client := newBrowserClient(t)
	completeLogin(t, environment, client)

	environment.probe.mu.Lock()
	environment.probe.rejectUserInfo = true
	environment.probe.mu.Unlock()
	for range 2 {
		status, body := getJSON(t, client, environment.partner.URL+"/api/sso/session")
		if status != http.StatusUnauthorized {
			t.Fatalf("session status = %d, body = %s", status, body)
		}
		var output errorOutput
		if err := json.Unmarshal(body, &output); err != nil || output.Code != "partner_session_missing" {
			t.Fatalf("unexpected missing session response: %s", body)
		}
	}
}

func TestWrongStateConsumesTransaction(t *testing.T) {
	environment := newTestEnvironment(t)
	client := newBrowserClient(t)
	bootstrap := beginTransaction(t, client, environment.partner.URL, []string{"auth_base"})

	for _, state := range []string{"wrong-state-value", bootstrap.State} {
		status, body := postJSON(t, client, environment.partner.URL+"/api/sso/complete", map[string]any{
			"transaction_id": bootstrap.TransactionID,
			"code":           "embcode_valid",
			"state":          state,
		})
		assertLoginFailure(t, status, body, http.StatusBadRequest)
	}

	environment.probe.mu.Lock()
	defer environment.probe.mu.Unlock()
	if environment.probe.tokenCalls != 0 {
		t.Fatalf("token endpoint calls = %d, want 0", environment.probe.tokenCalls)
	}
}

func TestTransactionIsBoundToBrowserSession(t *testing.T) {
	environment := newTestEnvironment(t)
	owner := newBrowserClient(t)
	otherBrowser := newBrowserClient(t)
	bootstrap := beginTransaction(t, owner, environment.partner.URL, []string{"auth_base"})

	for _, client := range []*http.Client{otherBrowser, owner} {
		status, body := postJSON(t, client, environment.partner.URL+"/api/sso/complete", map[string]any{
			"transaction_id": bootstrap.TransactionID,
			"code":           "embcode_valid",
			"state":          bootstrap.State,
		})
		assertLoginFailure(t, status, body, http.StatusBadRequest)
	}

	environment.probe.mu.Lock()
	defer environment.probe.mu.Unlock()
	if environment.probe.tokenCalls != 0 {
		t.Fatalf("token endpoint calls = %d, want 0", environment.probe.tokenCalls)
	}
}

func TestAuthorizationCodeFailureIsGenericAndNotReplayable(t *testing.T) {
	environment := newTestEnvironment(t)
	client := newBrowserClient(t)
	bootstrap := beginTransaction(t, client, environment.partner.URL, []string{"auth_base"})

	for range 2 {
		status, body := postJSON(t, client, environment.partner.URL+"/api/sso/complete", map[string]any{
			"transaction_id": bootstrap.TransactionID,
			"code":           "embcode_invalid",
			"state":          bootstrap.State,
		})
		assertLoginFailure(t, status, body, http.StatusBadRequest)
	}

	environment.probe.mu.Lock()
	defer environment.probe.mu.Unlock()
	if environment.probe.tokenCalls != 1 {
		t.Fatalf("token endpoint calls = %d, want 1", environment.probe.tokenCalls)
	}
}

func TestBootstrapRejectsInvalidScopes(t *testing.T) {
	environment := newTestEnvironment(t)
	client := newBrowserClient(t)
	tests := [][]string{
		{},
		{"auth_base", "auth_base"},
		{"profile.avatar"},
	}
	for _, scopes := range tests {
		status, body := postJSON(t, client, environment.partner.URL+"/api/sso/bootstrap", map[string]any{
			"scopes": scopes,
		})
		if status != http.StatusBadRequest {
			t.Fatalf("scopes %v status = %d, body = %s", scopes, status, body)
		}
		var output errorOutput
		if err := json.Unmarshal(body, &output); err != nil {
			t.Fatalf("decode invalid scope response: %v", err)
		}
		if output.Code != "invalid_scopes" {
			t.Fatalf("scopes %v code = %q, want invalid_scopes", scopes, output.Code)
		}
	}
}

func newTestEnvironment(t *testing.T) testEnvironment {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate test Partner key: %v", err)
	}
	probe := &superappProbe{publicKey: &privateKey.PublicKey}
	superapp := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/user/v1/open/embed/oauth/token":
			probe.handleToken(t, response, request)
		case "/api/user/v1/open/embed/userinfo":
			probe.handleUserInfo(t, response, request)
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(superapp.Close)
	probe.tokenEndpoint = superapp.URL + "/api/user/v1/open/embed/oauth/token"

	cfg := config.Config{
		SuperappBaseURL: superapp.URL,
		ClientID:        testClientID,
		KeyID:           testKeyID,
		AllowedScopes: map[string]struct{}{
			"auth_base":    {},
			"profile.name": {},
		},
		TransactionTTL: 5 * time.Minute,
		SessionTTL:     12 * time.Hour,
	}
	application := newWithDependencies(cfg, privateKey, superapp.Client(), nil)
	partner := httptest.NewServer(application.Handler())
	t.Cleanup(partner.Close)
	return testEnvironment{
		partner: partner, superapp: superapp, probe: probe, application: application,
	}
}

func (probe *superappProbe) handleToken(t *testing.T, response http.ResponseWriter, request *http.Request) {
	t.Helper()
	if err := request.ParseForm(); err != nil {
		http.Error(response, "invalid form", http.StatusBadRequest)
		return
	}
	probe.mu.Lock()
	probe.tokenCalls++
	probe.lastTokenForm = make(url.Values, len(request.PostForm))
	for key, values := range request.PostForm {
		probe.lastTokenForm[key] = append([]string(nil), values...)
	}
	expectedChallenge := probe.expectedChallenge
	grantType := request.PostForm.Get("grant_type")
	if grantType == "refresh_token" {
		probe.refreshCalls++
	}
	probe.mu.Unlock()

	if grantType == "refresh_token" {
		if request.PostForm.Get("refresh_token") != "refresh-token-secret" {
			writeTestJSON(response, http.StatusBadRequest, map[string]any{
				"error": "invalid_grant", "error_description": "refresh token is invalid",
			})
			return
		}
		writeTestJSON(response, http.StatusOK, embedsdk.Token{
			TokenType:             "Bearer",
			AccessToken:           "refreshed-access-token",
			ExpiresIn:             300,
			RefreshToken:          "refreshed-refresh-token",
			RefreshTokenExpiresIn: 3600,
			Scope:                 "auth_base profile.name",
			OpenID:                "open_test_123",
			ConsentVersion:        1,
		})
		return
	}

	if request.PostForm.Get("code") == "embcode_invalid" {
		writeTestJSON(response, http.StatusBadRequest, map[string]any{
			"error": "invalid_grant", "error_description": "the authorization grant is invalid",
		})
		return
	}
	if request.PostForm.Get("grant_type") != "authorization_code" ||
		request.PostForm.Get("client_id") != testClientID ||
		request.PostForm.Get("client_assertion_type") !=
			"urn:ietf:params:oauth:client-assertion-type:jwt-bearer" {
		writeTestJSON(response, http.StatusBadRequest, map[string]any{
			"error": "invalid_request", "error_description": "invalid token request",
		})
		return
	}
	verifier := request.PostForm.Get("code_verifier")
	digest := sha256.Sum256([]byte(verifier))
	if base64.RawURLEncoding.EncodeToString(digest[:]) != expectedChallenge {
		t.Error("code_verifier does not match SDK-generated code_challenge")
		writeTestJSON(response, http.StatusBadRequest, map[string]any{
			"error": "invalid_grant", "error_description": "PKCE verification failed",
		})
		return
	}
	if err := verifyClientAssertion(
		request.PostForm.Get("client_assertion"), probe.publicKey, probe.tokenEndpoint,
	); err != nil {
		t.Errorf("verify Go SDK client assertion: %v", err)
		writeTestJSON(response, http.StatusUnauthorized, map[string]any{
			"error": "invalid_client", "error_description": "client assertion is invalid",
		})
		return
	}

	writeTestJSON(response, http.StatusOK, embedsdk.Token{
		TokenType:             "Bearer",
		AccessToken:           "access-token-secret",
		ExpiresIn:             300,
		RefreshToken:          "refresh-token-secret",
		RefreshTokenExpiresIn: 3600,
		Scope:                 "auth_base profile.name",
		OpenID:                "open_test_123",
		ConsentVersion:        1,
	})
}

func (probe *superappProbe) handleUserInfo(t *testing.T, response http.ResponseWriter, request *http.Request) {
	t.Helper()
	probe.mu.Lock()
	reject := probe.rejectUserInfo
	probe.mu.Unlock()
	authorization := request.Header.Get("Authorization")
	if reject || (authorization != "Bearer access-token-secret" &&
		authorization != "Bearer refreshed-access-token") {
		writeTestJSON(response, http.StatusUnauthorized, map[string]any{
			"error": "invalid_token", "error_description": "access token is invalid",
		})
		return
	}
	displayName := "Demo Customer"
	email := "demo@example.test"
	writeTestJSON(response, http.StatusOK, embedsdk.UserInfo{
		OpenID:       "open_test_123",
		DisplayName:  &displayName,
		ContactEmail: &email,
	})
}

func completeLogin(t *testing.T, environment testEnvironment, client *http.Client) {
	t.Helper()
	bootstrap := beginTransaction(t, client, environment.partner.URL, []string{"profile.name"})
	environment.probe.mu.Lock()
	environment.probe.expectedChallenge = bootstrap.CodeChallenge
	environment.probe.mu.Unlock()
	status, body := postJSON(t, client, environment.partner.URL+"/api/sso/complete", map[string]any{
		"transaction_id": bootstrap.TransactionID,
		"code":           "embcode_valid",
		"state":          bootstrap.State,
	})
	if status != http.StatusOK {
		t.Fatalf("complete status = %d, body = %s", status, body)
	}
}

func verifyClientAssertion(assertion string, publicKey *ecdsa.PublicKey, audience string) error {
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		return errors.New("assertion must contain three JWT segments")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("decode header: %w", err)
	}
	var header map[string]any
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return fmt.Errorf("decode header JSON: %w", err)
	}
	if header["alg"] != "ES256" || header["kid"] != testKeyID || header["typ"] != "JWT" {
		return fmt.Errorf("unexpected assertion header: %v", header)
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return fmt.Errorf("decode claims JSON: %w", err)
	}
	jti, jtiOK := claims["jti"].(string)
	iat, iatOK := claims["iat"].(float64)
	exp, expOK := claims["exp"].(float64)
	if claims["iss"] != testClientID || claims["sub"] != testClientID ||
		claims["aud"] != audience || !jtiOK || jti == "" ||
		!iatOK || !expOK || exp-iat != 120 {
		return fmt.Errorf("unexpected assertion claims: %v", claims)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) != 64 {
		return errors.New("assertion must contain a 64-byte ES256 signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	if !ecdsa.Verify(publicKey, digest[:], r, s) {
		return errors.New("assertion signature is invalid")
	}
	return nil
}

func beginTransaction(
	t *testing.T,
	client *http.Client,
	partnerURL string,
	scopes []string,
) embedsdk.Bootstrap {
	t.Helper()
	status, body := postJSON(t, client, partnerURL+"/api/sso/bootstrap", map[string]any{
		"scopes": scopes,
	})
	if status != http.StatusCreated {
		t.Fatalf("bootstrap status = %d, body = %s", status, body)
	}
	var bootstrap embedsdk.Bootstrap
	if err := json.Unmarshal(body, &bootstrap); err != nil {
		t.Fatalf("decode bootstrap: %v", err)
	}
	return bootstrap
}

func postJSON(t *testing.T, client *http.Client, endpoint string, input any) (int, []byte) {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return response.StatusCode, responseBody
}

func getJSON(t *testing.T, client *http.Client, endpoint string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return response.StatusCode, body
}

func newBrowserClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	return &http.Client{Jar: jar, Timeout: 5 * time.Second}
}

func assertLoginFailure(t *testing.T, status int, body []byte, expectedStatus int) {
	t.Helper()
	if status != expectedStatus {
		t.Fatalf("status = %d, want %d; body = %s", status, expectedStatus, body)
	}
	var output errorOutput
	if err := json.Unmarshal(body, &output); err != nil {
		t.Fatalf("decode login failure: %v", err)
	}
	if output.Code != "partner_login_failed" ||
		output.Message != "Partner login could not be completed" {
		t.Fatalf("unexpected login failure: %+v", output)
	}
}

func writeTestJSON(response http.ResponseWriter, status int, body any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(body)
}
