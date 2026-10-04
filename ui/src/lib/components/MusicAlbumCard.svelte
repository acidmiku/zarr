<script>
	import Artwork from './Artwork.svelte';
	import MusicAcquisitionStatus from './MusicAcquisitionStatus.svelte';
	import { musicArtwork } from '$lib/music';
	export let album;
	export let href = '';
	export let onopen = () => {};
	$: saved = Boolean(album.in_library || href || album.library_id);
</script>

<svelte:element
	this={href ? 'a' : 'button'}
	{href}
	type={href ? undefined : 'button'}
	class="album-card"
	aria-label={`View ${album.title || album.name}`}
	on:click={href ? undefined : () => onopen(album)}
>
	<div class="album-cover">
		<Artwork src={musicArtwork(album)} title={album.title || album.name} />
	</div>
	<div class="album-info">
		<div class="album-title">{album.title || album.name}</div>
		<div class="album-artist">{album.artist || album.artist_name || ''}</div>
		<div class="album-meta">
			{String(album.year || '').slice(0, 4)}{album.type || album.album_type
				? ` · ${album.type || album.album_type}`
				: ''}
		</div>
		{#if album.track_name}<div class="album-meta">♫ {album.track_name}</div>{/if}
		{#if saved}<MusicAcquisitionStatus {album} compact />{/if}
		{#if album.monitored}<span class="monitor-label">Monitored</span>{/if}
		{#if album.favorite}<span class="monitor-label">♥ Favorite</span>{/if}
		{#if album.reason}<p class="album-reason">{album.reason}</p>{/if}
	</div>
</svelte:element>

<style>
	.album-card {
		display: block;
		min-width: 0;
		width: 100%;
		text-align: left;
		color: var(--text-primary);
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-md);
		overflow: hidden;
		padding: 0;
		font: inherit;
		cursor: pointer;
		text-decoration: none;
		transition:
			transform 0.2s,
			border-color 0.2s;
	}
	.album-card:hover {
		transform: translateY(-3px);
		border-color: var(--accent);
	}
	.album-card:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 3px;
	}
	.album-cover {
		aspect-ratio: 1;
		width: 100%;
		overflow: hidden;
		background: var(--bg-elevated);
	}
	.album-info {
		padding: 0.8rem;
	}
	.album-title {
		font-weight: 700;
		font-size: 0.92rem;
		line-height: 1.3;
		overflow-wrap: anywhere;
	}
	.album-artist {
		margin-top: 0.3rem;
		color: var(--text-secondary);
		font-size: 0.8rem;
		overflow-wrap: anywhere;
	}
	.album-meta,
	.monitor-label {
		color: var(--text-muted);
		font-size: 0.68rem;
		margin: 0.35rem 0;
	}
	.monitor-label {
		display: block;
	}
	.album-reason {
		margin: 0.6rem 0 0;
		padding-top: 0.6rem;
		border-top: 1px solid var(--glass-border);
		color: var(--text-secondary);
		font-size: 0.75rem;
		line-height: 1.5;
	}
</style>
