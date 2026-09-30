// Package moderation validates user-chosen names.
package moderation

import (
	_ "embed"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

//go:embed blocklist.txt
var blocklistFile string

var (
	ErrUsernameFormat     = errors.New("username must be 3-20 characters: letters, numbers, _ or -")
	ErrUsernameNotAllowed = errors.New("that username isn't allowed")
)

var usernameFormat = regexp.MustCompile(`^[A-Za-z0-9_-]{3,20}$`)

// leet maps common character substitutions back to the letter they stand in for.
var leet = strings.NewReplacer(
	"0", "o", "1", "i", "3", "e", "4", "a", "5", "s", "7", "t",
	"@", "a", "$", "s", "!", "i", "|", "i",
)

var blocklist = compileBlocklist(blocklistFile)

// ValidateUsername returns nil if the username is well-formed and doesn't
// contain a blocked term. The error never says which term matched.
func ValidateUsername(username string) error {
	if !usernameFormat.MatchString(username) {
		return ErrUsernameFormat
	}
	normalised := normalise(username)
	for _, pattern := range blocklist {
		if pattern.MatchString(normalised) {
			return ErrUsernameNotAllowed
		}
	}
	return nil
}

// normalise lowercases, undoes leetspeak, and drops everything that isn't a
// letter, so "N_1-g.g3r" and "nigger" look the same.
func normalise(s string) string {
	s = leet.Replace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// compileBlocklist turns each term into a regex that tolerates stretched
// letters but still requires a term's doubled letters to be doubled:
// "nigger" -> n+i+g{2,}e+r+ matches "niiiggger" but not "niger".
func compileBlocklist(file string) []*regexp.Regexp {
	var patterns []*regexp.Regexp
	for _, line := range strings.Split(file, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		exact := strings.HasPrefix(line, "=")
		term := normalise(strings.TrimPrefix(line, "="))
		if term == "" {
			continue
		}
		body := stretchPattern(term)
		if exact {
			body = "^" + body + "$"
		}
		patterns = append(patterns, regexp.MustCompile(body))
	}
	return patterns
}

func stretchPattern(term string) string {
	var b strings.Builder
	runes := []rune(term)
	for i := 0; i < len(runes); {
		j := i
		for j < len(runes) && runes[j] == runes[i] {
			j++
		}
		count := j - i
		letter := regexp.QuoteMeta(string(runes[i]))
		if count == 1 {
			b.WriteString(letter + "+")
		} else {
			b.WriteString(letter + "{" + strconv.Itoa(count) + ",}")
		}
		i = j
	}
	return b.String()
}
