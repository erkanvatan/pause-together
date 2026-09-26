// Package library turns file paths inside a library folder into videos and sidecar subtitles,
// following Plex naming. The parser is pure: it never touches the disk.
package library

import (
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Type is a library's type. It decides which naming rules apply.
type Type string

const (
	Movies      Type = "movies"
	TVShows     Type = "tv"
	OtherVideos Type = "other"
)

// Kind says what a file turned out to be.
type Kind int

const (
	KindIgnored Kind = iota // quietly not ours: not a video, hidden, extras, sample
	KindSkipped             // a video or subtitle we can't use; Reason says why
	KindVideo
	KindSubtitle
)

// Reason says why a file was skipped. It's a code, not English: the web strings file turns it into
// text, so the admin page can be translated.
type Reason string

const (
	ReasonMovieNoYear    Reason = "movie-no-year"     // not "Title (Year)"
	ReasonMovieSplit     Reason = "movie-split"       // one part of a movie split into files (pt1, cd1)
	ReasonTVNoShowFolder Reason = "tv-no-show-folder" // not inside a show folder
	ReasonTVBadFolder    Reason = "tv-bad-folder"     // not Show/file or Show/Season NN/file
	ReasonTVNoEpisode    Reason = "tv-no-episode"     // no s01e02; covers absolute-numbered (anime) names
	ReasonTVDate         Reason = "tv-date"           // date-based episode
	ReasonSubNoVideo     Reason = "sub-no-video"      // no video with a matching name next to it
	ReasonSubBadName     Reason = "sub-bad-name"      // text after the video's name isn't a language code and flags
	ReasonSubNoLang      Reason = "sub-no-lang"       // no language code in the name
	ReasonSubUnreadable  Reason = "sub-unreadable"    // couldn't be read, or holds no subtitles
)

// Video is what a video file's path says about it.
type Video struct {
	Title        string // movie title, show name (from the show folder), or other video's file name
	Year         int    // 0 = none
	Edition      string // movies: from {edition-…}
	Version      string // movies: text after the year, e.g. "1080p" in "Dune (2021) - 1080p.mkv"
	Season       int    // TV: 0 = specials
	Episode      int    // TV
	EpisodeEnd   int    // TV: last episode in the file; equals Episode for one-episode files
	EpisodeTitle string // TV
	Group        string // Other Videos: folder path relative to the library, "" at the root
}

// Subtitle is a sidecar subtitle file matched to the video next to it.
type Subtitle struct {
	Video  string // file name of its video, in the same folder
	Lang   string // 2- or 3-letter code, lower case, as written
	Forced bool
	SDH    bool // "sdh" or "hi"
}

// File is one parsed file. Video is set for KindVideo, Subtitle for KindSubtitle, Reason for
// KindSkipped.
type File struct {
	Name     string
	Kind     Kind
	Reason   Reason
	Video    Video
	Subtitle Subtitle
}

var videoExts = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".mov": true, ".avi": true, ".webm": true, ".ts": true, ".m2ts": true,
}

var subtitleExts = map[string]bool{".srt": true, ".vtt": true, ".ass": true}

// extrasFolders are Plex's extras folders, plus sample folders, in lower case.
var extrasFolders = map[string]bool{
	"behind the scenes": true, "deleted scenes": true, "featurettes": true, "interviews": true, "scenes": true,
	"shorts": true, "trailers": true, "other": true, "extras": true, "sample": true, "samples": true,
}

// extrasSuffixes mark an extra by its file name, as in "Inception (2010)-trailer.mkv".
var extrasSuffixes = []string{
	"-trailer", "-behindthescenes", "-deleted", "-featurette", "-interview", "-scene", "-short", "-other", "-sample",
}

var (
	tagRe       = regexp.MustCompile(`\s*\{([^{}]*)\}`)
	titleYearRe = regexp.MustCompile(`^(.+?)\s*\((\d{4})\)(.*)$`)
	// Not "dvd1": "DVD9" is a source label, not part 9.
	splitRe  = regexp.MustCompile(`(?i)(?:^|[\s._-])(?:pt|part|cd|disc|disk)\s?\d+$`)
	seasonRe = regexp.MustCompile(`(?i)^(?:season\s*\d{1,4}|specials)$`)
	// Groups: season, episode, then the last episode as "-e03"/"e03" (3), "-03" (4) or "-s01e03" (5, 6).
	episodeRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s(\d{1,4})e(\d{1,4})(?:-?e(\d{1,4})|-(\d{1,4})|-s(\d{1,4})e(\d{1,4}))?`)
	dateRe    = regexp.MustCompile(`(?:^|\D)\d{4}[-.]\d{2}[-.]\d{2}(?:\D|$)`)
	langRe    = regexp.MustCompile(`^[a-z]{2,3}$`)
)

// ParseDir parses the files of one folder. dir is the folder's path relative to the library folder,
// with "/" separators ("" for the library folder itself). names are the files in it. The result has
// one File per name, in the same order. Subtitles are matched to the videos among names.
func ParseDir(typ Type, dir string, names []string) []File {
	var folders []string
	if dir = path.Clean(dir); dir != "." {
		folders = strings.Split(dir, "/")
	}

	files := make([]File, len(names))
	byName := make(map[string]File, len(names))
	var videos []string // every video file, used or not, so an extra's subtitle stays quiet too
	for i, name := range names {
		files[i] = parseFile(typ, folders, name)
		byName[name] = files[i]
		if !strings.HasPrefix(name, ".") && videoExts[strings.ToLower(path.Ext(name))] {
			videos = append(videos, name)
		}
	}
	for i, f := range files {
		if f.Kind != KindSubtitle {
			continue
		}
		sub, reason := matchSubtitle(f.Name, videos)
		switch {
		case sub.Video != "" && byName[sub.Video].Kind != KindVideo:
			// Its video is ignored or already listed as skipped.
			f = File{Name: f.Name, Kind: KindIgnored}
		case reason != "":
			f = File{Name: f.Name, Kind: KindSkipped, Reason: reason}
		default:
			f.Subtitle = sub
		}
		files[i] = f
	}
	return files
}

// parseFile parses one file. A subtitle comes back as KindSubtitle with no Subtitle yet: matching it
// needs the other files.
func parseFile(typ Type, folders []string, name string) File {
	f := File{Name: name}
	ext := strings.ToLower(path.Ext(name))
	stem := strings.TrimSuffix(name, path.Ext(name))
	isSub := subtitleExts[ext]
	switch {
	case isHidden(folders, name), !videoExts[ext] && !isSub:
		return f
	case isExtra(typ, folders, stem):
		return f
	case isSub:
		f.Kind = KindSubtitle
		return f
	}

	var v Video
	var r Reason
	switch typ {
	case Movies:
		v, r = parseMovie(stem)
	case TVShows:
		v, r = parseEpisode(folders, stem)
	default:
		v = Video{Title: stem, Group: strings.Join(folders, "/")}
	}
	if r != "" {
		f.Kind, f.Reason = KindSkipped, r
	} else {
		f.Kind, f.Video = KindVideo, v
	}
	return f
}

func isHidden(folders []string, name string) bool {
	for _, p := range folders {
		if strings.HasPrefix(p, ".") {
			return true
		}
	}
	// Also covers macOS "._" files.
	return strings.HasPrefix(name, ".")
}

// isExtra reports whether a file is a Plex extra or a sample. An extras folder counts only inside a
// movie or show, so a collection or a show can itself be called "Shorts" or "Extras". In Other
// Videos nothing is an extra: a "Trailers" folder there is a real group.
func isExtra(typ Type, folders []string, stem string) bool {
	switch typ {
	case Movies:
		for i := 1; i < len(folders); i++ {
			if extrasFolders[strings.ToLower(folders[i])] && isMovieFolder(folders[i-1]) {
				return true
			}
		}
	case TVShows:
		for _, p := range folders[min(1, len(folders)):] {
			if extrasFolders[strings.ToLower(p)] {
				return true
			}
		}
		// An episode is never an extra, whatever its title or release group ends in ("x264-SCENE").
		if findEpisode(stem) != nil {
			return false
		}
	default:
		return false
	}
	stem = strings.ToLower(stem)
	if stem == "sample" {
		return true
	}
	for _, s := range extrasSuffixes {
		if strings.HasSuffix(stem, s) {
			return true
		}
	}
	return false
}

func isMovieFolder(name string) bool {
	name, _ = dropTags(name)
	_, _, _, ok := splitTitleYear(name)
	return ok
}

// parseMovie parses "Title (Year) {edition-X} - Version". Only the file name counts, so movies can
// sit loose, in their own folder, or in collection folders.
func parseMovie(stem string) (Video, Reason) {
	stem, edition := dropTags(stem)
	title, year, rest, ok := splitTitleYear(stem)
	if !ok {
		return Video{}, ReasonMovieNoYear
	}
	if splitRe.MatchString(rest) {
		return Video{}, ReasonMovieSplit
	}
	return Video{Title: title, Year: year, Edition: edition, Version: strings.Trim(rest, " -._")}, ""
}

// parseEpisode parses Show/Season NN/file or Show/file. The show comes from its folder, the season
// and episode from the file name.
func parseEpisode(folders []string, stem string) (Video, Reason) {
	switch {
	case len(folders) == 0 || seasonRe.MatchString(folders[0]):
		return Video{}, ReasonTVNoShowFolder
	case len(folders) == 1:
	case len(folders) == 2 && seasonRe.MatchString(folders[1]):
	default:
		return Video{}, ReasonTVBadFolder
	}

	show, _ := dropTags(folders[0])
	title, year, rest, ok := splitTitleYear(show)
	if !ok || strings.TrimSpace(rest) != "" {
		title, year = strings.TrimSpace(show), 0
	}

	m := findEpisode(stem)
	if m == nil {
		if dateRe.MatchString(stem) {
			return Video{}, ReasonTVDate
		}
		return Video{}, ReasonTVNoEpisode
	}
	season, episode := atoi(stem, m, 1), atoi(stem, m, 2)
	end := episode
	for _, g := range []int{3, 4, 6} {
		if m[2*g] >= 0 {
			end = atoi(stem, m, g)
		}
	}
	if end < episode || m[10] >= 0 && atoi(stem, m, 5) != season {
		return Video{}, ReasonTVNoEpisode
	}

	// The episode title follows a space and a dash (or en dash). Anything else after the episode
	// (".720p.BluRay", "-GROUP") is junk.
	var epTitle string
	after, _ := dropTags(stem[m[1]:])
	if t := strings.TrimLeft(after, " "); len(t) < len(after) {
		for _, dash := range []string{"-", "–"} {
			if t, ok := strings.CutPrefix(t, dash); ok {
				epTitle = strings.TrimSpace(t)
			}
		}
	}

	return Video{Title: title, Year: year, Season: season, Episode: episode, EpisodeEnd: end, EpisodeTitle: epTitle}, ""
}

// findEpisode returns the submatch indexes of the first s01e02 that isn't glued to more letters or
// digits ("s01e02x264").
func findEpisode(stem string) []int {
	for _, m := range episodeRe.FindAllStringSubmatchIndex(stem, -1) {
		if m[1] == len(stem) || !isAlnum(stem[m[1]]) {
			return m
		}
	}
	return nil
}

func isAlnum(b byte) bool {
	return 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || '0' <= b && b <= '9'
}

// atoi reads submatch group g, which the regexp limits to a few digits.
func atoi(s string, m []int, g int) int {
	n, _ := strconv.Atoi(s[m[2*g]:m[2*g+1]])
	return n
}

// dropTags removes {…} tags from s and returns the text of an {edition-…} tag, if any.
func dropTags(s string) (string, string) {
	var edition string
	for _, m := range tagRe.FindAllStringSubmatch(s, -1) {
		if len(m[1]) > len("edition-") && strings.EqualFold(m[1][:len("edition-")], "edition-") {
			edition = strings.TrimSpace(m[1][len("edition-"):])
		}
	}
	return tagRe.ReplaceAllString(s, ""), edition
}

// splitTitleYear splits "Title (Year) rest" at the first four-digit year in parentheses.
func splitTitleYear(s string) (title string, year int, rest string, ok bool) {
	m := titleYearRe.FindStringSubmatch(s)
	if m == nil {
		return "", 0, "", false
	}
	title = strings.TrimSpace(m[1])
	if title == "" {
		return "", 0, "", false
	}
	year, _ = strconv.Atoi(m[2])
	return title, year, m[3], true
}

// matchSubtitle matches a sidecar subtitle to one of the videos next to it. Its name must be the
// video's name, then a language code, then optional flags: "Title (Year).en.forced.srt".
// When several videos fit ("Talk.mkv" and "Talk.en.mkv"), the longest name wins. A name that fits a
// video but breaks the rules comes back with a Reason and only Video set, so the caller can check
// whether that video is used.
func matchSubtitle(name string, videos []string) (Subtitle, Reason) {
	stem := strings.TrimSuffix(name, path.Ext(name))
	var sub Subtitle
	var tokens string
	best := -1
	for _, v := range videos {
		vs := strings.TrimSuffix(v, path.Ext(v))
		if len(vs) <= best {
			continue
		}
		if stem == vs {
			sub.Video, tokens, best = v, "", len(vs)
		} else if t, ok := strings.CutPrefix(stem, vs+"."); ok {
			sub.Video, tokens, best = v, t, len(vs)
		}
	}
	if sub.Video == "" {
		return Subtitle{}, ReasonSubNoVideo
	}
	if tokens == "" {
		return Subtitle{Video: sub.Video}, ReasonSubNoLang
	}
	for i, t := range strings.Split(tokens, ".") {
		t = strings.ToLower(t)
		switch {
		case t == "forced":
			sub.Forced = true
		// "hi" is also Hindi's code. First, it's the language; after a language, the flag.
		case t == "sdh", t == "hi" && i > 0:
			sub.SDH = true
		case i == 0 && langRe.MatchString(t):
			sub.Lang = t
		default:
			return Subtitle{Video: sub.Video}, ReasonSubBadName
		}
	}
	if sub.Lang == "" {
		return Subtitle{Video: sub.Video}, ReasonSubNoLang
	}
	return sub, ""
}
