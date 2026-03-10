<script>
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';
	import SearchBar from '$lib/components/SearchBar.svelte';
	import MusicDetail from '$lib/components/MusicDetail.svelte';

	let searchQuery = '';
	let results = [];
	let trending = [];
	let profiles = [];
	let loading = false;
	let selectedItem = null;
	let activeTab = 'discover'; // 'discover' or 'library'
	let searchType = 'album'; // 'album', 'artist', or 'track'
	let libraryAlbums = [];
	let libraryArtists = [];
	let libraryFilter = 'all';
	let libraryView = 'albums'; // 'albums' or 'artists'
	let loadingLibrary = false;

	// Artist search results → show their albums
	let artistAlbums = [];
	let selectedArtistName = '';
	let loadingArtistAlbums = false;

	onMount(async () => {
		profiles = await api.getProfiles();
		loadTrending();
		loadLibrary();
	});

	async function loadTrending() {
		try {
			trending = await api.musicTrending(1);
			if (!Array.isArray(trending)) trending = [];
		} catch {
			trending = [];
		}
	}

	async function loadLibrary() {
		loadingLibrary = true;
		try {
			libraryAlbums = await api.getMusicLibrary(libraryFilter);
			if (!Array.isArray(libraryAlbums)) libraryAlbums = [];
		} catch {
			libraryAlbums = [];
		}
		try {
			libraryArtists = await api.getMusicArtists();
			if (!Array.isArray(libraryArtists)) libraryArtists = [];
		} catch {
			libraryArtists = [];
		}
		loadingLibrary = false;
	}

	async function handleSearch(e) {
		const query = e.detail;
		if (!query) {
			results = [];
			artistAlbums = [];
			selectedArtistName = '';
			return;
		}
		loading = true;
		artistAlbums = [];
		selectedArtistName = '';
		try {
			results = await api.musicSearch(query, searchType);
			if (!Array.isArray(results)) results = [];
		} catch (err) {
			notify(err?.message || 'Search failed', 'error');
			results = [];
		}
		loading = false;
	}

	async function viewArtistAlbums(artist) {
		loadingArtistAlbums = true;
		selectedArtistName = artist.name;
		try {
			const data = await api.musicArtist(artist.id);
			artistAlbums = (data.albums || []).map(rg => ({
				id: rg.id,
				title: rg.title,
				artist: artist.name,
				artist_id: artist.id,
				year: rg['first-release-date'] || '',
				type: rg['primary-type'] || '',
				cover_url: `https://coverartarchive.org/release-group/${rg.id}/front-250`
			}));
		} catch (err) {
			notify(err?.message || 'Failed to load artist albums', 'error');
			artistAlbums = [];
		}
		loadingArtistAlbums = false;
	}

	function showDetail(item) {
		selectedItem = item;
	}

	function handleAdded() {
		notify('Album added to library!', 'success');
		selectedItem = null;
		loadLibrary();
	}

	function setFilter(f) {
		libraryFilter = f;
		loadLibrary();
	}

	function clearSearch() {
		searchQuery = '';
		results = [];
		artistAlbums = [];
		selectedArtistName = '';
	}

	$: displayItems = searchQuery ? results : trending;
	$: placeholders = {
		album: 'Search albums...',
		artist: 'Search artists...',
		track: 'Search by track name...'
	};
</script>

<svelte:head>
	<title>Music - Zarr</title>
</svelte:head>

<div class="page">
	<header class="page-header">
		<h1>Music</h1>
		<div class="tab-selector">
			<button class:active={activeTab === 'discover'} on:click={() => activeTab = 'discover'}>Discover</button>
			<button class:active={activeTab === 'library'} on:click={() => activeTab = 'library'}>Library</button>
		</div>
	</header>

	{#if activeTab === 'discover'}
		<div class="search-row">
			<div class="search-type-selector">
				{#each [['album', 'Album'], ['artist', 'Artist'], ['track', 'Track']] as [value, label]}
					<button class:active={searchType === value} on:click={() => { searchType = value; clearSearch(); }}>
						{label}
					</button>
				{/each}
			</div>
			<div class="search-bar-wrap">
				<SearchBar bind:value={searchQuery} on:search={handleSearch} placeholder={placeholders[searchType]} />
			</div>
		</div>

		{#if loading}
			<div class="loading">Searching...</div>
		{/if}

		<!-- Artist search results -->
		{#if searchQuery && searchType === 'artist' && !loading}
			{#if selectedArtistName}
				<div class="breadcrumb">
					<button class="link-btn" on:click={() => { artistAlbums = []; selectedArtistName = ''; }}>
						← Back to artists
					</button>
					<h2 class="section-title">{selectedArtistName} — Albums</h2>
				</div>
				{#if loadingArtistAlbums}
					<div class="loading">Loading albums...</div>
				{:else}
					<div class="grid music-grid stagger-grid">
						{#each artistAlbums as item}
							<!-- svelte-ignore a11y-click-events-have-key-events -->
							<div class="album-card" on:click={() => showDetail(item)} role="button" tabindex="0">
								<div class="album-cover">
									{#if item.cover_url}
										<img src={api.imageUrl(item.cover_url)} alt={item.title} loading="lazy"
											on:error={(e) => e.target.style.display = 'none'} />
									{/if}
									<div class="cover-fallback">{(item.title || '?')[0]}</div>
								</div>
								<div class="album-info">
									<div class="album-title">{item.title}</div>
									{#if item.year}
										<div class="album-year">{typeof item.year === 'string' ? item.year.slice(0, 4) : item.year}</div>
									{/if}
									{#if item.type}<div class="album-type">{item.type}</div>{/if}
								</div>
							</div>
						{/each}
					</div>
					{#if artistAlbums.length === 0}
						<div class="empty">No albums found</div>
					{/if}
				{/if}
			{:else}
				<h2 class="section-title">{results.length} artists found</h2>
				<div class="artist-list">
					{#each results as artist}
						<!-- svelte-ignore a11y-click-events-have-key-events -->
						<div class="artist-row" on:click={() => viewArtistAlbums(artist)} role="button" tabindex="0">
							<div class="artist-initial">{(artist.name || '?')[0]}</div>
							<div class="artist-info">
								<div class="artist-name">{artist.name}</div>
								<div class="artist-meta">
									{#if artist.type}<span>{artist.type}</span>{/if}
									{#if artist.country}<span>{artist.country}</span>{/if}
								</div>
							</div>
							<span class="arrow">→</span>
						</div>
					{/each}
				</div>
			{/if}

		<!-- Album / Track search results -->
		{:else if searchQuery && !loading}
			{#if results.length > 0}
				<h2 class="section-title">{results.length} results</h2>
			{/if}
			<div class="grid music-grid stagger-grid">
				{#each displayItems as item}
					<!-- svelte-ignore a11y-click-events-have-key-events -->
					<div class="album-card" on:click={() => showDetail(item)} role="button" tabindex="0">
						<div class="album-cover">
							{#if item.cover_url || item.image_url}
								<img src={api.imageUrl(item.cover_url || item.image_url)} alt={item.title || item.name} loading="lazy"
									on:error={(e) => e.target.style.display = 'none'} />
							{/if}
							<div class="cover-fallback">{(item.title || item.name || '?')[0]}</div>
							{#if item.in_library}
								<span class="in-lib-badge">IN LIBRARY</span>
							{/if}
						</div>
						<div class="album-info">
							<div class="album-title">{item.title || item.name}</div>
							<div class="album-artist">{item.artist || item.artist_name || ''}</div>
							{#if item.track_name}
								<div class="album-track">♫ {item.track_name}</div>
							{/if}
							{#if item.year}
								<div class="album-year">{typeof item.year === 'string' ? item.year.slice(0, 4) : item.year}</div>
							{/if}
						</div>
					</div>
				{/each}
			</div>
			{#if results.length === 0}
				<div class="empty">No results found for "{searchQuery}"</div>
			{/if}

		<!-- Trending (no search query) -->
		{:else if !searchQuery && !loading}
			<h2 class="section-title">Trending Albums</h2>
			<div class="grid music-grid stagger-grid">
				{#each trending as item}
					<!-- svelte-ignore a11y-click-events-have-key-events -->
					<div class="album-card" on:click={() => showDetail(item)} role="button" tabindex="0">
						<div class="album-cover">
							{#if item.cover_url || item.image_url}
								<img src={api.imageUrl(item.cover_url || item.image_url)} alt={item.title || item.name} loading="lazy"
									on:error={(e) => e.target.style.display = 'none'} />
							{/if}
							<div class="cover-fallback">{(item.title || item.name || '?')[0]}</div>
						</div>
						<div class="album-info">
							<div class="album-title">{item.title || item.name}</div>
							<div class="album-artist">{item.artist || ''}</div>
						</div>
					</div>
				{/each}
			</div>
		{/if}

	{:else}
		<!-- Library tab -->
		<div class="library-controls">
			<div class="view-toggle">
				<button class:active={libraryView === 'albums'} on:click={() => libraryView = 'albums'}>Albums</button>
				<button class:active={libraryView === 'artists'} on:click={() => libraryView = 'artists'}>Artists</button>
			</div>
			{#if libraryView === 'albums'}
				<div class="filter-bar">
					{#each ['all', 'wanted', 'available', 'downloading'] as f}
						<button class="filter-btn" class:active={libraryFilter === f} on:click={() => setFilter(f)}>
							{f[0].toUpperCase() + f.slice(1)}
						</button>
					{/each}
				</div>
			{/if}
		</div>

		{#if loadingLibrary}
			<div class="loading">Loading library...</div>
		{:else if libraryView === 'albums'}
			{#if libraryAlbums.length === 0}
				<div class="empty">No albums in library{libraryFilter !== 'all' ? ` with status "${libraryFilter}"` : ''}</div>
			{:else}
				<div class="grid music-grid stagger-grid">
					{#each libraryAlbums as album}
						<a href="/music/{album.id}" class="album-card">
							<div class="album-cover">
								{#if album.release_group_id}
									<img src={api.musicCoverUrl(album.release_group_id)} alt={album.title} loading="lazy"
										on:error={(e) => e.target.style.display = 'none'} />
								{/if}
								{#if album.image_url}
									<img src={api.imageUrl(album.image_url)} alt={album.title} loading="lazy" class="fallback-img" />
								{/if}
								<div class="cover-fallback">{(album.title || '?')[0]}</div>
								<span class="status-badge status-{album.status}">{album.status}</span>
							</div>
							<div class="album-info">
								<div class="album-title">{album.title}</div>
								<div class="album-artist">{album.artist_name}</div>
								{#if album.year}
									<div class="album-year">{album.year}</div>
								{/if}
							</div>
						</a>
					{/each}
				</div>
			{/if}
		{:else}
			<!-- Artists view -->
			{#if libraryArtists.length === 0}
				<div class="empty">No artists in library</div>
			{:else}
				<div class="artist-list">
					{#each libraryArtists as artist}
						<a href="/music?artist={artist.id}" class="artist-row"
							on:click|preventDefault={() => { libraryFilter = 'all'; libraryView = 'albums'; libraryAlbums = libraryAlbums; }}
						>
							<div class="artist-initial">{(artist.name || '?')[0]}</div>
							<div class="artist-info">
								<div class="artist-name">{artist.name}</div>
								<div class="artist-meta">
									<span>{artist.album_count} album{artist.album_count !== 1 ? 's' : ''}</span>
									<span>{artist.available_count} available</span>
								</div>
							</div>
							<span class="arrow">→</span>
						</a>
					{/each}
				</div>
			{/if}
		{/if}
	{/if}
</div>

{#if selectedItem}
	<!-- svelte-ignore a11y-click-events-have-key-events -->
	<div class="modal-overlay" on:click={() => selectedItem = null} role="presentation">
		<div class="modal" on:click|stopPropagation on:keydown|stopPropagation role="dialog">
			<button class="modal-close" on:click={() => selectedItem = null}>✕</button>
			<MusicDetail
				item={selectedItem}
				{profiles}
				on:added={handleAdded}
				on:error={(e) => notify(e.detail, 'error')}
			/>
		</div>
	</div>
{/if}

<style>
	/* Page header */
	.page-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 1.25rem;
	}

	h1 {
		font-family: var(--font-display);
		font-size: 1.75rem;
		font-weight: 800;
		letter-spacing: -0.02em;
	}

	/* Tab selector & view toggle — glass panels */
	.tab-selector, .view-toggle {
		display: flex;
		gap: 0.25rem;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-md);
		padding: 3px;
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
	}

	.tab-selector button, .view-toggle button {
		padding: 0.4rem 1rem;
		border: none;
		background: transparent;
		color: var(--text-secondary);
		border-radius: calc(var(--radius-md) - 2px);
		font-size: 0.85rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.2s ease;
	}

	.tab-selector button.active, .view-toggle button.active {
		background: var(--accent);
		color: var(--text-inverse);
		box-shadow: 0 0 12px color-mix(in srgb, var(--accent) 40%, transparent);
	}

	/* Search row */
	.search-row {
		display: flex;
		gap: 0.75rem;
		align-items: center;
		margin-bottom: 1.75rem;
	}

	.search-type-selector {
		display: flex;
		gap: 0.25rem;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-md);
		padding: 3px;
		flex-shrink: 0;
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
	}

	.search-type-selector button {
		padding: 0.35rem 0.75rem;
		border: none;
		background: transparent;
		color: var(--text-secondary);
		border-radius: calc(var(--radius-md) - 2px);
		font-size: 0.8rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.2s ease;
	}

	.search-type-selector button.active {
		background: var(--accent);
		color: var(--text-inverse);
		box-shadow: 0 0 12px color-mix(in srgb, var(--accent) 40%, transparent);
	}

	.search-bar-wrap { flex: 1; }

	/* Section title — editorial style */
	.section-title {
		font-family: var(--font-display);
		font-size: 1.05rem;
		font-weight: 700;
		letter-spacing: -0.02em;
		color: var(--text-secondary);
		margin: 1.25rem 0 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		font-size: 0.7rem;
		opacity: 0.7;
	}

	/* Breadcrumb */
	.breadcrumb {
		margin-bottom: 0.5rem;
	}

	.link-btn {
		background: none;
		border: none;
		color: var(--accent);
		cursor: pointer;
		font-size: 0.85rem;
		padding: 0;
		margin-bottom: 0.25rem;
		transition: opacity 0.2s ease;
	}

	.link-btn:hover { text-decoration: underline; opacity: 0.8; }

	/* Music grid */
	.music-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
		gap: 1.25rem;
	}

	/* Album card — glass card */
	.album-card {
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-md);
		cursor: pointer;
		transition: transform 0.25s ease, box-shadow 0.25s ease;
		text-decoration: none;
		color: inherit;
		overflow: hidden;
	}

	.album-card:hover {
		transform: translateY(-2px);
		box-shadow: 0 8px 24px rgba(0, 0, 0, 0.15);
	}

	.album-cover {
		position: relative;
		aspect-ratio: 1/1;
		overflow: hidden;
		background: var(--bg-elevated);
	}

	.album-cover img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		position: absolute;
		top: 0;
		left: 0;
	}

	.album-cover .fallback-img {
		z-index: 0;
	}

	.cover-fallback {
		width: 100%;
		height: 100%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 2rem;
		font-weight: 700;
		color: var(--text-muted);
		background: linear-gradient(135deg, var(--bg-elevated), var(--bg-surface));
	}

	.in-lib-badge {
		position: absolute;
		top: 6px;
		right: 6px;
		background: var(--accent);
		color: var(--text-inverse);
		font-size: 0.6rem;
		font-weight: 700;
		padding: 2px 6px;
		border-radius: 3px;
	}

	.status-badge {
		position: absolute;
		bottom: 6px;
		left: 6px;
		font-size: 0.6rem;
		font-weight: 700;
		padding: 2px 6px;
		border-radius: 3px;
		text-transform: uppercase;
	}

	.status-wanted { background: var(--gold); color: #000; }
	.status-available { background: var(--success); color: #fff; }
	.status-downloading { background: var(--accent); color: var(--text-inverse); }

	.album-info {
		padding: 0.5rem 0.6rem;
	}

	.album-title {
		font-size: 0.85rem;
		font-weight: 600;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.album-artist {
		font-size: 0.75rem;
		color: var(--text-secondary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.album-track {
		font-size: 0.7rem;
		color: var(--accent);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.album-year {
		font-size: 0.7rem;
		color: var(--text-muted);
	}

	.album-type {
		font-size: 0.65rem;
		color: var(--text-muted);
		text-transform: uppercase;
	}

	/* Artist list */
	.artist-list {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}

	.artist-row {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 0.75rem;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-md);
		cursor: pointer;
		text-decoration: none;
		color: inherit;
		transition: transform 0.2s ease, box-shadow 0.2s ease, background 0.2s ease;
	}

	.artist-row:hover {
		background: var(--bg-hover);
		transform: translateY(-1px);
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
	}

	.artist-initial {
		width: 44px;
		height: 44px;
		border-radius: 50%;
		background: var(--bg-elevated);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 1.2rem;
		font-weight: 700;
		color: var(--text-muted);
		flex-shrink: 0;
	}

	.artist-info { flex: 1; min-width: 0; }
	.artist-name { font-weight: 600; font-size: 0.9rem; }
	.artist-meta { display: flex; gap: 0.75rem; font-size: 0.75rem; color: var(--text-muted); }
	.arrow { color: var(--text-muted); font-size: 1.2rem; transition: transform 0.2s ease; }
	.artist-row:hover .arrow { transform: translateX(2px); }

	/* Library controls */
	.library-controls {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-bottom: 1rem;
		flex-wrap: wrap;
	}

	.filter-bar {
		display: flex;
		gap: 0.5rem;
	}

	.filter-btn {
		padding: 0.35rem 0.75rem;
		border: 1px solid var(--glass-border);
		background: var(--glass-bg);
		color: var(--text-secondary);
		border-radius: 20px;
		font-size: 0.8rem;
		cursor: pointer;
		transition: all 0.2s ease;
	}

	.filter-btn.active {
		background: var(--accent);
		color: var(--text-inverse);
		border-color: var(--accent);
		box-shadow: 0 0 12px color-mix(in srgb, var(--accent) 40%, transparent);
	}

	/* Loading & empty states */
	.loading {
		text-align: center;
		color: var(--text-muted);
		padding: 3rem;
	}

	.empty {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		text-align: center;
		color: var(--text-muted);
		padding: 3rem;
		gap: 0.75rem;
	}

	.empty::before {
		content: '\1F3B5';
		font-size: 2.5rem;
		opacity: 0.35;
	}

	/* Modal — backdrop blur + scaleIn animation */
	@keyframes scaleIn {
		from {
			opacity: 0;
			transform: scale(0.95);
		}
		to {
			opacity: 1;
			transform: scale(1);
		}
	}

	.modal-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.5);
		backdrop-filter: blur(8px);
		-webkit-backdrop-filter: blur(8px);
		z-index: 200;
		padding: 2rem;
		overflow-y: auto;
	}

	.modal {
		width: 100%;
		max-width: 700px;
		margin: 0 auto;
		position: relative;
		border-radius: var(--radius-md);
		animation: scaleIn 0.25s ease forwards;
	}

	.modal-close {
		position: absolute;
		top: 12px;
		right: 12px;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		color: var(--text-primary);
		width: 32px;
		height: 32px;
		border-radius: 50%;
		font-size: 1rem;
		z-index: 10;
		cursor: pointer;
		transition: background 0.2s ease;
	}

	.modal-close:hover {
		background: var(--bg-hover);
	}

	@media (max-width: 768px) {
		.page-header {
			flex-direction: column;
			gap: 0.75rem;
			align-items: stretch;
		}

		.search-row {
			flex-direction: column;
		}

		.music-grid {
			grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
		}

		.library-controls {
			flex-direction: column;
			align-items: stretch;
		}
	}
</style>
