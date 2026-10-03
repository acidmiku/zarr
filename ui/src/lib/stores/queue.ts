import { writable } from 'svelte/store';
import { api } from '$lib/api';
import { downloads } from './app';

export const queueError = writable('');
export const queueLoaded = writable(false);
let pending: Promise<void> | null = null;

export function refreshDownloads() {
	if (pending) return pending;
	pending = (async () => {
		try {
			const data = await api.getDownloads();
			downloads.set(Array.isArray(data) ? data : []);
			queueError.set('');
		} catch (error) {
			queueError.set(error instanceof Error ? error.message : 'Could not load downloads');
		} finally {
			queueLoaded.set(true);
			pending = null;
		}
	})();
	return pending;
}

// One poller for the navigation badge, dashboard, and Activity page.
export function pollDownloads() {
	let stopped = false;
	let timer: ReturnType<typeof setTimeout>;
	async function poll() {
		if (!document.hidden) await refreshDownloads();
		if (!stopped) timer = setTimeout(poll, 5000);
	}
	void poll();
	return () => {
		stopped = true;
		clearTimeout(timer);
	};
}

export function progress(value: unknown) {
	const number = Number.parseFloat(String(value));
	return Number.isFinite(number) ? Math.max(0, Math.min(100, number)) : 0;
}

export function isActiveDownload(item: { status: string }) {
	return ['queued', 'downloading', 'processing', 'extracting', 'completed', 'seeding'].includes(
		item.status
	);
}
