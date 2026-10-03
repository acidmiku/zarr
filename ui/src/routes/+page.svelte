<script>
	import { onMount, tick } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';
	import { X, ArrowUpRight, Compass } from 'lucide-svelte';
	import ArchiveCategories from '$lib/components/ArchiveCategories.svelte';
	import ArchiveShowcase from '$lib/components/ArchiveShowcase.svelte';
	import SearchBar from '$lib/components/SearchBar.svelte';
	import PosterCard from '$lib/components/PosterCard.svelte';
	import MediaDetail from '$lib/components/MediaDetail.svelte';
	let results = [],
		profiles = [];
	let searchQuery = '',
		loading = true,
		error = '';
	let selectedItem = null,
		dialog;
	let mounted = false,
		requestId = 0;
	$: mediaType = ['movie', 'series', 'anime'].includes($page.url.searchParams.get('type'))
		? $page.url.searchParams.get('type')
		: 'movie';
	$: urlQuery = $page.url.searchParams.get('q') || '';
	$: if (mounted) load(mediaType, urlQuery);
	onMount(() => {
		mounted = true;
		api
			.getProfiles()
			.then((data) => (profiles = data))
			.catch((err) => notify(err.message, 'error'));
		return () => {
			mounted = false;
			requestId++;
		};
	});
	async function load(type, query) {
		const id = ++requestId;
		searchQuery = query;
		loading = true;
		error = '';
		try {
			const data = query ? await api.search(query, type) : await api.trending(type, 1);
			if (id !== requestId) return;
			results = Array.isArray(data)
				? data.map((item) => ({
						...item,
						type: type === 'anime' ? 'series' : type,
						is_anime: type === 'anime' || item.is_anime
					}))
				: [];
		} catch (err) {
			if (id === requestId) {
				error = err.message || 'Could not load discoveries';
				results = [];
			}
		} finally {
			if (id === requestId) loading = false;
		}
	}
	function search(event) {
		const params = new URLSearchParams({ type: mediaType });
		if (event.detail.trim()) params.set('q', event.detail.trim());
		goto(`/?${params}`, { replaceState: true, keepFocus: true, noScroll: true });
	}
	async function showDetail(item) {
		selectedItem = item;
		await tick();
		dialog.showModal();
	}
	function close() {
		dialog?.close();
		selectedItem = null;
	}
	function added() {
		notify('Added to library!', 'success');
		close();
		load(mediaType, urlQuery);
	}
</script>

<svelte:head><title>Discover — Zarr</title></svelte:head>
<ArchiveCategories mode="discover" selected={mediaType} />
<div class="discovery-heading">
	<div>
		<span class="eyebrow">NEXT UP / YOUR NEXT OBSESSION</span>
		<h1>Discover something different.</h1>
	</div>
	<a href="/library">Your collection <ArrowUpRight size={15} /></a>
</div>
<div class="discovery-search">
	<SearchBar
		bind:value={searchQuery}
		on:search={search}
		placeholder="Search {mediaType === 'movie'
			? 'movies'
			: mediaType === 'series'
				? 'series'
				: 'anime'}…"
	/>
</div>
{#if loading}<div class="archive-state" role="status">
		<Compass size={30} strokeWidth={1} />
		<p>{urlQuery ? 'Searching the archive…' : 'Finding what is trending…'}</p>
	</div>
{:else if error}<div class="archive-state" role="alert">
		<h2>Signal interrupted.</h2>
		<p>{error}</p>
		<button class="archive-button" on:click={() => load(mediaType, urlQuery)}>Try again</button>
	</div>
{:else if !results.length}<div class="archive-state">
		<h2>{urlQuery ? 'No matches. Keep digging.' : 'No discoveries yet.'}</h2>
		<p>
			{urlQuery
				? `Nothing found for “${urlQuery}”. Try another title or category.`
				: 'Check your metadata configuration or try a search.'}
		</p>
		{#if !urlQuery}<a class="archive-button secondary" href="/settings">Open settings</a>{/if}
	</div>
{:else}
	{#if !urlQuery}<ArchiveShowcase
			items={results}
			discovery
			on:select={(event) => showDetail(event.detail)}
		/>{/if}
	{#if urlQuery || results.length > 3}<div class="discovery-section">
			<h2>{urlQuery ? `${results.length} results` : 'More to discover'}</h2>
			<span class="eyebrow"
				>{mediaType === 'movie' ? 'Movies' : mediaType === 'series' ? 'Series' : 'Anime'}</span
			>
		</div>
		<div class="discovery-grid">
			{#each urlQuery ? results : results.slice(3) as item}<PosterCard
					title={item.title}
					year={item.year}
					posterUrl={item.poster_url}
					rating={item.rating}
					status={item.status}
					inLibrary={item.in_library}
					anime={item.is_anime}
					on:click={() => showDetail(item)}
				/>{/each}
		</div>{/if}
{/if}
<dialog
	bind:this={dialog}
	class="discovery-dialog"
	aria-label={selectedItem?.title || 'Media details'}
	on:close={() => (selectedItem = null)}
>
	{#if selectedItem}<button class="dialog-close" aria-label="Close media details" on:click={close}
			><X size={20} /></button
		><MediaDetail
			item={selectedItem}
			{profiles}
			mode="discovery"
			on:added={added}
			on:error={(event) => notify(event.detail, 'error')}
		/>{/if}
</dialog>

<style>
	.discovery-heading {
		display: flex;
		justify-content: space-between;
		align-items: flex-end;
		gap: 20px;
		margin: 28px 0 22px;
	}
	h1 {
		font-size: clamp(2rem, 3.5vw, 3.2rem);
		text-transform: uppercase;
		margin-top: 8px;
	}
	.discovery-heading a {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 0.72rem;
		color: var(--neon-cyan);
	}
	.discovery-search {
		max-width: 650px;
		margin-bottom: 25px;
	}
	.discovery-section {
		margin: 32px 0 20px;
		display: flex;
		justify-content: space-between;
		align-items: center;
	}
	h2 {
		font-size: 1.7rem;
		text-transform: uppercase;
	}
	.discovery-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
		gap: 18px;
	}
	.discovery-dialog {
		color: var(--text-primary);
		background: var(--bg-elevated);
		border: 1px solid var(--border-strong);
		padding: 0;
		margin: auto;
		width: min(850px, calc(100vw - 32px));
		max-height: 90dvh;
		overflow: auto;
	}
	.discovery-dialog::backdrop {
		background: #05040bcc;
		backdrop-filter: blur(5px);
	}
	.dialog-close {
		position: absolute;
		right: 14px;
		top: 14px;
		z-index: 10;
		width: 36px;
		height: 36px;
		display: grid;
		place-items: center;
		background: #100e17;
		border: 1px solid #706278;
		color: #fff;
	}
	@media (max-width: 650px) {
		.discovery-heading {
			align-items: flex-start;
			flex-direction: column;
			gap: 13px;
		}
		.discovery-grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			gap: 12px;
		}
	}
</style>
