// Package user holds people, known by their display name, and the browser tokens that keep them across
// visits.
package user

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// MaxNameRunes is the longest name, in runes. Not bytes (Turkish letters take two), and not UTF-16
// units (an input's maxlength counts those, so the server decides).
const MaxNameRunes = 32

// ErrBadName means a name breaks the rules in CleanName.
var ErrBadName = fmt.Errorf("name must be 1–%d characters with a letter or number, and no emoji", MaxNameRunes)

// notAllowed are the rune classes a name can't hold:
//   - Cc: control characters (tab, newline).
//   - Cf: format characters. Zero-width ones make blank or look-alike names; text-direction ones
//     make a name display reversed.
//   - Zl, Zp: Unicode line and paragraph breaks.
//   - So: "other symbol", where almost every emoji lives. It also holds ♥ ★ © °: Unicode doesn't
//     separate them, so they go too.
//   - emojiParts: the pieces that turn other characters into emoji.
var notAllowed = []*unicode.RangeTable{unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp, unicode.So, emojiParts}

var emojiParts = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x20e3, Hi: 0x20e3, Stride: 1}, // keycap, as in "1" + U+FE0F + U+20E3
		{Lo: 0xfe0e, Hi: 0xfe0f, Stride: 1}, // text and emoji style selectors, as in "❤" + U+FE0F
	},
	R32: []unicode.Range32{
		{Lo: 0x1f3fb, Hi: 0x1f3ff, Stride: 1}, // skin tones
	},
}

// CleanName trims s and checks it's a valid display name: 1–32 runes, at least one letter or
// number, nothing in notAllowed.
func CleanName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if n := utf8.RuneCountInString(s); n < 1 || n > MaxNameRunes {
		return "", ErrBadName
	}
	if strings.ContainsFunc(s, isNotAllowed) || !strings.ContainsFunc(s, isLetterOrNumber) {
		return "", ErrBadName
	}
	return s, nil
}

// NameKey is what makes two names one person: each run of spaces made one plain space (a phone may
// type a no-break space), trimmed, in NFC, then case-folded, so "ali" and "Ali" match. The fold is the
// same for every language: "ALİ" and "ali" stay two people.
func NameKey(name string) string {
	return cases.Fold().String(norm.NFC.String(strings.Join(strings.Fields(name), " ")))
}

func isNotAllowed(r rune) bool { return unicode.IsOneOf(notAllowed, r) }

func isLetterOrNumber(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }
