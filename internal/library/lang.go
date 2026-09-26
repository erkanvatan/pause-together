package library

import (
	"strings"

	"golang.org/x/text/language"
)

// bibliographic maps ISO 639-2's bibliographic codes to their terminology codes. MKV tags use either,
// and x/text knows only the terminology ones.
var bibliographic = map[string]string{
	"alb": "sqi", "arm": "hye", "baq": "eus", "bur": "mya", "chi": "zho", "cze": "ces", "dut": "nld",
	"fre": "fra", "geo": "kat", "ger": "deu", "gre": "ell", "ice": "isl", "mac": "mkd", "mao": "mri",
	"may": "msa", "per": "fas", "rum": "ron", "slo": "slk", "tib": "bod", "wel": "cym",
}

// NormalizeLang turns a 2- or 3-letter language code into its shortest form: "eng", "EN" and "en" all
// become "en", "ger" and "deu" become "de". So a track tagged "tur" matches a subtitle named ".tr.srt".
// A code it doesn't know, and "und" (undetermined), come back lower case, with ok false.
func NormalizeLang(code string) (string, bool) {
	code = strings.ToLower(strings.TrimSpace(code))
	if t, ok := bibliographic[code]; ok {
		code = t
	}
	b, err := language.ParseBase(code)
	if err != nil || b.String() == "und" {
		return code, false
	}
	return b.String(), true
}
