#!/bin/sh
# Makes tiny test clips with Plex names under testdata/media. Run through `task testdata`, inside the
# dev container, so the clips come from the same ffmpeg as prod. ffmpeg -y overwrites, so a rerun
# needs no clean-up. .done is touched last: the task skips when it's newer than this script.
set -eu

out=testdata/media

# clip PATH ARGS...: 1 s of a test picture, 128x72, plus the given inputs and codecs.
clip() {
	path=$out/$1
	shift
	mkdir -p "$(dirname "$path")"
	ffmpeg -v error -y -f lavfi -i testsrc2=size=128x72:rate=10 "$@" -t 1 "$path"
}

stereo='-f lavfi -i anullsrc=channel_layout=stereo:sample_rate=48000'

# Hidden, so a scan of these folders ignores it.
srt=$out/.hello.srt
mkdir -p "$out"
printf '1\n00:00:00,000 --> 00:00:00,900\nHello\n' >"$srt"

clip 'Movies/Stereo Test (2020)/Stereo Test (2020).mkv' $stereo -i "$srt" \
	-map 0 -map 1 -map 2 -c:v libx264 -pix_fmt yuv420p -c:a aac -c:s srt -metadata:s:s:0 language=eng
clip 'Movies/Surround Test (2021).mkv' -f lavfi -i anullsrc=channel_layout=5.1:sample_rate=48000 \
	-c:v libx264 -pix_fmt yuv420p -c:a ac3
clip 'Movies/Seven One Test (2022).mkv' -f lavfi -i anullsrc=channel_layout=7.1:sample_rate=48000 \
	-c:v libx264 -pix_fmt yuv420p -c:a aac
clip 'Movies/Ten Bit Test (2023).mkv' $stereo -c:v libx264 -pix_fmt yuv420p10le -c:a aac
clip 'TV/Test Show (2024)/Season 01/Test Show (2024) - s01e01 - Pilot.mkv' $stereo \
	-c:v libx265 -pix_fmt yuv420p -x265-params log-level=error -c:a aac

touch "$out/.done"
