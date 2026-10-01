// Room helpers for the homepage and the room page. Pure: no fetch, no DOM.
import type { Room, Who } from '$lib/api';
import { videoName, videoTitle } from '$lib/picker';
import type { Wait } from '$lib/protocol';
import { strings } from '$lib/strings';
import { formatTime } from '$lib/time';

// roomTitle names a room in big type: its own name, or else its video's without the version label
// ("1080p.BrRip.x264"). Release tags are noise in a heading; roomSubtitle carries them.
export function roomTitle(r: Room): string {
	return r.name || videoTitle(r.video);
}

// roomSubtitle is the quiet line under a room's title: the whole video name under a room name, or
// else the version label. '' when there's nothing more to say.
export function roomSubtitle(r: Room): string {
	return r.name ? videoName(r.video) : r.video.version;
}

// roomProgress says where a room is in its video, for the homepage: "1:02:13 of 2:34:27", "Not
// started" or "Finished". So rooms with the same video tell apart, and a guest sees where they left off.
export function roomProgress(r: Room): string {
	const { positionMs } = r;
	const { durationMs } = r.video;
	if (positionMs < 1000) return strings.notStarted;
	if (durationMs > 0 && positionMs >= durationMs) return strings.finished;
	return durationMs > 0
		? strings.positionOf(formatTime(positionMs), formatTime(durationMs))
		: formatTime(positionMs);
}

// splitArchived splits rooms into the ones in use and the archived ones, each in the order given.
// busyFirst puts rooms with people in them first: that's where a guest wants to go. Not while
// managing: a room jumping up as someone joins would put another room's Archive under the finger.
export function splitArchived<R extends Room & { watching: Who[] }>(
	rooms: R[],
	busyFirst: boolean
): { active: R[]; archived: R[] } {
	const active = rooms.filter((r) => !r.archived);
	const busy = busyFirst ? active.filter((r) => r.watching.length > 0) : [];
	return {
		active: [...busy, ...active.filter((r) => !busy.includes(r))],
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
// serverMs is the server's clock now, or null before the first pong: then no times. needsTap: this
// page waits for "Tap to join", which says the rest.
export function waitText(
	waiting: Wait[],
	userId: number,
	serverMs: number | null,
	needsTap: boolean
): WaitText {
	const me = waiting.find((w) => w.userId === userId);
	const others = waiting.filter((w) => w.userId !== userId);
	const names = others.map((w) => w.name);
	const lines: string[] = [];
	// Status is per socket, so the room may wait for this person's other tab or device. A page that
	// shows this isn't hidden: away means a fresh tap here, or another screen. Left is an old socket
	// of this page's person, waited for until this one is ready.
	if (me?.reason === 'buffering' || (me?.reason === 'left' && !needsTap)) {
		lines.push(strings.yourVideoLoading);
	} else if (me?.reason === 'away' && !needsTap) {
		lines.push(strings.otherScreenAway);
	}
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
