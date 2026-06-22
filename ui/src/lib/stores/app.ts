import { writable } from 'svelte/store';
import { browser } from '$app/environment';

export const setupComplete = writable(false);
export const downloads = writable<any[]>([]);
export const profiles = writable<any[]>([]);

// Theme
export type ThemeName = 'dark' | 'light' | 'ember' | 'violet' | 'rose' | 'mint' | 'boringcore';

export const THEMES: { id: ThemeName; label: string; group: 'dark' | 'light'; swatch: string }[] = [
	{ id: 'dark', label: 'Obsidian', group: 'dark', swatch: '#00d4ff' },
	{ id: 'ember', label: 'Ember', group: 'dark', swatch: '#f59e0b' },
	{ id: 'violet', label: 'Violet', group: 'dark', swatch: '#a78bfa' },
	{ id: 'light', label: 'Light', group: 'light', swatch: '#0891b2' },
	{ id: 'rose', label: 'Rose', group: 'light', swatch: '#e11d48' },
	{ id: 'mint', label: 'Mint', group: 'light', swatch: '#10b981' },
	{ id: 'boringcore', label: 'Boringcore', group: 'light', swatch: '#316ac5' },
];

const VALID_THEMES: Set<string> = new Set(THEMES.map(t => t.id));

function getInitialTheme(): ThemeName {
	if (browser) {
		const stored = localStorage.getItem('mf-theme');
		if (stored && VALID_THEMES.has(stored)) return stored as ThemeName;
	}
	return 'dark';
}

export const theme = writable<ThemeName>(getInitialTheme());

export function setTheme(name: ThemeName) {
	theme.set(name);
	if (browser) {
		localStorage.setItem('mf-theme', name);
		if (name === 'dark') {
			document.documentElement.removeAttribute('data-theme');
		} else {
			document.documentElement.setAttribute('data-theme', name);
		}
	}
}

export const notifications = writable<{ id: number; message: string; type: 'success' | 'error' | 'info' }[]>([]);

let notifId = 0;
export function notify(message: string, type: 'success' | 'error' | 'info' = 'info') {
	const id = ++notifId;
	notifications.update(n => [...n, { id, message, type }]);
	setTimeout(() => {
		notifications.update(n => n.filter(x => x.id !== id));
	}, 4000);
}
