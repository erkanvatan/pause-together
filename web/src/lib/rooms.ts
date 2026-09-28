// Room helpers for the homepage and the room page. Pure: no fetch, no DOM.
import type { Room } from '$lib/api';
import { videoName } from '$lib/picker';

// roomTitle names a room: its own name, or else its video's.
export function roomTitle(r: Room): string {
	return r.name || videoName(r.video);
}

// splitArchived splits rooms into the ones in use and the archived ones, each in the order given.
export function splitArchived<R extends Room>(rooms: R[]): { active: R[]; archived: R[] } {
	return {
		active: rooms.filter((r) => !r.archived),
		archived: rooms.filter((r) => r.archived)
	};
}

// needsConfirm says whether a switch asks first ("You're at 1:40:00. Switch to …?"). Not when nothing
// is lost: at 0:00, at the end, or when the video is missing, since that swap keeps the position.
export function needsConfirm(positionMs: number, durationMs: number, missing: boolean): boolean {
	if (missing || positionMs < 1000) return false;
	return durationMs === 0 || positionMs < durationMs;
}
