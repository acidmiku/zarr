<script>
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import PosterCard from '$lib/components/PosterCard.svelte';

	let items = [];
	let total = 0;
	let filterType = 'all';
	let filterStatus = 'all';
	let page = 1;
	let loading = false;

	onMount(() => loadLibrary());

	async function loadLibrary() {
		loading = true;
		try {
			const params = { page: String(page), limit: '50' };
			if (filterType !== 'all') params.type = filterType;
			if (filterStatus !== 'all') params.status = filterStatus;

			const data = await api.getLibrary(params);
			items = data.items || [];
			total = data.total || 0;
		} catch {}
		loading = false;
	}

	function setType(type) {
		filterType = type;
		page = 1;
		loadLibrary();
	}

	function setStatus(status) {
		filterStatus = status;
		page = 1;
		loadLibrary();
	}
</script>

<svelte:head>
	<title>Library - Zarr</title>
</svelte:head>

<div class="page">
	<header class="page-header">
		<h1>Library</h1>
		<span class="count">{total} items</span>
	</header>

	<div class="filters">
		<div class="filter-group">
			{#each [['all', 'All'], ['movie', 'Movies'], ['series', 'Series'], ['anime', 'Anime']] as [val, label]}
				<button class:active={filterType === val} on:click={() => setType(val)}>{label}</button>
			{/each}
		</div>
		<div class="filter-group">
			{#each [['all', 'All'], ['wanted', 'Wanted'], ['downloading', 'Downloading'], ['available', 'Available']] as [val, label]}
				<button class:active={filterStatus === val} on:click={() => setStatus(val)}>{label}</button>
			{/each}
		</div>
	</div>

	{#if loading}
		<div class="loading">
			<div class="loading-spinner"></div>
			<span>Loading...</span>
		</div>
	{:else if items.length === 0}
		<div class="empty">
			<div class="empty-icon">📚</div>
			<p>Your library is empty.</p>
			<a href="/" class="btn-link">Discover content to add</a>
		</div>
	{:else}
		<div class="grid stagger-grid">
			{#each items as item}
				<a href="/library/{item.id}">
					<PosterCard
						title={item.title}
						year={item.year}
						posterUrl={item.poster_url}
						rating={item.rating}
						status={item.status}
						inLibrary={true}
						anime={item.anime}
					/>
				</a>
			{/each}
		</div>
	{/if}

	{#if total > 50}
		<div class="pagination">
			<button disabled={page <= 1} on:click={() => { page--; loadLibrary(); }}>Prev</button>
			<span>Page {page}</span>
			<button disabled={page * 50 >= total} on:click={() => { page++; loadLibrary(); }}>Next</button>
		</div>
	{/if}
</div>

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
		letter-spacing: -0.02em;
	}

	.count {
		color: var(--text-muted);
		font-size: 0.85rem;
	}

	.filters {
		display: flex;
		gap: 1rem;
		margin-bottom: 1.5rem;
		flex-wrap: wrap;
	}

	/* Type selector - first filter group: glass bg, accent active with glow (matches Discover) */
	.filter-group {
		display: flex;
		gap: 2px;
		background: var(--glass-bg);
		backdrop-filter: var(--glass-blur);
		-webkit-backdrop-filter: var(--glass-blur);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-sm);
		padding: 3px;
	}

	.filter-group button {
		padding: 0.4rem 1rem;
		border: none;
		background: transparent;
		color: var(--text-muted);
		border-radius: var(--radius-sm);
		font-size: 0.82rem;
		font-weight: 600;
		font-family: var(--font-body);
		cursor: pointer;
		transition: all 0.2s;
	}

	.filter-group button:hover {
		color: var(--text-primary);
	}

	.filter-group button.active {
		background: var(--accent);
		color: var(--text-inverse);
		box-shadow: 0 2px 8px var(--accent-glow);
	}

	/* Status filter pills - second filter group: same glass bg, accent active */

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

	.empty-icon {
		font-size: 2.5rem;
		opacity: 0.3;
		margin-bottom: 0.75rem;
	}

	.empty p {
		font-size: 0.9rem;
	}

	.btn-link {
		display: inline-block;
		margin-top: 0.75rem;
		color: var(--accent);
		font-size: 0.85rem;
		font-weight: 600;
		transition: color 0.2s;
	}

	.btn-link:hover {
		color: var(--accent-hover);
	}

	.pagination {
		display: flex;
		justify-content: center;
		align-items: center;
		gap: 1rem;
		margin-top: 2rem;
	}

	.pagination button {
		padding: 0.45rem 1.1rem;
		background: var(--glass-bg);
		backdrop-filter: var(--glass-blur);
		-webkit-backdrop-filter: var(--glass-blur);
		border: 1px solid var(--glass-border);
		color: var(--text-primary);
		border-radius: var(--radius-sm);
		font-size: 0.82rem;
		font-weight: 600;
		cursor: pointer;
		transition: all 0.2s;
	}

	.pagination button:hover:not(:disabled) {
		background: var(--accent);
		color: var(--text-inverse);
		box-shadow: 0 2px 8px var(--accent-glow);
	}

	.pagination button:disabled {
		opacity: 0.3;
		cursor: default;
	}

	.pagination span {
		color: var(--text-secondary);
		font-size: 0.85rem;
	}

	@media (max-width: 768px) {
		.page-header {
			flex-direction: column;
			gap: 0.75rem;
			align-items: stretch;
		}
		.filters {
			flex-direction: column;
		}
		.grid {
			grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
		}
	}
</style>
