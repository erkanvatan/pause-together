// Every piece of UI text lives here, so a Turkish version is one more object later.
export const strings = {
	locale: 'en', // for language names, and sorting titles
	appName: 'PauseTogether',
	tagline: "Distance can't pause us.",

	namePrompt: 'What should we call you?',
	namePlaceholder: 'Your name',
	save: 'Save',
	cancel: 'Cancel',
	rename: 'Change name',
	nameInvalid: 'Use 1 to 32 letters, numbers, spaces or punctuation. No emoji.',
	saveFailed: "Couldn't save your name. Try again.",
	loadFailed: "Can't reach the server. Retrying…",

	admin: 'Admin',
	hostOnly: 'Only the host can open this page.',
	backHome: 'Back home',
	actionFailed: "That didn't work. Try again.",

	libraries: 'Libraries',
	noLibraries: 'No libraries yet. Add a folder below.',
	libraryTypes: { movies: 'Movies', tv: 'TV Shows', other: 'Other Videos' },
	videoCount: (n: number) => (n === 1 ? '1 video' : `${n} videos`),
	scanQueued: 'Waiting to scan…',
	scanLooking: 'Looking for files…',
	scanProgress: (done: number, total: number) => `Scanning ${done} of ${total}…`,
	scanFailed: 'Last scan failed:',
	rescan: 'Rescan',
	remove: 'Remove',
	removeConfirm: 'Remove? Its videos stay, marked missing, until you add the folder again.',

	addLibrary: 'Add a library',
	mediaFolder: 'Media folder',
	noFolders: 'No folders in here.',
	foldersFailed: "Can't open this folder.",
	libraryType: 'Type',
	add: 'Add this folder',
	addErrors: {
		overlap: 'This folder is, holds, or sits inside another library.',
		'not-folder': 'Pick a folder inside the media folder.',
		'bad-type': 'Pick a type.',
		failed: "Couldn't add the library. Try again."
	} as Record<string, string>,

	languageDefaults: 'Language defaults',
	languageDefaultsNote: 'The picker preselects these. Anyone can still change them for each video.',
	audioLanguage: 'Audio',
	audioLanguageHint: "A language code like en. Blank: the file's own default track.",
	subtitleLanguages: 'Subtitles',
	subtitleLanguagesHint:
		"Language codes in order of preference, like tr, en. Blank or no match: the file's own default track, if any.",
	original: 'Original',
	none: 'None',
	saved: 'Saved.',
	langErrors: {
		'bad-lang': 'Use 2- or 3-letter language codes like tr or en, at most 10.',
		failed: "Couldn't save. Try again."
	} as Record<string, string>,

	cantUse: "Files we can't use",
	allUsable: 'Every file is usable.',
	appleOnly: 'Apple devices only',
	appleOnlyNote: 'Dolby Vision profile 5. Other screens show it purple and green.',

	jobs: 'Prepare jobs',
	noJobs: 'Nothing to prepare.',
	diskUsage: (cache: string, free: string) => `Cache ${cache} · ${free} free on its disk`,
	jobRunning: (percent: number) => `Preparing… ${percent}%`,
	jobQueued: (place: number) =>
		place === 1 ? 'Queued, next in line' : `Queued, ${ordinal(place)} in line`,
	// Why a prepare job failed (media.FailNoSpace, media.FailPrepare).
	jobErrors: {
		'no-space': 'Not enough free disk space.',
		failed: 'Prepare failed.'
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
		'probe-failed': "ffprobe couldn't read the file."
	},

	watchSomething: 'Watch something',
	pickVideo: 'Pick a video',
	close: 'Close',
	back: 'Back',
	search: 'Search',
	sortTitle: 'A–Z',
	sortRecent: 'Recently added',
	noVideos: 'No videos yet. The host adds libraries on the admin page.',
	noMatches: 'Nothing matches.',
	season: (n: number) => (n === 0 ? 'Specials' : `Season ${n}`),
	seasonCount: (n: number) => (n === 1 ? '1 season' : `${n} seasons`),
	cantPlayHere: (codec: string) => `This device can't play ${codec}.`,
	appleOnlyPick: 'Dolby Vision 5: only Apple devices show the right colors.',
	audio: 'Audio',
	noAudio: 'No audio',
	subtitle: 'Subtitles',
	subtitleOff: 'Off',
	start: 'Start',
	unknownLanguage: 'Unknown language',
	forced: 'Forced',
	sdh: 'SDH',
	sidecarFile: 'File',
	channels: (n: number) =>
		({ 1: 'Mono', 2: 'Stereo', 6: '5.1', 8: '7.1' })[n] ?? (n > 0 ? `${n} channels` : ''),

	// Why a subtitle track can't be shown (media.SubtitleUnavailable).
	subtitleUnavailable: {
		image: 'Picture subtitles (PGS, VobSub) are not supported.',
		codec: 'This subtitle format is not supported.'
	}
};

// reasonText turns a reason code (strings.reasons) into text. Unknown codes show as they are.
export function reasonText(code: string): string {
	const reasons: Record<string, string> = strings.reasons;
	return reasons[code] ?? code;
}

// ordinal writes 1st, 2nd, 3rd, 4th, 11th, 21st…
function ordinal(n: number): string {
	const tens = n % 100;
	const suffix = tens >= 11 && tens <= 13 ? 'th' : ({ 1: 'st', 2: 'nd', 3: 'rd' }[n % 10] ?? 'th');
	return `${n}${suffix}`;
}
