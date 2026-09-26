package library

import (
	"context"
	"errors"
	"slices"
	"strings"
)

// maxSubtitleLangs caps the subtitle list. Nobody reads ten languages.
const maxSubtitleLangs = 10

var ErrBadLang = errors.New("not a language code, or too many")

// Languages are the host's defaults for new picks. The picker preselects from them.
type Languages struct {
	Audio     string   `json:"audio"`     // "" = original: the file's default track
	Subtitles []string `json:"subtitles"` // in order of preference
}

// GetLanguages returns the language defaults.
func (l *Libraries) GetLanguages(ctx context.Context) (Languages, error) {
	var audio, subs string
	err := l.DB.QueryRowContext(ctx, "SELECT audio, subtitles FROM language_defaults").Scan(&audio, &subs)
	if err != nil {
		return Languages{}, err
	}
	langs := Languages{Audio: audio, Subtitles: []string{}}
	if subs != "" {
		langs.Subtitles = strings.Split(subs, ",")
	}
	return langs, nil
}

// SetLanguages saves the language defaults and returns them as saved: normalized, duplicates
// dropped. A code NormalizeLang doesn't know, or more than maxSubtitleLangs subtitle languages, fail
// with ErrBadLang.
func (l *Libraries) SetLanguages(ctx context.Context, langs Languages) (Languages, error) {
	out := Languages{Subtitles: []string{}}
	if langs.Audio != "" {
		audio, ok := NormalizeLang(langs.Audio)
		if !ok {
			return Languages{}, ErrBadLang
		}
		out.Audio = audio
	}
	for _, s := range langs.Subtitles {
		code, ok := NormalizeLang(s)
		if !ok {
			return Languages{}, ErrBadLang
		}
		if !slices.Contains(out.Subtitles, code) {
			out.Subtitles = append(out.Subtitles, code)
		}
	}
	if len(out.Subtitles) > maxSubtitleLangs {
		return Languages{}, ErrBadLang
	}
	_, err := l.DB.ExecContext(ctx, "UPDATE language_defaults SET audio = ?, subtitles = ?",
		out.Audio, strings.Join(out.Subtitles, ","))
	if err != nil {
		return Languages{}, err
	}
	return out, nil
}
