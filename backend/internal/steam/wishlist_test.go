package steam_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/steam"
)

func fakeSteam(t *testing.T, handler http.HandlerFunc) *steam.WebAPI {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return steam.NewWebAPIAt("secret-key", srv.URL, srv.URL)
}

func TestGetWishlist(t *testing.T) {
	var query string
	api := fakeSteam(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Write([]byte(`{"response":{"items":[{"appid":20,"priority":0},{"appid":10},{"appid":20},{"appid":0},{"appid":-5},{"appid":30}]}}`))
	})
	ids, err := api.GetWishlist(context.Background(), "7656119")
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{20, 10, 30}; !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v, want %v (Steam's order, no duplicates or invalid ids)", ids, want)
	}
	if !strings.Contains(query, "steamid=7656119") || !strings.Contains(query, "key=secret-key") {
		t.Errorf("query = %q", query)
	}
}

func TestGetWishlistPrivateOrEmpty(t *testing.T) {
	api := fakeSteam(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"response":{}}`)) })
	ids, err := api.GetWishlist(context.Background(), "1")
	if err != nil || len(ids) != 0 {
		t.Errorf("ids = %v err = %v, want an empty list and no error", ids, err)
	}
}

func TestGetWishlistErrors(t *testing.T) {
	if _, err := steam.NewWebAPI("").GetWishlist(context.Background(), "1"); !errors.Is(err, steam.ErrNoAPIKey) {
		t.Errorf("err = %v, want ErrNoAPIKey", err)
	}
	api := fakeSteam(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", http.StatusForbidden) })
	_, err := api.GetWishlist(context.Background(), "1")
	if err == nil {
		t.Fatal("expected an error for a 403")
	}
	if strings.Contains(err.Error(), "secret-key") {
		t.Errorf("error leaks the API key: %v", err)
	}
}

func TestAppName(t *testing.T) {
	api := fakeSteam(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("appids") {
		case "10":
			w.Write([]byte(`{"10":{"success":true,"data":{"name":"Counter-Strike"}}}`))
		case "11":
			w.Write([]byte(`{"11":{"success":false}}`))
		case "12":
			w.Write([]byte(`not json`))
		default:
			http.Error(w, "x", http.StatusInternalServerError)
		}
	})
	for id, want := range map[int]string{10: "Counter-Strike", 11: "", 12: "", 13: ""} {
		if got := api.AppName(context.Background(), id); got != want {
			t.Errorf("AppName(%d) = %q, want %q", id, got, want)
		}
	}
}
