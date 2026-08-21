package sso

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

var (
	ErrTransactionNotFound = errors.New("Partner SSO transaction not found or expired")
	ErrStateMismatch       = errors.New("Partner SSO state mismatch")
)

// Transaction contains secrets that must stay inside Partner Backend.
type Transaction struct {
	ID           string
	State        string
	CodeVerifier string
	Binding      string
	Scopes       []string
	ExpiresAt    time.Time
}

// Bootstrap is the public transaction data returned to Partner H5.
type Bootstrap struct {
	TransactionID string   `json:"transaction_id"`
	ClientID      string   `json:"client_id"`
	State         string   `json:"state"`
	CodeChallenge string   `json:"code_challenge"`
	Scopes        []string `json:"scopes"`
}

// TransactionStore must atomically remove a transaction when it is read.
type TransactionStore interface {
	Put(context.Context, Transaction) error
	Take(context.Context, string) (Transaction, error)
}

type Manager struct {
	clientID string
	store    TransactionStore
	ttl      time.Duration
	now      func() time.Time
}

func NewManager(clientID string, store TransactionStore, ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Manager{clientID: clientID, store: store, ttl: ttl, now: time.Now}
}

func (manager *Manager) Begin(
	ctx context.Context,
	binding string,
	scopes []string,
) (Bootstrap, error) {
	id, err := randomValue(24)
	if err != nil {
		return Bootstrap{}, err
	}
	state, err := randomValue(32)
	if err != nil {
		return Bootstrap{}, err
	}
	verifier, err := randomValue(64)
	if err != nil {
		return Bootstrap{}, err
	}
	if len(scopes) == 0 {
		scopes = []string{"auth_base"}
	}
	transaction := Transaction{
		ID:           id,
		State:        state,
		CodeVerifier: verifier,
		Binding:      binding,
		Scopes:       append([]string(nil), scopes...),
		ExpiresAt:    manager.now().UTC().Add(manager.ttl),
	}
	if err := manager.store.Put(ctx, transaction); err != nil {
		return Bootstrap{}, err
	}
	digest := sha256.Sum256([]byte(verifier))
	return Bootstrap{
		TransactionID: id,
		ClientID:      manager.clientID,
		State:         state,
		CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]),
		Scopes:        append([]string(nil), scopes...),
	}, nil
}

// Complete consumes the transaction before validating it, so failed state or
// browser binding checks cannot be retried with the same transaction ID.
func (manager *Manager) Complete(
	ctx context.Context,
	transactionID string,
	returnedState string,
	binding string,
) (Transaction, error) {
	transaction, err := manager.store.Take(ctx, transactionID)
	if err != nil || !transaction.ExpiresAt.After(manager.now().UTC()) {
		return Transaction{}, ErrTransactionNotFound
	}
	if subtle.ConstantTimeCompare([]byte(transaction.State), []byte(returnedState)) != 1 {
		return Transaction{}, ErrStateMismatch
	}
	if subtle.ConstantTimeCompare([]byte(transaction.Binding), []byte(binding)) != 1 {
		return Transaction{}, ErrTransactionNotFound
	}
	return transaction, nil
}

// MemoryTransactionStore is suitable only for this single-process demo.
// Production deployments should use an encrypted shared store with atomic take.
type MemoryTransactionStore struct {
	mu    sync.Mutex
	items map[string]Transaction
}

func NewMemoryTransactionStore() *MemoryTransactionStore {
	return &MemoryTransactionStore{items: make(map[string]Transaction)}
}

func (store *MemoryTransactionStore) Put(_ context.Context, transaction Transaction) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.items[transaction.ID] = transaction
	return nil
}

func (store *MemoryTransactionStore) Take(
	_ context.Context,
	id string,
) (Transaction, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	transaction, ok := store.items[id]
	delete(store.items, id)
	if !ok {
		return Transaction{}, ErrTransactionNotFound
	}
	return transaction, nil
}

func randomValue(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
