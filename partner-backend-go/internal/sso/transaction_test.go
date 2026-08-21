package sso

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestManagerKeepsVerifierInBackendAndConsumesTransaction(t *testing.T) {
	manager := NewManager("embcli_demo", NewMemoryTransactionStore(), time.Minute)
	bootstrap, err := manager.Begin(
		context.Background(),
		"browser-session-1",
		[]string{"auth_base", "profile.name"},
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "verifier") {
		t.Fatalf("bootstrap leaked PKCE verifier: %s", encoded)
	}

	transaction, err := manager.Complete(
		context.Background(),
		bootstrap.TransactionID,
		bootstrap.State,
		"browser-session-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(transaction.CodeVerifier))
	if challenge := base64.RawURLEncoding.EncodeToString(digest[:]); challenge != bootstrap.CodeChallenge {
		t.Fatalf("code challenge = %q, want %q", challenge, bootstrap.CodeChallenge)
	}
	if _, err := manager.Complete(
		context.Background(),
		bootstrap.TransactionID,
		bootstrap.State,
		"browser-session-1",
	); !errors.Is(err, ErrTransactionNotFound) {
		t.Fatalf("replay error = %v, want ErrTransactionNotFound", err)
	}
}

func TestManagerConsumesTransactionOnStateOrBindingMismatch(t *testing.T) {
	for _, test := range []struct {
		name    string
		state   string
		binding string
	}{
		{name: "state", state: "wrong-state", binding: "browser-session-1"},
		{name: "binding", binding: "other-browser"},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := NewManager("embcli_demo", NewMemoryTransactionStore(), time.Minute)
			bootstrap, err := manager.Begin(
				context.Background(), "browser-session-1", []string{"auth_base"},
			)
			if err != nil {
				t.Fatal(err)
			}
			state := test.state
			if state == "" {
				state = bootstrap.State
			}
			if _, err := manager.Complete(
				context.Background(), bootstrap.TransactionID, state, test.binding,
			); err == nil {
				t.Fatal("mismatched transaction should fail")
			}
			if _, err := manager.Complete(
				context.Background(),
				bootstrap.TransactionID,
				bootstrap.State,
				"browser-session-1",
			); !errors.Is(err, ErrTransactionNotFound) {
				t.Fatalf("second attempt error = %v, want ErrTransactionNotFound", err)
			}
		})
	}
}
