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
		<div class="loading">Loading...</div>
	{:else if items.length === 0}
		<div class="empty">
			<p>Your library is empty.</p>
			<a href="/" class="btn-link">Discover content to add</a>
		</div>
	{:else}
		<div class="grid">
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
		align-items: baseline;
		gap: 0.75rem;
		margin-bottom: 1rem;
	}

	h1 { font-size: 1.5rem; font-weight: 700; }
	.count { color: var(--text-muted); font-size: 0.9rem; }

	.filters {
		display: flex;
		gap: 1rem;
		margin-bottom: 1.25rem;
		flex-wrap: wrap;
	}

	.filter-group {
		display: flex;
		gap: 0.25rem;
		background: var(--bg-surface);
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
		border-radius: 8px;
		padding: 3px;
	}

	.filter-group button {
		padding: 0.35rem 0.85rem;
		border: none;
		background: transparent;
		color: var(--text-secondary);
		border-radius: 6px;
		font-size: 0.8rem;
		cursor: pointer;
	}

	.filter-group button.active {
		background: var(--bg-active);
		color: var(--text-primary);
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

	.btn-link {
		display: inline-block;
		margin-top: 0.5rem;
		color: var(--accent);
		font-size: 0.9rem;
	}

	.pagination {
		display: flex;
		justify-content: center;
		align-items: center;
		gap: 1rem;
		margin-top: 2rem;
	}

	.pagination button {
		padding: 0.4rem 1rem;
		background: var(--bg-elevated);
		border: 1px solid var(--border-subtle);
		color: var(--text-primary);
		border-radius: 6px;
		cursor: pointer;
	}

	.pagination button:disabled {
		opacity: 0.3;
		cursor: default;
	}

	.pagination span {
		color: var(--text-secondary);
		font-size: 0.85rem;
	}
</style>
