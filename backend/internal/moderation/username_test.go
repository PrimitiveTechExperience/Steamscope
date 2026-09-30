package moderation

import (
	"errors"
	"testing"
)

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		want     error
	}{
		// Ordinary insults and self-deprecation are allowed.
		{"insult stupid", "stupid_gamer", nil},
		{"insult idiot", "idiot123", nil},
		{"insult dumb", "DumbDumb", nil},
		{"plain name", "PrimitiveTech", nil},

		// Innocent names containing short blocked terms as substrings.
		{"spicy", "spicy_taco", nil},
		{"raccoon", "raccoon_king", nil},
		{"japan", "Japan_lover", nil},
		{"pakistan", "Pakistani_gamer", nil},
		{"nigeria", "Nigeria_fan", nil},
		{"niger", "niger_river", nil},

		// Format.
		{"too short", "ab", ErrUsernameFormat},
		{"too long", "abcdefghijklmnopqrstu", ErrUsernameFormat},
		{"bad chars", "hello world", ErrUsernameFormat},

		// Blocked terms, including common obfuscations.
		{"plain slur", "faggot", ErrUsernameNotAllowed},
		{"embedded slur", "xX_faggot_Xx", ErrUsernameNotAllowed},
		{"leetspeak", "N1gg3r", ErrUsernameNotAllowed},
		{"separators", "n_i_g_g_e_r", ErrUsernameNotAllowed},
		{"stretched", "niiiiggggger", ErrUsernameNotAllowed},
		{"exact short term", "spic", ErrUsernameNotAllowed},
		{"exact short term leet", "c00n", ErrUsernameNotAllowed},
		{"disability slur", "retard99", ErrUsernameNotAllowed},
		{"hate slogan", "SiegHeil", ErrUsernameNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateUsername(tt.username)
			if !errors.Is(got, tt.want) {
				t.Errorf("ValidateUsername(%q) = %v, want %v", tt.username, got, tt.want)
			}
		})
	}
}
