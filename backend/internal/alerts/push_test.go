package alerts

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/database"
)

// subscriber makes the keys a real browser would hand over when subscribing.
func subscriber(t *testing.T, endpoint string) database.PushSubscription {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	rand.Read(auth)
	return database.PushSubscription{
		Endpoint: endpoint,
		P256dh:   base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString(auth),
	}
}

func TestWebPusherSendsAnEncryptedSignedMessage(t *testing.T) {
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	var header http.Header
	var body []byte
	status := http.StatusCreated
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Clone()
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
	}))
	defer srv.Close()

	p := NewWebPusher(config.AlertsConfig{VAPIDPublicKey: public, VAPIDPrivateKey: private, VAPIDSubject: "mailto:admin@example.com"})
	sub := subscriber(t, srv.URL+"/push/abc")
	const message = `{"title":"Target price reached","body":"Hades is now $9.99 - secret-marker"}`

	t.Run("delivers it encrypted and signed with the server's VAPID key", func(t *testing.T) {
		gone, err := p.Send(context.Background(), sub, []byte(message))
		if err != nil || gone {
			t.Fatalf("gone=%v err=%v", gone, err)
		}
		if !strings.HasPrefix(header.Get("Authorization"), "vapid t=") || !strings.Contains(header.Get("Authorization"), "k="+public) {
			t.Errorf("Authorization = %q, want a VAPID header naming our public key", header.Get("Authorization"))
		}
		if header.Get("Content-Encoding") != "aes128gcm" {
			t.Errorf("Content-Encoding = %q", header.Get("Content-Encoding"))
		}
		if header.Get("TTL") == "" {
			t.Error("no TTL header: the push service would drop an undelivered message at once")
		}
		if len(body) == 0 || strings.Contains(string(body), "secret-marker") {
			t.Errorf("the body is empty or readable in plain text: %q", body)
		}
	})

	t.Run("a browser that has unsubscribed is reported as gone", func(t *testing.T) {
		for _, s := range []int{http.StatusGone, http.StatusNotFound} {
			status = s
			gone, err := p.Send(context.Background(), sub, []byte(message))
			if !gone || err != nil {
				t.Errorf("status %d: gone=%v err=%v, want gone and no error", s, gone, err)
			}
		}
	})

	t.Run("a push service failure is an error, not 'gone'", func(t *testing.T) {
		for _, s := range []int{http.StatusInternalServerError, http.StatusTooManyRequests, http.StatusBadRequest} {
			status = s
			gone, err := p.Send(context.Background(), sub, []byte(message))
			if gone || err == nil {
				t.Errorf("status %d: gone=%v err=%v, want an error that is not 'gone'", s, gone, err)
			}
		}
	})

	t.Run("a subscription with broken keys fails cleanly", func(t *testing.T) {
		status = http.StatusCreated
		bad := sub
		bad.P256dh = "not-a-key"
		if gone, err := p.Send(context.Background(), bad, []byte(message)); err == nil || gone {
			t.Errorf("gone=%v err=%v, want an error", gone, err)
		}
	})
}
