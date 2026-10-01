// Room helpers for the homepage and the room page. Pure: no fetch, no DOM.
import type { Room, Who } from '$lib/api';
import { videoName } from '$lib/picker';
import type { Wait } from '$lib/protocol';
import { strings } from '$lib/strings';
import { formatTime } from '$lib/time';

// roomTitle names a room in big type: its own name, or else its video's without the version label
// ("1080p.BrRip.x264"). Release tags are noise in a heading; roomSubtitle carries them.
export function roomTitle(r: Room): string {
	return r.name || videoName({ ...r.video, version: '' });
}

// roomSubtitle is the quiet line under a room's title: the whole video name under a room name, or
// else the version label. '' when there's nothing more to say.
export function roomSubtitle(r: Room): string {
	return r.name ? videoName(r.video) : r.video.version;
}

// splitArchived splits rooms into the ones in use and the archived ones, each in the order given,
// except that rooms with people in them come first: that's where a guest wants to go.
export function splitArchived<R extends Room & { watching: Who[] }>(
	rooms: R[]
): { active: R[]; archived: R[] } {
	const active = rooms.filter((r) => !r.archived);
	return {
		active: [...active.filter((r) => r.watching.length > 0), ...active.filter((r) => !r.watching.length)],
		archived: rooms.filter((r) => r.archived)
	};
}

// needsConfirm says whether a switch asks first ("You're at 1:40:00. Switch to …?"). Not when nothing
// is lost: at 0:00, at the end, or when the video is missing, since that swap keeps the position.
export function needsConfirm(positionMs: number, durationMs: number, missing: boolean): boolean {
	if (missing || positionMs < 1000) return false;
	return durationMs === 0 || positionMs < durationMs;
}

// WaitText is what the "Waiting for …" panel says to one person.
export type WaitText = {
	headline: string;
	lines: string[]; // why, and for how long: one line, or one per person when it waits for several
	action: string; // the "Play anyway" button
};

// waitText says who the room waits for, as userId sees it: the person it waits for hears it's them.
// serverMs is the server's clock now, or null before the first pong: then no times.
export function waitText(waiting: Wait[], userId: number, serverMs: number | null): WaitText {
	const me = waiting.find((w) => w.userId === userId);
	const others = waiting.filter((w) => w.userId !== userId);
	const names = others.map((w) => w.name);
	const lines: string[] = [];
	// Away, on a page that shows this, means the browser wants a fresh tap: "Tap to join" asks for it.
	if (me && me.reason !== 'away') lines.push(strings.yourVideoLoading);
	for (const w of others) {
		const reason = strings.waitReasons[w.reason];
		const time = serverMs === null ? '' : formatTime(serverMs - w.sinceMs);
		lines.push(
			others.length > 1 || me ? strings.waitLineNamed(w.name, reason, time) : strings.waitLine(reason, time)
		);
	}
	return {
		headline: me ? strings.waitingForYou : strings.waitingFor(names),
		lines,
		action: !me ? strings.playWithout(names) : others.length ? strings.playAnyway : strings.dontWaitForMe
	};
}
