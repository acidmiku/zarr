<script>
	import { createEventDispatcher, onMount } from 'svelte';
	import { api } from '$lib/api';
	import { mediaTypeLabel } from '$lib/media';
	import StatusBadge from './StatusBadge.svelte';

	export let item = null;
	export let profiles = [];
	export let mode = 'discovery'; // 'discovery' or 'library'

	const dispatch = createEventDispatcher();

	let selectedProfile = (() => {
		const videoProfiles = profiles.filter(p => p.profile_type !== 'music');
		const keyword = item?.is_anime || item?.anime ? 'anime' : item?.type === 'movie' ? 'movie' : 'series';
		const match = videoProfiles.find(p => p.name.toLowerCase().includes(keyword));
		return match?.id || videoProfiles[0]?.id || 0;
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
		if (mode === 'discovery' && item?.tmdb_id && item.type === 'series') {
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
		if (adding || loadingSeasons) return;
		// For TMDB series with potentially multiple seasons, show season picker
		if (item.tmdb_id && item.type === 'series' && !showSeasonPicker) {
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
				payload.anime = item.is_anime || item.anime || false;
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
					<span class="media-type-tag">{mediaTypeLabel(item)}</span>
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
										{#each profiles.filter(p => p.profile_type !== 'music') as p}
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
									{#each profiles.filter(p => p.profile_type !== 'music') as p}
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
	/* ── Container ────────────────────────────────────────────── */
	.detail {
		position: relative;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-xl);
		overflow: hidden;
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		box-shadow: var(--shadow-lg);
	}

	/* ── Cinematic backdrop ───────────────────────────────────── */
	.backdrop {
		position: relative;
		height: 200px;
		overflow: hidden;
	}

	.backdrop img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		opacity: 0.5;
		filter: saturate(1.15) brightness(0.85);
		transition: opacity 0.25s ease;
	}

	.backdrop-fade {
		position: absolute;
		inset: 0;
		background: linear-gradient(
			to bottom,
			transparent 30%,
			var(--glass-bg) 100%
		);
	}

	/* ── Content area ─────────────────────────────────────────── */
	.detail-content {
		padding: 1.75rem 2rem 2rem;
		position: relative;
	}

	.detail-content.has-backdrop {
		margin-top: -60px;
	}

	.detail-header {
		display: flex;
		gap: 1.75rem;
	}

	/* ── Poster ────────────────────────────────────────────────── */
	.detail-poster {
		width: 160px;
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
		flex-shrink: 0;
		object-fit: cover;
		aspect-ratio: 2/3;
		border: 1px solid var(--glass-border);
		transition: box-shadow 0.25s ease, transform 0.25s ease;
	}

	.detail-poster:hover {
		box-shadow: var(--shadow-glow);
		transform: scale(1.02);
	}

	.detail-info {
		flex: 1;
		min-width: 0;
	}

	/* ── Title ─────────────────────────────────────────────────── */
	h2 {
		font-family: var(--font-display);
		font-size: 1.65rem;
		font-weight: 800;
		letter-spacing: -0.02em;
		margin-bottom: 0.5rem;
		color: var(--text-primary);
		line-height: 1.2;
	}

	/* ── Meta row (year / rating / seasons / status / type) ──── */
	.meta-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.85rem;
		flex-wrap: wrap;
	}

	.meta-item {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--text-secondary);
	}

	.rating-val {
		color: var(--accent);
		font-weight: 700;
	}

	.media-type-tag {
		background: var(--accent);
		color: var(--text-inverse);
		padding: 3px 10px;
		border-radius: var(--radius-sm);
		font-family: var(--font-display);
		font-size: 0.65rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		box-shadow: 0 0 8px var(--accent-glow);
	}

	/* ── Genre tags ────────────────────────────────────────────── */
	.genres {
		display: flex;
		gap: 0.4rem;
		flex-wrap: wrap;
		margin-bottom: 0.85rem;
	}

	.genre-tag {
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		padding: 3px 12px;
		border-radius: var(--radius-md);
		font-family: var(--font-body);
		font-size: 0.75rem;
		color: var(--text-secondary);
		transition: color 0.2s ease, border-color 0.2s ease;
	}

	.genre-tag:hover {
		color: var(--text-primary);
		border-color: var(--accent-subtle);
	}

	/* ── Overview ──────────────────────────────────────────────── */
	.overview {
		font-family: var(--font-body);
		font-size: 0.875rem;
		color: var(--text-secondary);
		line-height: 1.65;
		max-height: 5.25em;
		overflow: hidden;
		margin-bottom: 1.25rem;
		-webkit-mask-image: linear-gradient(to bottom, #000 60%, transparent 100%);
		mask-image: linear-gradient(to bottom, #000 60%, transparent 100%);
	}

	/* ── Actions area ──────────────────────────────────────────── */
	.actions {
		display: flex;
		gap: 0.75rem;
		align-items: center;
	}

	.add-row {
		display: flex;
		gap: 0.6rem;
		align-items: center;
	}

	/* ── Select / dropdown ─────────────────────────────────────── */
	select {
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		color: var(--text-primary);
		padding: 0.5rem 0.85rem;
		border-radius: var(--radius-md);
		font-family: var(--font-body);
		font-size: 0.85rem;
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		transition: border-color 0.2s ease, box-shadow 0.2s ease;
		cursor: pointer;
	}

	select:focus {
		outline: none;
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	/* ── Buttons ───────────────────────────────────────────────── */
	.btn {
		padding: 0.55rem 1.35rem;
		border-radius: var(--radius-md);
		font-family: var(--font-display);
		font-size: 0.85rem;
		font-weight: 700;
		letter-spacing: -0.02em;
		border: none;
		cursor: pointer;
		transition: background 0.2s ease, box-shadow 0.2s ease, transform 0.2s ease;
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
	}

	.btn:focus-visible {
		outline: none;
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	.btn-primary {
		background: var(--accent);
		color: var(--text-inverse);
		box-shadow: var(--shadow-sm);
	}

	.btn-primary:hover:not(:disabled) {
		background: var(--accent-hover);
		box-shadow: var(--shadow-glow);
		transform: translateY(-1px);
	}

	.btn-primary:active:not(:disabled) {
		transform: translateY(0);
	}

	.btn-primary:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}

	.btn-secondary {
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		color: var(--text-primary);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
	}

	.btn-secondary:hover {
		border-color: var(--accent-subtle);
		box-shadow: 0 0 0 3px var(--accent-subtle);
		transform: translateY(-1px);
	}

	.btn-secondary:active {
		transform: translateY(0);
	}

	/* ── Season picker ─────────────────────────────────────────── */
	.season-picker {
		width: 100%;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-lg);
		padding: 1rem 1.15rem;
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
	}

	.season-picker-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 0.65rem;
	}

	.season-picker-title {
		font-family: var(--font-display);
		font-size: 0.85rem;
		font-weight: 700;
		letter-spacing: -0.02em;
		color: var(--text-primary);
	}

	.season-picker-actions {
		display: flex;
		gap: 0.6rem;
	}

	.link-btn {
		background: none;
		border: none;
		color: var(--accent);
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-weight: 600;
		padding: 0;
		cursor: pointer;
		transition: color 0.2s ease;
	}

	.link-btn:hover {
		color: var(--accent-hover);
		text-decoration: underline;
	}

	.season-grid {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		margin-bottom: 0.85rem;
		max-height: 200px;
		overflow-y: auto;
		scrollbar-width: thin;
	}

	.season-check {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.45rem 0.65rem;
		border-radius: var(--radius-sm);
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--text-secondary);
		cursor: pointer;
		transition: background 0.2s ease, color 0.2s ease;
	}

	.season-check:hover {
		background: var(--accent-subtle);
		color: var(--text-primary);
	}

	.season-check.checked {
		color: var(--text-primary);
		background: var(--accent-subtle);
	}

	.season-check input {
		accent-color: var(--accent);
	}

	.ep-count {
		font-size: 0.72rem;
		color: var(--text-muted);
		font-family: var(--font-body);
	}

	/* ── Responsive ────────────────────────────────────────────── */
	@media (max-width: 600px) {
		.detail-content {
			padding: 1.25rem 1.25rem 1.5rem;
		}

		.detail-header {
			flex-direction: column;
			align-items: center;
			text-align: center;
		}

		.detail-poster {
			width: 130px;
		}

		.genres, .meta-row {
			justify-content: center;
		}

		.add-row {
			flex-wrap: wrap;
			justify-content: center;
		}

		.season-picker {
			padding: 0.85rem;
		}
	}
</style>
