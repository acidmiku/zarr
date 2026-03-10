<script>
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';
	import SearchBar from '$lib/components/SearchBar.svelte';
	import PosterCard from '$lib/components/PosterCard.svelte';
	import MediaDetail from '$lib/components/MediaDetail.svelte';

	let searchQuery = '';
	let mediaType = 'movie';
	let results = [];
	let trending = [];
	let profiles = [];
	let loading = false;
	let selectedItem = null;

	onMount(async () => {
		profiles = await api.getProfiles();
		loadTrending();
	});

	async function loadTrending() {
		try {
			const data = await api.trending(mediaType, 1);
			trending = Array.isArray(data) ? data : [];
		} catch (err) {
			console.error('Trending error:', err);
			trending = [];
		}
	}

	async function handleSearch(e) {
		const query = e.detail;
		if (!query) {
			results = [];
			return;
		}
		loading = true;
		try {
			const data = await api.search(query, mediaType);
			results = Array.isArray(data) ? data : [];
		} catch (err) {
			console.error('Search error:', err);
			notify(err?.message || 'Search failed', 'error');
			results = [];
		} finally {
			loading = false;
		}
	}

	function selectType(type) {
		if (type === mediaType) return;
		mediaType = type;
		results = [];
		searchQuery = '';
		loading = false;
		loadTrending();
	}

	function showDetail(item) {
		selectedItem = {
			...item,
			tmdb_id: item.tmdb_id,
			anilist_id: item.anilist_id,
			type: mediaType === 'anime' ? 'series' : mediaType,
			is_anime: mediaType === 'anime' || item.is_anime
		};
	}

	function handleAdded() {
		notify('Added to library!', 'success');
		selectedItem = null;
		if (searchQuery) handleSearch({ detail: searchQuery });
	}

	$: displayItems = searchQuery ? (results || []) : (trending || []);
	$: heroItem = (!searchQuery && trending.length > 0) ? trending[0] : null;
	$: heroBackdrop = heroItem?.backdrop_url ? api.imageUrl(heroItem.backdrop_url) : '';
	$: heroPoster = heroItem?.poster_url ? api.imageUrl(heroItem.poster_url) : '';
	$: gridItems = searchQuery ? displayItems : displayItems.slice(1);
</script>

<svelte:head>
	<title>Discover - Zarr</title>
</svelte:head>

<div class="page">
	<header class="page-header">
		<h1>Discover</h1>
		<div class="type-selector">
			{#each [['movie', 'Movies'], ['series', 'Series'], ['anime', 'Anime']] as [val, label]}
				<button class:active={mediaType === val} on:click={() => selectType(val)}>{label}</button>
			{/each}
		</div>
	</header>

	<div class="search-wrapper">
		<SearchBar bind:value={searchQuery} on:search={handleSearch} placeholder="Search {mediaType === 'anime' ? 'anime' : mediaType === 'series' ? 'series' : 'movies'}..." />
	</div>

	{#if loading}
		<div class="loading">
			<div class="loading-spinner"></div>
			<span>Searching...</span>
		</div>
	{/if}

	{#if heroItem && !loading}
		<div class="hero" on:click={() => showDetail(heroItem)} on:keydown role="button" tabindex="0">
			{#if heroBackdrop}
				<img class="hero-backdrop" src={heroBackdrop} alt="" />
			{/if}
			<div class="hero-fade"></div>
			<div class="hero-content">
				{#if heroPoster}
					<img class="hero-poster" src={heroPoster} alt={heroItem.title} />
				{/if}
				<div class="hero-info">
					<div class="hero-label">Trending {mediaType === 'movie' ? 'Movie' : mediaType === 'series' ? 'Series' : 'Anime'}</div>
					<h2 class="hero-title">{heroItem.title}</h2>
					<div class="hero-meta">
						{#if heroItem.year}<span>{heroItem.year}</span>{/if}
						{#if heroItem.rating}<span class="hero-rating">★ {heroItem.rating.toFixed(1)}</span>{/if}
					</div>
					{#if heroItem.overview}
						<p class="hero-overview">{heroItem.overview.replace(/<[^>]*>/g, '').slice(0, 200)}{heroItem.overview.length > 200 ? '...' : ''}</p>
					{/if}
					<button class="hero-cta" on:click|stopPropagation={() => showDetail(heroItem)}>
						{heroItem.in_library ? 'View Details' : 'Add to Library'}
					</button>
				</div>
			</div>
		</div>
	{/if}

	{#if !loading}
		{#if !searchQuery && gridItems.length > 0}
			<h2 class="section-title">More Trending</h2>
		{:else if searchQuery && results.length > 0}
			<h2 class="section-title">{results.length} results</h2>
		{/if}
	{/if}

	{#if (searchQuery ? displayItems : gridItems).length > 0}
		<div class="grid stagger-grid">
			{#each (searchQuery ? displayItems : gridItems) as item}
				<PosterCard
					title={item.title}
					year={item.year}
					posterUrl={item.poster_url}
					rating={item.rating}
					status={item.status}
					inLibrary={item.in_library}
					anime={mediaType === 'anime'}
					on:click={() => showDetail(item)}
				/>
			{/each}
		</div>
	{/if}

	{#if searchQuery && !loading && results.length === 0}
		<div class="empty">
			<div class="empty-icon">⌕</div>
			<p>No results found for "{searchQuery}"</p>
		</div>
	{/if}
</div>

{#if selectedItem}
	<!-- svelte-ignore a11y-click-events-have-key-events -->
	<div class="modal-overlay" on:click={() => selectedItem = null} role="presentation">
		<div class="modal" on:click|stopPropagation on:keydown|stopPropagation role="dialog">
			<button class="modal-close" on:click={() => selectedItem = null}>✕</button>
			<MediaDetail
				item={selectedItem}
				{profiles}
				mode="discovery"
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
		margin-bottom: 1.5rem;
	}
	h1 {
		font-size: 1.75rem;
		font-weight: 800;
		font-family: var(--font-display);
		letter-spacing: -0.03em;
	}
	.type-selector {
		display: flex;
		gap: 2px;
		background: var(--glass-bg);
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-sm);
		padding: 3px;
	}
	.type-selector button {
		padding: 0.4rem 1.1rem;
		border: none;
		background: transparent;
		color: var(--text-muted);
		border-radius: 6px;
		font-size: 0.82rem;
		font-weight: 600;
		font-family: var(--font-body);
		transition: all 0.2s;
	}
	.type-selector button:hover { color: var(--text-primary); }
	.type-selector button.active {
		background: var(--accent);
		color: var(--text-inverse);
		box-shadow: 0 2px 8px var(--accent-glow);
	}
	.search-wrapper { margin-bottom: 1.75rem; }

	/* Hero */
	.hero {
		position: relative;
		height: 360px;
		border-radius: var(--radius-xl);
		overflow: hidden;
		cursor: pointer;
		margin-bottom: 2rem;
		animation: fadeSlideUp 0.5s cubic-bezier(0.16, 1, 0.3, 1);
	}
	.hero-backdrop {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
		transition: transform 0.6s cubic-bezier(0.16, 1, 0.3, 1);
	}
	.hero:hover .hero-backdrop { transform: scale(1.03); }
	.hero-fade {
		position: absolute;
		inset: 0;
		background:
			linear-gradient(to right, rgba(0,0,0,0.85) 0%, rgba(0,0,0,0.4) 50%, transparent 75%),
			linear-gradient(to top, rgba(0,0,0,0.6) 0%, transparent 40%);
	}
	.hero-content {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: flex-end;
		gap: 1.5rem;
		padding: 2rem 2.5rem;
	}
	.hero-poster {
		width: 130px;
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-lg);
		flex-shrink: 0;
		aspect-ratio: 2/3;
		object-fit: cover;
	}
	.hero-info { flex: 1; min-width: 0; padding-bottom: 0.25rem; }
	.hero-label {
		font-size: 0.68rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.12em;
		color: var(--accent);
		margin-bottom: 0.5rem;
	}
	.hero-title {
		font-family: var(--font-display);
		font-size: 2rem;
		font-weight: 800;
		color: white;
		line-height: 1.1;
		letter-spacing: -0.03em;
		margin-bottom: 0.5rem;
	}
	.hero-meta {
		display: flex;
		gap: 0.75rem;
		font-size: 0.85rem;
		color: rgba(255,255,255,0.65);
		margin-bottom: 0.6rem;
	}
	.hero-rating { color: var(--gold); }
	.hero-overview {
		font-size: 0.82rem;
		color: rgba(255,255,255,0.55);
		line-height: 1.5;
		margin-bottom: 1rem;
		max-width: 500px;
	}
	.hero-cta {
		padding: 0.55rem 1.5rem;
		background: var(--accent);
		color: var(--text-inverse);
		border: none;
		border-radius: var(--radius-sm);
		font-size: 0.82rem;
		font-weight: 700;
		font-family: var(--font-body);
		cursor: pointer;
		transition: all 0.2s;
	}
	.hero-cta:hover {
		background: var(--accent-hover);
		box-shadow: var(--shadow-glow);
		transform: translateY(-1px);
	}

	.section-title {
		font-family: var(--font-display);
		font-size: 1rem;
		font-weight: 700;
		color: var(--text-secondary);
		margin: 0 0 1rem;
		letter-spacing: -0.01em;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(155px, 1fr));
		gap: 1.25rem;
	}
	.loading {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.75rem;
		color: var(--text-muted);
		padding: 3rem;
		font-size: 0.85rem;
	}
	.loading-spinner {
		width: 18px;
		height: 18px;
		border: 2px solid var(--border);
		border-top-color: var(--accent);
		border-radius: 50%;
		animation: spin 0.7s linear infinite;
	}
	.empty {
		text-align: center;
		color: var(--text-muted);
		padding: 4rem 1rem;
	}
	.empty-icon { font-size: 2.5rem; opacity: 0.3; margin-bottom: 0.75rem; }
	.empty p { font-size: 0.9rem; }

	.modal-overlay {
		position: fixed;
		inset: 0;
		background: var(--bg-overlay);
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
		z-index: 200;
		padding: 2rem;
		overflow-y: auto;
		animation: fadeIn 0.2s ease;
	}
	.modal {
		width: 100%;
		max-width: 700px;
		margin: 2rem auto;
		position: relative;
		border-radius: var(--radius-lg);
		animation: scaleIn 0.3s cubic-bezier(0.16, 1, 0.3, 1);
	}
	.modal-close {
		position: absolute;
		top: 12px;
		right: 12px;
		background: rgba(0,0,0,0.5);
		backdrop-filter: blur(8px);
		-webkit-backdrop-filter: blur(8px);
		border: 1px solid rgba(255,255,255,0.1);
		color: white;
		width: 32px;
		height: 32px;
		border-radius: 50%;
		font-size: 0.9rem;
		z-index: 10;
		cursor: pointer;
		transition: all 0.2s;
	}
	.modal-close:hover { background: rgba(255,255,255,0.15); transform: scale(1.1); }

	@media (max-width: 768px) {
		.page-header { flex-direction: column; gap: 0.75rem; align-items: stretch; }
		.hero { height: 280px; }
		.hero-content { padding: 1.25rem; gap: 1rem; }
		.hero-poster { width: 90px; }
		.hero-title { font-size: 1.4rem; }
		.hero-overview { display: none; }
		.grid { grid-template-columns: repeat(auto-fill, minmax(120px, 1fr)); }
	}
</style>
