#!/bin/sh
# Makes the library the README's screenshots show, in $1/media: Blender's open films with their real
# lengths, as plain grey clips. The pages show only names and lengths, so the picture doesn't matter.
# Run through `task screenshots`, inside the dev container. $1/.done is touched last: the task skips
# this script when it is newer. Libraries: Movies (movies), TV (tv), Home Videos (other).
set -eu

out=$1/media

# clip PATH SECONDS: a grey clip at 1 fps with silent stereo AAC, so prepare copies both.
clip() {
	mkdir -p "$(dirname "$out/$1")"
	ffmpeg -v error -y -f lavfi -i color=c=0x222222:s=128x72:r=1 \
		-f lavfi -i anullsrc=channel_layout=stereo:sample_rate=48000 \
		-c:v libx264 -preset ultrafast -pix_fmt yuv420p -c:a aac -t "$2" "$out/$1"
}

clip 'Movies/Big Buck Bunny (2008)/Big Buck Bunny (2008).mp4' 596
clip 'Movies/Elephants Dream (2006)/Elephants Dream (2006).mkv' 654
clip 'Movies/Sintel (2010)/Sintel (2010).mkv' 888
clip 'Movies/Tears of Steel (2012)/Tears of Steel (2012).mkv' 734
clip 'Movies/Cosmos Laundromat (2015)/Cosmos Laundromat (2015).mkv' 730
clip 'Movies/Spring (2019)/Spring (2019).mkv' 464
clip 'Movies/Sprite Fright (2021)/Sprite Fright (2021).mkv' 629

show='TV/Caminandes (2013)/Season 01'
clip "$show/Caminandes (2013) - s01e01 - Llama Drama.mkv" 90
clip "$show/Caminandes (2013) - s01e02 - Gran Dillama.mkv" 146
clip "$show/Caminandes (2013) - s01e03 - Llamigos.mkv" 150

clip 'Home Videos/Wedding.mp4' 1810
clip 'Home Videos/Summer 2024/Beach Day.mp4' 412
clip 'Home Videos/Summer 2024/Ferry Ride.mp4' 95

# Misnamed, so the admin page has files we can't use.
clip 'Movies/Night of the Living Dead.mkv' 5
clip 'Movies/Metropolis (1927)/Metropolis (1927) - pt1.mkv' 5
clip 'Movies/Metropolis (1927)/Metropolis (1927) - pt2.mkv' 5

touch "$1/.done"
