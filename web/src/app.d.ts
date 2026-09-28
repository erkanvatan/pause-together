// See https://svelte.dev/docs/kit/types#app.d.ts
declare global {
	namespace App {
		interface PageState {
			roomDeleted?: boolean; // the room page went home because its room was deleted
		}
	}
}

export {};
