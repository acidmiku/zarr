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
</script>

<svelte:head>
	<title>Discover - Zarr</title>
</svelte:head>

<div class="page">
	<header class="page-header">
		<h1>Discover</h1>
		<div class="type-selector">
			<button class:active={mediaType === 'movie'} on:click={() => selectType('movie')}>Movies</button>
			<button class:active={mediaType === 'series'} on:click={() => selectType('series')}>Series</button>
			<button class:active={mediaType === 'anime'} on:click={() => selectType('anime')}>Anime</button>
		</div>
	</header>

	<SearchBar bind:value={searchQuery} on:search={handleSearch} placeholder="Search {mediaType}..." />

	{#if loading}
		<div class="loading">Searching...</div>
	{/if}

	{#if !searchQuery && !loading}
		<h2 class="section-title">Trending {mediaType === 'movie' ? 'Movies' : mediaType === 'series' ? 'Series' : 'Anime'}</h2>
	{:else if searchQuery && results.length > 0}
		<h2 class="section-title">{results.length} results</h2>
	{/if}

	<div class="grid">
		{#each displayItems as item}
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

	{#if searchQuery && !loading && results.length === 0}
		<div class="empty">No results found for "{searchQuery}"</div>
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
		margin-bottom: 1.25rem;
	}

	h1 {
		font-size: 1.5rem;
		font-weight: 700;
	}

	.type-selector {
		display: flex;
		gap: 0.25rem;
		background: var(--bg-surface);
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
		border-radius: 8px;
		padding: 3px;
	}

	.type-selector button {
		padding: 0.4rem 1rem;
		border: none;
		background: transparent;
		color: var(--text-secondary);
		border-radius: 6px;
		font-size: 0.85rem;
		font-weight: 500;
	}

	.type-selector button.active {
		background: var(--accent);
		color: var(--text-inverse);
	}

	.section-title {
		font-size: 1rem;
		font-weight: 600;
		color: var(--text-secondary);
		margin: 1.25rem 0 0.75rem;
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
		gap: 1rem;
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

		.grid {
			grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
		}
	}
</style>
