#!/bin/sh
# Joins record.mjs's two videos side by side, from where the story starts, into docs/demo.webp. Run
# through `task demo`, inside the dev container. Animated WebP, not GIF: GIF's 256 colors speckle the
# film, and at full size and 15 fps it comes out six times bigger.
set -eu

out=$1
. "$out/times.env"

ffmpeg -v error -y -i "$out/laptop.webm" -i "$out/phone.webm" -filter_complex "
  [0]trim=start=$laptop:duration=$duration,setpts=PTS-STARTPTS,fps=15,pad=iw+4:ih+4:2:2:0x3a3358[l];
  [1]trim=start=$phone:duration=$duration,setpts=PTS-STARTPTS,fps=15,pad=iw+4:ih+4:2:2:0x3a3358[p];
  color=c=0x0b0916:s=1464x692:r=15:d=$duration[bg];
  [bg][l]overlay=24:24[t];[t][p]overlay=1076:24" \
	-c:v libwebp_anim -quality 90 -compression_level 6 -loop 0 docs/demo.webp
