package moderation

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Blacklist rule fields.
const (
	FieldAppID     = "app_id"
	FieldName      = "name"
	FieldDeveloper = "developer"
	FieldPublisher = "publisher"
)

const maxPatternLength = 200

// GameMeta is what rules are matched against.
type GameMeta struct {
	AppID      int
	Name       string
	Developers []string
	Publishers []string
}

// Rule is a compiled blacklist rule. Go's regexp is RE2, which runs in
// linear time, so an admin-supplied pattern can't cause catastrophic
// backtracking.
type Rule struct {
	field string
	appID int
	re    *regexp.Regexp
}

// ValidateRule checks a rule's field and pattern without keeping it.
func ValidateRule(field, pattern string) error {
	_, err := CompileRule(field, pattern)
	return err
}

// CompileRule parses a rule. app_id patterns are plain numbers; the others
// are case-insensitive regular expressions.
func CompileRule(field, pattern string) (Rule, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || len(pattern) > maxPatternLength {
		return Rule{}, fmt.Errorf("pattern must be 1-%d characters", maxPatternLength)
	}
	switch field {
	case FieldAppID:
		id, err := strconv.Atoi(pattern)
		if err != nil || id <= 0 {
			return Rule{}, errors.New("app ID must be a positive number")
		}
		return Rule{field: field, appID: id}, nil
	case FieldName, FieldDeveloper, FieldPublisher:
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			return Rule{}, fmt.Errorf("invalid regular expression: %v", err)
		}
		return Rule{field: field, re: re}, nil
	}
	return Rule{}, errors.New("field must be app_id, name, developer or publisher")
}

// IsAppID reports whether this is an exact app-ID rule.
func (r Rule) IsAppID() bool { return r.field == FieldAppID }

// Matches reports whether the rule blocks this game.
func (r Rule) Matches(g GameMeta) bool {
	switch r.field {
	case FieldAppID:
		return g.AppID == r.appID
	case FieldName:
		return r.re.MatchString(g.Name)
	case FieldDeveloper:
		return anyMatch(r.re, g.Developers)
	case FieldPublisher:
		return anyMatch(r.re, g.Publishers)
	}
	return false
}

func anyMatch(re *regexp.Regexp, values []string) bool {
	for _, v := range values {
		if re.MatchString(v) {
			return true
		}
	}
	return false
}
