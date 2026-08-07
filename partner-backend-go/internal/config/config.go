package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddress       string
	SuperappBaseURL     string
	ClientID            string
	KeyID               string
	PrivateKeyPath      string
	H5DistDir           string
	AllowedScopes       map[string]struct{}
	TransactionTTL      time.Duration
	SessionTTL          time.Duration
	SessionCookieSecure bool
}

func Load() (Config, error) {
	baseURL, err := required("SUPERAPP_BASE_URL")
	if err != nil {
		return Config{}, err
	}
	baseURL = strings.TrimRight(baseURL, "/")
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil || parsedBaseURL.Scheme == "" || parsedBaseURL.Host == "" ||
		(parsedBaseURL.Scheme != "http" && parsedBaseURL.Scheme != "https") ||
		(parsedBaseURL.Path != "" && parsedBaseURL.Path != "/") ||
		parsedBaseURL.RawQuery != "" || parsedBaseURL.Fragment != "" {
		return Config{}, fmt.Errorf("SUPERAPP_BASE_URL must be an absolute HTTP(S) origin")
	}

	clientID, err := required("SUPERAPP_CLIENT_ID")
	if err != nil {
		return Config{}, err
	}
	keyID, err := required("SUPERAPP_KEY_ID")
	if err != nil {
		return Config{}, err
	}
	privateKeyPath, err := required("SUPERAPP_PRIVATE_KEY_PATH")
	if err != nil {
		return Config{}, err
	}
	privateKeyPath, err = resolvePath(privateKeyPath)
	if err != nil {
		return Config{}, err
	}
	h5DistDir, err := resolveH5DistDir(strings.TrimSpace(os.Getenv("PARTNER_H5_DIST_DIR")))
	if err != nil {
		return Config{}, err
	}

	allowedScopesValue, err := required("PARTNER_ALLOWED_SCOPES")
	if err != nil {
		return Config{}, err
	}
	allowedScopes := make(map[string]struct{})
	for _, scope := range strings.Split(allowedScopesValue, ",") {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		allowedScopes[scope] = struct{}{}
	}
	if _, ok := allowedScopes["auth_base"]; !ok {
		return Config{}, fmt.Errorf("PARTNER_ALLOWED_SCOPES must include auth_base")
	}

	port, err := positiveInt("PORT", 3000)
	if err != nil {
		return Config{}, err
	}
	transactionTTLSeconds, err := positiveInt("SSO_TRANSACTION_TTL_SECONDS", 300)
	if err != nil {
		return Config{}, err
	}
	sessionTTLSeconds, err := positiveInt("PARTNER_SESSION_TTL_SECONDS", 12*60*60)
	if err != nil {
		return Config{}, err
	}
	cookieSecure, err := boolValue("SESSION_COOKIE_SECURE", false)
	if err != nil {
		return Config{}, err
	}

	return Config{
		ListenAddress:       fmt.Sprintf(":%d", port),
		SuperappBaseURL:     baseURL,
		ClientID:            clientID,
		KeyID:               keyID,
		PrivateKeyPath:      privateKeyPath,
		H5DistDir:           h5DistDir,
		AllowedScopes:       allowedScopes,
		TransactionTTL:      time.Duration(transactionTTLSeconds) * time.Second,
		SessionTTL:          time.Duration(sessionTTLSeconds) * time.Second,
		SessionCookieSecure: cookieSecure,
	}, nil
}

func resolveH5DistDir(value string) (string, error) {
	candidates := []string{value}
	if value == "" {
		candidates = []string{"../partner-h5-js/dist", "partner-h5-js/dist"}
	} else if !filepath.IsAbs(value) {
		candidates = append(candidates, filepath.Join("..", value))
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if info, err := os.Stat(absolute); err == nil && info.IsDir() {
			return absolute, nil
		}
	}
	absolute, err := filepath.Abs(candidates[0])
	if err != nil {
		return "", fmt.Errorf("resolve PARTNER_H5_DIST_DIR: %w", err)
	}
	return absolute, nil
}

func required(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func positiveInt(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func boolValue(name string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return value, nil
}

func resolvePath(value string) (string, error) {
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}

	candidates := []string{value, filepath.Join("..", value)}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(absolute); err == nil {
			return absolute, nil
		}
	}

	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve SUPERAPP_PRIVATE_KEY_PATH: %w", err)
	}
	return absolute, nil
}
