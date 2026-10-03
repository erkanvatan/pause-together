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

func TestNameKey(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		same bool
	}{
		{name: "lower and title case", a: "Ali", b: "ali", same: true},
		{name: "upper and title case", a: "ALI", b: "Ali", same: true},
		{name: "dotted capital I is not i", a: "ALİ", b: "ali", same: false},
		{name: "NFC and NFD ş", a: "ş", b: "ş", same: true},
		{name: "spaces trimmed", a: "  Ali ", b: "ali", same: true},
		{name: "inner space counts", a: "Al i", b: "Ali", same: false},
		{name: "no-break space", a: "Ali\u00a0Veli", b: "ali veli", same: true},
		{name: "two spaces", a: "Ali  Veli", b: "ali veli", same: true},
		{name: "two names", a: "Ali", b: "Can", same: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ka, kb := NameKey(tt.a), NameKey(tt.b)
			if (ka == kb) != tt.same {
				t.Errorf("NameKey(%q) = %q, NameKey(%q) = %q; same = %v, want %v", tt.a, ka, tt.b, kb, ka == kb, tt.same)
			}
		})
	}
}
