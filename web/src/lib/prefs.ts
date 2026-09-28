// Player settings kept per device in localStorage. Everything shared lives in the room's state instead.

export type PlayerPrefs = { volume: number; muted: boolean };

// PrefsStorage is the part of localStorage used here.
export type PrefsStorage = Pick<Storage, 'getItem' | 'setItem'>;

const key = 'pt.player';
const defaults: PlayerPrefs = { volume: 1, muted: false };

// loadPlayerPrefs reads the stored settings. Anything missing or broken gets its default. storage is
// a getter: with site data blocked, even reading window.localStorage throws, which means defaults too.
export function loadPlayerPrefs(storage: () => PrefsStorage): PlayerPrefs {
	let v: unknown;
	try {
		v = JSON.parse(storage().getItem(key) ?? 'null');
	} catch {
		return { ...defaults };
	}
	if (typeof v !== 'object' || v === null) return { ...defaults };
	const { volume, muted } = v as Record<string, unknown>;
	return {
		volume: typeof volume === 'number' && Number.isFinite(volume) ? Math.min(Math.max(volume, 0), 1) : defaults.volume,
		muted: typeof muted === 'boolean' ? muted : defaults.muted
	};
}

// savePlayerPrefs stores the settings. A storage that throws just forgets them.
export function savePlayerPrefs(storage: () => PrefsStorage, p: PlayerPrefs) {
	try {
		storage().setItem(key, JSON.stringify(p));
	} catch {
		// Blocked or full: the settings last until the page closes.
	}
}
