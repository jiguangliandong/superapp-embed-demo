package session

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"sync"
	"time"
)

const cookieName = "partner_demo_session"

type record struct {
	expiresAt      time.Time
	authentication any
}

type Manager struct {
	mu     sync.Mutex
	items  map[string]record
	ttl    time.Duration
	secure bool
	now    func() time.Time
}

func NewManager(ttl time.Duration, secure bool) *Manager {
	return &Manager{
		items:  make(map[string]record),
		ttl:    ttl,
		secure: secure,
		now:    time.Now,
	}
}

func (manager *Manager) GetOrCreate(response http.ResponseWriter, request *http.Request) (string, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	now := manager.now().UTC()
	manager.prune(now)
	if cookie, err := request.Cookie(cookieName); err == nil {
		if existing, ok := manager.items[cookie.Value]; ok && existing.expiresAt.After(now) {
			existing.expiresAt = now.Add(manager.ttl)
			manager.items[cookie.Value] = existing
			manager.setCookie(response, cookie.Value)
			return cookie.Value, nil
		}
	}

	id, err := randomID()
	if err != nil {
		return "", err
	}
	manager.items[id] = record{expiresAt: now.Add(manager.ttl)}
	manager.setCookie(response, id)
	return id, nil
}

// GetAuthenticated returns an existing authenticated browser session and extends
// its idle timeout. It never creates a new session for an unauthenticated request.
func (manager *Manager) GetAuthenticated(
	response http.ResponseWriter,
	request *http.Request,
) (string, any, bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	now := manager.now().UTC()
	manager.prune(now)
	cookie, err := request.Cookie(cookieName)
	if err != nil {
		return "", nil, false
	}
	existing, ok := manager.items[cookie.Value]
	if !ok || existing.authentication == nil || !existing.expiresAt.After(now) {
		return "", nil, false
	}
	existing.expiresAt = now.Add(manager.ttl)
	manager.items[cookie.Value] = existing
	manager.setCookie(response, cookie.Value)
	return cookie.Value, existing.authentication, true
}

func (manager *Manager) ReplaceAuthentication(id string, authentication any) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	existing, ok := manager.items[id]
	if !ok || existing.authentication == nil {
		return errors.New("partner session no longer exists")
	}
	existing.authentication = authentication
	manager.items[id] = existing
	return nil
}

func (manager *Manager) Delete(response http.ResponseWriter, id string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	delete(manager.items, id)
	http.SetCookie(response, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0).UTC(),
		HttpOnly: true,
		Secure:   manager.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (manager *Manager) Rotate(
	response http.ResponseWriter,
	oldID string,
	authentication any,
) (string, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if _, ok := manager.items[oldID]; !ok {
		return "", errors.New("partner session no longer exists")
	}
	newID, err := randomID()
	if err != nil {
		return "", err
	}
	delete(manager.items, oldID)
	manager.items[newID] = record{
		expiresAt:      manager.now().UTC().Add(manager.ttl),
		authentication: authentication,
	}
	manager.setCookie(response, newID)
	return newID, nil
}

func (manager *Manager) prune(now time.Time) {
	for id, item := range manager.items {
		if !item.expiresAt.After(now) {
			delete(manager.items, id)
		}
	}
}

func (manager *Manager) setCookie(response http.ResponseWriter, value string) {
	http.SetCookie(response, &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int(manager.ttl.Seconds()),
		HttpOnly: true,
		Secure:   manager.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func randomID() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
