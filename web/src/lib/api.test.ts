import { describe, expect, it } from 'vitest';
import { isNameReason } from './api';

describe('isNameReason', () => {
	it.each([
		['name-symbol', true],
		['name-invalid', true],
		['bad-request', false],
		['not-found', false],
		['', false],
		[undefined, false],
		[null, false],
		[42, false]
	])('%j is %s', (code, want) => {
		expect(isNameReason(code)).toBe(want);
	});
});
