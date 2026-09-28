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
