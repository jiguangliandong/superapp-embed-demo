package superapp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	clientAssertionType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
	maximumResponseSize = 1 << 20
)

type Client struct {
	BaseURL    string
	ClientID   string
	KeyID      string
	PrivateKey crypto.Signer
	HTTPClient *http.Client
	Now        func() time.Time
}

type Token struct {
	TokenType             string `json:"token_type"`
	AccessToken           string `json:"access_token"`
	ExpiresIn             int64  `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
	Scope                 string `json:"scope"`
	OpenID                string `json:"open_id"`
	ConsentVersion        int    `json:"consent_version"`
}

type UserInfo struct {
	OpenID       string  `json:"open_id"`
	DisplayName  *string `json:"display_name,omitempty"`
	AvatarURL    *string `json:"avatar_url,omitempty"`
	ContactEmail *string `json:"contact_email,omitempty"`
	ContactPhone *string `json:"contact_phone,omitempty"`
	KYCStatus    *string `json:"kyc_status,omitempty"`
}

type ProtocolError struct {
	Code        string `json:"error"`
	Description string `json:"error_description"`
	Status      int    `json:"-"`
}

func (err *ProtocolError) Error() string {
	if err.Description == "" {
		return err.Code
	}
	return err.Code + ": " + err.Description
}

func (client *Client) ExchangeAuthorizationCode(
	ctx context.Context,
	code string,
	codeVerifier string,
) (Token, error) {
	if code == "" || codeVerifier == "" {
		return Token{}, errors.New("authorization code and PKCE verifier are required")
	}
	return client.token(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {codeVerifier},
	})
}

func (client *Client) Refresh(ctx context.Context, refreshToken string) (Token, error) {
	if refreshToken == "" {
		return Token{}, errors.New("refresh token is required")
	}
	return client.token(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
}

func (client *Client) Revoke(ctx context.Context, token string) error {
	if token == "" {
		return errors.New("token is required")
	}
	endpoint := strings.TrimRight(client.BaseURL, "/") + "/api/embed/v1/oauth/revoke"
	values := url.Values{"token": {token}}
	if err := client.authenticateForm(values, endpoint); err != nil {
		return err
	}
	response, err := client.doForm(ctx, endpoint, values)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return decodeProtocolError(response)
	}
	return nil
}

func (client *Client) UserInfo(ctx context.Context, accessToken string) (UserInfo, error) {
	if accessToken == "" {
		return UserInfo{}, errors.New("access token is required")
	}
	endpoint := strings.TrimRight(client.BaseURL, "/") + "/api/embed/v1/userinfo"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return UserInfo{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := client.httpClient().Do(request)
	if err != nil {
		return UserInfo{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return UserInfo{}, decodeProtocolError(response)
	}
	var result UserInfo
	if err := decodeJSON(response.Body, &result); err != nil {
		return UserInfo{}, err
	}
	return result, nil
}

func (client *Client) token(ctx context.Context, values url.Values) (Token, error) {
	endpoint := strings.TrimRight(client.BaseURL, "/") + "/api/embed/v1/oauth/token"
	if err := client.authenticateForm(values, endpoint); err != nil {
		return Token{}, err
	}
	response, err := client.doForm(ctx, endpoint, values)
	if err != nil {
		return Token{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Token{}, decodeProtocolError(response)
	}
	var result Token
	if err := decodeJSON(response.Body, &result); err != nil {
		return Token{}, err
	}
	return result, nil
}

func (client *Client) authenticateForm(values url.Values, audience string) error {
	assertion, err := client.clientAssertion(audience)
	if err != nil {
		return err
	}
	values.Set("client_id", client.ClientID)
	values.Set("client_assertion_type", clientAssertionType)
	values.Set("client_assertion", assertion)
	return nil
}

func (client *Client) clientAssertion(audience string) (string, error) {
	if client.PrivateKey == nil || client.ClientID == "" || client.KeyID == "" || audience == "" {
		return "", errors.New("client ID, key ID, private key, and audience are required")
	}
	publicKey, ok := client.PrivateKey.Public().(*ecdsa.PublicKey)
	if !ok || publicKey.Curve != elliptic.P256() {
		return "", errors.New("Partner private key must be an ES256 P-256 key")
	}
	now := client.now().UTC()
	jti, err := randomValue(24)
	if err != nil {
		return "", err
	}
	header, err := json.Marshal(map[string]string{
		"alg": "ES256", "kid": client.KeyID, "typ": "JWT",
	})
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]any{
		"iss": client.ClientID,
		"sub": client.ClientID,
		"aud": audience,
		"iat": now.Unix(),
		"exp": now.Add(2 * time.Minute).Unix(),
		"jti": jti,
	})
	if err != nil {
		return "", err
	}
	signingInput := encodeSegment(header) + "." + encodeSegment(claims)
	digest := sha256.Sum256([]byte(signingInput))
	asn1Signature, err := client.PrivateKey.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		return "", err
	}
	var signature struct{ R, S *big.Int }
	rest, err := asn1.Unmarshal(asn1Signature, &signature)
	if err != nil || len(rest) != 0 || signature.R == nil || signature.S == nil {
		return "", errors.New("ES256 signer returned an invalid ASN.1 signature")
	}
	return signingInput + "." + encodeSegment(
		fixedWidthECDSASignature(signature.R, signature.S, 32),
	), nil
}

func (client *Client) doForm(
	ctx context.Context,
	endpoint string,
	values url.Values,
) (*http.Response, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		strings.NewReader(values.Encode()),
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return client.httpClient().Do(request)
}

func (client *Client) httpClient() *http.Client {
	if client.HTTPClient != nil {
		return client.HTTPClient
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (client *Client) now() time.Time {
	if client.Now != nil {
		return client.Now()
	}
	return time.Now()
}

func decodeProtocolError(response *http.Response) error {
	var result ProtocolError
	result.Status = response.StatusCode
	if err := decodeJSON(response.Body, &result); err != nil || result.Code == "" {
		return fmt.Errorf("SuperApp Embed endpoint returned HTTP %d", response.StatusCode)
	}
	return &result
}

func decodeJSON(reader io.Reader, target any) error {
	payload, err := io.ReadAll(io.LimitReader(reader, maximumResponseSize+1))
	if err != nil {
		return err
	}
	if len(payload) > maximumResponseSize {
		return errors.New("SuperApp response exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("SuperApp response must contain exactly one JSON object")
	}
	return nil
}

func encodeSegment(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func fixedWidthECDSASignature(r, s *big.Int, size int) []byte {
	result := make([]byte, size*2)
	r.FillBytes(result[:size])
	s.FillBytes(result[size:])
	return result
}

func randomValue(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
