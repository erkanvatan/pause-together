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
		['nothing stored → defaults', null, { volume: 1, muted: false, subtitleSize: 'medium' }],
		['junk → defaults', '{nope', { volume: 1, muted: false, subtitleSize: 'medium' }],
		['not an object → defaults', '3', { volume: 1, muted: false, subtitleSize: 'medium' }],
		['stored values', '{"volume":0.4,"muted":true}', { volume: 0.4, muted: true, subtitleSize: 'medium' }],
		['volume out of range → clamped', '{"volume":7,"muted":false}', { volume: 1, muted: false, subtitleSize: 'medium' }],
		['negative volume → 0', '{"volume":-1}', { volume: 0, muted: false, subtitleSize: 'medium' }],
		[
			'subtitle size',
			'{"volume":1,"muted":false,"subtitleSize":"large"}',
			{ volume: 1, muted: false, subtitleSize: 'large' }
		],
		[
			'unknown subtitle size → default',
			'{"subtitleSize":"huge"}',
			{ volume: 1, muted: false, subtitleSize: 'medium' }
		],
		['wrong types → defaults for those', '{"volume":"loud","muted":"yes"}', { volume: 1, muted: false, subtitleSize: 'medium' }]
	])('%s', (_, stored, want) => {
		const s = storage(stored);
		expect(loadPlayerPrefs(() => s)).toEqual(want);
	});

	it('blocked storage → defaults', () => {
		expect(loadPlayerPrefs(blocked)).toEqual({ volume: 1, muted: false, subtitleSize: 'medium' });
	});
});

describe('savePlayerPrefs', () => {
	it('saves what loads back', () => {
		const s = storage(null);
		const p = { volume: 0.25, muted: true, subtitleSize: 'small' } as const;
		savePlayerPrefs(() => s, p);
		expect(loadPlayerPrefs(() => s)).toEqual(p);
	});

	it('blocked storage → ignored', () => {
		expect(() => savePlayerPrefs(blocked, { volume: 1, muted: false, subtitleSize: 'medium' })).not.toThrow();
	});
});
