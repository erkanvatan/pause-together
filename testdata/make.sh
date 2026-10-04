#!/bin/sh
# Makes tiny test clips with Plex names under testdata/media: one for each case the scan, the picker
# and the player handle, so DEV_MEDIA_ROOT=testdata/media shows them all. Libraries: Movies (movies),
# TV (tv), Other (other). Run through `task testdata`, inside the dev container, so the clips come from
# the same ffmpeg as prod. ffmpeg -y overwrites, so a rerun needs no clean-up. .done is touched last:
# the task skips when it's newer than this script.
#
# Not here, since ffmpeg can't make them: Dolby Vision profile 5 ("Apple devices only") and image
# subtitles (PGS, VobSub: ffmpeg encodes them only from other image subtitles).
set -eu

out=testdata/media

# clip PATH ARGS...: 1 s of a test picture, 128x72, plus the given inputs and codecs.
clip() {
	path=$out/$1
	shift
	mkdir -p "$(dirname "$path")"
	ffmpeg -v error -y -f lavfi -i testsrc2=size=128x72:rate=10 "$@" -t 1 "$path"
}

# file PATH TEXT: a small file that isn't a real video or subtitle.
file() {
	mkdir -p "$(dirname "$out/$1")"
	printf '%s' "$2" >"$out/$1"
}

stereo='-f lavfi -i anullsrc=channel_layout=stereo:sample_rate=48000'
h264='-c:v libx264 -pix_fmt yuv420p'
plain="$stereo $h264 -c:a aac" # plays everywhere

# Hidden, so a scan of these folders ignores it.
srt=$out/.hello.srt
mkdir -p "$out"
printf '1\n00:00:00,000 --> 00:00:00,900\nHello\n' >"$srt"

# Movies: audio. Stereo AAC is copied, the rest becomes stereo AAC.
clip 'Movies/Stereo Test (2020)/Stereo Test (2020).mkv' $stereo -i "$srt" \
	-map 0 -map 1 -map 2 $h264 -c:a aac -c:s srt -metadata:s:s:0 language=eng
clip 'Movies/Surround Test (2021).mkv' -f lavfi -i anullsrc=channel_layout=5.1:sample_rate=48000 \
	$h264 -c:a ac3
clip 'Movies/Seven One Test (2022).mkv' -f lavfi -i anullsrc=channel_layout=7.1:sample_rate=48000 \
	$h264 -c:a aac
clip 'Movies/Two Audio Test (2025).mkv' $stereo -f lavfi -i anullsrc=channel_layout=5.1:sample_rate=48000 \
	-map 0 -map 1 -map 2 $h264 -c:a:0 aac -c:a:1 ac3 \
	-metadata:s:a:0 language=eng -metadata:s:a:1 language=tur

# Movies: video codecs. Playable: H.264, HEVC (the TV pilot), VP9, AV1. The rest are unplayable.
clip 'Movies/Ten Bit Test (2023).mkv' $stereo -c:v libx264 -pix_fmt yuv420p10le -c:a aac
clip 'Movies/VP9 Test (2015).webm' -f lavfi -i anullsrc=channel_layout=mono:sample_rate=48000 \
	-c:v libvpx-vp9 -deadline realtime -cpu-used 8 -c:a libopus
clip 'Movies/AV1 Test (2014).mp4' $stereo -i "$srt" -i "$srt" -map 0 -map 1 -map 2 -map 3 \
	-c:v libaom-av1 -cpu-used 8 -c:a aac -c:s:0 mov_text -c:s:1 ttml \
	-metadata:s:s:0 language=eng -metadata:s:s:1 language=tur
clip 'Movies/VP8 Test (2013).webm' $stereo -c:v libvpx -deadline realtime -c:a libopus
clip 'Movies/MPEG-2 Test (2012).ts' $stereo -c:v mpeg2video -r 25 -c:a mp2
mkdir -p "$out/Movies"
ffmpeg -v error -y -f lavfi -i anullsrc=channel_layout=stereo:sample_rate=48000 -c:a aac -t 1 \
	"$out/Movies/No Video Test (2011).mkv"
file 'Movies/Broken Test (2010).mkv' 'not a video'

# Movies: embedded subtitle tracks, with each flag ffprobe reports.
clip 'Movies/Subtitle Tracks Test (2009).mkv' $stereo -i "$srt" -i "$srt" -i "$srt" -i "$srt" -i "$srt" \
	-map 0 -map 1 -map 2 -map 3 -map 4 -map 5 -map 6 $h264 -c:a aac -metadata:s:a:0 language=eng \
	-c:s srt -c:s:4 ass \
	-metadata:s:s:0 language=eng -metadata:s:s:1 language=eng -metadata:s:s:2 language=eng \
	-metadata:s:s:3 language=tur -metadata:s:s:4 language=ger \
	-disposition:s:0 default -disposition:s:1 forced -disposition:s:2 hearing_impaired \
	-disposition:s:3 0 -disposition:s:4 0

# Movies: names. Editions and versions keep three copies of one film apart.
clip 'Movies/Versions Test (2019)/Versions Test (2019).mkv' $plain
clip "Movies/Versions Test (2019)/Versions Test (2019) {edition-Director's Cut}.mkv" $plain
clip 'Movies/Versions Test (2019)/Versions Test (2019) - 4K.mkv' $plain
clip 'Movies/Test Collection/Collection Test (2018).mp4' $plain
clip 'Movies/Shorts/Shorts Collection Test (2017).mkv' $plain # a collection, not an extras folder
clip 'Movies/No Year Test.mkv' $plain
clip 'Movies/Split Test (2016)/Split Test (2016) - pt1.mkv' $plain
clip 'Movies/Split Test (2016)/Split Test (2016) - cd2.mkv' $plain

# Movies: quietly ignored. Extras, a sample, a macOS ._ file and a non-video.
dir='Movies/Stereo Test (2020)'
clip "$dir/Trailers/Stereo Test Trailer.mkv" $plain
clip "$dir/Stereo Test (2020)-trailer.mkv" $plain
printf '1\n00:00:00,000 --> 00:00:00,900\nTrailer\n' >"$out/$dir/Stereo Test (2020)-trailer.en.srt"
clip "$dir/sample.mkv" $plain
file "$dir/._Stereo Test (2020).mkv" 'macOS metadata'
file "$dir/Stereo Test (2020).nfo" '<movie/>'

# Sidecar subtitles for the stereo clip. In Windows-1254, \376 \360 \375 \335 are ş ğ ı İ.
movie="$out/$dir/Stereo Test (2020)"
for lang in tr tur; do
	printf '1\n00:00:00,000 --> 00:00:00,900\n\376 \360 \375 \335\n' >"$movie.$lang.srt"
done
printf '\357\273\2771\n00:00:00,000 --> 00:00:00,900\nWith a BOM\n' >"$movie.en.srt"
# Quoted EOF: no escapes, so {\an8} stays as written.
cat >"$movie.en.sdh.ass" <<'EOF'
[Script Info]
ScriptType: v4.00+

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, Bold, Italic
Style: Default,Arial,20,&H00FFFFFF,0,0

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:00.00,0:00:00.90,Default,,0,0,0,,{\an8}{\i1}Up top{\i0} and plain
EOF

# More sidecars: flags, WebVTT, Hindi (UTF-8 only), and each way a sidecar is skipped.
dir='Movies/Sidecar Test (2008)'
clip "$dir/Sidecar Test (2008).mkv" $plain -metadata:s:a:0 language=eng
movie="$out/$dir/Sidecar Test (2008)"
cue='1\n00:00:00,000 --> 00:00:00,900\n%s\n'
printf "$cue" 'Plain' >"$movie.en.srt"
printf "$cue" 'Forced' >"$movie.en.forced.srt"
printf "$cue" 'SDH, as hi' >"$movie.en.hi.srt"
printf "$cue" 'नमस्ते' >"$movie.hi.srt"
printf 'WEBVTT\n\n00:00.000 --> 00:00.900\nHallo\n' >"$movie.de.vtt"
printf "$cue" 'No language' >"$movie.srt"
printf "$cue" 'Not a code' >"$movie.english.srt"
printf "$cue" 'No video' >"$out/$dir/Nothing Here (2008).en.srt"
printf '1\n00:00:00,000 --> 00:00:00,900\n\377\376\n' >"$movie.zh.srt" # not UTF-8, and no code page
printf 'no cues here\n' >"$movie.fr.srt"

# TV: a show with two seasons, specials and a loose episode. The pilot is HEVC, for "can't play";
# the rest is H.264. Episodes 2 to 4 have English and Turkish audio and subtitles, for "Next episode"
# carrying both over.
show='TV/Test Show (2024)'
clip "$show/Season 01/Test Show (2024) - s01e01 - Pilot.mkv" $stereo \
	-c:v libx265 -pix_fmt yuv420p -x265-params log-level=error -c:a aac
two="$stereo $stereo -i $srt -i $srt -map 0 -map 1 -map 2 -map 3 -map 4 $h264 -c:a aac -c:s srt
	-metadata:s:a:0 language=eng -metadata:s:a:1 language=tur
	-metadata:s:s:0 language=eng -metadata:s:s:1 language=tur"
clip "$show/Season 01/Test Show (2024) - s01e02-e03 - Double.mkv" $two
clip "$show/Season 2/Test Show (2024) - s02e01.mkv" $two
clip "$show/Test Show (2024) - s02e02 - Loose.mkv" $plain
clip "$show/Specials/Test Show (2024) - s00e01 - Special.mkv" $plain
clip 'TV/No Year Show/Season 01/No Year Show - s01e01.mkv' $plain

# TV: skipped, each with its reason, and an ignored extra.
clip "$show/Season 01/Disc 1/Test Show (2024) - s01e04.mkv" $plain
clip "$show/Season 01/Test Show (2024) - 05.mkv" $plain
clip "$show/Test Show (2024) - 2024-05-01.mkv" $plain
clip 'TV/Loose Show - s01e01.mkv' $plain
clip "$show/Featurettes/Making Of.mkv" $plain

# Other Videos: the file name is the title, folders are groups. A Trailers folder is a real group here.
clip 'Other/Root Clip.mp4' $plain
clip 'Other/Silent Clip.mkv' $h264
clip 'Other/Trailers/Trailer Clip.mkv' $plain
clip 'Other/Holidays/2023/Beach.mkv' $plain

touch "$out/.done"
