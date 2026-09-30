package steam

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var ErrInvalidStoreURL = errors.New("enter a Steam store link, like https://store.steampowered.com/app/730/")

var storeAppURL = regexp.MustCompile(`^https?://store\.steampowered\.com/app/(\d+)(?:[/?#].*)?$`)

// ParseStoreAppURL extracts the app ID from a Steam store page link. Only
// store.steampowered.com/app/<id> links are accepted - no other hosts, so a
// submission can't point the scraper somewhere arbitrary.
func ParseStoreAppURL(raw string) (int, error) {
	match := storeAppURL.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return 0, ErrInvalidStoreURL
	}
	appID, err := strconv.Atoi(match[1])
	if err != nil || appID <= 0 || appID > math.MaxInt32 {
		return 0, ErrInvalidStoreURL
	}
	return appID, nil
}
