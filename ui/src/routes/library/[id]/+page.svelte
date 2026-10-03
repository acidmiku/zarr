<script>
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';
	import MediaDetail from '$lib/components/MediaDetail.svelte';
	import EpisodeList from '$lib/components/EpisodeList.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import StarRating from '$lib/components/StarRating.svelte';

	let item = null;
	let seasons = [];
	let profiles = [];
	let loading = true;
	let showReleases = false;
	let releases = [];
	let searchingReleases = false;
	let releasesContext = ''; // '' for movie/series-wide, or 'Episode X' label
	let releasesEpisodeId = 0;
	let error = '';

	// Add Seasons state
	let showSeasonPicker = false;
	let availableNewSeasons = [];
	let selectedNewSeasons = new Set();
	let loadingSeasons = false;
	let addingSeasons = false;

	// Rating state
	let showRatingModal = false;
	let userRating = 0;
	let userComment = '';
	let existingRating = null;
	let savingRating = false;

	$: id = parseInt($page.params.id);
	$: hasWantedEpisodes = seasons.some(s => s.episodes?.some(e => e.status === 'wanted'));

	onMount(async () => {
		await loadItem();
		profiles = await api.getProfiles();
		loading = false;
		if (item && $page.url.searchParams.get('tab') === 'releases') viewReleases();
	});

	async function loadItem() {
		try {
			item = await api.getLibraryItem(id);
			if (item.type === 'series') {
				seasons = await api.getEpisodes(id);
			}
			// Fetch user rating
			if (item.tmdb_id) {
				try {
					existingRating = await api.getRating(item.tmdb_id, item.type);
					if (existingRating) {
						userRating = existingRating.rating;
						userComment = existingRating.comment || '';
					}
				} catch {}
			}
		} catch (e) {
			notify('Failed to load item', 'error');
		}
	}

	async function searchAll() {
		error = '';
		try {
			await api.searchAll(id);
			notify('Search triggered for all wanted items', 'success');
		} catch (e) {
			error = e.message;
		}
	}

	async function deleteItem(withFiles = false) {
		if (!confirm(withFiles ? 'Delete from library AND delete files from disk?' : 'Remove from library?')) return;
		error = '';
		try {
			await api.deleteLibraryItem(id, withFiles);
			notify('Removed from library', 'success');
			goto('/library');
		} catch (e) {
			error = e.message;
		}
	}

	async function updateProfile(profileId) {
		error = '';
		try {
			await api.updateLibraryItem(id, { quality_profile_id: profileId });
			notify('Profile updated', 'success');
		} catch (e) {
			error = e.message;
		}
	}

	async function viewReleases(episodeId = 0, label = '') {
		showReleases = true;
		searchingReleases = true;
		releasesContext = label;
		releasesEpisodeId = episodeId;
		releases = [];
		error = '';
		try {
			releases = await api.searchReleases(id, episodeId || undefined);
		} catch (e) {
			error = e.message;
		}
		searchingReleases = false;
	}

	async function grabRelease(rel) {
		error = '';
		try {
			const data = {
				release_url: rel.nzb_url,
				title: rel.title,
				media_item_id: id
			};
			if (releasesEpisodeId) {
				data.episode_id = releasesEpisodeId;
			}
			if (rel.download_type === 'torrent') {
				data.download_type = 'torrent';
				data.topic_id = rel.topic_id;
			}
			await api.grabRelease(data);
			notify('Release grabbed!', 'success');
			showReleases = false;
		} catch (e) {
			error = e.message;
		}
	}

	// -- Add Seasons --
	async function openSeasonPicker() {
		loadingSeasons = true;
		showSeasonPicker = true;
		error = '';
		try {
			const meta = await api.metadata(item.tmdb_id, 'series');
			const allSeasons = (meta.seasons || []).filter(s => item.anime || s.season_number > 0);
			const monitoredNums = new Set(seasons.map(s => s.number));
			availableNewSeasons = allSeasons.filter(s => !monitoredNums.has(s.season_number));
			if (availableNewSeasons.length === 0) {
				error = 'All available seasons are already monitored';
				showSeasonPicker = false;
			} else {
				selectedNewSeasons = new Set(availableNewSeasons.map(s => s.season_number));
			}
		} catch (e) {
			error = 'Failed to load season data: ' + e.message;
			showSeasonPicker = false;
		}
		loadingSeasons = false;
	}

	async function addSelectedSeasons() {
		addingSeasons = true;
		error = '';
		try {
			await api.addSeasons(id, [...selectedNewSeasons]);
			notify('Seasons added successfully', 'success');
			showSeasonPicker = false;
			seasons = await api.getEpisodes(id);
		} catch (e) {
			error = e.message;
		}
		addingSeasons = false;
	}

	function toggleNewSeason(num) {
		if (selectedNewSeasons.has(num)) selectedNewSeasons.delete(num);
		else selectedNewSeasons.add(num);
		selectedNewSeasons = selectedNewSeasons;
	}

	function selectAllNew() {
		selectedNewSeasons = new Set(availableNewSeasons.map(s => s.season_number));
	}

	function deselectAllNew() {
		selectedNewSeasons = new Set();
	}

	// -- Rating --
	function openRatingModal() {
		if (existingRating) {
			userRating = existingRating.rating;
			userComment = existingRating.comment || '';
		} else {
			userRating = 0;
			userComment = '';
		}
		showRatingModal = true;
	}

	async function saveRating() {
		if (userRating === 0) {
			if (existingRating) {
				try {
					await api.deleteRating(item.tmdb_id, item.type);
					existingRating = null;
					notify('Rating removed', 'success');
				} catch (e) {
					error = e.message;
				}
			}
			showRatingModal = false;
			return;
		}
		savingRating = true;
		error = '';
		try {
			await api.upsertRating({
				tmdb_id: item.tmdb_id,
				media_type: item.type,
				rating: userRating,
				comment: userComment,
				title: item.title,
				poster_url: item.poster_url,
				year: item.year,
				anime: item.anime || false
			});
			existingRating = { rating: userRating, comment: userComment };
			notify('Rating saved', 'success');
			showRatingModal = false;
		} catch (e) {
			error = e.message;
		}
		savingRating = false;
	}

	$: genresArr = (() => {
		try {
			if (typeof item?.genres === 'string') return JSON.parse(item.genres);
		} catch {}
		return [];
	})();

	$: posterSrc = item?.poster_url ? api.imageUrl(item.poster_url) : '';
	$: backdropSrc = item?.backdrop_url ? api.imageUrl(item.backdrop_url) : '';
</script>

<svelte:head>
	<title>{item?.title || 'Loading'} - Zarr</title>
</svelte:head>

{#if loading}
	<div class="loading">Loading...</div>
{:else if item}
	<div class="detail-page">
		{#if backdropSrc}
			<div class="backdrop">
				<img src={backdropSrc} alt="" />
				<div class="backdrop-fade"></div>
			</div>
		{/if}

		<div class="content">
			<a href="/library" class="back-link">← Library</a>

			<div class="header">
				{#if posterSrc}
					<img class="poster" src={posterSrc} alt={item.title} />
				{/if}
				<div class="info">
					<h1>{item.title}</h1>
					<div class="meta-row">
						{#if item.year}<span>{item.year}</span>{/if}
						{#if item.rating}<span class="rating">★ {item.rating.toFixed(1)}</span>{/if}
						{#if existingRating}
							<span class="user-rating" title="Your rating">
								{'★'.repeat(existingRating.rating)}{'☆'.repeat(5 - existingRating.rating)}
							</span>
						{/if}
						<StatusBadge status={item.status} />
						{#if item.anime}<span class="anime-tag">ANIME {item.type === 'movie' ? 'MOVIE' : 'SERIES'}</span>{/if}
					</div>
					{#if genresArr.length > 0}
						<div class="genres">
							{#each genresArr as g}<span class="genre">{g}</span>{/each}
						</div>
					{/if}
					{#if item.overview}
						<p class="overview">{item.overview}</p>
					{/if}

					<div class="meta-details">
						{#if item.tmdb_id}<span>TMDB: {item.tmdb_id}</span>{/if}
						{#if item.imdb_id}<span>IMDB: {item.imdb_id}</span>{/if}
						{#if item.anilist_id}<span>AniList: {item.anilist_id}</span>{/if}
						{#if item.tvdb_id}<span>TVDB: {item.tvdb_id}</span>{/if}
					</div>

					<div class="toolbar">
						{#if hasWantedEpisodes}
							<button class="btn btn-primary" on:click={searchAll}>Search All Wanted</button>
						{/if}
						<button class="btn btn-secondary" on:click={() => viewReleases()}>View Releases</button>
						{#if item.type === 'series' && item.tmdb_id}
							<button class="btn btn-secondary" on:click={openSeasonPicker} disabled={loadingSeasons}>
								{loadingSeasons ? 'Loading...' : 'Add Seasons'}
							</button>
						{/if}
						{#if item.tmdb_id}
							<button class="btn btn-secondary" on:click={openRatingModal}>
								{existingRating ? 'Edit Rating' : 'Rate'}
							</button>
						{/if}
						<select value={item.quality_profile_id} on:change={(e) => updateProfile(parseInt(e.target.value))}>
							{#each profiles as p}
								<option value={p.id}>{p.name}</option>
							{/each}
						</select>
						<button class="btn btn-danger" on:click={() => deleteItem(false)}>Remove</button>
						{#if item.has_files}
							<button class="btn btn-danger" on:click={() => deleteItem(true)}>Delete Files</button>
						{/if}
					</div>

					{#if error}
						<div class="error-banner">{error}</div>
					{/if}
				</div>
			</div>

			{#if item.type === 'series' && seasons.length > 0}
				<div class="episodes-section">
					<h2>Episodes</h2>
					<EpisodeList mediaId={id} {seasons} onViewReleases={viewReleases} />
				</div>
			{/if}
		</div>
	</div>

	{#if showReleases}
		<!-- svelte-ignore a11y-click-events-have-key-events -->
		<div class="modal-overlay" on:click={() => showReleases = false} role="presentation">
			<div class="modal releases-modal" on:click|stopPropagation on:keydown|stopPropagation role="dialog">
				<h2>Available Releases{releasesContext ? ` — ${releasesContext}` : ''}</h2>
				{#if searchingReleases}
					<div class="loading">Searching indexers...</div>
				{:else if releases.length === 0}
					<div class="empty">No releases found</div>
				{:else}
					<div class="releases-list">
						{#each releases.sort((a, b) => b.score - a.score) as rel}
							<div class="release" class:rejected={!rel.acceptable}>
								<div class="rel-info">
									<div class="rel-title">{rel.title}</div>
									<div class="rel-meta">
										<span class="rel-type-badge" class:torrent={rel.download_type === 'torrent'}>{rel.download_type === 'torrent' ? 'Torrent' : 'NZB'}</span>
										{#if rel.quality}<span class="rel-quality">{rel.quality}</span>{/if}
										{#if rel.indexer}<span>via {rel.indexer}</span>{/if}
										{#if rel.size}<span>{(rel.size / 1024 / 1024 / 1024).toFixed(1)} GB</span>{/if}
										{#if rel.download_type === 'torrent' && rel.seeders != null}
											<span class="rel-seeders">{rel.seeders}S / {rel.leechers || 0}L</span>
										{/if}
										{#each rel.tags || [] as tag}<span class="rel-tag">{tag}</span>{/each}
									</div>
									{#if !rel.acceptable}
										<div class="reject-reason">{rel.reject_reason}</div>
									{/if}
								</div>
								<div class="rel-score">{rel.score || 0}</div>
								{#if rel.acceptable}
									<button class="btn btn-small" on:click={() => grabRelease(rel)}>Grab</button>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	{/if}

	{#if showSeasonPicker}
		<!-- svelte-ignore a11y-click-events-have-key-events -->
		<div class="modal-overlay" on:click={() => showSeasonPicker = false} role="presentation">
			<div class="modal season-modal" on:click|stopPropagation on:keydown|stopPropagation role="dialog">
				<h2>Add Seasons</h2>
				{#if availableNewSeasons.length === 0}
					<div class="empty">No additional seasons available</div>
				{:else}
					<div class="season-picker">
						<div class="season-picker-header">
							<span class="season-picker-title">Select seasons to add</span>
							<div class="season-picker-actions">
								<button class="link-btn" on:click={selectAllNew}>All</button>
								<button class="link-btn" on:click={deselectAllNew}>None</button>
							</div>
						</div>
						<div class="season-grid">
							{#each availableNewSeasons as season}
								<label class="season-check" class:checked={selectedNewSeasons.has(season.season_number)}>
									<input type="checkbox"
										checked={selectedNewSeasons.has(season.season_number)}
										on:change={() => toggleNewSeason(season.season_number)} />
									<span class="season-label">
										{season.name || `Season ${season.season_number}`}
										{#if season.episode_count}
											<span class="ep-count">({season.episode_count} ep)</span>
										{/if}
									</span>
								</label>
							{/each}
						</div>
						<div class="modal-actions">
							<button class="btn btn-primary" on:click={addSelectedSeasons}
								disabled={addingSeasons || selectedNewSeasons.size === 0}>
								{addingSeasons ? 'Adding...' : `Add ${selectedNewSeasons.size} season${selectedNewSeasons.size !== 1 ? 's' : ''}`}
							</button>
							<button class="btn btn-secondary" on:click={() => showSeasonPicker = false}>Cancel</button>
						</div>
					</div>
				{/if}
			</div>
		</div>
	{/if}

	{#if showRatingModal}
		<!-- svelte-ignore a11y-click-events-have-key-events -->
		<div class="modal-overlay" on:click={() => showRatingModal = false} role="presentation">
			<div class="modal rating-modal" on:click|stopPropagation on:keydown|stopPropagation role="dialog">
				<h2>{existingRating ? 'Edit Rating' : 'Rate'}</h2>
				<div class="rating-input">
					<StarRating bind:value={userRating} />
					{#if userRating > 0}
						<span class="rating-label">{userRating}/5</span>
					{/if}
				</div>
				<div class="comment-input">
					<label for="rating-comment">Comment (optional)</label>
					<textarea
						id="rating-comment"
						bind:value={userComment}
						placeholder="What did you think?"
						rows="3"
					></textarea>
				</div>
				<div class="modal-actions">
					<button class="btn btn-primary" on:click={saveRating} disabled={savingRating}>
						{savingRating ? 'Saving...' : 'Save Rating'}
					</button>
					<button class="btn btn-secondary" on:click={() => showRatingModal = false}>Cancel</button>
				</div>
			</div>
		</div>
	{/if}
{/if}

<style>
	/* ── Layout ── */
	.detail-page {
		position: relative;
		min-height: 100vh;
	}

	/* ── Cinematic Backdrop ── */
	.backdrop {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		height: 420px;
		overflow: hidden;
		z-index: 0;
	}

	.backdrop img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		opacity: 0.3;
		filter: saturate(1.15) brightness(0.85);
		transition: opacity 0.4s ease;
	}

	.backdrop-fade {
		position: absolute;
		bottom: 0;
		left: 0;
		right: 0;
		height: 260px;
		background: linear-gradient(transparent, var(--bg-base));
	}

	/* ── Content ── */
	.content {
		position: relative;
		z-index: 1;
		padding-top: 0.5rem;
	}

	/* ── Back Link ── */
	.back-link {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		color: var(--text-secondary);
		font-family: var(--font-display);
		font-size: 0.82rem;
		font-weight: 600;
		letter-spacing: -0.01em;
		margin-bottom: 1.25rem;
		padding: 0.35rem 0.7rem;
		border-radius: var(--radius-sm);
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		text-decoration: none;
		transition: color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
	}

	.back-link:hover {
		color: var(--accent);
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	/* ── Header (Poster + Info) ── */
	.header {
		display: flex;
		gap: 2rem;
		margin-bottom: 2rem;
	}

	.poster {
		width: 220px;
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg), 0 0 40px rgba(0, 0, 0, 0.4);
		flex-shrink: 0;
		transition: transform 0.25s ease, box-shadow 0.25s ease;
	}

	.poster:hover {
		transform: scale(1.02);
		box-shadow: var(--shadow-lg), var(--shadow-glow);
	}

	/* ── Info Panel ── */
	.info {
		flex: 1;
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-lg);
		padding: 1.5rem 1.75rem;
		box-shadow: var(--shadow-md);
	}

	h1 {
		font-family: var(--font-display);
		font-size: 1.85rem;
		font-weight: 800;
		letter-spacing: -0.02em;
		color: var(--text-primary);
		margin-bottom: 0.5rem;
		line-height: 1.2;
	}

	/* ── Meta Row ── */
	.meta-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.75rem;
		color: var(--text-secondary);
		font-family: var(--font-body);
		font-size: 0.9rem;
	}

	.rating {
		color: var(--gold);
		font-weight: 600;
	}

	.anime-tag {
		background: var(--accent);
		color: var(--text-inverse);
		padding: 2px 8px;
		border-radius: var(--radius-sm);
		font-family: var(--font-display);
		font-size: 0.65rem;
		font-weight: 700;
		letter-spacing: 0.04em;
		text-transform: uppercase;
	}

	/* ── Genres ── */
	.genres {
		display: flex;
		gap: 0.4rem;
		flex-wrap: wrap;
		margin-bottom: 0.75rem;
	}

	.genre {
		background: var(--accent-subtle);
		padding: 3px 12px;
		border-radius: var(--radius-xl);
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--text-secondary);
		border: 1px solid var(--glass-border);
		transition: color 0.2s ease, border-color 0.2s ease;
	}

	.genre:hover {
		color: var(--text-primary);
		border-color: var(--accent);
	}

	/* ── Overview ── */
	.overview {
		font-family: var(--font-body);
		font-size: 0.875rem;
		color: var(--text-secondary);
		line-height: 1.65;
		margin-bottom: 1rem;
		max-width: 620px;
	}

	/* ── Meta Details (IDs) ── */
	.meta-details {
		display: flex;
		gap: 1rem;
		font-family: var(--font-body);
		font-size: 0.75rem;
		color: var(--text-muted);
		margin-bottom: 1.25rem;
		padding-top: 0.75rem;
		border-top: 1px solid var(--glass-border);
	}

	/* ── Toolbar ── */
	.toolbar {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
		align-items: center;
	}

	/* ── Buttons ── */
	.btn {
		padding: 0.5rem 1rem;
		border-radius: var(--radius-sm);
		font-family: var(--font-display);
		font-size: 0.82rem;
		font-weight: 600;
		border: none;
		cursor: pointer;
		transition: background 0.2s ease, color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease, transform 0.1s ease;
	}

	.btn:active {
		transform: translateY(1px);
	}

	.btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
		transform: none;
	}

	.btn-primary {
		background: var(--accent);
		color: var(--text-inverse);
		box-shadow: var(--shadow-sm);
	}

	.btn-primary:hover:not(:disabled) {
		background: var(--accent-hover);
		box-shadow: var(--shadow-md), 0 0 12px var(--accent-glow);
		transform: translateY(-1px);
	}

	.btn-secondary {
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		color: var(--text-secondary);
	}

	.btn-secondary:hover:not(:disabled) {
		color: var(--text-primary);
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	.btn-danger {
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		color: var(--danger);
		border: 1px solid var(--glass-border);
	}

	.btn-danger:hover:not(:disabled) {
		background: var(--danger-bg);
		border-color: var(--danger);
		box-shadow: 0 0 0 3px rgba(239, 68, 68, 0.15);
	}

	.btn-small {
		padding: 0.3rem 0.75rem;
		font-family: var(--font-display);
		font-size: 0.78rem;
		font-weight: 600;
		background: var(--accent);
		color: var(--text-inverse);
		border: none;
		border-radius: var(--radius-sm);
		cursor: pointer;
		transition: background 0.2s ease, box-shadow 0.2s ease, transform 0.1s ease;
	}

	.btn-small:hover {
		background: var(--accent-hover);
		box-shadow: 0 0 8px var(--accent-glow);
		transform: translateY(-1px);
	}

	/* ── Select ── */
	select {
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		color: var(--text-primary);
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius-sm);
		font-family: var(--font-body);
		font-size: 0.85rem;
		outline: none;
		transition: border-color 0.2s ease, box-shadow 0.2s ease;
	}

	select:focus {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	/* ── Episodes Section ── */
	.episodes-section h2 {
		font-family: var(--font-display);
		font-size: 1.2rem;
		font-weight: 700;
		letter-spacing: -0.02em;
		color: var(--text-primary);
		margin-bottom: 0.75rem;
	}

	/* ── Error Banner ── */
	.error-banner {
		background: var(--danger-bg);
		border: 1px solid var(--danger-border);
		color: var(--danger);
		padding: 0.6rem 1rem;
		border-radius: var(--radius-sm);
		font-family: var(--font-body);
		font-size: 0.85rem;
		margin-top: 0.75rem;
	}

	/* ── Loading / Empty ── */
	.loading, .empty {
		text-align: center;
		color: var(--text-muted);
		font-family: var(--font-body);
		padding: 3rem 2rem;
		font-size: 0.9rem;
	}

	/* ── Modal Overlay ── */
	.modal-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.6);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 200;
		padding: 2rem;
	}

	/* ── Releases Modal ── */
	.releases-modal {
		width: 100%;
		max-width: 800px;
		max-height: 80vh;
		overflow-y: auto;
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-lg);
		padding: 1.5rem;
		box-shadow: var(--shadow-lg);
	}

	.releases-modal h2 {
		font-family: var(--font-display);
		font-size: 1.15rem;
		font-weight: 700;
		letter-spacing: -0.02em;
		color: var(--text-primary);
		margin-bottom: 1rem;
	}

	.releases-list {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.release {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 0.75rem;
		background: var(--glass-bg);
		border-radius: var(--radius-md);
		border: 1px solid var(--glass-border);
		transition: border-color 0.2s ease, box-shadow 0.2s ease;
	}

	.release:hover:not(.rejected) {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	.release.rejected {
		opacity: 0.4;
	}

	.rel-info {
		flex: 1;
		min-width: 0;
	}

	.rel-title {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--text-primary);
		word-break: break-all;
	}

	.rel-meta {
		display: flex;
		gap: 0.5rem;
		font-family: var(--font-body);
		font-size: 0.75rem;
		color: var(--text-muted);
		margin-top: 0.3rem;
		flex-wrap: wrap;
	}

	.rel-quality {
		color: var(--accent);
		font-weight: 600;
	}

	.rel-type-badge {
		background: var(--accent-subtle);
		color: var(--accent);
		padding: 1px 6px;
		border-radius: var(--radius-sm);
		font-family: var(--font-display);
		font-size: 0.68rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.02em;
	}

	.rel-type-badge.torrent {
		background: var(--success-bg);
		color: var(--success);
	}

	.rel-seeders {
		color: var(--success);
		font-weight: 600;
	}

	.rel-tag {
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		padding: 0 6px;
		border-radius: var(--radius-sm);
		font-size: 0.7rem;
	}

	.reject-reason {
		font-size: 0.7rem;
		color: var(--danger);
		margin-top: 3px;
	}

	.rel-score {
		font-family: var(--font-display);
		font-size: 1.1rem;
		font-weight: 800;
		color: var(--accent);
		min-width: 40px;
		text-align: center;
	}

	/* ── User Rating Inline ── */
	.user-rating {
		color: var(--gold);
		font-size: 0.85rem;
		letter-spacing: 1px;
	}

	/* ── Season Picker & Rating Modals ── */
	.season-modal, .rating-modal {
		width: 100%;
		max-width: 500px;
		max-height: 70vh;
		overflow-y: auto;
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-lg);
		padding: 1.5rem;
		box-shadow: var(--shadow-lg);
	}

	.season-modal h2, .rating-modal h2 {
		font-family: var(--font-display);
		font-size: 1.15rem;
		font-weight: 700;
		letter-spacing: -0.02em;
		color: var(--text-primary);
		margin-bottom: 1rem;
	}

	.modal-actions {
		display: flex;
		gap: 0.5rem;
		margin-top: 1rem;
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
		font-family: var(--font-display);
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
		font-family: var(--font-display);
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
		gap: 0.25rem;
		margin-bottom: 0.75rem;
		max-height: 300px;
		overflow-y: auto;
	}

	.season-check {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.4rem 0.6rem;
		border-radius: var(--radius-sm);
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--text-secondary);
		cursor: pointer;
		transition: background 0.2s ease, color 0.2s ease;
	}

	.season-check:hover {
		background: var(--accent-subtle);
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

	/* ── Rating Modal ── */
	.rating-input {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 1rem;
	}

	.rating-label {
		font-family: var(--font-display);
		font-size: 0.9rem;
		font-weight: 600;
		color: var(--text-secondary);
	}

	.comment-input {
		margin-bottom: 1rem;
	}

	.comment-input label {
		display: block;
		font-family: var(--font-display);
		font-size: 0.8rem;
		font-weight: 600;
		color: var(--text-secondary);
		margin-bottom: 0.35rem;
	}

	.comment-input textarea {
		width: 100%;
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		color: var(--text-primary);
		border-radius: var(--radius-sm);
		padding: 0.6rem 0.75rem;
		resize: vertical;
		font-family: var(--font-body);
		font-size: 0.85rem;
		outline: none;
		transition: border-color 0.2s ease, box-shadow 0.2s ease;
	}

	.comment-input textarea:focus {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	/* ── Responsive ── */
	@media (max-width: 768px) {
		.header {
			flex-direction: column;
			align-items: center;
			text-align: center;
		}

		.poster {
			width: 160px;
		}

		.info {
			padding: 1.25rem;
		}

		.toolbar {
			justify-content: center;
		}

		.backdrop {
			height: 280px;
		}

		h1 {
			font-size: 1.4rem;
		}

		.meta-row {
			justify-content: center;
			flex-wrap: wrap;
		}

		.genres {
			justify-content: center;
		}

		.meta-details {
			justify-content: center;
			flex-wrap: wrap;
		}
	}
</style>
