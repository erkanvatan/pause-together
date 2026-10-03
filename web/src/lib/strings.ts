const nameInvalid = 'Use 1 to 32 letters, numbers, spaces or punctuation. No emoji.';

// Every piece of UI text lives here, so a Turkish version is one more object later.
export const strings = {
	locale: 'en', // for language names, sorting titles, and joining names into a list
	appName: 'PauseTogether',
	tagline: "Distance can't pause us.",

	namePrompt: 'What should we call you?',
	nameWhy: "Pick a name so the others know it's you.",
	joining: (room: string) => `You're joining ${room}.`,
	namePlaceholder: 'Your name',
	save: 'Save',
	continue: 'Continue',
	cancel: 'Cancel',
	rename: 'Change name',
	nameInvalid,
	saveFailed: "Couldn't save your name. Try again.",
	loadFailed: "Can't reach the server. Retrying…",

	admin: 'Admin',
	guestLink: 'Guest link',
	copy: 'Copy',
	copied: 'Copied',
	copyFailed: "Couldn't copy",
	noGuestLink:
		"No guest link yet. Set PUBLIC_BIND in .env to this machine's Tailscale IP, then run task up.",
	hostOnly: 'Only the host can open this page.',
	backHome: 'Back home',
	actionFailed: "That didn't work. Try again.",

	libraries: 'Libraries',
	noLibraries: 'No libraries yet. Pick the folder that holds your videos.',
	libraryTypes: { movies: 'Movies', tv: 'TV Shows', other: 'Other Videos' },
	videoCount: (n: number) => (n === 1 ? '1 video' : `${n} videos`),
	libraryLine: (type: string, videos: string) => `${type}, ${videos}`,
	scanQueued: 'Waiting to scan…',
	scanLooking: 'Looking for files…',
	scanProgress: (done: number, total: number) => `Scanning ${done} of ${total}…`,
	scanFailed: 'Last scan failed:',
	folderGone:
		"This folder is gone or empty. Was it moved or renamed, or is its drive not mounted? Put it back, or remove this library.",
	rescan: 'Rescan',
	remove: 'Remove',
	removeConfirm: 'Remove? Its videos stay, marked missing, until you add the folder again.',

	addLibrary: 'Add a library',
	mediaFolder: 'Media folder',
	noFolders: 'No folders in here.',
	foldersFailed: "Can't open this folder.",
	libraryType: 'Type',
	add: 'Add this folder',
	addAs: (folder: string, type: string) => `Add ${folder} as ${type}`,
	openFolderHint: 'Open the folder that holds your videos, then add it.',
	// How each type expects its files named. Files that don't fit show under "Files we can't use".
	libraryTypeHints: {
		movies: 'Names like "Title (Year).mkv", alone or in a "Title (Year)" folder.',
		tv: 'A folder per show, with names like "Show/Season 01/Show - s01e02.mkv".',
		other: 'Any names. Sub-folders become groups in the picker.'
	},
	addErrors: {
		overlap: 'This folder is, holds, or sits inside another library.',
		'not-folder': 'Pick a folder inside the media folder.',
		'bad-type': 'Pick a type.',
		failed: "Couldn't add the library. Try again."
	} as Record<string, string>,

	languageDefaults: 'Language defaults',
	languageDefaultsNote:
		'New picks start with these. Anyone can change the audio when picking, and the subtitle in the player.',
	audioLanguage: 'Audio',
	audioLanguageHint: "A language code like en. Blank: the file's own default track.",
	subtitleLanguages: 'Subtitles',
	subtitleLanguagesHint:
		"Language codes in order of preference, like tr, en. Blank or no match: the file's own default track, if any.",
	original: 'Original',
	none: 'None',
	saved: 'Saved.',
	readsAs: (what: string) => `Reads as: ${what}`,
	langErrors: {
		'bad-lang': 'Use 2- or 3-letter language codes like tr or en, at most 10.',
		failed: "Couldn't save. Try again."
	} as Record<string, string>,

	cacheCleanup: 'Cache clean-up',
	unusedDays: 'Delete prepared copies unused for (days)',
	unusedDaysHint: 'From 1 to 365. Opening a room prepares its copy again.',
	cacheErrors: {
		'bad-days': 'Use a whole number of days from 1 to 365.',
		failed: "Couldn't save. Try again."
	} as Record<string, string>,

	cantUse: "Files we can't use",
	allUsable: 'Every file is usable.',
	fileCount: (n: number) => (n === 1 ? '1 file' : `${n} files`),
	appleOnly: 'Apple devices only',
	appleOnlyNote: 'Dolby Vision profile 5. Other screens show it purple and green.',

	jobs: 'Prepare jobs',
	jobsNote:
		"Before a room can play, its video is copied once into the cache: same quality, in a form every browser can seek. One copy at a time.",
	noJobs: 'Nothing to prepare.',
	diskUsage: (cache: string, free: string) => `Cache ${cache}, with ${free} free on its disk`,
	clearCache: 'Clear cache',
	clearCacheConfirm:
		'Delete every prepared copy? Anyone watching now is cut off, and each room prepares its video again when opened.',
	jobRunning: (percent: number) => `Preparing… ${percent}%`,
	jobQueued: (place: number) =>
		place === 1 ? 'Queued, next in line' : `Queued, ${ordinal(place)} in line`,
	// Under "Preparing…" in the video box: what the wait is, for a guest who never heard of prepare jobs.
	preparingWhy: "The host's computer is getting this video ready to stream. You can chat meanwhile.",
	// Why a prepare job failed (media.FailNoSpace, media.FailPrepare).
	jobErrors: {
		'no-space': 'Not enough free disk space.',
		failed: "Couldn't prepare this video. The file may be damaged: check that it plays on this computer."
	} as Record<string, string>,

	// Why a file can't be used: skipped by the scan (library.Reason) or unplayable (media.Unplayable).
	reasons: {
		'movie-no-year': 'Movie name has no year. Use "Title (Year)".',
		'movie-split': 'One part of a split movie. Join the parts into one file.',
		'tv-no-show-folder': 'Episode is not inside a show folder.',
		'tv-bad-folder': 'Episode is not in "Show/Season 01/" or loose in the show folder.',
		'tv-no-episode': 'No season and episode (s01e02) in the name.',
		'tv-date': 'Date-based episode names are not supported.',
		'sub-no-video': 'Subtitle has no video with a matching name.',
		'sub-bad-name': "Text after the video's name is not a language code.",
		'sub-no-lang': 'Subtitle name has no language code. Use "Title (Year).en.srt".',
		'sub-unreadable': "Subtitle couldn't be read, or holds no subtitles.",
		'no-video': 'No video stream.',
		codec: "Browsers can't play this codec.",
		'h264-profile': "H.264 that browsers can't decode (10-bit, 4:2:2 or 4:4:4).",
		'probe-failed': "Couldn't read the file. It may be damaged, or still downloading: rescan once it's done."
	},

	watchSomething: 'Watch something',
	join: 'Join',
	joinRoom: (title: string) => `Join ${title}`,
	manageRooms: 'Manage',
	doneManaging: 'Done',
	rooms: 'Rooms',
	noRooms: 'No rooms yet. Pick something to watch.',
	archivedRooms: 'Archived',
	archive: 'Archive',
	unarchive: 'Unarchive',
	deleteRoom: 'Delete',
	deleteRoomConfirm: 'Delete this room for good?',
	videoMissing: 'Video missing',
	// The homepage's folded group of rooms whose video is gone.
	videoMissingRooms: (n: number) => `Video missing (${n})`,
	noPlayableRooms: 'None of the rooms can play right now.',
	// When a room was last used, on its homepage card.
	ago: (n: number, unit: Intl.RelativeTimeFormatUnit) =>
		// 'always': a floored day count of 1 can be two calendar days back, so never "yesterday".
		new Intl.RelativeTimeFormat(strings.locale, { numeric: 'always' }).format(-n, unit),
	justNow: 'just now',
	// Where a room is in its video, on the homepage; otherwise positionOf: "1:02:13 of 2:34:27".
	notStarted: 'Not started',
	finished: 'Finished',
	// The "Video missing" panel: the new pick resumes where the room was.
	videoGone: (at: string) =>
		at
			? `The file is gone from the host's library. Pick another video and everyone carries on from ${at}.`
			: "The file is gone from the host's library. Pick another video to watch.",
	roomNotFound: 'This room is gone.',
	roomArchived: 'This room is archived. Unarchive it to watch.',
	switchVideo: 'Switch video',
	renameRoom: 'Rename room',
	roomActions: 'More',
	roomName: 'Room name',
	roomNameHint: "Leave it blank to show the video's name.",
	// Why a room change failed (the room API's error codes). Others show actionFailed.
	roomErrors: {
		'bad-pick': "That video can't be picked any more. Try another.",
		'bad-name': nameInvalid,
		archived: 'This room is archived.',
		watching: 'Someone is in this room. Archive it once it is empty.'
	} as Record<string, string>,
	roomDeleted: 'That room was deleted.',
	watchingNow: 'Watching now',
	watchingList: (names: string) => `Watching: ${names}`,
	tapToJoin: 'Tap to join',
	clickToJoin: 'Click to join',
	play: 'Play',
	pause: 'Pause',
	skipBack: (s: number) => `Back ${s} seconds`,
	skipForward: (s: number) => `Forward ${s} seconds`,
	mute: 'Mute',
	unmute: 'Unmute',
	volume: 'Volume',
	position: 'Position',
	// The seek bar and volume as a screen reader says them: "1:02:13 of 2:34:27", "80%".
	positionOf: (at: string, total: string) => `${at} of ${total}`,
	percent: (n: number) => new Intl.NumberFormat(strings.locale, { style: 'percent' }).format(n),
	// A control's tooltip with its key: "Play (Space)".
	withKey: (label: string, key: string) => `${label} (${key})`,
	// The player's keys as its tooltips name them.
	keys: { play: 'Space', back: '←', forward: '→', mute: 'M', subtitles: 'C', chat: 'H', fullscreen: 'F' },
	playAnyway: 'Play anyway',
	playWithout: (names: string[]) => `Play without ${list(names)}`,
	dontWaitForMe: "Don't wait for me",
	waitingFor: (names: string[]) => `Waiting for ${list(names)}`,
	waitingForYou: "Everyone's waiting for you",
	yourVideoLoading: 'Your video is still loading…',
	otherScreenAway: 'Another of your screens stepped away.',
	// Why the room waits for someone (protocol Wait reasons), and for how long: "stepped away · 0:12".
	waitReasons: { buffering: 'loading', away: 'stepped away', left: 'left the room' },
	waitLine: (reason: string, time: string) => (time ? `${reason} · ${time}` : reason),
	waitLineNamed: (name: string, reason: string, time: string) =>
		time ? `${name}: ${reason} · ${time}` : `${name}: ${reason}`,
	behind: (name: string, ms: number) => `${name} is ${Math.round(ms / 1000)} s behind`,
	pausedBy: (name: string) => `${name} paused`,
	hostOffline: 'Host is offline, reconnecting…',
	hostOfflineWhy: 'Play, pause and chat come back with it.',
	gettingReady: 'Getting the room ready…',
	fullscreen: 'Fullscreen',
	exitFullscreen: 'Exit fullscreen',
	subtitleTiming: 'Timing',
	subtitleSooner: 'Show subtitles sooner',
	subtitleLater: 'Show subtitles later',
	// A subtitle offset: "+0.25 s", "−1.5 s", "0 s".
	subtitleOffset: (ms: number) => `${ms > 0 ? '+' : ms < 0 ? '−' : ''}${Math.abs(ms) / 1000} s`,
	reset: 'Reset',
	subtitleSize: 'Size',
	forEveryone: 'For everyone',
	onThisScreen: 'On this screen',
	subtitleSizes: { small: 'Small', medium: 'Medium', large: 'Large' },
	subtitleFailed: "Couldn't load the subtitles.",
	subtitleRetrying: "Couldn't load the subtitles. Retrying…",
	notInCopy: 'not in the prepared copy',
	// A subtitle choice with why it can't be picked.
	withNote: (label: string, note: string) => `${label} (${note})`,
	nextEpisode: 'Next episode',
	nextEpisodeNamed: (code: string, title: string) =>
		title ? `Next episode: ${code} ${title}` : `Next episode: ${code}`,
	chat: 'Chat',
	// A new chat message, as a screen reader hears it.
	said: (name: string, text: string) => `${name}: ${text}`,
	closeChat: 'Close chat',
	noMessages: 'No messages yet.',
	messagePlaceholder: 'Message',
	send: 'Send',
	reply: 'Reply',
	deleteMessage: 'Delete',
	deleteMessageConfirm: 'Delete for everyone?',
	deletedMessage: 'Deleted message',
	replyingTo: (name: string) => `Replying to ${name}`,
	cancelReply: 'Cancel reply',
	chatReadOnly: 'The chat is read-only.',
	chatOffline: "Can't send while the host is offline. Try again when it's back.",
	olderFailed: "Couldn't load older messages.",
	// Where the video was when a message was sent.
	inVideoAt: (at: string) => `At ${at} in the video`,
	// When a message was sent, on the wall clock: "Sep 28, 2026, 9:41 PM".
	sentAt: (ms: number) =>
		new Intl.DateTimeFormat(strings.locale, { dateStyle: 'medium', timeStyle: 'short' }).format(ms),
	switchConfirm: (at: string, name: string) => `You're at ${at}. Switch to ${name}?`,
	switchAction: 'Switch',
	pickAnother: 'Pick another video',
	theEnd: 'The end',
	episodeEnded: (code: string) => `End of ${code}`,
	watchSomethingElse: 'Watch something else',
	pickVideo: 'Pick a video',
	close: 'Close',
	back: 'Back',
	search: 'Search',
	sort: 'Sort',
	sortTitle: 'A–Z',
	sortRecent: 'Recently added',
	noVideos: 'No videos yet. Ask the host to add some.',
	noVideosHost:
		"No videos yet. Add the folders that hold your movies and shows on the admin page. It also lists files it couldn't use.",
	addVideos: 'Open the admin page',
	noMatches: 'Nothing matches.',
	season: (n: number) => (n === 0 ? 'Specials' : `Season ${n}`),
	seasonCount: (n: number) => (n === 1 ? '1 season' : `${n} seasons`),
	cantPlayHere: (codec: string) => `This device can't play ${codec}.`,
	// Under "can't play" in the video box: what's left to do, by codec (codecName's names). A device that
	// can't play never holds up the room, and its play and seek are off.
	cantPlayHereWhy: (codec: string) =>
		`${
			({
				HEVC: 'Safari on an iPhone, iPad or Mac usually plays it.',
				AV1: 'A recent phone or computer usually plays it.',
				VP9: 'Chrome, Edge or Firefox usually play it.'
			} as Record<string, string>)[codec] ?? 'Another device may play it.'
		} The room won't wait for this screen, and chat still works.`,
	cantPlayAnywhere: 'No screen can play this file. Pick another video to watch.',
	// A room's prepare failed (media.FailNoSpace, media.FailPrepare), in the video box.
	prepareFailed: "Couldn't get this video ready",
	prepareFailedWhy: {
		'no-space': "The host's computer is out of disk space. Pick another video, or ask the host to free some.",
		failed: "Something went wrong on the host's computer. Pick another video, or ask the host to check the admin page."
	} as Record<string, string>,
	appleOnlyPick: 'Dolby Vision 5: only Apple devices show the right colors.',
	audio: 'Audio',
	subtitle: 'Subtitles',
	subtitleOff: 'Off',
	start: 'Start',
	// A picked video that already has a room: join it, or make another.
	videoHasRooms: (n: number) =>
		n === 1 ? 'This already has a room. Join it to watch together.' : 'This already has rooms. Join one to watch together.',
	startNewRoom: 'Start a new room',
	// Under a video in the homepage's picker: where its one room is, or how many it has.
	inRoom: (progress: string) => `In a room · ${progress}`,
	inRooms: (n: number) => `In ${n} rooms`,
	unknownLanguage: 'Unknown language',
	forced: 'Forced',
	sdh: 'SDH',
	sidecarFile: 'Separate file',
	channels: (n: number) =>
		({ 1: 'Mono', 2: 'Stereo', 6: '5.1', 8: '7.1' })[n] ?? (n > 0 ? `${n} channels` : ''),

	// Why a subtitle track can't be shown (media.SubtitleUnavailable), as a note in the player's menu.
	subtitleUnavailable: {
		image: 'picture subtitles, not supported',
		codec: 'format not supported'
	} as Record<string, string>
};

// reasonText turns a reason code (strings.reasons) into text. Unknown codes show as they are.
export function reasonText(code: string): string {
	const reasons: Record<string, string> = strings.reasons;
	return reasons[code] ?? code;
}

// list joins names: "Alice", "Alice and Bob", "Alice, Bob, and Carol".
function list(names: string[]): string {
	return new Intl.ListFormat(strings.locale, { type: 'conjunction' }).format(names);
}

// ordinal writes 1st, 2nd, 3rd, 4th, 11th, 21st…
function ordinal(n: number): string {
	const tens = n % 100;
	const suffix = tens >= 11 && tens <= 13 ? 'th' : ({ 1: 'st', 2: 'nd', 3: 'rd' }[n % 10] ?? 'th');
	return `${n}${suffix}`;
}
