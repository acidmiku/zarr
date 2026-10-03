<script>
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import {
		Sparkles,
		ArrowRight,
		ArrowUpRight,
		Star,
		AlertTriangle,
		Plus,
		FolderInput,
		SlidersHorizontal,
		Disc3,
		RefreshCw
	} from 'lucide-svelte';
	import { api } from '$lib/api';
	import { downloads } from '$lib/stores/app';
	import ArchiveCategories from '$lib/components/ArchiveCategories.svelte';
	import ArchiveShowcase from '$lib/components/ArchiveShowcase.svelte';
	import DownloadQueue from '$lib/components/DownloadQueue.svelte';
	import PosterCard from '$lib/components/PosterCard.svelte';
	import Artwork from '$lib/components/Artwork.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';

	let items = [],
		albums = [];
	let total = 0,
		featured = null;
	let loading = true,
		error = '',
		musicError = '';
	let mounted = false,
		requestId = 0;
	let prompt = '';
	$: filterType = ['movie', 'series', 'anime'].includes($page.url.searchParams.get('type'))
		? $page.url.searchParams.get('type')
		: 'all';
	$: filterStatus = ['wanted', 'searching', 'downloading', 'available', 'unavailable'].includes(
		$page.url.searchParams.get('status')
	)
		? $page.url.searchParams.get('status')
		: 'all';
	$: currentPage = Math.max(1, parseInt($page.url.searchParams.get('page') || '1') || 1);
	$: if (mounted) loadLibrary(filterType, filterStatus, currentPage);
	$: dashboard = filterType === 'all' && filterStatus === 'all' && currentPage === 1;
	$: failed = $downloads.filter((item) => item.status === 'failed');
	onMount(() => {
		mounted = true;
		loadMusic();
		return () => {
			mounted = false;
			requestId++;
		};
	});
	async function loadLibrary(type, status, pagination) {
		const id = ++requestId;
		loading = true;
		error = '';
		featured = null;
		try {
			const data = await api.getLibrary({ page: String(pagination), limit: '50', type, status });
			if (id !== requestId) return;
			items = data.items || [];
			total = data.total || 0;
			loading = false;
			if (items[0]) {
				try {
					const detail = await api.getLibraryItem(items[0].id);
					if (id === requestId) featured = detail;
				} catch {
					/* The list still provides a usable feature. */
				}
			}
		} catch (err) {
			if (id === requestId) {
				error = err.message || 'Could not load your collection';
				loading = false;
			}
		}
	}
	async function loadMusic() {
		musicError = '';
		try {
			const data = await api.getMusicLibrary();
			albums = Array.isArray(data) ? data : [];
		} catch (err) {
			musicError = err.message || 'Could not load albums';
		}
	}
	function filter(key, value) {
		const params = new URLSearchParams($page.url.searchParams);
		if (value === 'all') params.delete(key);
		else params.set(key, value);
		if (key !== 'page') params.delete('page');
		goto(`/library?${params}`);
	}
	function ask() {
		goto(`/assistant${prompt.trim() ? `?prompt=${encodeURIComponent(prompt.trim())}` : ''}`);
	}
</script>

<svelte:head><title>Collection — Zarr</title></svelte:head>
<ArchiveCategories selected={filterType} />
<div class="collection-toolbar">
	<div>
		<h1>
			{dashboard
				? 'Your collection'
				: filterType === 'all'
					? 'Collection'
					: filterType === 'movie'
						? 'Movies'
						: filterType === 'series'
							? 'Series'
							: 'Anime'}
		</h1>
		<span>{total} titles</span>
	</div>
	<div class="collection-filters">
		{#if filterType !== 'all'}<a href="/library" class="all-link">All media</a>{/if}<label
			><SlidersHorizontal size={14} /><span class="sr-only">Filter by status</span><select
				value={filterStatus}
				on:change={(e) => filter('status', e.currentTarget.value)}
				><option value="all">All statuses</option><option value="wanted">Wanted</option><option
					value="searching">Searching</option
				><option value="downloading">Downloading</option><option value="available">Available</option
				><option value="unavailable">Unavailable</option></select
			></label
		><a href="/" class="add-link"><Plus size={15} /> Add media</a>
	</div>
</div>
<div class="collection-layout" class:filtered={!dashboard}>
	<div class="collection-main">
		{#if loading}<div class="archive-state" role="status">
				<RefreshCw size={26} />
				<p>Loading your collection…</p>
			</div>
		{:else if error}<div class="archive-state" role="alert">
				<h2>Archive unavailable</h2>
				<p>{error}</p>
				<button
					class="archive-button"
					on:click={() => loadLibrary(filterType, filterStatus, currentPage)}>Try again</button
				>
			</div>
		{:else if items.length === 0}<div class="archive-state">
				<h2>{dashboard ? 'Make room for your obsessions.' : 'Nothing here. Yet.'}</h2>
				<p>
					{dashboard
						? 'Movies, series, anime, and music. Add your first discovery or import a collection you already own.'
						: 'No titles match these filters. Try another category or status.'}
				</p>
				<div class="empty-actions">
					<a class="archive-button" href="/">Discover media <ArrowUpRight size={17} /></a
					>{#if dashboard}<a class="archive-button secondary" href="/import">Import a folder</a
						>{:else}<a class="archive-button secondary" href="/library">Reset filters</a>{/if}
				</div>
			</div>
		{:else if dashboard}<ArchiveShowcase {items} {featured} />
		{:else}<div class="collection-grid">
				{#each items as item (item.id)}<PosterCard
						href="/library/{item.id}"
						title={item.title}
						year={item.year}
						posterUrl={item.poster_url}
						rating={item.rating}
						status={item.status}
						inLibrary
						anime={item.anime}
					/>{/each}
			</div>{/if}
		{#if dashboard}
			<div class="collection-lower">
				<section class="music-panel cut-panel">
					<header class="panel-heading">
						<h2><Disc3 size={19} /> On record</h2>
						<a href="/music?tab=library">View all <ArrowUpRight size={13} /></a>
					</header>
					{#if musicError}<div class="panel-empty">
							<p>Music is unavailable.</p>
							<button on:click={loadMusic}>Retry</button>
						</div>
					{:else if albums.length}<div class="music-shelf">
							<a class="album-feature" href="/music/{albums[0].id}"
								><div class="album-cover">
									<Artwork src={albums[0].image_url} title={albums[0].title} />
								</div>
								<strong>{albums[0].title}</strong><span>{albums[0].artist_name}</span><StatusBadge
									status={albums[0].status}
									small
								/></a
							>
							<div class="album-list">
								{#each albums.slice(1, 3) as album}<a class="album-row" href="/music/{album.id}"
										><div class="album-thumb">
											<Artwork src={album.image_url} title={album.title} />
										</div>
										<div>
											<strong>{album.title}</strong><span>{album.artist_name}</span><small
												>{album.year || '—'} · {album.track_count || 0} tracks</small
											>
										</div></a
									>{/each}<a class="music-discover" href="/music"
									>Find your next record <ArrowUpRight size={14} /></a
								>
							</div>
						</div>
					{:else}<div class="panel-empty">
							<Disc3 size={30} strokeWidth={1} />
							<p>Your record shelf starts here.</p>
							<a href="/music">Discover music <ArrowUpRight size={14} /></a>
						</div>{/if}
				</section>
				<section class="downloads-panel cut-panel">
					<header class="panel-heading">
						<h2>Downloads <span>/ {String($downloads.length).padStart(2, '0')}</span></h2>
						<a href="/activity">View all <ArrowUpRight size={13} /></a>
					</header>
					<DownloadQueue compact />
				</section>
			</div>
			{#if !loading && !error && items.length > 3}<section class="rest-of-collection">
					<header class="panel-heading">
						<h2>More from your archive</h2>
						<span class="eyebrow">Recently added</span>
					</header>
					<div class="collection-grid">
						{#each items.slice(3) as item (item.id)}<PosterCard
								href="/library/{item.id}"
								title={item.title}
								year={item.year}
								posterUrl={item.poster_url}
								rating={item.rating}
								status={item.status}
								inLibrary
								anime={item.anime}
							/>{/each}
					</div>
				</section>{/if}
		{/if}
		{#if total > 50 && !loading && !error}<nav
				class="collection-pagination"
				aria-label="Collection pages"
			>
				<button
					class="archive-button secondary"
					disabled={currentPage <= 1}
					on:click={() => filter('page', String(currentPage - 1))}>Previous</button
				><span>Page {currentPage} / {Math.ceil(total / 50)}</span><button
					class="archive-button secondary"
					disabled={currentPage * 50 >= total}
					on:click={() => filter('page', String(currentPage + 1))}>Next</button
				>
			</nav>{/if}
	</div>
	{#if dashboard}<aside class="archive-utilities" aria-label="Collection tools">
			<section class="assistant-panel cut-panel">
				<span class="utility-label"><Sparkles size={15} /> AI ASSISTANT</span>
				<h2>Find something<br />stranger.</h2>
				<p>Go beyond the algorithm. Find your next obsession, guided by your taste.</p>
				<form on:submit|preventDefault={ask}>
					<input
						bind:value={prompt}
						aria-label="Ask your assistant"
						placeholder="What should I watch next?"
					/><button class="archive-button">Ask assistant <ArrowRight size={17} /></button>
				</form>
			</section>
			<a href="/ratings" class="ratings-panel cut-panel"
				><Star size={31} strokeWidth={1.4} />
				<div>
					<span class="utility-label">YOUR RATINGS</span>
					<h3>A little more you.</h3>
					<p>Rate what you love.<br />Discover more like it.</p>
				</div>
				<ArrowUpRight size={15} /></a
			>
			{#if failed.length}<a class="attention-panel cut-panel" href="/activity"
					><AlertTriangle size={27} />
					<div>
						<strong>{failed.length} failed download{failed.length > 1 ? 's' : ''}</strong>
						<p>{failed[0].media_title || failed[0].nzb_title}</p>
						<span>Review activity <ArrowRight size={14} /></span>
					</div></a
				>{/if}
			<section class="quick-panel cut-panel">
				<span class="utility-label">QUICK ACTIONS</span><a href="/"
					><Plus size={16} /> Add from search <ArrowUpRight size={13} /></a
				><a href="/import"><FolderInput size={16} /> Import a folder <ArrowUpRight size={13} /></a
				><a href="/settings"
					><SlidersHorizontal size={16} /> Quality & settings <ArrowUpRight size={13} /></a
				>
			</section>
		</aside>{/if}
</div>

<style>
	.collection-toolbar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 16px;
		margin: 0 0 18px;
	}
	.collection-toolbar > div:first-child {
		display: flex;
		gap: 12px;
		align-items: baseline;
	}
	h1 {
		font-size: 1.6rem;
		text-transform: uppercase;
	}
	.collection-toolbar span {
		color: var(--text-muted);
		font-size: 0.67rem;
	}
	.collection-filters {
		display: flex;
		gap: 18px;
		align-items: center;
	}
	.collection-filters label {
		display: flex;
		gap: 7px;
		align-items: center;
		color: var(--text-muted);
	}
	.collection-filters select {
		border: 0;
		color: var(--text-secondary);
		background: transparent;
		font-size: 0.68rem;
		padding: 5px 0;
	}
	.add-link,
	.all-link {
		display: flex;
		gap: 5px;
		align-items: center;
		color: var(--neon-cyan);
		font-size: 0.68rem;
	}
	.collection-layout {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 255px;
		gap: 18px;
		align-items: start;
	}
	.collection-layout.filtered {
		grid-template-columns: minmax(0, 1fr);
	}
	.collection-main {
		min-width: 0;
	}
	.collection-lower {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
		gap: 16px;
		margin-top: 18px;
	}
	.music-panel,
	.downloads-panel {
		padding: 18px;
		min-width: 0;
	}
	.panel-heading {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 10px;
		padding-bottom: 15px;
	}
	.panel-heading h2 {
		display: flex;
		gap: 8px;
		align-items: center;
		text-transform: uppercase;
		font-size: 1.5rem;
	}
	.panel-heading h2 span {
		font-weight: 400;
		color: var(--text-muted);
	}
	.panel-heading a {
		color: var(--text-secondary);
		font-size: 0.59rem;
		display: flex;
		align-items: center;
		gap: 5px;
	}
	.music-shelf {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
		gap: 18px;
	}
	.album-feature {
		min-width: 0;
	}
	.album-cover {
		aspect-ratio: 1;
		margin-bottom: 13px;
	}
	.album-feature strong,
	.album-row strong {
		font-size: 0.76rem;
		display: block;
		line-height: 1.5;
	}
	.album-feature > span {
		display: block;
		color: var(--neon-cyan);
		font-size: 0.66rem;
		margin: 5px 0 10px;
	}
	.album-list {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
	.album-row {
		display: flex;
		gap: 10px;
		align-items: center;
	}
	.album-thumb {
		flex: 0 0 56px;
		width: 56px;
		height: 56px;
	}
	.album-row span {
		display: block;
		color: var(--neon-cyan);
		font-size: 0.6rem;
		line-height: 1.5;
		margin: 4px 0;
	}
	.album-row small {
		font-size: 0.56rem;
		color: var(--text-muted);
	}
	.music-discover {
		font-size: 0.63rem;
		color: var(--text-muted);
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 8px;
		margin-top: auto;
	}
	.archive-utilities {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
	.archive-utilities .cut-panel {
		--panel-accent: var(--neon-cyan);
	}
	.assistant-panel {
		padding: 22px;
	}
	.utility-label {
		display: flex;
		align-items: center;
		gap: 8px;
		font: 0.59rem var(--font-mono);
		letter-spacing: 0.12em;
		color: var(--neon-cyan);
	}
	.assistant-panel h2 {
		font-size: 2.5rem;
		line-height: 0.98;
		margin: 21px 0 16px;
		text-transform: uppercase;
	}
	.archive-utilities p {
		font-size: 0.72rem;
		line-height: 1.7;
		color: var(--text-secondary);
	}
	.assistant-panel form {
		margin-top: 22px;
		display: grid;
		gap: 10px;
	}
	.assistant-panel input {
		width: 100%;
		min-width: 0;
		font-size: 0.65rem;
		background: var(--bg-input);
		color: var(--text-primary);
		border: 1px solid var(--border-strong);
		padding: 12px 9px;
	}
	.assistant-panel .archive-button {
		justify-content: space-between;
		padding: 10px 14px;
	}
	.ratings-panel {
		display: flex;
		gap: 14px;
		align-items: flex-start;
		padding: 20px;
	}
	.ratings-panel > :global(svg:first-child) {
		flex-shrink: 0;
		color: var(--text-secondary);
	}
	.ratings-panel > :global(svg:last-child) {
		margin-left: auto;
		flex-shrink: 0;
	}
	.ratings-panel h3 {
		font-size: 1.4rem;
		margin: 9px 0;
	}
	.ratings-panel:hover {
		border-color: var(--neon-cyan);
	}
	.attention-panel {
		--panel-accent: var(--warning) !important;
		padding: 18px;
		display: flex;
		gap: 12px;
		color: var(--warning);
	}
	.attention-panel > :global(svg) {
		flex-shrink: 0;
	}
	.attention-panel strong {
		font-size: 0.75rem;
	}
	.attention-panel p {
		margin: 7px 0;
		word-break: break-word;
	}
	.attention-panel span {
		display: flex;
		align-items: center;
		gap: 12px;
		font-size: 0.65rem;
	}
	.quick-panel {
		padding: 20px;
	}
	.quick-panel .utility-label {
		margin-bottom: 16px;
	}
	.quick-panel a {
		display: flex;
		gap: 10px;
		align-items: center;
		padding: 11px 0;
		font-size: 0.69rem;
		color: var(--text-secondary);
	}
	.quick-panel a:hover {
		color: var(--neon-cyan);
	}
	.quick-panel a :global(svg:last-child) {
		margin-left: auto;
	}
	.collection-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(155px, 1fr));
		gap: 18px;
	}
	.rest-of-collection {
		margin-top: 30px;
	}
	.collection-pagination {
		margin-top: 24px;
		display: flex;
		justify-content: center;
		align-items: center;
		gap: 20px;
		font-size: 0.72rem;
		color: var(--text-secondary);
	}
	.empty-actions {
		display: flex;
		gap: 12px;
		flex-wrap: wrap;
		justify-content: center;
	}
	.panel-empty {
		min-height: 200px;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 18px;
		font-size: 0.75rem;
		color: var(--text-muted);
		text-align: center;
	}
	.panel-empty a,
	.panel-empty button {
		color: var(--neon-cyan);
		background: none;
		border: none;
		display: flex;
		align-items: center;
		gap: 6px;
	}
	@media (max-width: 1250px) {
		.collection-layout {
			grid-template-columns: minmax(0, 1fr) 220px;
		}
		.assistant-panel {
			padding: 18px;
		}
		.assistant-panel h2 {
			font-size: 2.1rem;
		}
		.album-row {
			flex-direction: column;
			align-items: flex-start;
		}
		.album-thumb {
			display: none;
		}
	}
	@media (max-width: 1100px) {
		.collection-layout {
			grid-template-columns: minmax(0, 1fr);
		}
		.archive-utilities {
			display: grid;
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
		.assistant-panel {
			grid-row: span 2;
		}
		.album-row {
			flex-direction: row;
			align-items: center;
		}
		.album-thumb {
			display: block;
		}
	}
	@media (max-width: 650px) {
		.collection-toolbar {
			align-items: flex-start;
			flex-wrap: wrap;
		}
		.collection-filters {
			width: 100%;
			justify-content: space-between;
		}
		.collection-lower {
			grid-template-columns: minmax(0, 1fr);
		}
		.archive-utilities {
			grid-template-columns: minmax(0, 1fr);
		}
		.assistant-panel {
			grid-row: auto;
		}
		.collection-grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			gap: 12px;
		}
		.panel-heading h2 {
			font-size: 1.4rem;
		}
	}
</style>
