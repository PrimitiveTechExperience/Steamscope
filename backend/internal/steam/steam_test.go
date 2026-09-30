package steam

import (
	"errors"
	"net/url"
	"testing"
)

func TestParseStoreAppURL(t *testing.T) {
	tests := []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{"https://store.steampowered.com/app/730/CounterStrike_2/", 730, false},
		{"https://store.steampowered.com/app/730", 730, false},
		{"  https://store.steampowered.com/app/1174180/Red_Dead_Redemption_2/?l=english  ", 1174180, false},
		{"http://store.steampowered.com/app/570#reviews", 570, false},
		{"https://steamcommunity.com/app/730", 0, true},
		{"https://store.steampowered.com.evil.com/app/730", 0, true},
		{"https://evil.com/?u=https://store.steampowered.com/app/730", 0, true},
		{"https://store.steampowered.com/sub/730", 0, true},
		{"https://store.steampowered.com/app/0", 0, true},
		{"https://store.steampowered.com/app/99999999999", 0, true},
		{"730", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseStoreAppURL(tt.raw)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ParseStoreAppURL(%q) = %d, %v; want %d, err=%v", tt.raw, got, err, tt.want, tt.wantErr)
		}
	}
}

const returnTo = "http://localhost:8080/api/auth/steam/callback?state=abc"

func validCallback() url.Values {
	return url.Values{
		"openid.ns":          {openIDNamespace},
		"openid.mode":        {"id_res"},
		"openid.op_endpoint": {openIDEndpoint},
		"openid.return_to":   {returnTo},
		"openid.claimed_id":  {"https://steamcommunity.com/openid/id/76561197960287930"},
		"openid.identity":    {"https://steamcommunity.com/openid/id/76561197960287930"},
	}
}

func TestValidateCallback(t *testing.T) {
	steamID, err := validateCallback(validCallback(), returnTo)
	if err != nil || steamID != "76561197960287930" {
		t.Fatalf("valid callback: got %q, %v", steamID, err)
	}

	cancelled := validCallback()
	cancelled.Set("openid.mode", "cancel")
	if _, err := validateCallback(cancelled, returnTo); !errors.Is(err, ErrCancelled) {
		t.Errorf("cancel: got %v, want ErrCancelled", err)
	}

	mutations := map[string]func(url.Values){
		"wrong return_to": func(q url.Values) { q.Set("openid.return_to", "http://evil.com/callback?state=abc") },
		"wrong endpoint":  func(q url.Values) { q.Set("openid.op_endpoint", "https://evil.com/openid/login") },
		"foreign claimed_id": func(q url.Values) {
			q.Set("openid.claimed_id", "https://evil.com/openid/id/76561197960287930")
			q.Set("openid.identity", "https://evil.com/openid/id/76561197960287930")
		},
		"identity mismatch": func(q url.Values) { q.Set("openid.identity", "https://steamcommunity.com/openid/id/76561197960287931") },
		"short steam id": func(q url.Values) {
			q.Set("openid.claimed_id", "https://steamcommunity.com/openid/id/123")
			q.Set("openid.identity", "https://steamcommunity.com/openid/id/123")
		},
	}
	for name, mutate := range mutations {
		q := validCallback()
		mutate(q)
		if _, err := validateCallback(q, returnTo); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
