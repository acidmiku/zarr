import { writable } from 'svelte/store';
import { browser } from '$app/environment';

export const setupComplete = writable(false);
export const downloads = writable<any[]>([]);
export const profiles = writable<any[]>([]);

// Theme
function getInitialTheme(): 'dark' | 'light' {
	if (browser) {
		return (localStorage.getItem('mf-theme') as 'dark' | 'light') || 'dark';
	}
	return 'dark';
}
export const theme = writable<'dark' | 'light'>(getInitialTheme());

export function toggleTheme() {
	theme.update(t => {
		const next = t === 'dark' ? 'light' : 'dark';
		if (browser) {
			localStorage.setItem('mf-theme', next);
			if (next === 'light') {
				document.documentElement.setAttribute('data-theme', 'light');
			} else {
				document.documentElement.removeAttribute('data-theme');
			}
		}
		return next;
	});
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
