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
					<div class="grid music-grid">
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
			<div class="grid music-grid">
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
			<div class="grid music-grid">
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
				<div class="grid music-grid">
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
	.page-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 1.25rem;
	}

	h1 { font-size: 1.5rem; font-weight: 700; }

	.tab-selector, .view-toggle {
		display: flex;
		gap: 0.25rem;
		background: var(--bg-surface);
		border-radius: 8px;
		padding: 3px;
	}

	.tab-selector button, .view-toggle button {
		padding: 0.4rem 1rem;
		border: none;
		background: transparent;
		color: var(--text-secondary);
		border-radius: 6px;
		font-size: 0.85rem;
		font-weight: 500;
		cursor: pointer;
	}

	.tab-selector button.active, .view-toggle button.active {
		background: var(--accent);
		color: var(--text-inverse);
	}

	.search-row {
		display: flex;
		gap: 0.75rem;
		align-items: center;
		margin-bottom: 1rem;
	}

	.search-type-selector {
		display: flex;
		gap: 0.25rem;
		background: var(--bg-surface);
		border-radius: 8px;
		padding: 3px;
		flex-shrink: 0;
	}

	.search-type-selector button {
		padding: 0.35rem 0.75rem;
		border: none;
		background: transparent;
		color: var(--text-secondary);
		border-radius: 6px;
		font-size: 0.8rem;
		font-weight: 500;
		cursor: pointer;
	}

	.search-type-selector button.active {
		background: var(--accent);
		color: var(--text-inverse);
	}

	.search-bar-wrap { flex: 1; }

	.section-title {
		font-size: 1rem;
		font-weight: 600;
		color: var(--text-secondary);
		margin: 1.25rem 0 0.75rem;
	}

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
	}

	.link-btn:hover { text-decoration: underline; }

	.music-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
		gap: 1rem;
	}

	.album-card {
		cursor: pointer;
		transition: transform 0.2s;
		text-decoration: none;
		color: inherit;
	}

	.album-card:hover {
		transform: translateY(-4px);
	}

	.album-cover {
		position: relative;
		aspect-ratio: 1/1;
		border-radius: 8px;
		overflow: hidden;
		background: var(--bg-elevated);
		margin-bottom: 0.5rem;
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
		padding: 0 0.25rem;
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
		gap: 0.25rem;
	}

	.artist-row {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 0.75rem;
		border-radius: 8px;
		cursor: pointer;
		text-decoration: none;
		color: inherit;
	}

	.artist-row:hover {
		background: var(--bg-hover);
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
	.arrow { color: var(--text-muted); font-size: 1.2rem; }

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
		border: 1px solid var(--border);
		background: var(--bg-surface);
		color: var(--text-secondary);
		border-radius: 20px;
		font-size: 0.8rem;
		cursor: pointer;
	}

	.filter-btn.active {
		background: var(--accent);
		color: var(--text-inverse);
		border-color: var(--accent);
	}

	.loading, .empty {
		text-align: center;
		color: var(--text-muted);
		padding: 3rem;
	}

	.modal-overlay {
		position: fixed;
		inset: 0;
		background: var(--bg-overlay);
		z-index: 200;
		padding: 2rem;
		overflow-y: auto;
	}

	.modal {
		width: 100%;
		max-width: 700px;
		margin: 0 auto;
		position: relative;
		border-radius: 12px;
	}

	.modal-close {
		position: absolute;
		top: 12px;
		right: 12px;
		background: var(--bg-overlay);
		border: none;
		color: var(--text-primary);
		width: 32px;
		height: 32px;
		border-radius: 50%;
		font-size: 1rem;
		z-index: 10;
		cursor: pointer;
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
