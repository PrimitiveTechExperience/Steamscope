// Package steam implements "Sign in through Steam" (OpenID 2.0) and the
// bits of the Steam Web API used for profile cards.
package steam

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	openIDEndpoint         = "https://steamcommunity.com/openid/login"
	openIDNamespace        = "http://specs.openid.net/auth/2.0"
	openIDIdentifierSelect = "http://specs.openid.net/auth/2.0/identifier_select"
)

var claimedIDPattern = regexp.MustCompile(`^https://steamcommunity\.com/openid/id/(\d{17})$`)

var ErrCancelled = errors.New("steam sign-in was cancelled")

// LoginURL is where to send the browser to start Steam sign-in. Steam sends
// it back to returnTo (which must live under realm) with the OpenID response.
func LoginURL(returnTo, realm string) string {
	q := url.Values{}
	q.Set("openid.ns", openIDNamespace)
	q.Set("openid.mode", "checkid_setup")
	q.Set("openid.return_to", returnTo)
	q.Set("openid.realm", realm)
	q.Set("openid.identity", openIDIdentifierSelect)
	q.Set("openid.claimed_id", openIDIdentifierSelect)
	return openIDEndpoint + "?" + q.Encode()
}

// Verify checks a Steam OpenID callback and returns the verified SteamID64.
// The response is validated locally first, then confirmed with Steam itself
// (check_authentication) - the claimed ID in the query string alone is
// attacker-controlled and proves nothing.
func Verify(ctx context.Context, client *http.Client, query url.Values, expectedReturnTo string) (string, error) {
	steamID, err := validateCallback(query, expectedReturnTo)
	if err != nil {
		return "", err
	}
	if err := checkAuthentication(ctx, client, query); err != nil {
		return "", err
	}
	return steamID, nil
}

// validateCallback performs every check that doesn't need a network call.
func validateCallback(query url.Values, expectedReturnTo string) (string, error) {
	switch query.Get("openid.mode") {
	case "id_res":
	case "cancel":
		return "", ErrCancelled
	default:
		return "", fmt.Errorf("unexpected openid.mode %q", query.Get("openid.mode"))
	}
	if query.Get("openid.ns") != openIDNamespace {
		return "", errors.New("unexpected openid.ns")
	}
	if query.Get("openid.op_endpoint") != openIDEndpoint {
		return "", errors.New("unexpected openid.op_endpoint")
	}
	if query.Get("openid.return_to") != expectedReturnTo {
		return "", errors.New("openid.return_to does not match")
	}
	claimed := query.Get("openid.claimed_id")
	if query.Get("openid.identity") != claimed {
		return "", errors.New("openid.identity does not match claimed_id")
	}
	match := claimedIDPattern.FindStringSubmatch(claimed)
	if match == nil {
		return "", errors.New("invalid openid.claimed_id")
	}
	return match[1], nil
}

// checkAuthentication asks Steam to confirm it really issued this response.
func checkAuthentication(ctx context.Context, client *http.Client, query url.Values) error {
	form := url.Values{}
	for key, values := range query {
		if strings.HasPrefix(key, "openid.") && len(values) > 0 {
			form.Set(key, values[0])
		}
	}
	form.Set("openid.mode", "check_authentication")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openIDEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to verify with steam: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("failed to read steam verification response: %w", err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == "is_valid:true" {
			return nil
		}
	}
	return errors.New("steam did not confirm the sign-in")
}
