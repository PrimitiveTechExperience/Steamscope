package database

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// A search is cut to this many words, so one request cannot build an enormous query.
	maxSearchTerms = 8
	// And each word to this many characters.
	maxSearchTermLen = 40
)

var wordPattern = regexp.MustCompile(`[\p{L}\p{N}]+`)

// SearchTerms splits what a user typed into lower-case words (runs of letters
// and digits). Everything else, including every character that means something
// in a full-text query (& | ! : * ( ) ' \ <), is dropped here, so text can
// never alter the query it is searched with.
func SearchTerms(raw string) []string {
	var terms []string
	for _, w := range wordPattern.FindAllString(strings.ToLower(raw), -1) {
		if r := []rune(w); len(r) > maxSearchTermLen {
			w = string(r[:maxSearchTermLen])
		}
		terms = append(terms, w)
		if len(terms) == maxSearchTerms {
			break
		}
	}
	return terms
}

// prefixQuery is a tsquery matching names that start with every word, so a
// half-typed word still finds things ("half li" finds "Half-Life"). Names are
// indexed as typed ('simple'), without stemming or stop words, which is why
// "The Forest" and "Left 4 Dead" are found by exactly what is on the box.
//
// Words go in as bind parameters; addArg returns each one's placeholder.
func prefixQuery(terms []string, addArg func(any) string) string {
	parts := make([]string, len(terms))
	for i, t := range terms {
		parts[i] = fmt.Sprintf("to_tsquery('simple', %s)", addArg(t+":*"))
	}
	return "(" + strings.Join(parts, " && ") + ")"
}

// fullQuery matches a game by name (prefix, as above) or by its description,
// which is indexed as English: "running" there finds a description saying "run".
// Each word must be matched one way or the other.
func fullQuery(terms []string, addArg func(any) string) string {
	parts := make([]string, len(terms))
	for i, t := range terms {
		prefix := addArg(t + ":*")
		plain := addArg(t)
		parts[i] = fmt.Sprintf("(to_tsquery('simple', %s) || plainto_tsquery('english', %s))", prefix, plain)
	}
	return "(" + strings.Join(parts, " && ") + ")"
}

// escapeLike makes text safe to put inside a LIKE pattern.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
