package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/redis/go-redis/v9"
)

const (
	CookieName = "ss_session"
	sessionTTL = 7 * 24 * time.Hour
)

// SessionManager stores sessions server-side in Redis and hands the browser
// only an opaque session ID - encrypted (AES-256) and authenticated
// (HMAC-SHA256) via securecookie, so the cookie value is unreadable and
// any tampering makes it fail to decode rather than resolve to another
// session.
type SessionManager struct {
	rdb    *redis.Client
	codec  *securecookie.SecureCookie
	secure bool
}

func NewSessionManager(rdb *redis.Client, hashKeyB64, blockKeyB64 string, secure bool) (*SessionManager, error) {
	hashKey, err := base64.StdEncoding.DecodeString(hashKeyB64)
	if err != nil || len(hashKey) < 32 {
		return nil, fmt.Errorf("SESSION_HASH_KEY must be base64 of at least 32 random bytes")
	}
	blockKey, err := base64.StdEncoding.DecodeString(blockKeyB64)
	if err != nil || len(blockKey) != 32 {
		return nil, fmt.Errorf("SESSION_BLOCK_KEY must be base64 of exactly 32 random bytes (AES-256)")
	}

	codec := securecookie.New(hashKey, blockKey)
	codec.MaxAge(int(sessionTTL.Seconds()))

	return &SessionManager{rdb: rdb, codec: codec, secure: secure}, nil
}

func sessionKey(id string) string {
	return "session:" + id
}

// Create starts a new session for userID and sets the encrypted cookie.
func (m *SessionManager) Create(ctx context.Context, w http.ResponseWriter, userID int64) error {
	id, err := RandomToken(32)
	if err != nil {
		return err
	}
	if err := m.rdb.Set(ctx, sessionKey(id), userID, sessionTTL).Err(); err != nil {
		return fmt.Errorf("failed to store session: %w", err)
	}
	encoded, err := m.codec.Encode(CookieName, id)
	if err != nil {
		return fmt.Errorf("failed to encode session cookie: %w", err)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    encoded,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// Resolve returns the user ID for the request's session, if any. A missing,
// expired, tampered-with, or undecryptable cookie all resolve to "no session".
func (m *SessionManager) Resolve(ctx context.Context, r *http.Request) (int64, bool) {
	id, ok := m.sessionID(r)
	if !ok {
		return 0, false
	}
	value, err := m.rdb.Get(ctx, sessionKey(id)).Result()
	if err != nil {
		return 0, false
	}
	userID, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false
	}
	return userID, true
}

// Destroy deletes the server-side session and clears the cookie.
func (m *SessionManager) Destroy(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if id, ok := m.sessionID(r); ok {
		m.rdb.Del(ctx, sessionKey(id))
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *SessionManager) sessionID(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return "", false
	}
	var id string
	if err := m.codec.Decode(CookieName, cookie.Value, &id); err != nil {
		return "", false
	}
	return id, true
}

// RandomToken returns n cryptographically random bytes, base64url-encoded.
func RandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
