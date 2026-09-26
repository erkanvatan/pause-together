package library

import (
	"path"
	"testing"
)

func video(v Video) File       { return File{Kind: KindVideo, Video: v} }
func skipped(r Reason) File    { return File{Kind: KindSkipped, Reason: r} }
func subtitle(s Subtitle) File { return File{Kind: KindSubtitle, Subtitle: s} }

var ignored = File{Kind: KindIgnored}

// parseOne parses a single file, given by its path relative to the library folder.
func parseOne(typ Type, p string) File {
	dir, name := path.Split(p)
	return ParseDir(typ, dir, []string{name})[0]
}

func TestParseMovies(t *testing.T) {
	inception := video(Video{Title: "Inception", Year: 2010})

	tests := []struct {
		name string
		path string
		want File
	}{
		{"in its own folder", "Inception (2010)/Inception (2010).mkv", inception},
		{"loose", "Inception (2010).mp4", inception},
		{"in a collection folder", "Nolan/Inception (2010)/Inception (2010).mkv", inception},
		{"upper-case extension", "Inception (2010).MKV", inception},
		{".m4v", "Inception (2010).m4v", inception},
		{".mov", "Inception (2010).mov", inception},
		{".avi", "Inception (2010).avi", inception},
		{".webm", "Inception (2010).webm", inception},
		{".ts", "Inception (2010).ts", inception},
		{".m2ts", "Inception (2010).m2ts", inception},
		{"title is a number", "1917 (2019)/1917 (2019).mkv", video(Video{Title: "1917", Year: 2019})},
		{"title ends in a number", "Blade Runner 2049 (2017).mkv", video(Video{Title: "Blade Runner 2049", Year: 2017})},
		{"title with parentheses", "Birdman or (The Unexpected Virtue of Ignorance) (2014).mkv",
			video(Video{Title: "Birdman or (The Unexpected Virtue of Ignorance)", Year: 2014})},
		{"turkish title", "Kış Uykusu (2014)/Kış Uykusu (2014).mkv", video(Video{Title: "Kış Uykusu", Year: 2014})},

		// Editions and other {…} tags.
		{"edition", "Blade Runner (1982)/Blade Runner (1982) {edition-Final Cut}.mkv",
			video(Video{Title: "Blade Runner", Year: 1982, Edition: "Final Cut"})},
		{"second edition in the same folder", "Blade Runner (1982)/Blade Runner (1982) {edition-Theatrical}.mkv",
			video(Video{Title: "Blade Runner", Year: 1982, Edition: "Theatrical"})},
		{"edition with apostrophe", "Aliens (1986) {edition-Director's Cut}.mkv",
			video(Video{Title: "Aliens", Year: 1986, Edition: "Director's Cut"})},
		{"imdb tag ignored", "Heat (1995) {imdb-tt0113277}.mkv", video(Video{Title: "Heat", Year: 1995})},
		{"imdb and edition tags", "Heat (1995) {imdb-tt0113277} {edition-Extended}.mkv",
			video(Video{Title: "Heat", Year: 1995, Edition: "Extended"})},
		{"tag in the folder only", "Heat (1995) {imdb-tt0113277}/Heat (1995).mkv", video(Video{Title: "Heat", Year: 1995})},

		// Versions: text after the year.
		{"version 1080p", "Dune (2021)/Dune (2021) - 1080p.mkv", video(Video{Title: "Dune", Year: 2021, Version: "1080p"})},
		{"version 4K", "Dune (2021)/Dune (2021) - 4K.mkv", video(Video{Title: "Dune", Year: 2021, Version: "4K"})},
		{"dotted version", "Dune (2021).1080p.BluRay.mkv", video(Video{Title: "Dune", Year: 2021, Version: "1080p.BluRay"})},
		{"DVD9 is a source, not a part", "Heat (1995) - DVD9.mkv", video(Video{Title: "Heat", Year: 1995, Version: "DVD9"})},
		{"edition and version", "Dune (2021) {edition-IMAX} - 4K.mkv",
			video(Video{Title: "Dune", Year: 2021, Edition: "IMAX", Version: "4K"})},

		// Split files.
		{"pt1", "Gone with the Wind (1939)/Gone with the Wind (1939) - pt1.mkv", skipped(ReasonMovieSplit)},
		{"pt2", "Gone with the Wind (1939)/Gone with the Wind (1939) - pt2.mkv", skipped(ReasonMovieSplit)},
		{"cd2", "Gone with the Wind (1939) - cd2.avi", skipped(ReasonMovieSplit)},
		{"Part 1", "Gone with the Wind (1939) - Part 1.mkv", skipped(ReasonMovieSplit)},
		{"disc1", "Gone with the Wind (1939) - disc1.mkv", skipped(ReasonMovieSplit)},
		{"disk1", "Gone with the Wind (1939) - disk1.mkv", skipped(ReasonMovieSplit)},
		{"dotted pt1", "Gone with the Wind (1939).pt1.mkv", skipped(ReasonMovieSplit)},

		// Names without a year.
		{"scene name", "Inception.2010.1080p.BluRay.x264-GROUP.mkv", skipped(ReasonMovieNoYear)},
		{"no year", "Inception/Inception.mkv", skipped(ReasonMovieNoYear)},
		{"year in folder only", "Inception (2010)/Inception.mkv", skipped(ReasonMovieNoYear)},
		{"year only", "(2010).mkv", skipped(ReasonMovieNoYear)},
		{"year in brackets", "Inception [2010].mkv", skipped(ReasonMovieNoYear)},

		// Samples and extras, skipped quietly, and collections that only share an extras name.
		{"sample.mkv", "Inception (2010)/sample.mkv", ignored},
		{"Sample.mkv", "Inception (2010)/Sample.mkv", ignored},
		{"-sample suffix", "Inception (2010)/Inception (2010)-sample.mkv", ignored},
		{"-trailer suffix", "Inception (2010)/Inception (2010)-trailer.mp4", ignored},
		{"-featurette suffix", "Inception (2010)/Dream Levels-featurette.mkv", ignored},
		{"-behindthescenes suffix", "Inception (2010)/On Set-behindthescenes.mkv", ignored},
		{"-deleted suffix", "Inception (2010)/Limbo-deleted.mkv", ignored},
		{"-interview suffix", "Inception (2010)/Nolan-interview.mkv", ignored},
		{"-scene suffix", "Inception (2010)/Hallway-scene.mkv", ignored},
		{"-short suffix", "Inception (2010)/Something-short.mkv", ignored},
		{"-other suffix", "Inception (2010)/Something-other.mkv", ignored},
		{"Sample folder", "Inception (2010)/Sample/inception.mkv", ignored},
		{"Samples folder", "Inception (2010)/Samples/inception.mkv", ignored},
		{"Featurettes folder", "Inception (2010)/Featurettes/Making Of.mkv", ignored},
		{"Behind The Scenes folder", "Inception (2010)/Behind The Scenes/On Set.mkv", ignored},
		{"Deleted Scenes folder", "Inception (2010)/Deleted Scenes/Limbo.mkv", ignored},
		{"Interviews folder", "Inception (2010)/Interviews/Nolan.mkv", ignored},
		{"Scenes folder", "Inception (2010)/Scenes/Hallway.mkv", ignored},
		{"Shorts folder", "Inception (2010)/Shorts/Short.mkv", ignored},
		{"Trailers folder", "Inception (2010)/Trailers/Inception (2010).mkv", ignored},
		{"Other folder", "Inception (2010)/Other/Clip.mkv", ignored},
		{"Extras folder", "Inception (2010)/Extras/Clip.mkv", ignored},
		{"lower-case extras folder", "Inception (2010)/extras/Clip.mkv", ignored},
		{"extras folder in a collection", "Nolan/Inception (2010)/Featurettes/Making Of.mkv", ignored},
		{"collection named Shorts", "Shorts/Bao (2018).mkv", video(Video{Title: "Bao", Year: 2018})},
		{"collection named Other", "Other/Bao (2018).mkv", video(Video{Title: "Bao", Year: 2018})},
		{"collection named Shorts, nested", "Pixar/Shorts/Bao (2018).mkv", video(Video{Title: "Bao", Year: 2018})},

		// Not videos, or hidden.
		{".nfo", "Inception (2010)/Inception (2010).nfo", ignored},
		{".jpg", "Inception (2010)/poster.jpg", ignored},
		{".iso", "Inception (2010)/Inception (2010).iso", ignored},
		{"no extension", "Inception (2010)/Inception (2010)", ignored},
		{"hidden file", "Inception (2010)/.Inception (2010).mkv", ignored},
		{"macOS ._ file", "Inception (2010)/._Inception (2010).mkv", ignored},
		{".DS_Store", "Inception (2010)/.DS_Store", ignored},
		{"hidden folder", ".Trash-1000/files/Inception (2010).mkv", ignored},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.want.Name = path.Base(tt.path)
			if got := parseOne(Movies, tt.path); got != tt.want {
				t.Errorf("%q:\n got %+v\nwant %+v", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseTVShows(t *testing.T) {
	bb := func(season, episode int, title string) File {
		return video(Video{Title: "Breaking Bad", Year: 2008, Season: season, Episode: episode, EpisodeEnd: episode, EpisodeTitle: title})
	}

	tests := []struct {
		name string
		path string
		want File
	}{
		{"full Plex name", "Breaking Bad (2008)/Season 01/Breaking Bad (2008) - s01e02 - Cat's in the Bag....mkv",
			bb(1, 2, "Cat's in the Bag...")},
		{"no episode title", "Breaking Bad (2008)/Season 01/Breaking Bad (2008) - s01e02.mkv", bb(1, 2, "")},
		{"upper case", "Breaking Bad (2008)/Season 01/Breaking Bad (2008) - S01E02.mkv", bb(1, 2, "")},
		{"scene name", "Breaking Bad (2008)/Season 01/Breaking.Bad.S01E02.720p.BluRay.x264-GROUP.mkv", bb(1, 2, "")},
		{"underscores", "Breaking Bad (2008)/Season 01/Breaking_Bad_S01E02.mkv", bb(1, 2, "")},
		{"show name differs from folder", "Breaking Bad (2008)/Season 01/BB - s01e02 - Pilot.mkv", bb(1, 2, "Pilot")},
		{"three-digit episode", "Breaking Bad (2008)/Season 01/Breaking Bad - s01e102.mkv", bb(1, 102, "")},
		{"en dash before title", "Breaking Bad (2008)/Season 01/Breaking Bad - s01e01 – Pilot.mkv", bb(1, 1, "Pilot")},
		{"no space after dash", "Breaking Bad (2008)/Season 01/Breaking Bad - s01e01 -Pilot.mkv", bb(1, 1, "Pilot")},
		{"release group after dash", "Breaking Bad (2008)/Season 01/Breaking.Bad.S01E01-GROUP.mkv", bb(1, 1, "")},
		{"release group SCENE", "Breaking Bad (2008)/Season 01/Breaking.Bad.S01E01.720p.x264-SCENE.mkv", bb(1, 1, "")},
		{"title ends like an extra", "Breaking Bad (2008)/Season 01/Breaking Bad - s01e01 - Crime-Scene.mkv", bb(1, 1, "Crime-Scene")},
		{"tag in episode title", "Breaking Bad (2008)/Season 01/Breaking Bad - s01e01 - Pilot {tvdb-349232}.mkv", bb(1, 1, "Pilot")},
		{"tag on show folder", "Breaking Bad (2008) {tvdb-81189}/Season 01/Breaking Bad - s01e02.mkv", bb(1, 2, "")},
		{"no year", "Breaking Bad/Season 01/Breaking Bad - s01e02 - Cat's in the Bag.mkv",
			video(Video{Title: "Breaking Bad", Season: 1, Episode: 2, EpisodeEnd: 2, EpisodeTitle: "Cat's in the Bag"})},
		{"turkish show", "Leyla ile Mecnun (2011)/Season 01/Leyla ile Mecnun - s01e01 - Kısmet.mkv",
			video(Video{Title: "Leyla ile Mecnun", Year: 2011, Season: 1, Episode: 1, EpisodeEnd: 1, EpisodeTitle: "Kısmet"})},

		// Season folders.
		{"Season 1 without a zero", "Breaking Bad (2008)/Season 1/Breaking Bad (2008) - s01e02.mkv", bb(1, 2, "")},
		{"lower-case season folder", "Breaking Bad (2008)/season 02/Breaking Bad (2008) - s02e01.mkv", bb(2, 1, "")},
		{"season from the file wins", "Breaking Bad (2008)/Season 01/Breaking Bad (2008) - s02e01.mkv", bb(2, 1, "")},
		{"Specials folder", "Breaking Bad (2008)/Specials/Breaking Bad (2008) - s00e01 - Minisode.mkv", bb(0, 1, "Minisode")},
		{"Season 00 folder", "Breaking Bad (2008)/Season 00/Breaking Bad (2008) - s00e01.mkv", bb(0, 1, "")},
		{"loose in the show folder", "Breaking Bad (2008)/Breaking Bad (2008) - s02e03.mkv", bb(2, 3, "")},

		// Several episodes in one file.
		{"two episodes", "Breaking Bad (2008)/Season 05/Breaking Bad - s05e15-e16 - Finale.mkv",
			video(Video{Title: "Breaking Bad", Year: 2008, Season: 5, Episode: 15, EpisodeEnd: 16, EpisodeTitle: "Finale"})},
		{"two episodes, no dash", "Breaking Bad (2008)/Season 05/Breaking.Bad.S05E15E16.mkv",
			video(Video{Title: "Breaking Bad", Year: 2008, Season: 5, Episode: 15, EpisodeEnd: 16})},
		{"two episodes, season repeated", "Breaking Bad (2008)/Season 05/Breaking Bad - s05e15-s05e16.mkv",
			video(Video{Title: "Breaking Bad", Year: 2008, Season: 5, Episode: 15, EpisodeEnd: 16})},
		{"range of three", "Breaking Bad (2008)/Season 05/Breaking Bad - s05e14-e16.mkv",
			video(Video{Title: "Breaking Bad", Year: 2008, Season: 5, Episode: 14, EpisodeEnd: 16})},
		{"two episodes, bare number", "Breaking Bad (2008)/Season 05/Breaking Bad - s05e15-16 - Finale.mkv",
			video(Video{Title: "Breaking Bad", Year: 2008, Season: 5, Episode: 15, EpisodeEnd: 16, EpisodeTitle: "Finale"})},
		{"range backwards", "Breaking Bad (2008)/Season 05/Breaking Bad - s05e16-e15.mkv", skipped(ReasonTVNoEpisode)},
		{"range across seasons", "Breaking Bad (2008)/Season 05/Breaking Bad - s04e13-s05e01.mkv", skipped(ReasonTVNoEpisode)},

		// Wrong layout.
		{"at the library root", "Breaking Bad - s01e01.mkv", skipped(ReasonTVNoShowFolder)},
		{"season folder at the root", "Season 01/Breaking Bad - s01e01.mkv", skipped(ReasonTVNoShowFolder)},
		{"folder inside a season", "Breaking Bad (2008)/Season 01/Disc 1/Breaking Bad - s01e01.mkv", skipped(ReasonTVBadFolder)},
		{"non-season folder", "Breaking Bad (2008)/Bonus/Breaking Bad - s01e01.mkv", skipped(ReasonTVBadFolder)},
		{"S01 folder", "Breaking Bad (2008)/S01/Breaking Bad - s01e01.mkv", skipped(ReasonTVBadFolder)},
		{"shows in a collection folder", "Drama/Breaking Bad (2008)/Season 01/Breaking Bad - s01e01.mkv", skipped(ReasonTVBadFolder)},

		// Names we don't support.
		{"date-based", "The Daily Show/Season 2024/The Daily Show - 2024-05-01 - Guest.mkv", skipped(ReasonTVDate)},
		{"dotted date", "The Daily Show/The.Daily.Show.2024.05.01.720p.mkv", skipped(ReasonTVDate)},
		{"anime release", "Frieren (2023)/[SubsPlease] Sousou no Frieren - 01 (1080p) [ABCD1234].mkv", skipped(ReasonTVNoEpisode)},
		{"absolute number", "One Piece (1999)/One Piece - 101.mkv", skipped(ReasonTVNoEpisode)},
		{"1x02 style", "Breaking Bad (2008)/Season 01/Breaking Bad - 1x02.mkv", skipped(ReasonTVNoEpisode)},
		{"episode glued to text", "Breaking Bad (2008)/Season 01/Breaking.Bad.s01e02x264.mkv", skipped(ReasonTVNoEpisode)},

		// Extras, samples, hidden.
		{"extras folder in show", "Breaking Bad (2008)/Featurettes/Making Of.mkv", ignored},
		{"extras folder in season", "Breaking Bad (2008)/Season 01/Behind The Scenes/On Set.mkv", ignored},
		{"show named Extras", "Extras/Season 01/Extras - s01e01.mkv",
			video(Video{Title: "Extras", Season: 1, Episode: 1, EpisodeEnd: 1})},
		{"-trailer suffix", "Breaking Bad (2008)/Breaking Bad-trailer.mkv", ignored},
		{"sample", "Breaking Bad (2008)/Season 01/sample.mkv", ignored},
		{"macOS ._ file", "Breaking Bad (2008)/Season 01/._Breaking Bad - s01e01.mkv", ignored},
		{".nfo", "Breaking Bad (2008)/tvshow.nfo", ignored},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.want.Name = path.Base(tt.path)
			if got := parseOne(TVShows, tt.path); got != tt.want {
				t.Errorf("%q:\n got %+v\nwant %+v", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseOtherVideos(t *testing.T) {
	tests := []struct {
		name string
		path string
		want File
	}{
		{"at the root", "Birthday.mp4", video(Video{Title: "Birthday"})},
		{"in a group", "Concerts/Live at Wembley.mkv", video(Video{Title: "Live at Wembley", Group: "Concerts"})},
		{"nested group", "Concerts/2019/Live at Wembley.mkv", video(Video{Title: "Live at Wembley", Group: "Concerts/2019"})},
		{"dots kept", "Holiday.Day.mkv", video(Video{Title: "Holiday.Day"})},
		{"year kept in title", "Family (2015).mkv", video(Video{Title: "Family (2015)"})},
		{"episode-like name kept", "Vlogs/Trip s01e02.mp4", video(Video{Title: "Trip s01e02", Group: "Vlogs"})},
		{"Trailers is a real group", "Trailers/Dune Trailer.mp4", video(Video{Title: "Dune Trailer", Group: "Trailers"})},
		{"sample is a real video", "sample.mkv", video(Video{Title: "sample"})},
		{"-trailer suffix kept", "Dune-trailer.mp4", video(Video{Title: "Dune-trailer"})},
		{"macOS ._ file", "._Birthday.mp4", ignored},
		{"hidden folder", ".hidden/Birthday.mp4", ignored},
		{"not a video", "notes.txt", ignored},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.want.Name = path.Base(tt.path)
			if got := parseOne(OtherVideos, tt.path); got != tt.want {
				t.Errorf("%q:\n got %+v\nwant %+v", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseSubtitles(t *testing.T) {
	const heat = "Heat (1995).mkv"

	tests := []struct {
		name   string
		typ    Type
		dir    string
		videos []string // other files in the folder
		sub    string
		want   File
	}{
		{"2-letter code", Movies, "Heat (1995)", []string{heat}, "Heat (1995).en.srt", subtitle(Subtitle{Video: heat, Lang: "en"})},
		{"3-letter code", Movies, "Heat (1995)", []string{heat}, "Heat (1995).eng.srt", subtitle(Subtitle{Video: heat, Lang: "eng"})},
		{"upper-case code", Movies, "Heat (1995)", []string{heat}, "Heat (1995).TR.srt", subtitle(Subtitle{Video: heat, Lang: "tr"})},
		{"forced", Movies, "Heat (1995)", []string{heat}, "Heat (1995).en.forced.srt", subtitle(Subtitle{Video: heat, Lang: "en", Forced: true})},
		{"sdh", Movies, "Heat (1995)", []string{heat}, "Heat (1995).en.sdh.srt", subtitle(Subtitle{Video: heat, Lang: "en", SDH: true})},
		{"hi after a language", Movies, "Heat (1995)", []string{heat}, "Heat (1995).en.hi.srt", subtitle(Subtitle{Video: heat, Lang: "en", SDH: true})},
		{"sdh and forced", Movies, "Heat (1995)", []string{heat}, "Heat (1995).en.sdh.forced.srt",
			subtitle(Subtitle{Video: heat, Lang: "en", Forced: true, SDH: true})},
		{"hi alone is Hindi", Movies, "Heat (1995)", []string{heat}, "Heat (1995).hi.srt", subtitle(Subtitle{Video: heat, Lang: "hi"})},
		{".vtt", Movies, "Heat (1995)", []string{heat}, "Heat (1995).tr.vtt", subtitle(Subtitle{Video: heat, Lang: "tr"})},
		{".ass", Movies, "Heat (1995)", []string{heat}, "Heat (1995).tr.ass", subtitle(Subtitle{Video: heat, Lang: "tr"})},
		{"upper-case extension", Movies, "Heat (1995)", []string{heat}, "Heat (1995).en.SRT", subtitle(Subtitle{Video: heat, Lang: "en"})},
		{"loose movie", Movies, "", []string{heat}, "Heat (1995).en.srt", subtitle(Subtitle{Video: heat, Lang: "en"})},

		{"region code", Movies, "Heat (1995)", []string{heat}, "Heat (1995).pt-BR.srt", skipped(ReasonSubBadName)},
		{"no language", Movies, "Heat (1995)", []string{heat}, "Heat (1995).srt", skipped(ReasonSubNoLang)},
		{"forced, no language", Movies, "Heat (1995)", []string{heat}, "Heat (1995).forced.srt", skipped(ReasonSubNoLang)},
		{"sdh, no language", Movies, "Heat (1995)", []string{heat}, "Heat (1995).sdh.srt", skipped(ReasonSubNoLang)},
		{"language name", Movies, "Heat (1995)", []string{heat}, "Heat (1995).english.srt", skipped(ReasonSubBadName)},
		{"two languages", Movies, "Heat (1995)", []string{heat}, "Heat (1995).en.fr.srt", skipped(ReasonSubBadName)},
		{"flag before language", Movies, "Heat (1995)", []string{heat}, "Heat (1995).forced.en.srt", skipped(ReasonSubBadName)},
		{"no video next to it", Movies, "Heat (1995)", []string{heat}, "Ronin (1998).en.srt", skipped(ReasonSubNoVideo)},
		{"only a non-video next to it", Movies, "Heat (1995)", []string{"Heat (1995).nfo"}, "Heat (1995).en.srt", skipped(ReasonSubNoVideo)},
		{"next to a skipped video", Movies, "", []string{"Heat (1995) - pt1.mkv"}, "Heat (1995) - pt1.en.srt", ignored},
		{"no language, next to a skipped video", Movies, "", []string{"Heat.mkv"}, "Heat.srt", ignored},
		{"bad name, next to a skipped video", Movies, "", []string{"Heat.mkv"}, "Heat.english.srt", ignored},
		{"of a trailer", Movies, "Heat (1995)", []string{heat, "Heat (1995)-trailer.mkv"}, "Heat (1995)-trailer.en.srt", ignored},
		{"of a sample", Movies, "Heat (1995)", []string{heat, "sample.mkv"}, "sample.srt", ignored},
		{"not a subtitle format", Movies, "Heat (1995)", []string{heat}, "Heat (1995).en.sub", ignored},
		{"Subs folder not read", Movies, "Heat (1995)/Subs", nil, "English.srt", skipped(ReasonSubNoVideo)},
		{"in an extras folder", Movies, "Heat (1995)/Featurettes", []string{"Making Of.mkv"}, "Making Of.en.srt", ignored},
		{"macOS ._ file", Movies, "Heat (1995)", []string{heat}, "._Heat (1995).en.srt", ignored},

		{"episode", TVShows, "Breaking Bad (2008)/Season 01", []string{"Breaking Bad - s01e02.mkv"}, "Breaking Bad - s01e02.en.srt",
			subtitle(Subtitle{Video: "Breaking Bad - s01e02.mkv", Lang: "en"})},
		{"episode next to another episode", TVShows, "Breaking Bad (2008)/Season 01",
			[]string{"Breaking Bad - s01e01.mkv", "Breaking Bad - s01e02.mkv"}, "Breaking Bad - s01e02.tr.srt",
			subtitle(Subtitle{Video: "Breaking Bad - s01e02.mkv", Lang: "tr"})},

		{"dotted video name, no language", OtherVideos, "", []string{"Holiday.Day.mkv"}, "Holiday.Day.srt", skipped(ReasonSubNoLang)},
		{"dotted video name with language", OtherVideos, "", []string{"Holiday.Day.mkv"}, "Holiday.Day.en.srt",
			subtitle(Subtitle{Video: "Holiday.Day.mkv", Lang: "en"})},
		{"longest video name wins", OtherVideos, "", []string{"Talk.mkv", "Talk.en.mkv"}, "Talk.en.tr.srt",
			subtitle(Subtitle{Video: "Talk.en.mkv", Lang: "tr"})},
		{"longest video name wins, even with no language", OtherVideos, "", []string{"Talk.mkv", "Talk.en.mkv"}, "Talk.en.srt",
			skipped(ReasonSubNoLang)},
		{"longest video name wins over a bad name", OtherVideos, "", []string{"Holiday.mkv", "Holiday.Day.mkv"}, "Holiday.Day.srt",
			skipped(ReasonSubNoLang)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names := append(append([]string{}, tt.videos...), tt.sub)
			files := ParseDir(tt.typ, tt.dir, names)
			if len(files) != len(names) {
				t.Fatalf("got %d files, want %d", len(files), len(names))
			}
			tt.want.Name = tt.sub
			if got := files[len(files)-1]; got != tt.want {
				t.Errorf("%q:\n got %+v\nwant %+v", tt.sub, got, tt.want)
			}
		})
	}
}

// The subtitle comes before its video in the listing, as it does in sorted folder listings.
func TestParseDirKeepsOrder(t *testing.T) {
	names := []string{"Heat (1995).en.srt", "Heat (1995).mkv", "Heat (1995).nfo"}
	want := []File{
		{Name: names[0], Kind: KindSubtitle, Subtitle: Subtitle{Video: names[1], Lang: "en"}},
		{Name: names[1], Kind: KindVideo, Video: Video{Title: "Heat", Year: 1995}},
		{Name: names[2], Kind: KindIgnored},
	}
	got := ParseDir(Movies, "Heat (1995)", names)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("file %d:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}
}

func TestParseDirRoot(t *testing.T) {
	for _, dir := range []string{"", ".", "./"} {
		got := ParseDir(OtherVideos, dir, []string{"Birthday.mp4"})[0]
		if got.Kind != KindVideo || got.Video.Group != "" {
			t.Errorf("dir %q: got %+v, want a video with no group", dir, got)
		}
	}
}
