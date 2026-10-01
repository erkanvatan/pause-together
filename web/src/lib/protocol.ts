// The room socket's messages: JSON {type: ...}. internal/room/protocol.go defines the same ones; change
// both together. internal/room/testdata/protocol.json holds one of each, checked on both sides.
import type { ChatMessage, Room, SubtitleChoice, Who } from '$lib/api';

// The close code of a socket whose room doesn't exist (any more): deleted while this page was away.
export const CLOSE_NOT_FOUND = 4404;

export type Status = 'ready' | 'buffering' | 'away' | 'cantPlay';

// What a client sends. ping's t is performance.now(), sent back in the pong.
export type ClientMessage =
	| { type: 'ping'; t: number }
	| { type: 'play' }
	| { type: 'pause' }
	| { type: 'seek'; positionMs: number }
	| { type: 'playAnyway' }
	| { type: 'subtitle'; subtitle: SubtitleChoice | null }
	| { type: 'offset'; ms: number }
	| { type: 'status'; status: Status; positionMs: number }
	| { type: 'chat'; text: string; replyTo: number | null } // replyTo: the message it answers
	| { type: 'deleteChat'; id: number };

// Someone the room waits for: why, and since when (server ms).
export type Wait = Who & { reason: 'buffering' | 'away' | 'left'; sinceMs: number };

// A room's playback state. Times are server ms.
export type RoomState = {
	videoId: number;
	audio: number | null;
	subtitle: SubtitleChoice | null;
	subtitleOffsetMs: number;
	durationMs: number; // 0: unknown
	playing: boolean; // what people asked for
	positionMs: number; // at server time atMs
	atMs: number;
	waiting: Wait[]; // who the room waits for
	behind: (Who & { ms: number })[]; // skipped by "Play anyway", in whole seconds
};

// Where the room's prepared copy stands. state '' = no job: the video is gone or can't play, or the
// room is archived.
export type Prepare = {
	state: 'ready' | 'running' | 'queued' | 'failed' | '';
	place: number; // queued: 1 = next in line
	progress: number; // running: 0 to 1
	error: string; // failed: a strings.jobErrors code
	key: string; // ready: the copy's /stream key
	subtitles: number[]; // ready: the embedded subtitle streams the copy has as WebVTT
};

// What the server sends. hello comes first; buildId is the server's web build.
export type ServerMessage =
	| { type: 'hello'; buildId: string; userId: number }
	| { type: 'state'; state: RoomState }
	| { type: 'room'; room: Room }
	| { type: 'presence'; watching: Who[] }
	| { type: 'prepare'; prepare: Prepare }
	| { type: 'paused'; by: Who }
	| { type: 'pong'; t: number; serverMs: number }
	| { type: 'deleted' }
	| { type: 'chatHistory'; messages: ChatMessage[] } // on join: the newest, oldest first
	| { type: 'chat'; message: ChatMessage }
	| { type: 'chatDeleted'; id: number };
