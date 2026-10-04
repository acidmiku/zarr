<script>
	import { createEventDispatcher, onMount } from 'svelte';
	import { api } from '$lib/api';
	import { acquisitionState, activeAcquisition } from '$lib/music';
	import MusicAcquisitionStatus from './MusicAcquisitionStatus.svelte';
	export let album;
	const dispatch = createEventDispatcher();
	let busy = false;
	let error = '';
	let disposed = false;
	let refreshing = false;
	$: state = acquisitionState(album);
	$: active = activeAcquisition(album);
	$: retry = ['failed', 'no_results', 'blocked', 'cancelled'].includes(state.status);
	onMount(() => {
		const timer = setInterval(() => {
			if ((active || (album.monitored && state.status !== 'available')) && !busy) refresh();
		}, 5000);
		return () => {
			disposed = true;
			clearInterval(timer);
		};
	});
	async function refresh() {
		if (refreshing || disposed) return;
		refreshing = true;
		try {
			const data = await api.getMusicLibraryItem(album.id);
			if (!disposed && data.album) {
				album = data.album;
				error = '';
				dispatch('updated', album);
			}
		} catch (e) {
			if (!disposed) error = `Status could not refresh: ${e.message}`;
		} finally {
			refreshing = false;
		}
	}
	async function acquire() {
		if (busy || active) return;
		busy = true;
		error = '';
		try {
			const result =
				state.status === 'failed' && state.download_id
					? await api.retryDownload(state.download_id)
					: await api.downloadMusicAlbum(album.id);
			if (disposed) return;
			if (result?.acquisition) album = { ...album, ...result };
			await refresh();
		} catch (e) {
			if (!disposed) error = e.message;
		} finally {
			busy = false;
		}
	}
	async function monitor(event) {
		const value = event.currentTarget.checked;
		if (busy) return;
		busy = true;
		error = '';
		try {
			const result = await api.monitorMusicAlbum(album.id, value);
			if (!disposed) {
				album = { ...album, ...result, monitored: result?.monitored ?? value };
				dispatch('updated', album);
			}
		} catch (e) {
			if (!disposed) {
				error = e.message;
				event.target.checked = Boolean(album.monitored);
			}
		} finally {
			busy = false;
		}
	}
	async function favorite() {
		if (busy) return;
		busy = true;
		error = '';
		try {
			const value = !album.favorite;
			await api.favoriteMusicAlbum(album.id, value);
			if (!disposed) {
				album = { ...album, favorite: value };
				dispatch('updated', album);
			}
		} catch (e) {
			if (!disposed) error = e.message;
		} finally {
			busy = false;
		}
	}
</script>

<div class="music-acquisition">
	<MusicAcquisitionStatus {album} />
	<div class="acquisition-actions">
		{#if state.status !== 'available'}<button
				class="acquire"
				disabled={busy || active || (retry && state.retryable === false)}
				on:click={acquire}
				>{busy
					? 'Working…'
					: active
						? 'Acquisition in progress'
						: retry
							? 'Retry acquisition'
							: 'Download now'}</button
			>{/if}
		<button
			class="favorite"
			aria-pressed={Boolean(album.favorite)}
			disabled={busy}
			on:click={favorite}>{album.favorite ? '♥ Favorited' : '♡ Favorite'}</button
		>
	</div>
	<label class="monitor"
		><input
			type="checkbox"
			checked={Boolean(album.monitored)}
			disabled={busy}
			on:change={monitor}
		/> Monitor album</label
	>
	<p class="monitor-hint">
		Automatically look for a matching release when this album is missing. Turning this off keeps any
		current download.
	</p>
	{#if error}<p class="acquisition-error" role="alert">{error}</p>
		<button class="refresh" on:click={refresh} disabled={refreshing}>Refresh status</button>{/if}
</div>

<style>
	.music-acquisition {
		margin: 1rem 0;
		padding: 1rem;
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-md);
		background: var(--bg-input);
	}
	.acquisition-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.6rem;
		margin-top: 0.8rem;
	}
	button {
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-sm);
		padding: 0.65rem 1rem;
		font: 600 0.8rem var(--font-body);
		color: var(--text-primary);
		background: var(--glass-bg);
		cursor: pointer;
	}
	.acquire {
		background: var(--accent);
		color: var(--text-inverse);
		border-color: var(--accent);
	}
	button:disabled {
		opacity: 0.55;
		cursor: default;
	}
	button:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 3px;
	}
	.monitor {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.8rem;
		margin-top: 0.9rem;
	}
	.monitor input {
		accent-color: var(--accent);
	}
	.monitor-hint {
		color: var(--text-muted);
		font-size: 0.75rem;
		line-height: 1.5;
		margin: 0.35rem 0 0;
	}
	.acquisition-error {
		color: var(--danger);
		font-size: 0.8rem;
		line-height: 1.5;
		overflow-wrap: anywhere;
	}
</style>
