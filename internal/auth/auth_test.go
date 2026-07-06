package auth

import (
	"testing"
	"time"
)

func TestHashAndCheck(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	a := New(true, hash)
	if !a.CheckPassword("correct horse") {
		t.Error("correct password rejected")
	}
	if a.CheckPassword("wrong") {
		t.Error("wrong password accepted")
	}
}

func TestCheckEmptyHash(t *testing.T) {
	a := New(true, "")
	if a.CheckPassword("anything") {
		t.Error("empty hash must reject every password")
	}
}

func TestDisabledModeAllowsEverything(t *testing.T) {
	a := New(false, "")
	if !a.CheckPassword("whatever") {
		t.Error("disabled auth should accept any password")
	}
	if !a.ValidSession("") {
		t.Error("disabled auth should accept empty session")
	}
}

func TestSessionLifecycle(t *testing.T) {
	hash, _ := HashPassword("pw")
	a := New(true, hash)

	if a.ValidSession("") {
		t.Error("empty token must be invalid")
	}
	if a.ValidSession("bogus") {
		t.Error("unknown token must be invalid")
	}

	tok := a.NewSession()
	if !a.ValidSession(tok) {
		t.Error("fresh session should be valid")
	}
	// Tokens are unique.
	if tok == a.NewSession() {
		t.Error("sessions should be unique")
	}

	a.EndSession(tok)
	if a.ValidSession(tok) {
		t.Error("ended session should be invalid")
	}
}

func TestSessionExpiry(t *testing.T) {
	hash, _ := HashPassword("pw")
	a := New(true, hash)
	tok := a.NewSession()

	// Force the session into the past (white-box: same package).
	a.mu.Lock()
	a.sessions[tok] = time.Now().Add(-time.Minute)
	a.mu.Unlock()

	if a.ValidSession(tok) {
		t.Error("expired session should be invalid")
	}
	// And it should have been pruned.
	a.mu.Lock()
	_, still := a.sessions[tok]
	a.mu.Unlock()
	if still {
		t.Error("expired session should be pruned on check")
	}
}
