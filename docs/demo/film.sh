#!/bin/sh
# Downloads Big Buck Bunny (276 MB, CC BY 3.0, Blender Foundation) into $1/media with a Plex name.
# Run through `task demo`, in Playwright's image: it has curl, and python3 for the zip.
set -eu

out=$1
dir="$out/media/Movies/Big Buck Bunny (2008)"
mkdir -p "$dir"
curl -fsSL -o "$out/film.zip" \
	https://download.blender.org/demo/movies/BBB/bbb_sunflower_1080p_30fps_normal.mp4.zip
python3 -c '
import shutil, sys, zipfile
z = zipfile.ZipFile(sys.argv[1])
with z.open(z.namelist()[0]) as src, open(sys.argv[2], "wb") as dst:
    shutil.copyfileobj(src, dst)
' "$out/film.zip" "$dir/Big Buck Bunny (2008).mp4.part"
mv "$dir/Big Buck Bunny (2008).mp4.part" "$dir/Big Buck Bunny (2008).mp4"
rm "$out/film.zip"
