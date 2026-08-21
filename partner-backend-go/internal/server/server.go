package server

import (
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jiguangliandong/superapp-embed-demo/partner-backend-go/internal/config"
	"github.com/jiguangliandong/superapp-embed-demo/partner-backend-go/internal/session"
	"github.com/jiguangliandong/superapp-embed-demo/partner-backend-go/internal/sso"
	"github.com/jiguangliandong/superapp-embed-demo/partner-backend-go/internal/superapp"
)

const maxJSONBodyBytes = 64 * 1024

type Server struct {
	config       config.Config
	transactions *sso.Manager
	client       *superapp.Client
	sessions     *session.Manager
	logger       *log.Logger
	handler      http.Handler
	appHTML      []byte
	privacyHTML  []byte
	assetHandler http.Handler
	refreshMu    sync.Mutex
}

type bootstrapInput struct {
	Scopes []string `json:"scopes"`
}

type completeInput struct {
	TransactionID string `json:"transaction_id"`
	Code          string `json:"code"`
	State         string `json:"state"`
}

type publicAuthentication struct {
	Authenticated  bool              `json:"authenticated"`
	User           superapp.UserInfo `json:"user"`
	Scope          []string          `json:"scope"`
	ConsentVersion int               `json:"consent_version"`
	SessionExpires time.Time         `json:"session_expires_at"`
}

type storedAuthentication struct {
	AuthenticatedAt  time.Time
	ClientID         string
	OpenID           string
	User             superapp.UserInfo
	Scope            []string
	ConsentVersion   int
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

type errorOutput struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func New(cfg config.Config, logger *log.Logger) (*Server, error) {
	privateKey, err := loadPrivateKey(cfg.PrivateKeyPath)
	if err != nil {
		return nil, err
	}
	result := newWithDependencies(cfg, privateKey, nil, logger)
	if err := result.configureH5(); err != nil {
		return nil, err
	}
	return result, nil
}

func newWithDependencies(
	cfg config.Config,
	privateKey crypto.Signer,
	httpClient *http.Client,
	logger *log.Logger,
) *Server {
	if logger == nil {
		logger = log.Default()
	}
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	result := &Server{
		config: cfg,
		transactions: sso.NewManager(
			cfg.ClientID,
			sso.NewMemoryTransactionStore(),
			cfg.TransactionTTL,
		),
		client: &superapp.Client{
			BaseURL:    cfg.SuperappBaseURL,
			ClientID:   cfg.ClientID,
			KeyID:      cfg.KeyID,
			PrivateKey: privateKey,
			HTTPClient: httpClient,
		},
		sessions: session.NewManager(cfg.SessionTTL, cfg.SessionCookieSecure),
		logger:   logger,
	}
	result.handler = result.routes()
	return result
}

func (server *Server) Handler() http.Handler {
	return server.handler
}

func (server *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", server.home)
	mux.HandleFunc("GET /app", server.app)
	mux.HandleFunc("GET /app/profile", server.app)
	mux.HandleFunc("GET /privacy", server.privacy)
	mux.HandleFunc("GET /assets/", server.asset)
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("POST /api/sso/bootstrap", server.bootstrap)
	mux.HandleFunc("POST /api/sso/complete", server.complete)
	mux.HandleFunc("GET /api/sso/session", server.currentSession)
	return securityHeaders(mux)
}

func (server *Server) configureH5() error {
	appHTML, err := os.ReadFile(filepath.Join(server.config.H5DistDir, "app.html"))
	if err != nil {
		return fmt.Errorf("read Partner H5 app.html: %w", err)
	}
	privacyHTML, err := os.ReadFile(filepath.Join(server.config.H5DistDir, "privacy.html"))
	if err != nil {
		return fmt.Errorf("read Partner H5 privacy.html: %w", err)
	}
	appHTML = []byte(strings.ReplaceAll(
		string(appHTML), "__SUPERAPP_CLIENT_ID__", html.EscapeString(server.config.ClientID),
	))
	server.appHTML = appHTML
	server.privacyHTML = privacyHTML
	server.assetHandler = http.StripPrefix(
		"/assets/",
		http.FileServer(http.Dir(filepath.Join(server.config.H5DistDir, "assets"))),
	)
	return nil
}

func (server *Server) home(response http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(response, request)
		return
	}
	http.Redirect(response, request, "/app", http.StatusTemporaryRedirect)
}

func (server *Server) app(response http.ResponseWriter, _ *http.Request) {
	if len(server.appHTML) == 0 {
		http.Error(response, "Partner H5 is not built", http.StatusServiceUnavailable)
		return
	}
	setPageHeaders(response)
	setNoStore(response)
	response.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = response.Write(server.appHTML)
}

func (server *Server) privacy(response http.ResponseWriter, _ *http.Request) {
	if len(server.privacyHTML) == 0 {
		http.Error(response, "Partner H5 is not built", http.StatusServiceUnavailable)
		return
	}
	setPageHeaders(response)
	response.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = response.Write(server.privacyHTML)
}

func (server *Server) asset(response http.ResponseWriter, request *http.Request) {
	if server.assetHandler == nil {
		http.NotFound(response, request)
		return
	}
	response.Header().Set("Cache-Control", "no-cache")
	server.assetHandler.ServeHTTP(response, request)
}

func (server *Server) health(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "partner-backend-go",
	})
}

func (server *Server) bootstrap(response http.ResponseWriter, request *http.Request) {
	setNoStore(response)
	browserSessionID, err := server.sessions.GetOrCreate(response, request)
	if err != nil {
		server.loginFailure(response, http.StatusInternalServerError)
		return
	}

	var input bootstrapInput
	if err := decodeJSON(response, request, &input); err != nil {
		writeJSON(response, http.StatusBadRequest, errorOutput{
			Code: "invalid_request", Message: "Request body must be valid JSON",
		})
		return
	}
	scopes, err := server.normalizeScopes(input.Scopes)
	if err != nil {
		writeJSON(response, http.StatusBadRequest, errorOutput{
			Code: "invalid_scopes", Message: err.Error(),
		})
		return
	}

	bootstrap, err := server.transactions.Begin(request.Context(), browserSessionID, scopes)
	if err != nil {
		server.logger.Printf("begin Partner SSO transaction failed: %T", err)
		server.loginFailure(response, http.StatusInternalServerError)
		return
	}
	writeJSON(response, http.StatusCreated, bootstrap)
}

func (server *Server) complete(response http.ResponseWriter, request *http.Request) {
	setNoStore(response)
	browserSessionID, err := server.sessions.GetOrCreate(response, request)
	if err != nil {
		server.loginFailure(response, http.StatusInternalServerError)
		return
	}

	var input completeInput
	if err := decodeJSON(response, request, &input); err != nil ||
		!bounded(input.TransactionID, 256) ||
		!bounded(input.Code, 2048) ||
		!bounded(input.State, 512) {
		server.loginFailure(response, http.StatusBadRequest)
		return
	}

	transaction, err := server.transactions.Complete(
		request.Context(), input.TransactionID, input.State, browserSessionID,
	)
	if err != nil {
		server.loginFailure(response, http.StatusBadRequest)
		return
	}

	token, err := server.client.ExchangeAuthorizationCode(
		request.Context(), input.Code, transaction.CodeVerifier,
	)
	if err != nil {
		server.loginFailure(response, protocolStatus(err))
		return
	}
	scopes, err := validateToken(token, transaction.Scopes)
	if err != nil {
		server.logger.Printf("invalid Superapp token response: %T", err)
		server.loginFailure(response, http.StatusBadGateway)
		return
	}

	userinfo, err := server.client.UserInfo(request.Context(), token.AccessToken)
	if err != nil {
		server.loginFailure(response, protocolStatus(err))
		return
	}
	if userinfo.OpenID == "" || userinfo.OpenID != token.OpenID {
		server.logger.Printf("Superapp Token and UserInfo subject mismatch")
		server.loginFailure(response, http.StatusBadGateway)
		return
	}

	authentication := storedAuthentication{
		AuthenticatedAt:  time.Now().UTC(),
		ClientID:         server.config.ClientID,
		OpenID:           token.OpenID,
		User:             userinfo,
		Scope:            scopes,
		ConsentVersion:   token.ConsentVersion,
		AccessToken:      token.AccessToken,
		AccessExpiresAt:  time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second),
		RefreshToken:     token.RefreshToken,
		RefreshExpiresAt: time.Now().UTC().Add(time.Duration(token.RefreshTokenExpiresIn) * time.Second),
	}
	if _, err := server.sessions.Rotate(response, browserSessionID, authentication); err != nil {
		server.logger.Printf("rotate authenticated Partner session failed: %T", err)
		server.loginFailure(response, http.StatusInternalServerError)
		return
	}

	writeJSON(response, http.StatusOK, publicAuthentication{
		Authenticated:  true,
		User:           userinfo,
		Scope:          scopes,
		ConsentVersion: token.ConsentVersion,
		SessionExpires: time.Now().UTC().Add(server.config.SessionTTL),
	})
}

func (server *Server) currentSession(response http.ResponseWriter, request *http.Request) {
	setNoStore(response)
	// Refresh tokens are rotating credentials. This demo serializes restoration so
	// two concurrent WebView requests cannot replay the same refresh token.
	server.refreshMu.Lock()
	defer server.refreshMu.Unlock()

	sessionID, value, ok := server.sessions.GetAuthenticated(response, request)
	if !ok {
		server.sessionMissing(response)
		return
	}
	authentication, ok := value.(storedAuthentication)
	if !ok || authentication.ClientID != server.config.ClientID {
		server.sessions.Delete(response, sessionID)
		server.sessionMissing(response)
		return
	}

	refreshed, err := server.restoreAuthentication(request, authentication)
	if err != nil {
		if credentialsRejected(err) {
			server.sessions.Delete(response, sessionID)
			server.sessionMissing(response)
			return
		}
		server.logger.Printf("restore Partner session failed: %T", err)
		writeJSON(response, http.StatusBadGateway, errorOutput{
			Code: "partner_session_unavailable", Message: "Partner session could not be verified",
		})
		return
	}
	if err := server.sessions.ReplaceAuthentication(sessionID, refreshed); err != nil {
		server.sessionMissing(response)
		return
	}
	writeJSON(response, http.StatusOK, publicAuthentication{
		Authenticated:  true,
		User:           refreshed.User,
		Scope:          refreshed.Scope,
		ConsentVersion: refreshed.ConsentVersion,
		SessionExpires: time.Now().UTC().Add(server.config.SessionTTL),
	})
}

func (server *Server) restoreAuthentication(
	request *http.Request,
	authentication storedAuthentication,
) (storedAuthentication, error) {
	now := time.Now().UTC()
	if !authentication.AccessExpiresAt.After(now.Add(30 * time.Second)) {
		if !authentication.RefreshExpiresAt.After(now.Add(30 * time.Second)) {
			return storedAuthentication{}, errPartnerSessionExpired
		}
		token, err := server.client.Refresh(request.Context(), authentication.RefreshToken)
		if err != nil {
			return storedAuthentication{}, err
		}
		scopes, err := validateToken(token, authentication.Scope)
		if err != nil || token.OpenID != authentication.OpenID {
			return storedAuthentication{}, errPartnerSessionExpired
		}
		authentication.AccessToken = token.AccessToken
		authentication.AccessExpiresAt = now.Add(time.Duration(token.ExpiresIn) * time.Second)
		authentication.RefreshToken = token.RefreshToken
		authentication.RefreshExpiresAt = now.Add(time.Duration(token.RefreshTokenExpiresIn) * time.Second)
		authentication.Scope = scopes
		authentication.ConsentVersion = token.ConsentVersion
	}

	userinfo, err := server.client.UserInfo(request.Context(), authentication.AccessToken)
	if err != nil {
		return storedAuthentication{}, err
	}
	if userinfo.OpenID == "" || userinfo.OpenID != authentication.OpenID {
		return storedAuthentication{}, errPartnerSessionExpired
	}
	authentication.User = userinfo
	return authentication, nil
}

func (server *Server) sessionMissing(response http.ResponseWriter) {
	writeJSON(response, http.StatusUnauthorized, errorOutput{
		Code: "partner_session_missing", Message: "Partner login is required",
	})
}

var errPartnerSessionExpired = errors.New("partner session credentials expired")

func credentialsRejected(err error) bool {
	if errors.Is(err, errPartnerSessionExpired) {
		return true
	}
	var protocolError *superapp.ProtocolError
	return errors.As(err, &protocolError) &&
		(protocolError.Code == "invalid_grant" || protocolError.Code == "invalid_token")
}

func (server *Server) normalizeScopes(requested []string) ([]string, error) {
	if len(requested) == 0 {
		return nil, errors.New("scopes must be a non-empty array")
	}
	seen := make(map[string]struct{}, len(requested)+1)
	result := make([]string, 0, len(requested)+1)
	for _, scope := range requested {
		if scope == "" {
			return nil, errors.New("each scope must be a non-empty string")
		}
		if _, ok := seen[scope]; ok {
			return nil, errors.New("scopes must not contain duplicates")
		}
		if _, ok := server.config.AllowedScopes[scope]; !ok {
			return nil, fmt.Errorf("scope is not allowed: %s", scope)
		}
		seen[scope] = struct{}{}
		result = append(result, scope)
	}
	if _, ok := seen["auth_base"]; !ok {
		result = append([]string{"auth_base"}, result...)
	}
	return result, nil
}

func (server *Server) loginFailure(response http.ResponseWriter, status int) {
	setNoStore(response)
	writeJSON(response, status, errorOutput{
		Code: "partner_login_failed", Message: "Partner login could not be completed",
	})
}

func validateToken(token superapp.Token, expectedScopes []string) ([]string, error) {
	scopes := strings.Fields(token.Scope)
	if token.TokenType != "Bearer" || token.AccessToken == "" || token.ExpiresIn <= 0 ||
		token.RefreshToken == "" || token.RefreshTokenExpiresIn <= 0 ||
		token.OpenID == "" || token.ConsentVersion <= 0 ||
		!slices.Contains(scopes, "auth_base") || len(scopes) != len(expectedScopes) {
		return nil, errors.New("required token response field is missing")
	}
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		if _, ok := seen[scope]; ok {
			return nil, errors.New("token response contains duplicate scope")
		}
		seen[scope] = struct{}{}
	}
	for _, expected := range expectedScopes {
		if _, ok := seen[expected]; !ok {
			return nil, errors.New("token response scope does not match SSO transaction")
		}
	}
	return scopes, nil
}

func protocolStatus(err error) int {
	var protocolError *superapp.ProtocolError
	if errors.As(err, &protocolError) &&
		(protocolError.Code == "invalid_grant" || protocolError.Code == "invalid_scope") {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

func decodeJSON(response http.ResponseWriter, request *http.Request, target any) error {
	if !strings.HasPrefix(strings.ToLower(request.Header.Get("Content-Type")), "application/json") {
		return errors.New("content type must be application/json")
	}
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, maxJSONBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain exactly one JSON object")
	}
	return nil
}

func bounded(value string, maximum int) bool {
	return value != "" && len(value) <= maximum
}

func setNoStore(response http.ResponseWriter) {
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Pragma", "no-cache")
}

func setPageHeaders(response http.ResponseWriter) {
	response.Header().Set("Content-Security-Policy", strings.Join([]string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self'",
		"img-src 'self' https: data:",
		"connect-src 'self'",
		"object-src 'none'",
		"base-uri 'none'",
		"frame-ancestors 'none'",
	}, "; "))
}

func writeJSON(response http.ResponseWriter, status int, body any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(body)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(response, request)
	})
}
