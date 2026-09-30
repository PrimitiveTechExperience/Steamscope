package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("unexpected hash format: %s", hash)
	}

	if ok, err := VerifyPassword("correct horse battery staple", hash); err != nil || !ok {
		t.Errorf("correct password: ok=%v err=%v", ok, err)
	}
	if ok, _ := VerifyPassword("wrong password", hash); ok {
		t.Error("wrong password verified")
	}

	other, _ := HashPassword("correct horse battery staple")
	if other == hash {
		t.Error("same password produced identical hashes; salt not random")
	}

	if _, err := VerifyPassword("x", "not-a-hash"); err == nil {
		t.Error("expected error for malformed hash")
	}
}

func randomKeyB64(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func requestWithCookie(value string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: value})
	return r
}

func TestSessionCookieEncryption(t *testing.T) {
	m, err := NewSessionManager(nil, randomKeyB64(t, 64), randomKeyB64(t, 32), false)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	const sessionID = "plain-session-id-123"
	encoded, err := m.codec.Encode(CookieName, sessionID)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if strings.Contains(encoded, sessionID) {
		t.Fatal("cookie value contains the raw session ID; it isn't encrypted")
	}

	got, ok := m.sessionID(requestWithCookie(encoded))
	if !ok || got != sessionID {
		t.Fatalf("round trip: got %q ok=%v", got, ok)
	}

	// Flip one character: must fail HMAC verification, not decode to something else.
	tampered := []byte(encoded)
	if tampered[10] == 'A' {
		tampered[10] = 'B'
	} else {
		tampered[10] = 'A'
	}
	if _, ok := m.sessionID(requestWithCookie(string(tampered))); ok {
		t.Error("tampered cookie was accepted")
	}

	// A cookie from a server with different keys must not decode.
	other, _ := NewSessionManager(nil, randomKeyB64(t, 64), randomKeyB64(t, 32), false)
	if _, ok := other.sessionID(requestWithCookie(encoded)); ok {
		t.Error("cookie decoded with the wrong keys")
	}
}

func TestNewSessionManagerRejectsBadKeys(t *testing.T) {
	if _, err := NewSessionManager(nil, randomKeyB64(t, 64), randomKeyB64(t, 16), false); err == nil {
		t.Error("expected error for 16-byte block key (need 32 for AES-256)")
	}
	if _, err := NewSessionManager(nil, "not base64!", randomKeyB64(t, 32), false); err == nil {
		t.Error("expected error for invalid hash key")
	}
}
