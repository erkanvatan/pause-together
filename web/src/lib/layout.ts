// Where the player's control bar, subtitles and middle sit over the video box. Pure: sizes in, px out.

export type BarLayoutIn = {
	tall: boolean; // fullscreen held upright: the picture in the box's middle, black above and below
	barOver: boolean; // the bar lies over the box's foot, not in the black under the picture
	panelOver: boolean; // the subtitle panel is open, over the video's foot above the bar
	transport: boolean; // back, play or pause, forward hold the middle, not a card
	barHeight: number; // the bar's, with the subtitle panel where that lies over the video
	boxWidth: number;
	boxHeight: number;
	aspect: number; // the picture's width over its height
};

export type BarLayout = {
	lift: number; // px under the subtitles
	middleBottom: number; // px under what holds the middle
	shade: boolean; // the bar's dark fade
};

// The transport's height with the middle's padding: its play button at its largest (80 px), and 16 px
// above and below.
const TRANSPORT_PX = 112;

export function barLayout(l: BarLayoutIn): BarLayout {
	// In fullscreen held upright: the black under the picture. A phone's often leaves less than the bar
	// takes, and the bar then lies on the picture's foot. None under a picture narrower than the box's
	// shape (a clip filmed upright): it fills the box's height.
	const black = l.tall ? Math.max(0, (l.boxHeight - l.boxWidth / l.aspect) / 2) : 0;
	// The transport centers on the picture, the box's middle, while that leaves it clear of the bar: not
	// when the keyboard has squeezed the box.
	const centered = l.tall && l.transport && l.boxHeight >= 2 * l.barHeight + TRANSPORT_PX;
	return {
		// Subtitles sit in one place, whether the bar shows or not: low over the picture, where the bar
		// covers them while it shows (in fullscreen held upright, on the picture's foot, over the black).
		// Above the bar where it sits under the picture, and above the subtitle panel while it lies over
		// the video, so its timing is set by text in view.
		lift: l.tall ? black : !l.barOver || l.panelOver ? l.barHeight : 0,
		// Otherwise what holds the middle centers above the bar: a card may need more room than a wide
		// film's picture has.
		middleBottom: centered ? 0 : l.barHeight,
		// Where the bar lies on the picture. Not where it sits in the black under it.
		shade: l.barOver && !(l.tall && black >= l.barHeight)
	};
}
