package steam

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var ErrInvalidStoreURL = errors.New("enter a Steam store link for a game or bundle, like https://store.steampowered.com/app/730/")

type StoreKind string

const (
	StoreKindApp    StoreKind = "app"
	StoreKindBundle StoreKind = "bundle"
)

var storeURL = regexp.MustCompile(`^https?://store\.steampowered\.com/(app|bundle)/(\d+)(?:[/?#].*)?$`)

// ParseStoreURL extracts the kind and ID from a Steam store link for a game
// (/app/<id>) or bundle (/bundle/<id>). Only store.steampowered.com is
// accepted - no other hosts, so a submission can't point the scraper
// somewhere arbitrary.
func ParseStoreURL(raw string) (StoreKind, int, error) {
	match := storeURL.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return "", 0, ErrInvalidStoreURL
	}
	id, err := strconv.Atoi(match[2])
	if err != nil || id <= 0 || id > math.MaxInt32 {
		return "", 0, ErrInvalidStoreURL
	}
	return StoreKind(match[1]), id, nil
}

// ParseStoreAppURL is ParseStoreURL restricted to games.
func ParseStoreAppURL(raw string) (int, error) {
	kind, id, err := ParseStoreURL(raw)
	if err != nil || kind != StoreKindApp {
		return 0, ErrInvalidStoreURL
	}
	return id, nil
}
