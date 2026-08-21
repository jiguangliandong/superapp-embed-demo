package superapp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientUsesPKCEAndUniquePrivateKeyJWT(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	assertions := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if err := request.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		if request.Form.Get("code_verifier") != "server-side-verifier" {
			t.Errorf("code_verifier = %q", request.Form.Get("code_verifier"))
		}
		if request.Form.Get("client_assertion_type") != clientAssertionType {
			t.Errorf("client_assertion_type = %q", request.Form.Get("client_assertion_type"))
		}
		assertions = append(assertions, request.Form.Get("client_assertion"))
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(
			`{"token_type":"Bearer","access_token":"at","expires_in":300,` +
				`"refresh_token":"rt","refresh_token_expires_in":3600,` +
				`"scope":"auth_base","open_id":"open_1","consent_version":1}`,
		))
	}))
	defer server.Close()

	client := Client{
		BaseURL: server.URL, ClientID: "embcli_demo", KeyID: "key-demo", PrivateKey: privateKey,
	}
	for range 2 {
		if _, err := client.ExchangeAuthorizationCode(
			context.Background(), "authorization-code", "server-side-verifier",
		); err != nil {
			t.Fatal(err)
		}
	}
	if len(assertions) != 2 || assertions[0] == assertions[1] {
		t.Fatalf("client assertions must be unique: %v", assertions)
	}
	for _, assertion := range assertions {
		parts := strings.Split(assertion, ".")
		if len(parts) != 3 {
			t.Fatalf("assertion has %d segments", len(parts))
		}
		headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
		if err != nil {
			t.Fatal(err)
		}
		var header map[string]string
		if err := json.Unmarshal(headerBytes, &header); err != nil {
			t.Fatal(err)
		}
		if header["alg"] != "ES256" || header["kid"] != "key-demo" || header["typ"] != "JWT" {
			t.Fatalf("unexpected assertion header: %v", header)
		}
		signature, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil || len(signature) != 64 {
			t.Fatalf("invalid JOSE ES256 signature length: %d", len(signature))
		}
	}
}

func TestClientReturnsTypedProtocolError(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusBadRequest)
		_, _ = response.Write([]byte(
			`{"error":"invalid_grant","error_description":"authorization code rejected"}`,
		))
	}))
	defer server.Close()

	client := Client{
		BaseURL: server.URL, ClientID: "embcli_demo", KeyID: "key-demo", PrivateKey: privateKey,
	}
	_, err = client.ExchangeAuthorizationCode(context.Background(), "bad-code", "verifier")
	var protocolError *ProtocolError
	if !errors.As(err, &protocolError) ||
		protocolError.Code != "invalid_grant" ||
		protocolError.Status != http.StatusBadRequest {
		t.Fatalf("error = %#v, want invalid_grant ProtocolError", err)
	}
}
