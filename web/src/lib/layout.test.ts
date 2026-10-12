import { describe, expect, it } from 'vitest';
import { barLayout, type BarLayoutIn } from './layout';

// A phone held upright: 412 px wide, a two-row bar.
const phone: BarLayoutIn = {
	tall: false,
	barOver: true,
	panelOver: false,
	transport: true,
	barHeight: 96,
	boxWidth: 412,
	boxHeight: 232,
	aspect: 16 / 9
};

describe('barLayout', () => {
	it('keeps subtitles low under a bar over the picture', () => {
		expect(barLayout(phone)).toEqual({ lift: 0, middleBottom: 96, shade: true });
	});

	it('lifts subtitles above the subtitle panel while it lies over the video', () => {
		expect(barLayout({ ...phone, panelOver: true, barHeight: 150 }).lift).toBe(150);
	});

	it('lifts subtitles above a bar under the picture, with no shade', () => {
		expect(barLayout({ ...phone, barOver: false })).toEqual({
			lift: 96,
			middleBottom: 96,
			shade: false
		});
	});

	describe('fullscreen held upright', () => {
		// 40% of an 893 px screen; a 16:9 picture is 231.75 px tall, leaving 62.625 px of black under it.
		const tall = { ...phone, tall: true, boxHeight: 357 };

		it("puts the subtitles on the picture's foot, and the bar over them with its shade", () => {
			expect(barLayout(tall)).toEqual({ lift: 62.625, middleBottom: 0, shade: true });
		});

		it('puts the bar in the black where there is room for it', () => {
			// 500 px of box leaves 134.125 px of black under the picture.
			expect(barLayout({ ...tall, boxHeight: 500 })).toEqual({
				lift: 134.125,
				middleBottom: 0,
				shade: false
			});
		});

		it('centers a card above the bar, not on the picture', () => {
			expect(barLayout({ ...tall, transport: false }).middleBottom).toBe(96);
		});

		it('keeps the transport clear of the bar in a box the keyboard squeezed', () => {
			expect(barLayout({ ...tall, boxHeight: 180 }).middleBottom).toBe(96);
		});

		it('leaves no black under a clip filmed upright', () => {
			expect(barLayout({ ...tall, aspect: 9 / 16 })).toEqual({
				lift: 0,
				middleBottom: 0,
				shade: true
			});
		});
	});
});
