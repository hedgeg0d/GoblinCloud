// Package auth provides the single shared credential and web sessions.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns a bcrypt hash of the given password.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// Authenticator checks credentials and manages web sessions. When Enabled is
// false every check passes (open LAN mode).
type Authenticator struct {
	Enabled bool
	hash    string
	ttl     time.Duration

	mu       sync.Mutex
	sessions map[string]time.Time // token -> expiry
}

// New builds an Authenticator from a bcrypt hash.
func New(enabled bool, hash string) *Authenticator {
	return &Authenticator{
		Enabled:  enabled,
		hash:     hash,
		ttl:      12 * time.Hour,
		sessions: make(map[string]time.Time),
	}
}

// CheckPassword reports whether password matches the configured hash.
func (a *Authenticator) CheckPassword(password string) bool {
	if !a.Enabled {
		return true
	}
	if a.hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(a.hash), []byte(password)) == nil
}

// NewSession issues a session token valid for the TTL.
func (a *Authenticator) NewSession() string {
	tok := randomToken()
	a.mu.Lock()
	a.sessions[tok] = time.Now().Add(a.ttl)
	a.mu.Unlock()
	return tok
}

// ValidSession reports whether a token is known and unexpired. Open mode always
// returns true.
func (a *Authenticator) ValidSession(token string) bool {
	if !a.Enabled {
		return true
	}
	if token == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	exp, ok := a.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(a.sessions, token)
		return false
	}
	return true
}

// EndSession invalidates a token.
func (a *Authenticator) EndSession(token string) {
	a.mu.Lock()
	delete(a.sessions, token)
	a.mu.Unlock()
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
