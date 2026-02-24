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

	async function viewReleases() {
		showReleases = true;
		searchingReleases = true;
		error = '';
		try {
			releases = await api.searchReleases(id);
		} catch (e) {
			error = e.message;
		}
		searchingReleases = false;
	}

	async function grabRelease(rel) {
		error = '';
		try {
			await api.grabRelease({
				release_url: rel.nzb_url,
				media_item_id: id
			});
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
						{#if item.anime}<span class="anime-tag">ANIME</span>{/if}
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
						{#if item.type === 'movie'}
							<button class="btn btn-secondary" on:click={viewReleases}>View Releases</button>
						{/if}
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
					<EpisodeList mediaId={id} {seasons} />
				</div>
			{/if}
		</div>
	</div>

	{#if showReleases}
		<!-- svelte-ignore a11y-click-events-have-key-events -->
		<div class="modal-overlay" on:click={() => showReleases = false} role="presentation">
			<div class="modal releases-modal" on:click|stopPropagation on:keydown|stopPropagation role="dialog">
				<h2>Available Releases</h2>
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
										{#if rel.quality}<span class="rel-quality">{rel.quality}</span>{/if}
										{#if rel.indexer}<span>via {rel.indexer}</span>{/if}
										{#if rel.size}<span>{(rel.size / 1024 / 1024 / 1024).toFixed(1)} GB</span>{/if}
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
	.detail-page { position: relative; }

	.backdrop {
		position: absolute;
		top: 0;
		left: -2rem;
		right: -2rem;
		height: 300px;
		overflow: hidden;
		z-index: 0;
	}

	.backdrop img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		opacity: 0.25;
	}

	.backdrop-fade {
		position: absolute;
		bottom: 0;
		left: 0;
		right: 0;
		height: 150px;
		background: linear-gradient(transparent, var(--bg-base));
	}

	.content {
		position: relative;
		z-index: 1;
	}

	.back-link {
		display: inline-block;
		color: var(--accent);
		font-size: 0.85rem;
		margin-bottom: 1rem;
	}

	.header {
		display: flex;
		gap: 2rem;
		margin-bottom: 2rem;
	}

	.poster {
		width: 200px;
		border-radius: 10px;
		box-shadow: var(--shadow-lg);
		flex-shrink: 0;
	}

	.info { flex: 1; }

	h1 {
		font-size: 1.75rem;
		font-weight: 700;
		margin-bottom: 0.5rem;
	}

	.meta-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.75rem;
		color: var(--text-secondary);
		font-size: 0.9rem;
	}

	.rating { color: var(--gold); }

	.anime-tag {
		background: var(--accent);
		color: var(--text-inverse);
		padding: 2px 8px;
		border-radius: 4px;
		font-size: 0.65rem;
		font-weight: 700;
	}

	.genres {
		display: flex;
		gap: 0.4rem;
		flex-wrap: wrap;
		margin-bottom: 0.75rem;
	}

	.genre {
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
		margin-bottom: 1rem;
		max-width: 600px;
	}

	.meta-details {
		display: flex;
		gap: 1rem;
		font-size: 0.75rem;
		color: var(--text-muted);
		margin-bottom: 1rem;
	}

	.toolbar {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
		align-items: center;
	}

	.btn {
		padding: 0.5rem 1rem;
		border-radius: 6px;
		font-size: 0.85rem;
		font-weight: 600;
		border: none;
		cursor: pointer;
	}

	.btn-primary { background: var(--accent); color: var(--text-inverse); }
	.btn-primary:hover { background: var(--accent-hover); }
	.btn-secondary { background: var(--bg-elevated); border: 1px solid var(--border); color: var(--text-secondary); }
	.btn-secondary:hover { background: var(--bg-hover); color: var(--text-primary); }
	.btn-danger { background: var(--danger-bg); color: var(--danger); border: 1px solid var(--danger-border); }
	.btn-danger:hover { background: var(--danger-bg); border-color: var(--danger); }
	.btn-small { padding: 0.3rem 0.75rem; font-size: 0.8rem; background: var(--accent); color: var(--text-inverse); border: none; border-radius: 4px; }

	select {
		background: var(--bg-input);
		border: 1px solid var(--border-subtle);
		color: var(--text-primary);
		padding: 0.5rem 0.75rem;
		border-radius: 6px;
		font-size: 0.85rem;
	}

	.episodes-section h2 {
		font-size: 1.1rem;
		margin-bottom: 0.75rem;
	}

	.error-banner {
		background: var(--danger-bg);
		border: 1px solid var(--danger-border);
		color: var(--danger);
		padding: 0.6rem 1rem;
		border-radius: 6px;
		font-size: 0.85rem;
		margin-top: 0.75rem;
	}

	.loading, .empty {
		text-align: center;
		color: var(--text-muted);
		padding: 2rem;
	}

	.modal-overlay {
		position: fixed;
		inset: 0;
		background: var(--bg-overlay);
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 200;
		padding: 2rem;
	}

	.releases-modal {
		width: 100%;
		max-width: 800px;
		max-height: 80vh;
		overflow-y: auto;
		background: var(--bg-surface);
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
		border: 1px solid var(--border);
		border-radius: 12px;
		padding: 1.5rem;
	}

	.releases-modal h2 {
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
		background: var(--bg-surface);
		border-radius: 6px;
		border: 1px solid var(--border);
	}

	.release.rejected {
		opacity: 0.4;
	}

	.rel-info { flex: 1; min-width: 0; }

	.rel-title {
		font-size: 0.85rem;
		word-break: break-all;
	}

	.rel-meta {
		display: flex;
		gap: 0.5rem;
		font-size: 0.75rem;
		color: var(--text-muted);
		margin-top: 0.25rem;
		flex-wrap: wrap;
	}

	.rel-quality {
		color: var(--accent);
	}

	.rel-tag {
		background: var(--bg-elevated);
		padding: 0 6px;
		border-radius: 3px;
	}

	.reject-reason {
		font-size: 0.7rem;
		color: var(--danger);
		margin-top: 2px;
	}

	.rel-score {
		font-size: 1.1rem;
		font-weight: 700;
		color: var(--accent);
		min-width: 40px;
		text-align: center;
	}

	/* User rating inline */
	.user-rating {
		color: var(--gold);
		font-size: 0.85rem;
		letter-spacing: 1px;
	}

	/* Season picker modal */
	.season-modal, .rating-modal {
		width: 100%;
		max-width: 500px;
		max-height: 70vh;
		overflow-y: auto;
		background: var(--bg-surface);
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
		border: 1px solid var(--border);
		border-radius: 12px;
		padding: 1.5rem;
	}

	.season-modal h2, .rating-modal h2 {
		margin-bottom: 1rem;
		font-size: 1.1rem;
	}

	.modal-actions {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.75rem;
	}

	.season-picker { width: 100%; }

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
		max-height: 300px;
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

	.season-check:hover { background: var(--bg-hover); }
	.season-check.checked { color: var(--text-primary); }
	.season-check input { accent-color: var(--accent); }
	.ep-count { font-size: 0.75rem; color: var(--text-muted); }

	/* Rating modal */
	.rating-input {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 1rem;
	}

	.rating-label {
		font-size: 0.9rem;
		color: var(--text-secondary);
	}

	.comment-input {
		margin-bottom: 1rem;
	}

	.comment-input label {
		display: block;
		font-size: 0.8rem;
		color: var(--text-secondary);
		margin-bottom: 0.35rem;
	}

	.comment-input textarea {
		width: 100%;
		background: var(--bg-input);
		border: 1px solid var(--border-subtle);
		color: var(--text-primary);
		border-radius: 6px;
		padding: 0.6rem 0.75rem;
		resize: vertical;
		font-size: 0.85rem;
		font-family: inherit;
	}

	@media (max-width: 768px) {
		.header {
			flex-direction: column;
			align-items: center;
			text-align: center;
		}

		.poster { width: 150px; }
		.toolbar { justify-content: center; }
	}
</style>
