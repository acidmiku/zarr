<script>
	import { createEventDispatcher, onMount } from 'svelte';
	import { api } from '$lib/api';
	import StatusBadge from './StatusBadge.svelte';

	export let item = null;
	export let profiles = [];
	export let mode = 'discovery'; // 'discovery' or 'library'

	const dispatch = createEventDispatcher();

	let selectedProfile = (() => {
		const keyword = item?.is_anime ? 'anime' : item?.type === 'movie' ? 'movie' : 'series';
		const match = profiles.find(p => p.name.toLowerCase().includes(keyword));
		return match?.id || profiles[0]?.id || 1;
	})();
	let adding = false;
	let searching = false;
	let showSeasonPicker = false;
	let availableSeasons = [];
	let selectedSeasons = new Set();
	let loadingSeasons = false;
	let seasonCount = null;

	let prefetchedSeasons = null;

	onMount(async () => {
		// Fetch season info for series in discovery mode
		if (mode === 'discovery' && item?.tmdb_id && (item.type === 'series' || item.is_anime)) {
			try {
				const meta = await api.metadata(item.tmdb_id, 'series');
				const seasons = (meta.seasons || []).filter(s => item.is_anime || s.season_number > 0);
				seasonCount = seasons.length;
				prefetchedSeasons = seasons;
			} catch {}
		}
	});

	$: posterSrc = item?.poster_url ? api.imageUrl(item.poster_url) : '';
	$: backdropSrc = item?.backdrop_url ? api.imageUrl(item.backdrop_url) : '';
	$: genres = (() => {
		try {
			if (typeof item?.genres === 'string') return JSON.parse(item.genres);
			if (Array.isArray(item?.genres)) return item.genres;
		} catch {}
		return [];
	})();

	async function addToLibrary() {
		// For TMDB series with potentially multiple seasons, show season picker
		if (item.tmdb_id && (item.type === 'series' || item.is_anime) && !showSeasonPicker) {
			loadingSeasons = true;
			try {
				// Use prefetched data if available, otherwise fetch
				const seasons = prefetchedSeasons ||
					((await api.metadata(item.tmdb_id, 'series')).seasons || []).filter(s => item.is_anime || s.season_number > 0);
				if (seasons.length > 1) {
					availableSeasons = seasons;
					selectedSeasons = new Set(seasons.map(s => s.season_number));
					showSeasonPicker = true;
					loadingSeasons = false;
					return;
				}
			} catch {
				// If metadata fetch fails, just add all seasons
			}
			loadingSeasons = false;
		}

		adding = true;
		try {
			const payload = {};
			if (item.anilist_id) {
				payload.anilist_id = item.anilist_id;
			} else {
				payload.tmdb_id = item.tmdb_id;
				payload.type = item.type || 'movie';
				payload.anime = item.is_anime || false;
			}
			payload.quality_profile_id = selectedProfile;
			if (showSeasonPicker && selectedSeasons.size > 0) {
				payload.seasons = [...selectedSeasons];
			}

			const result = await api.addToLibrary(payload);
			showSeasonPicker = false;
			dispatch('added', result);
		} catch (e) {
			dispatch('error', e.message);
		}
		adding = false;
	}

	function toggleSeason(num) {
		if (selectedSeasons.has(num)) {
			selectedSeasons.delete(num);
		} else {
			selectedSeasons.add(num);
		}
		selectedSeasons = selectedSeasons;
	}

	function selectAllSeasons() {
		selectedSeasons = new Set(availableSeasons.map(s => s.season_number));
	}

	function deselectAllSeasons() {
		selectedSeasons = new Set();
	}

	async function searchAll() {
		searching = true;
		try {
			await api.searchAll(item.id);
			dispatch('searched');
		} catch (e) {
			dispatch('error', e.message);
		}
		searching = false;
	}
</script>

<div class="detail" on:click|stopPropagation on:keydown|stopPropagation role="dialog">
	{#if backdropSrc}
		<div class="backdrop">
			<img src={backdropSrc} alt="" />
			<div class="backdrop-fade"></div>
		</div>
	{/if}

	<div class="detail-content" class:has-backdrop={backdropSrc}>
		<div class="detail-header">
			{#if posterSrc}
				<img class="detail-poster" src={posterSrc} alt={item.title} />
			{/if}
			<div class="detail-info">
				<h2>{item.title}</h2>
				<div class="meta-row">
					{#if item.year}<span class="meta-item">{item.year}</span>{/if}
					{#if item.rating}<span class="meta-item rating-val">★ {item.rating?.toFixed(1)}</span>{/if}
					{#if seasonCount !== null}<span class="meta-item">{seasonCount} season{seasonCount !== 1 ? 's' : ''}</span>{/if}
					{#if item.status}<StatusBadge status={item.status} />{/if}
					<span class="media-type-tag">{item.is_anime ? 'Anime' : item.type === 'movie' ? 'Movie' : 'Series'}</span>
				</div>
				{#if genres.length > 0}
					<div class="genres">
						{#each genres as genre}
							<span class="genre-tag">{genre}</span>
						{/each}
					</div>
				{/if}
				{#if item.overview}
					<p class="overview">{item.overview?.replace(/<[^>]*>/g, '')}</p>
				{/if}

				<div class="actions">
					{#if mode === 'discovery' && !item.in_library}
						{#if showSeasonPicker}
							<div class="season-picker">
								<div class="season-picker-header">
									<span class="season-picker-title">Select seasons to monitor</span>
									<div class="season-picker-actions">
										<button class="link-btn" on:click={selectAllSeasons}>All</button>
										<button class="link-btn" on:click={deselectAllSeasons}>None</button>
									</div>
								</div>
								<div class="season-grid">
									{#each availableSeasons as season}
										<label class="season-check" class:checked={selectedSeasons.has(season.season_number)}>
											<input type="checkbox"
												checked={selectedSeasons.has(season.season_number)}
												on:change={() => toggleSeason(season.season_number)} />
											<span class="season-label">
												{season.name || `Season ${season.season_number}`}
												{#if season.episode_count}
													<span class="ep-count">({season.episode_count} ep)</span>
												{/if}
											</span>
										</label>
									{/each}
								</div>
								<div class="add-row">
									<select bind:value={selectedProfile}>
										{#each profiles as p}
											<option value={p.id}>{p.name}</option>
										{/each}
									</select>
									<button class="btn btn-primary" on:click={addToLibrary} disabled={adding || selectedSeasons.size === 0}>
										{adding ? 'Adding...' : `Add ${selectedSeasons.size} season${selectedSeasons.size !== 1 ? 's' : ''}`}
									</button>
									<button class="btn btn-secondary" on:click={() => showSeasonPicker = false}>Cancel</button>
								</div>
							</div>
						{:else}
							<div class="add-row">
								<select bind:value={selectedProfile}>
									{#each profiles as p}
										<option value={p.id}>{p.name}</option>
									{/each}
								</select>
								<button class="btn btn-primary" on:click={addToLibrary} disabled={adding || loadingSeasons}>
									{loadingSeasons ? 'Loading...' : adding ? 'Adding...' : 'Add to Library'}
								</button>
							</div>
						{/if}
					{:else if mode === 'discovery' && item.in_library}
						<a href="/library/{item.library_id}" class="btn btn-secondary">View in Library</a>
					{:else if mode === 'library'}
						<button class="btn btn-primary" on:click={searchAll} disabled={searching}>
							{searching ? 'Searching...' : 'Search All Wanted'}
						</button>
					{/if}
				</div>
			</div>
		</div>
	</div>
</div>

<style>
	.detail {
		position: relative;
		background: var(--bg-surface);
		border-radius: 12px;
		overflow: hidden;
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
	}

	.backdrop {
		position: relative;
		height: 160px;
		overflow: hidden;
	}

	.backdrop img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		opacity: 0.4;
	}

	.backdrop-fade {
		position: absolute;
		bottom: 0;
		left: 0;
		right: 0;
		height: 80px;
		background: linear-gradient(transparent, var(--bg-surface));
	}

	.detail-content {
		padding: 1.5rem;
		position: relative;
	}

	.detail-content.has-backdrop {
		margin-top: -50px;
	}

	.detail-header {
		display: flex;
		gap: 1.5rem;
	}

	.detail-poster {
		width: 150px;
		border-radius: 8px;
		box-shadow: var(--shadow-md);
		flex-shrink: 0;
		object-fit: cover;
		aspect-ratio: 2/3;
	}

	.detail-info {
		flex: 1;
		min-width: 0;
	}

	h2 {
		font-size: 1.5rem;
		font-weight: 700;
		margin-bottom: 0.5rem;
		color: var(--text-primary);
	}

	.meta-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.75rem;
		flex-wrap: wrap;
	}

	.meta-item {
		font-size: 0.85rem;
		color: var(--text-secondary);
	}

	.rating-val {
		color: var(--gold);
	}

	.media-type-tag {
		background: var(--accent);
		color: var(--text-inverse);
		padding: 2px 8px;
		border-radius: 4px;
		font-size: 0.7rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.genres {
		display: flex;
		gap: 0.4rem;
		flex-wrap: wrap;
		margin-bottom: 0.75rem;
	}

	.genre-tag {
		background: var(--bg-elevated);
		padding: 2px 10px;
		border-radius: 20px;
		font-size: 0.75rem;
		color: var(--text-secondary);
	}

	.overview {
		font-size: 0.875rem;
		color: var(--text-secondary);
		line-height: 1.5;
		max-height: 4.5em;
		overflow: hidden;
		margin-bottom: 1rem;
	}

	.actions {
		display: flex;
		gap: 0.75rem;
		align-items: center;
	}

	.add-row {
		display: flex;
		gap: 0.5rem;
		align-items: center;
	}

	select {
		background: var(--bg-input);
		border: 1px solid var(--border-subtle);
		color: var(--text-primary);
		padding: 0.5rem 0.75rem;
		border-radius: 6px;
		font-size: 0.85rem;
	}

	.btn {
		padding: 0.5rem 1.25rem;
		border-radius: 6px;
		font-size: 0.85rem;
		font-weight: 600;
		border: none;
		transition: background 0.15s;
		display: inline-flex;
		align-items: center;
	}

	.btn-primary {
		background: var(--accent);
		color: var(--text-inverse);
	}

	.btn-primary:hover:not(:disabled) {
		background: var(--accent-hover);
	}

	.btn-primary:disabled {
		opacity: 0.5;
	}

	.btn-secondary {
		background: var(--bg-elevated);
		color: var(--text-primary);
	}

	.btn-secondary:hover {
		background: var(--bg-hover);
	}

	.season-picker {
		width: 100%;
	}

	.season-picker-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.5rem;
	}

	.season-picker-title {
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--text-primary);
	}

	.season-picker-actions {
		display: flex;
		gap: 0.5rem;
	}

	.link-btn {
		background: none;
		border: none;
		color: var(--accent);
		font-size: 0.8rem;
		padding: 0;
		cursor: pointer;
	}

	.link-btn:hover {
		text-decoration: underline;
	}

	.season-grid {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		margin-bottom: 0.75rem;
		max-height: 200px;
		overflow-y: auto;
	}

	.season-check {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.4rem 0.6rem;
		border-radius: 6px;
		font-size: 0.85rem;
		color: var(--text-secondary);
		cursor: pointer;
	}

	.season-check:hover {
		background: var(--bg-elevated);
	}

	.season-check.checked {
		color: var(--text-primary);
	}

	.season-check input {
		accent-color: var(--accent);
	}

	.ep-count {
		font-size: 0.75rem;
		color: var(--text-muted);
	}

	@media (max-width: 600px) {
		.detail-header {
			flex-direction: column;
			align-items: center;
			text-align: center;
		}

		.detail-poster { width: 120px; }
		.genres, .meta-row { justify-content: center; }
	}
</style>
