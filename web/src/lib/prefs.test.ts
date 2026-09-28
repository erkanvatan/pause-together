import { describe, expect, it } from 'vitest';
import { loadPlayerPrefs, savePlayerPrefs, type PrefsStorage } from './prefs';

function storage(value: string | null): PrefsStorage & { saved: string | null } {
	return {
		saved: value,
		getItem() {
			return this.saved;
		},
		setItem(_key, v) {
			this.saved = v;
		}
	};
}

// Like window.localStorage with site data blocked: even getting it throws.
const blocked = (): PrefsStorage => {
	throw new DOMException('blocked', 'SecurityError');
};

describe('loadPlayerPrefs', () => {
	it.each([
		['nothing stored → defaults', null, { volume: 1, muted: false }],
		['junk → defaults', '{nope', { volume: 1, muted: false }],
		['not an object → defaults', '3', { volume: 1, muted: false }],
		['stored values', '{"volume":0.4,"muted":true}', { volume: 0.4, muted: true }],
		['volume out of range → clamped', '{"volume":7,"muted":false}', { volume: 1, muted: false }],
		['negative volume → 0', '{"volume":-1}', { volume: 0, muted: false }],
		['wrong types → defaults for those', '{"volume":"loud","muted":"yes"}', { volume: 1, muted: false }]
	])('%s', (_, stored, want) => {
		const s = storage(stored);
		expect(loadPlayerPrefs(() => s)).toEqual(want);
	});

	it('blocked storage → defaults', () => {
		expect(loadPlayerPrefs(blocked)).toEqual({ volume: 1, muted: false });
	});
});

describe('savePlayerPrefs', () => {
	it('saves what loads back', () => {
		const s = storage(null);
		savePlayerPrefs(() => s, { volume: 0.25, muted: true });
		expect(loadPlayerPrefs(() => s)).toEqual({ volume: 0.25, muted: true });
	});

	it('blocked storage → ignored', () => {
		expect(() => savePlayerPrefs(blocked, { volume: 1, muted: false })).not.toThrow();
	});
});
