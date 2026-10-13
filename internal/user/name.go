// Package user holds people, known by their display name, and the browser tokens that keep them across
// visits.
package user

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// MaxNameRunes is the longest name, in runes. Not bytes (Turkish letters take two), and not UTF-16
// units (an input's maxlength counts those, so pages count runes too: MAX_NAME in me.svelte.ts).
const MaxNameRunes = 32

// ErrBadName means a name breaks the rules in CleanName. CleanName's errors are *NameError, which
// match it with errors.Is.
var ErrBadName = errors.New("bad name")

// Why CleanName refused a name, as codes: the page turns them into words (nameRefused in
// web/src/lib/strings.ts). Add a new one to both.
const (
	ReasonEmpty    = "name-empty"
	ReasonLong     = "name-long"
	ReasonSymbol   = "name-symbol" // an emoji or other symbol: NameError.Symbol holds it
	ReasonHidden   = "name-hidden" // a control, format or line break character, or an emoji's invisible part
	ReasonNoLetter = "name-no-letter"
	ReasonOther    = "name-invalid" // no reason known: the page lists every rule
)

// NameError says why a name was refused.
type NameError struct {
	Reason string
	Symbol string // ReasonSymbol: the first symbol, so the page can show which one
}

func (e *NameError) Error() string        { return "bad name: " + e.Reason }
func (e *NameError) Is(target error) bool { return target == ErrBadName }

// A name can't hold these rune classes. hidden are the ones nobody can see, so a refusal can't show
// them:
//   - Cc: control characters (tab, newline).
//   - Cf: format characters. Zero-width ones make blank or look-alike names; text-direction ones
//     make a name display reversed.
//   - Zl, Zp: Unicode line and paragraph breaks.
//   - emojiParts: the invisible pieces that turn other characters into emoji.
//
// symbols can be seen, and a refusal names the first one:
//   - So: "other symbol", where almost every emoji lives. It also holds ♥ ★ © °: Unicode doesn't
//     separate them, so they go too.
//   - skinTones: they show as a colored square when alone.
var hidden = []*unicode.RangeTable{unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp, emojiParts}

var symbols = []*unicode.RangeTable{unicode.So, skinTones}

var emojiParts = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x20e3, Hi: 0x20e3, Stride: 1}, // keycap, as in "1" + U+FE0F + U+20E3
		{Lo: 0xfe0e, Hi: 0xfe0f, Stride: 1}, // text and emoji style selectors, as in "❤" + U+FE0F
	},
}

var skinTones = &unicode.RangeTable{
	R32: []unicode.Range32{{Lo: 0x1f3fb, Hi: 0x1f3ff, Stride: 1}},
}

// CleanName trims s and checks it's a valid display name: 1–32 runes, at least one letter or
// number, nothing in hidden or symbols. A refusal is a *NameError.
func CleanName(s string) (string, error) {
	s = strings.TrimSpace(s)
	switch n := utf8.RuneCountInString(s); {
	case n < 1:
		return "", &NameError{Reason: ReasonEmpty}
	case n > MaxNameRunes:
		return "", &NameError{Reason: ReasonLong}
	}
	prev := 0 // where the rune before this one starts
	for i, r := range s {
		switch {
		case unicode.IsOneOf(symbols, r):
			return "", &NameError{Reason: ReasonSymbol, Symbol: s[i:emojiEnd(s, i)]}
		case unicode.Is(emojiParts, r) && i > 0:
			if e := emojiFrom(s, prev, i); e != "" {
				return "", &NameError{Reason: ReasonSymbol, Symbol: e}
			}
			return "", &NameError{Reason: ReasonHidden}
		case unicode.IsOneOf(hidden, r):
			return "", &NameError{Reason: ReasonHidden}
		}
		prev = i
	}
	if !strings.ContainsFunc(s, isLetterOrNumber) {
		return "", &NameError{Reason: ReasonNoLetter}
	}
	return s, nil
}

// emojiFrom says what emoji the emojiParts at s[at] make with the rune before them, at s[prev], or ""
// when they make none a person would see: a keycap (1️⃣ #️⃣), or a selector after a symbol that isn't
// So (↔️ ™️). After anything else (a letter, a space, "!"), a lone selector draws nothing: it's hidden.
func emojiFrom(s string, prev, at int) string {
	end := emojiEnd(s, prev)
	base, _ := utf8.DecodeRuneInString(s[prev:])
	if !strings.ContainsRune(s[at:end], 0x20e3) && !unicode.Is(unicode.S, base) {
		return ""
	}
	return s[prev:end]
}

const zwj = 0x200d // zero-width joiner: 👨 + ZWJ + 👩 draws one emoji

// tags spell a subdivision flag after 🏴, as in England's.
var tags = &unicode.RangeTable{R32: []unicode.Range32{{Lo: 0xe0020, Hi: 0xe007f, Stride: 1}}}

// emojiEnd says where the emoji that starts at s[start] ends, so a refusal shows it as it's drawn:
// with its style selectors, keycap, skin tone and tags, a flag's second letter, and the symbols a
// zero-width joiner joins to it. A joiner before a letter ends it there: the letter is allowed.
func emojiEnd(s string, start int) int {
	first, end := utf8.DecodeRuneInString(s[start:])
	end += start
	flag := unicode.Is(unicode.Regional_Indicator, first)
	for end < len(s) {
		r, size := utf8.DecodeRuneInString(s[end:])
		switch {
		case r == zwj:
			next, n := utf8.DecodeRuneInString(s[end+size:])
			if !unicode.Is(unicode.S, next) {
				return end
			}
			size += n
		case unicode.Is(emojiParts, r), unicode.Is(skinTones, r), unicode.Is(tags, r):
		case flag && unicode.Is(unicode.Regional_Indicator, r):
			flag = false // a flag is two letters
		default:
			return end
		}
		end += size
	}
	return end
}

// NameKey is what makes two names one person: each run of spaces made one plain space (a phone may
// type a no-break space), trimmed, in NFC, then case-folded, so "ali" and "Ali" match. The fold is the
// same for every language: "ALİ" and "ali" stay two people.
func NameKey(name string) string {
	return cases.Fold().String(norm.NFC.String(strings.Join(strings.Fields(name), " ")))
}

func isLetterOrNumber(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }
