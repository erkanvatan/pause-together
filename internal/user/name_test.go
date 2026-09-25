package user

import (
	"errors"
	"strings"
	"testing"
)

func TestCleanName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string // cleaned name; empty means ErrBadName
	}{
		{name: "plain", in: "Alice", want: "Alice"},
		{name: "trimmed", in: "  Alice  ", want: "Alice"},
		{name: "inner space kept", in: "Alice B", want: "Alice B"},
		{name: "turkish", in: "Şükrü Işıkçı", want: "Şükrü Işıkçı"},
		{name: "32 turkish letters (64 bytes)", in: strings.Repeat("ş", 32), want: strings.Repeat("ş", 32)},
		{name: "cyrillic", in: "Иван", want: "Иван"},
		{name: "chinese", in: "李小龍", want: "李小龍"},
		{name: "combining accent", in: "Jose\u0301", want: "Jose\u0301"},
		{name: "punctuation", in: "O'Brien (host)", want: "O'Brien (host)"},
		{name: "digits", in: "R2-D2", want: "R2-D2"},
		{name: "exactly 32", in: strings.Repeat("a", 32), want: strings.Repeat("a", 32)},
		{name: "32 after trim", in: "  " + strings.Repeat("a", 32) + "  ", want: strings.Repeat("a", 32)},
		{name: "33", in: strings.Repeat("a", 33)},
		{name: "33 turkish letters", in: strings.Repeat("ı", 33)},
		{name: "empty", in: ""},
		{name: "spaces only", in: "   "},
		{name: "tab", in: "a\tb"},
		{name: "newline", in: "a\nb"},
		{name: "nul", in: "a\x00b"},
		{name: "delete", in: "a\x7fb"},
		{name: "C1 control", in: "a\u0085b"},
		{name: "line separator", in: "a\u2028b"},
		{name: "paragraph separator", in: "a\u2029b"},
		{name: "zero-width space only", in: "\u200b"},
		{name: "zero-width space inside", in: "Al\u200bice"},
		{name: "text direction override", in: "Al\u202eecila"},
		{name: "BOM", in: "\ufeffAlice"},
		{name: "no letter or number", in: "..."},

		// Emoji, and the other symbols Unicode files with them.
		{name: "emoji", in: "🎬 Film"},
		{name: "emoji only", in: "😀"},
		{name: "heart", in: "Ali ❤"},
		{name: "heart with emoji style", in: "Ali ❤\ufe0f"},
		{name: "flag", in: "Ali 🇹🇷"},
		{name: "keycap", in: "Ali 1\ufe0f\u20e3"},
		{name: "skin tone alone", in: "Ali \U0001f3fd"},
		{name: "family (joined)", in: "Ali 👨\u200d👩\u200d👧"},
		{name: "star", in: "Ali ★"},
		{name: "copyright", in: "Ali ©"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CleanName(tt.in)
			if tt.want == "" {
				if !errors.Is(err, ErrBadName) {
					t.Errorf("CleanName(%q) = %q, %v; want ErrBadName", tt.in, got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("CleanName(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
			}
		})
	}
}
