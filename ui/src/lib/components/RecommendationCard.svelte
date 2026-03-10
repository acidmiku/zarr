<script>
	import { createEventDispatcher } from 'svelte';
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';

	export let rec;

	const dispatch = createEventDispatcher();
	let adding = false;

	function typeLabel(t) {
		if (t === 'music') return 'Music';
		if (t === 'anime') return 'Anime';
		if (t === 'movie') return 'Movie';
		return 'Series';
	}

	function typeColor(t) {
		if (t === 'music') return '#1db954';
		if (t === 'anime') return 'var(--badge-anime)';
		if (t === 'movie') return 'var(--badge-movie)';
		return 'var(--badge-series)';
	}

	async function addToLibrary() {
		if (adding) return;
		adding = true;

		try {
			if (rec.media_type === 'music') {
				// Search MusicBrainz for the album
				const results = await api.musicSearch(rec.title, 'album');
				if (!results || results.length === 0) {
					notify(`Could not find "${rec.title}" on MusicBrainz`, 'error');
					adding = false;
					return;
				}
				const album = results[0];
				dispatch('showMusicDetail', {
					id: album.id,
					title: album.title,
					artist: album.artist,
					artist_id: album.artist_id,
					year: album.year,
					type: album.type,
					cover_url: album.cover_url,
					in_library: album.in_library,
					library_id: album.library_id
				});
				adding = false;
				return;
			}

			// Search for the title to get metadata
			const results = await api.search(rec.title, rec.media_type);

			if (!results || results.length === 0) {
				notify(`Could not find "${rec.title}" in metadata database`, 'error');
				adding = false;
				return;
			}

			// Use the first result (best match)
			const item = results[0];

			// Prepare item for MediaDetail modal
			const detailItem = {
				...item,
				tmdb_id: item.tmdb_id,
				anilist_id: item.anilist_id,
				type: rec.media_type === 'anime' ? 'series' : rec.media_type,
				is_anime: rec.media_type === 'anime' || item.is_anime
			};

			// Dispatch event to parent to show modal
			dispatch('showDetail', detailItem);
		} catch (err) {
			notify(err.message || 'Failed to search for title', 'error');
		}

		adding = false;
	}
</script>

<div class="card">
	{#if rec.poster_url}
		<div class="poster">
			<img src={rec.poster_url} alt={rec.title} loading="lazy" />
		</div>
	{/if}
	<div class="body">
		<div class="header">
			<span class="title">{rec.title}</span>
			<span class="badge" style="background: {typeColor(rec.media_type)}">{typeLabel(rec.media_type)}</span>
			{#if rec.score}
				<span class="score">{rec.score.toFixed(1)}</span>
			{/if}
		</div>
		<p class="reason">{rec.reason}</p>
		<button class="add-btn" on:click={addToLibrary} disabled={adding}>
			{adding ? 'Searching...' : '+ Add to Library'}
		</button>
	</div>
</div>

<style>
	.card {
		display: flex;
		gap: 0.75rem;
		padding: 0.65rem;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-lg);
		margin-top: 0.35rem;
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		box-shadow: var(--shadow-md);
		transition: transform 0.25s ease, box-shadow 0.25s ease, border-color 0.25s ease;
	}

	.card:hover {
		transform: translateY(-2px);
		box-shadow: var(--shadow-lg), 0 0 20px var(--accent-glow);
		border-color: var(--accent);
	}

	.poster {
		width: 60px;
		min-width: 60px;
		height: 85px;
		border-radius: var(--radius-md);
		overflow: hidden;
		background: var(--glass-bg);
		flex-shrink: 0;
		box-shadow: var(--shadow-sm);
	}

	.poster img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		transition: transform 0.25s ease;
	}

	.card:hover .poster img {
		transform: scale(1.05);
	}

	.body {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.header {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		flex-wrap: wrap;
	}

	.title {
		font-family: var(--font-display);
		font-weight: 600;
		font-size: 0.82rem;
		color: var(--text-primary);
	}

	.badge {
		padding: 0.1rem 0.45rem;
		border-radius: var(--radius-sm);
		font-family: var(--font-display);
		font-size: 0.6rem;
		font-weight: 700;
		color: var(--text-inverse);
		text-transform: uppercase;
		flex-shrink: 0;
		letter-spacing: 0.03em;
	}

	.score {
		font-family: var(--font-display);
		font-size: 0.7rem;
		color: var(--gold);
		font-weight: 600;
		text-shadow: 0 0 6px var(--gold);
	}

	.reason {
		font-family: var(--font-body);
		font-size: 0.78rem;
		color: var(--text-secondary);
		line-height: 1.35;
		margin: 0;
	}

	.add-btn {
		align-self: flex-start;
		margin-top: 0.2rem;
		padding: 0.2rem 0.55rem;
		background: var(--accent-subtle);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-sm);
		color: var(--accent);
		font-family: var(--font-display);
		font-size: 0.7rem;
		font-weight: 600;
		cursor: pointer;
		transition: all 0.2s ease;
	}

	.add-btn:hover {
		background: var(--accent);
		color: var(--text-inverse);
		border-color: var(--accent-hover);
		box-shadow: 0 0 12px var(--accent-glow);
	}

	.add-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
