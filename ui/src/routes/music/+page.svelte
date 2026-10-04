<script>
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import ArchiveCategories from '$lib/components/ArchiveCategories.svelte';
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';
	import { acquisitionState, activeAcquisition, releaseGroupId } from '$lib/music';
	import SearchBar from '$lib/components/SearchBar.svelte';
	import MusicDetail from '$lib/components/MusicDetail.svelte';
	import MusicAlbumCard from '$lib/components/MusicAlbumCard.svelte';
	let mounted = false;
	let disposed = false;
	let searchQuery = '';
	let searchType = 'album';
	let activeTab = 'discover';
	let results = [];
	let profiles = [];
	let selectedItem = null;
	let detailOpenRequest = 0;
	let resolvingAlbum = false;
	let detailOpenError = '';
	let searchRequest = 0;
	let artistRequest = 0;
	let discoveryRequest = 0;
	let libraryRequest = 0;
	let loading = false;
	let searchError = '';
	let searchRefreshing = false;
	let discovery = {
		recommendations: [],
		recently_saved: [],
		similar_artists: [],
		played_not_owned: []
	};
	let discoveryTimer;
	let enrichmentPending = false;
	let discoveryError = '';
	let loadingDiscovery = true;
	let libraryAlbums = [];
	let libraryArtists = [];
	let libraryArtist = '';
	let libraryView = 'albums';
	let libraryFilter = 'all';
	let libraryError = '';
	let loadingLibrary = true;
	let artistAlbums = [];
	let selectedArtistName = '';
	let artistType = 'all';
	let loadingArtistAlbums = false;
	let artistError = '';
	$: if (mounted) applyLocation($page.url.search);
	$: filteredArtistAlbums = artistAlbums.filter(
		(album) => artistType === 'all' || (album.type || '').toLowerCase() === artistType.toLowerCase()
	);
	$: visibleLibraryAlbums = libraryAlbums.filter((album) => {
		if (libraryArtist && album.artist_mbid !== libraryArtist) return false;
		const status = acquisitionState(album).status;
		if (libraryFilter === 'all') return true;
		if (libraryFilter === 'favorites') return album.favorite;
		if (libraryFilter === 'monitored') return album.monitored;
		if (libraryFilter === 'attention')
			return ['failed', 'no_results', 'blocked', 'cancelled'].includes(status);
		if (libraryFilter === 'downloading') return activeAcquisition(album);
		return status === libraryFilter;
	});
	$: shelves = [
		{
			title: 'Recommended for you',
			description: 'A few records to start your next listening session.',
			items: discovery.recommendations
		},
		{
			title: 'Recently saved',
			description: 'Your growing collection, ready when you are.',
			items: discovery.recently_saved
		},
		{
			title: 'Played but not owned',
			description: 'Favorites from your listening history, waiting for a place in your collection.',
			items: discovery.played_not_owned
		},
		{
			title: 'From similar artists',
			description: 'Follow the thread from artists you already enjoy.',
			kind: 'artist',
			items: discovery.similar_artists
		}
	];
	function applyLocation(search) {
		const params = new URLSearchParams(search);
		activeTab = params.get('tab') === 'library' ? 'library' : 'discover';
		libraryArtist = params.get('artist') || '';
		if (libraryArtist) libraryView = 'albums';
		searchType = ['album', 'artist', 'track'].includes(params.get('type'))
			? params.get('type')
			: 'album';
		searchQuery = params.get('q') || '';
		handleSearch({ detail: searchQuery });
	}
	onMount(() => {
		mounted = true;
		api
			.getProfiles()
			.then((data) => {
				if (!disposed) profiles = data;
			})
			.catch(() => {});
		loadDiscovery();
		loadLibrary();
		const timer = setInterval(() => {
			if (libraryAlbums.some((album) => activeAcquisition(album) || album.monitored))
				loadLibrary(true);
		}, 5000);
		return () => {
			disposed = true;
			searchRequest++;
			artistRequest++;
			libraryRequest++;
			discoveryRequest++;
			detailOpenRequest++;
			clearTimeout(discoveryTimer);
			clearInterval(timer);
		};
	});
	async function loadDiscovery(quiet = false, attempt = 0) {
		clearTimeout(discoveryTimer);
		const request = ++discoveryRequest;
		if (!quiet) loadingDiscovery = true;
		discoveryError = '';
		try {
			const data = await api.musicDiscover();
			if (request !== discoveryRequest) return;
			discovery = Object.fromEntries(
				['recommendations', 'recently_saved', 'similar_artists', 'played_not_owned'].map((key) => [
					key,
					Array.isArray(data[key]) ? data[key] : []
				])
			);
			enrichmentPending = !!data.enriching && attempt < 3;
			if (enrichmentPending)
				discoveryTimer = setTimeout(() => loadDiscovery(true, attempt + 1), 4000);
			if (
				!data.enriching &&
				data.configured?.lastfm !== false &&
				!Object.values(discovery).some((items) => items.length)
			) {
				const fallback = await api.musicTrending(1).catch(() => []);
				if (request === discoveryRequest && Array.isArray(fallback))
					discovery = { ...discovery, recommendations: fallback };
			}
		} catch (e) {
			if (request !== discoveryRequest) return;
			discoveryError = e.message;
			enrichmentPending = false;
			try {
				const fallback = await api.musicTrending(1);
				if (request === discoveryRequest && Array.isArray(fallback))
					discovery = { ...discovery, recommendations: fallback };
			} catch {}
		} finally {
			if (request === discoveryRequest) loadingDiscovery = false;
		}
	}
	async function loadLibrary(quiet = false) {
		const request = ++libraryRequest;
		if (!quiet) loadingLibrary = true;
		try {
			const [albums, artists] = await Promise.all([
				api.getMusicLibrary(),
				api.getMusicArtists().catch(() => [])
			]);
			if (request !== libraryRequest) return;
			libraryAlbums = Array.isArray(albums) ? albums : [];
			libraryArtists = Array.isArray(artists) ? artists : [];
			libraryError = '';
		} catch (e) {
			if (request === libraryRequest) libraryError = e.message;
		} finally {
			if (request === libraryRequest) loadingLibrary = false;
		}
	}
	async function handleSearch(event) {
		const request = ++searchRequest;
		artistRequest++;
		selectedArtistName = '';
		artistAlbums = [];
		loadingArtistAlbums = false;
		searchError = '';
		searchRefreshing = false;
		results = [];
		if (!event.detail.trim()) {
			results = [];
			loading = false;
			return;
		}
		loading = true;
		const query = event.detail.trim();
		const type = searchType;
		if (type !== 'track') {
			try {
				const cached = await api.musicSearch(query, type, true);
				if (request !== searchRequest) return;
				if (Array.isArray(cached) && cached.length) {
					results = cached;
					loading = false;
					searchRefreshing = true;
				}
			} catch {}
		}
		if (request !== searchRequest) return;
		try {
			const data = await api.musicSearch(query, type);
			if (request === searchRequest) results = Array.isArray(data) ? data : [];
		} catch (e) {
			if (request === searchRequest) {
				searchError = e.message;
			}
		} finally {
			if (request === searchRequest) {
				loading = false;
				searchRefreshing = false;
			}
		}
	}
	async function viewArtistAlbums(artist) {
		const request = ++artistRequest;
		selectedArtistName = artist.name || artist.title;
		loadingArtistAlbums = true;
		artistError = '';
		artistType = 'all';
		try {
			const artistID = artist.artist_mbid || artist.mbid || artist.id;
			const data = await api.musicArtist(artistID);
			if (request !== artistRequest) return;
			artistAlbums = (data.albums || []).map((album) => ({
				...album,
				release_group_id: album.release_group_id || album.id,
				artist: selectedArtistName,
				artist_mbid: artistID,
				year: album.year || album['first-release-date'] || '',
				type: album.type || album['primary-type'] || ''
			}));
		} catch (e) {
			if (request === artistRequest) {
				artistError = e.message;
				artistAlbums = [];
			}
		} finally {
			if (request === artistRequest) loadingArtistAlbums = false;
		}
	}
	function openDiscoveryArtist(artist) {
		searchRequest++;
		searchQuery = artist.name;
		searchType = 'artist';
		results = [artist];
		searchError = '';
		loading = false;
		viewArtistAlbums(artist);
	}
	function enrich(item) {
		const rgid = releaseGroupId(item);
		const saved = libraryAlbums.find(
			(album) =>
				(rgid && releaseGroupId(album) === rgid) ||
				(item.library_id && album.id === item.library_id)
		);
		return saved
			? {
					...item,
					in_library: true,
					library_id: saved.id,
					acquisition: saved.acquisition,
					monitored: saved.monitored,
					favorite: saved.favorite
				}
			: item;
	}
	async function showDetail(item) {
		const request = ++detailOpenRequest;
		detailOpenError = '';
		resolvingAlbum = item.provider === 'lastfm';
		try {
			const canonical =
				item.provider === 'lastfm'
					? await api.musicResolve(
							item.artist || item.artist_name || '',
							item.title || item.name || '',
							String(item.year || '').slice(0, 4)
						)
					: item;
			if (request === detailOpenRequest) selectedItem = enrich(canonical);
		} catch (e) {
			if (request === detailOpenRequest)
				detailOpenError = `Album details could not be opened: ${e.message}. Try searching by artist and album title.`;
		} finally {
			if (request === detailOpenRequest) resolvingAlbum = false;
		}
	}
	function closeDetail() {
		detailOpenRequest++;
		resolvingAlbum = false;
		selectedItem = null;
	}
	function modalAccessibility(node) {
		const previous = document.activeElement;
		const overflow = document.body.style.overflow;
		document.body.style.overflow = 'hidden';
		node.querySelector('button')?.focus();
		function keydown(event) {
			if (event.key === 'Escape') {
				event.preventDefault();
				event.stopPropagation();
				closeDetail();
			}
			if (event.key !== 'Tab') return;
			const focusable = [
				...node.querySelectorAll(
					'button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), [tabindex="0"]'
				)
			].filter((element) => element.getClientRects().length);
			const first = focusable[0];
			const last = focusable[focusable.length - 1];
			if (event.shiftKey && document.activeElement === first) {
				event.preventDefault();
				last?.focus();
			} else if (!event.shiftKey && document.activeElement === last) {
				event.preventDefault();
				first?.focus();
			}
		}
		node.addEventListener('keydown', keydown, true);
		return {
			destroy() {
				node.removeEventListener('keydown', keydown, true);
				document.body.style.overflow = overflow;
				if (previous?.isConnected) previous.focus();
			}
		};
	}
	function handleAdded(event) {
		notify(
			event.detail?.downloadNow ? 'Album saved. Acquisition requested.' : 'Album saved to library.',
			'success'
		);
		loadLibrary(true);
		loadDiscovery();
	}
	function clearSearch() {
		searchRequest++;
		artistRequest++;
		searchQuery = '';
		results = [];
		selectedArtistName = '';
		loading = false;
		loadingArtistAlbums = false;
		searchError = '';
		searchRefreshing = false;
	}
</script>

<svelte:window
	on:keydown={(event) => {
		if (event.key === 'Escape') closeDetail();
	}}
/>
<svelte:head><title>Music - Zarr</title></svelte:head>
<ArchiveCategories mode={activeTab === 'library' ? 'library' : 'discover'} selected="music" />
<div class="page music-page">
	<header class="page-header">
		<div>
			<span class="eyebrow">Your listening room</span>
			<h1>Music</h1>
		</div>
		<div class="tab-selector">
			<button class:active={activeTab === 'discover'} on:click={() => goto('/music')}
				>Discover</button
			><button class:active={activeTab === 'library'} on:click={() => goto('/music?tab=library')}
				>Library</button
			>
		</div>
	</header>
	{#if resolvingAlbum}<p role="status">Finding this album in the catalog…</p>{/if}
	{#if detailOpenError}<p class="discovery-warning" role="alert">{detailOpenError}</p>{/if}
	{#if activeTab === 'discover'}
		<div class="search-row">
			<div class="search-type-selector">
				{#each [['album', 'Album'], ['artist', 'Artist'], ['track', 'Track']] as [value, label]}<button
						class:active={searchType === value}
						on:click={() => {
							searchType = value;
							clearSearch();
						}}>{label}</button
					>{/each}
			</div>
			<div class="search-bar-wrap">
				<SearchBar
					bind:value={searchQuery}
					on:search={handleSearch}
					placeholder={searchType === 'artist'
						? 'Search artists…'
						: searchType === 'track'
							? 'Search by track name…'
							: 'Search albums…'}
				/>
			</div>
		</div>
		{#if loading}<div class="empty" role="status">Searching music…</div>
		{:else if searchQuery}
			{#if searchRefreshing}<p role="status">
					Showing cached results. Refreshing the catalog…
				</p>{/if}
			{#if searchError && results.length}<p class="discovery-warning" role="status">
					Showing cached results. The catalog could not refresh: {searchError}
				</p>{/if}
			{#if searchError && !results.length}<div class="empty" role="alert">
					<h2>Search is unavailable</h2>
					<p>{searchError}</p>
					<button on:click={() => handleSearch({ detail: searchQuery })}>Retry search</button>
				</div>
			{:else if searchType === 'artist'}
				{#if selectedArtistName}
					<div class="section-heading">
						<div>
							<button
								class="text-button"
								on:click={() => {
									artistRequest++;
									selectedArtistName = '';
								}}>← Back to artists</button
							>
							<h2>{selectedArtistName}</h2>
						</div>
						<label
							>Release type<select aria-label="Discography release type" bind:value={artistType}
								><option value="all">All releases</option><option value="Album">Albums</option
								><option value="EP">EPs</option><option value="Single">Singles</option></select
							></label
						>
					</div>
					{#if loadingArtistAlbums}<div class="empty">
							Loading discography…
						</div>{:else if artistError}<div class="empty" role="alert">
							Discography unavailable: {artistError}
						</div>{:else if !filteredArtistAlbums.length}<div class="empty">
							No {artistType === 'all' ? 'releases' : artistType + ' releases'} found.
						</div>{:else}<div class="music-grid">
							{#each filteredArtistAlbums as item}<MusicAlbumCard
									album={enrich(item)}
									onopen={showDetail}
								/>{/each}
						</div>{/if}
				{:else}<h2 class="section-title">{results.length} artists found</h2>
					<div class="artist-list">
						{#each results as artist}<button
								class="artist-row"
								on:click={() => viewArtistAlbums(artist)}
								><span class="artist-initial">{(artist.name || '?')[0]}</span><span
									><strong>{artist.name}</strong><small
										>{[artist.type, artist.country].filter(Boolean).join(' · ')}</small
									></span
								><span class="arrow">→</span></button
							>{/each}
					</div>{/if}
			{:else}<h2 class="section-title">{results.length} results</h2>
				{#if results.length}<div class="music-grid">
						{#each results as item}<MusicAlbumCard
								album={enrich(item)}
								onopen={showDetail}
							/>{/each}
					</div>{:else}<div class="empty">
						No results for “{searchQuery}”. Try the artist name or a different spelling.
					</div>{/if}{/if}
		{:else}
			<section class="music-intro">
				<div>
					<span class="eyebrow">Keep what catches your ear</span>
					<h2>Find your next repeat.</h2>
					<p>
						Collect albums first. Choose what to download, and let monitored albums find their way
						into your library.
					</p>
				</div>
				<a href="/music?tab=library">{libraryAlbums.length} saved albums <span>↗</span></a>
			</section>
			{#if discoveryError}<div class="discovery-warning" role="status">
					Discovery could not fully refresh: {discoveryError}. Search is still available.<button
						class="text-button"
						on:click={() => loadDiscovery()}>Try again</button
					>
				</div>{/if}
			{#if loadingDiscovery}<div class="empty" role="status">Finding your next records…</div>{:else}
				{#if enrichmentPending}<p role="status">
						Refining recommendations from your collection…
					</p>{/if}
				{#each shelves as shelf}{#if shelf.items.length}<section
							class="music-shelf"
							aria-label={shelf.title}
						>
							<div class="section-heading">
								<div>
									<h2>{shelf.title}</h2>
									<p>{shelf.description}</p>
								</div>
								<span class="shelf-count">{shelf.items.length}</span>
							</div>
							{#if shelf.kind === 'artist'}<div class="artist-list">
									{#each shelf.items as artist}<button
											class="artist-row"
											aria-label="View artist {artist.name}"
											on:click={() => openDiscoveryArtist(artist)}
											><span class="artist-initial">{(artist.name || '?')[0]}</span><span
												><strong>{artist.name}</strong><small>{artist.reason}</small></span
											><span class="arrow">→</span></button
										>{/each}
								</div>{:else}<div class="music-grid">
									{#each shelf.items as item}<MusicAlbumCard
											album={enrich(item)}
											onopen={showDetail}
										/>{/each}
								</div>{/if}
						</section>{/if}{/each}
				{#if !shelves.some((shelf) => shelf.items.length)}<div class="empty">
						<h2>Start with an album you love</h2>
						<p>
							Search an artist or album above. Save a few favorites to give discovery a starting
							point.
						</p>
						{#if !discoveryError}<button on:click={() => loadDiscovery()}>Refresh discovery</button
							>{/if}
					</div>{/if}
			{/if}
		{/if}
	{:else}
		<div class="library-controls">
			<div class="view-toggle">
				<button class:active={libraryView === 'albums'} on:click={() => (libraryView = 'albums')}
					>Albums</button
				><button class:active={libraryView === 'artists'} on:click={() => (libraryView = 'artists')}
					>Artists</button
				>
			</div>
			{#if libraryView === 'albums'}<div class="filter-bar">
					{#each [['all', 'All'], ['saved', 'Saved'], ['available', 'Available'], ['downloading', 'Downloading'], ['attention', 'Needs attention'], ['monitored', 'Monitored'], ['favorites', 'Favorites']] as [value, label]}<button
							class:active={libraryFilter === value}
							on:click={() => (libraryFilter = value)}>{label}</button
						>{/each}
				</div>{/if}
		</div>
		{#if loadingLibrary}<div class="empty">Loading library…</div>{:else if libraryError}<div
				class="empty"
				role="alert"
			>
				Music library unavailable: {libraryError}<button on:click={() => loadLibrary()}
					>Try again</button
				>
			</div>{:else if libraryView === 'albums'}
			{#if libraryArtist}<div class="section-heading">
					<a href="/music?tab=library">← All artists</a>
					<h2>
						{libraryArtists.find((artist) => artist.mbid === libraryArtist)?.name ||
							'Artist albums'}
					</h2>
				</div>{/if}
			{#if visibleLibraryAlbums.length}<div class="music-grid">
					{#each visibleLibraryAlbums as album}<MusicAlbumCard
							{album}
							href="/music/{album.id}"
						/>{/each}
				</div>{:else}<div class="empty">
					<h2>
						{libraryFilter === 'all' ? 'Your collection starts here' : 'No albums in this view'}
					</h2>
					<p>Saved albums stay here even when you have not downloaded them.</p>
					<a href="/music">Discover music →</a>
				</div>{/if}
		{:else if !libraryArtists.length}<div class="empty">No artists in library</div>{:else}<div
				class="artist-list"
			>
				{#each libraryArtists as artist}<a
						href="/music?tab=library&artist={encodeURIComponent(artist.mbid)}"
						class="artist-row"
						><span class="artist-initial">{(artist.name || '?')[0]}</span><span
							><strong>{artist.name}</strong><small
								>{artist.album_count} albums · {artist.available_count} available</small
							></span
						><span class="arrow">→</span></a
					>{/each}
			</div>{/if}
	{/if}
</div>
{#if selectedItem}<div class="modal-overlay" on:click={closeDetail} role="presentation">
		<div
			class="modal"
			use:modalAccessibility
			on:click|stopPropagation
			on:keydown|stopPropagation
			role="dialog"
			aria-modal="true"
			aria-label="Album details"
		>
			<button class="modal-close" aria-label="Close album details" on:click={closeDetail}>✕</button
			>{#key releaseGroupId(selectedItem) || selectedItem.id}<MusicDetail
					item={selectedItem}
					{profiles}
					on:added={handleAdded}
					on:updated={() => loadLibrary(true)}
					on:error={(e) => notify(e.detail, 'error')}
				/>{/key}
		</div>
	</div>{/if}

<style>
	.music-page {
		max-width: 1440px;
	}
	.page-header,
	.section-heading,
	.library-controls,
	.search-row {
		display: flex;
		justify-content: space-between;
		gap: 1rem;
		align-items: center;
		flex-wrap: wrap;
	}
	.page-header {
		margin-bottom: 1.5rem;
	}
	h1 {
		font: 800 2.5rem var(--font-display);
		line-height: 1;
		margin-top: 0.25rem;
	}
	h2 {
		font: 700 1.5rem var(--font-display);
		margin: 0;
	}
	.eyebrow {
		font-size: 0.65rem;
		text-transform: uppercase;
		letter-spacing: 0.16em;
		color: var(--text-muted);
	}
	button,
	select {
		color: var(--text-primary);
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-sm);
		font: inherit;
		padding: 0.6rem 0.8rem;
		cursor: pointer;
	}
	button:focus-visible,
	select:focus-visible,
	a:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 3px;
	}
	.tab-selector,
	.view-toggle,
	.search-type-selector,
	.filter-bar {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
		font-size: 0.8rem;
	}
	button.active {
		color: var(--text-inverse);
		background: var(--accent);
		border-color: var(--accent);
	}
	.search-row {
		margin-bottom: 1.4rem;
	}
	.search-bar-wrap {
		flex: 1;
		min-width: min(100%, 260px);
	}
	.music-intro {
		padding: 2rem;
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-lg);
		background: linear-gradient(115deg, var(--accent-subtle), var(--bg-elevated));
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 2rem;
		margin-bottom: 2rem;
	}
	.music-intro h2 {
		font-size: clamp(2.3rem, 5vw, 4rem);
		margin: 0.4rem 0;
		line-height: 1;
	}
	.music-intro p {
		font-size: 0.9rem;
		color: var(--text-secondary);
		line-height: 1.7;
		max-width: 580px;
		margin: 0.8rem 0 0;
	}
	.music-intro a {
		white-space: nowrap;
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-sm);
		padding: 1rem;
		font-size: 0.8rem;
	}
	.music-intro a span {
		margin-left: 0.8rem;
		color: var(--accent);
	}
	.music-shelf {
		margin: 2rem 0;
	}
	.section-heading {
		margin: 1.25rem 0;
	}
	.section-heading p {
		margin: 0.4rem 0 0;
		color: var(--text-muted);
		font-size: 0.8rem;
	}
	.section-heading label {
		display: grid;
		gap: 0.35rem;
		font-size: 0.75rem;
	}
	.section-title {
		margin: 1rem 0;
	}
	.shelf-count {
		color: var(--text-muted);
		font: 700 1.5rem var(--font-display);
	}
	.music-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(175px, 1fr));
		gap: 1rem;
	}
	.artist-list {
		display: grid;
		gap: 0.7rem;
	}
	.artist-row {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 1rem;
		text-align: left;
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-md);
		background: var(--glass-bg);
	}
	.artist-initial {
		font: 700 1.5rem var(--font-display);
		color: var(--accent);
		width: 2rem;
	}
	.artist-row small {
		display: block;
		margin-top: 0.2rem;
		font-size: 0.75rem;
		color: var(--text-muted);
	}
	.arrow {
		margin-left: auto;
		color: var(--text-muted);
	}
	.library-controls {
		margin-bottom: 1.5rem;
	}
	.empty {
		text-align: center;
		padding: 2.5rem 1rem;
		color: var(--text-secondary);
		line-height: 1.7;
		border: 1px dashed var(--glass-border);
		border-radius: var(--radius-md);
	}
	.empty h2 {
		color: var(--text-primary);
	}
	.empty button,
	.empty a {
		display: inline-block;
		margin: 0.7rem;
	}
	.discovery-warning {
		padding: 1rem;
		font-size: 0.8rem;
		line-height: 1.6;
		border: 1px solid var(--glass-border);
	}
	.text-button {
		border: 0;
		color: var(--accent);
		background: transparent;
		padding: 0.3rem;
	}
	.modal-overlay {
		position: fixed;
		inset: 0;
		z-index: 1000;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 1rem;
		background: rgba(0, 0, 0, 0.78);
		backdrop-filter: blur(6px);
	}
	.modal {
		max-width: 840px;
		width: 100%;
		max-height: calc(100dvh - 2rem);
		overflow-y: auto;
		overscroll-behavior: contain;
		background: var(--bg-primary);
		border-radius: var(--radius-xl);
		position: relative;
	}
	.modal-close {
		position: absolute;
		right: 0.7rem;
		top: 0.7rem;
		z-index: 2;
	}
	@media (max-width: 700px) {
		.music-intro {
			padding: 1.25rem;
			align-items: flex-start;
			flex-direction: column;
			gap: 1rem;
		}
		.music-grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			gap: 0.7rem;
		}
		.search-row {
			align-items: stretch;
		}
		.search-bar-wrap {
			width: 100%;
		}
		.filter-bar {
			gap: 0.3rem;
		}
		.filter-bar button {
			padding: 0.5rem 0.6rem;
		}
	}
</style>
