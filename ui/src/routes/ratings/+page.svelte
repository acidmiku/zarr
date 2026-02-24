<script>
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';
	import StarRating from '$lib/components/StarRating.svelte';

	let ratings = [];
	let loading = true;
	let filterType = 'all';
	let sortBy = 'rating';
	let sortOrder = 'desc';
	let editingId = null;
	let editRating = 0;
	let editComment = '';
	let saving = false;

	onMount(() => loadRatings());

	async function loadRatings() {
		loading = true;
		try {
			ratings = await api.listRatings(filterType, sortBy, sortOrder);
		} catch (e) {
			notify(e.message, 'error');
			ratings = [];
		}
		loading = false;
	}

	function setType(type) {
		filterType = type;
		loadRatings();
	}

	function setSort(sort) {
		if (sortBy === sort) {
			sortOrder = sortOrder === 'desc' ? 'asc' : 'desc';
		} else {
			sortBy = sort;
			sortOrder = sort === 'title' ? 'asc' : 'desc';
		}
		loadRatings();
	}

	function startEdit(item) {
		editingId = item.id;
		editRating = item.rating;
		editComment = item.comment || '';
	}

	function cancelEdit() {
		editingId = null;
	}

	async function saveEdit(item) {
		if (editRating < 1) return;
		saving = true;
		try {
			await api.upsertRating({
				tmdb_id: item.tmdb_id,
				media_type: item.media_type,
				rating: editRating,
				comment: editComment
			});
			notify('Rating updated', 'success');
			editingId = null;
			await loadRatings();
		} catch (e) {
			notify(e.message, 'error');
		}
		saving = false;
	}

	async function deleteRating(item) {
		if (!confirm(`Remove your rating for "${item.title || 'this item'}"?`)) return;
		try {
			await api.deleteRating(item.tmdb_id, item.media_type);
			notify('Rating removed', 'success');
			await loadRatings();
		} catch (e) {
			notify(e.message, 'error');
		}
	}

	function typeLabel(item) {
		if (item.anime) return 'Anime';
		if (item.media_type === 'movie') return 'Movie';
		return 'Series';
	}

	function typeColor(item) {
		if (item.anime) return 'var(--badge-anime)';
		if (item.media_type === 'movie') return 'var(--badge-movie)';
		return 'var(--badge-series)';
	}

	function timeAgo(dateStr) {
		if (!dateStr) return '';
		const d = new Date(dateStr.replace(' ', 'T') + 'Z');
		if (isNaN(d.getTime())) return ''; // Invalid date
		const diff = Date.now() - d.getTime();
		const mins = Math.floor(diff / 60000);
		if (mins < 1) return 'just now';
		if (mins < 60) return `${mins}m ago`;
		const hrs = Math.floor(mins / 60);
		if (hrs < 24) return `${hrs}h ago`;
		const days = Math.floor(hrs / 24);
		if (days < 30) return `${days}d ago`;
		const months = Math.floor(days / 30);
		if (months < 12) return `${months}mo ago`;
		return `${Math.floor(months / 12)}y ago`;
	}

	function posterUrl(item) {
		if (!item.poster_url) return null;
		return api.imageUrl(item.poster_url);
	}

	function sortArrow(col) {
		if (sortBy !== col) return '';
		return sortOrder === 'asc' ? ' ↑' : ' ↓';
	}
</script>

<svelte:head>
	<title>Ratings - Zarr</title>
</svelte:head>

<div class="page">
	<header class="page-header">
		<h1>My Ratings</h1>
		<span class="count">{ratings.length} rated</span>
	</header>

	<div class="controls">
		<div class="filter-group">
			{#each [['all', 'All'], ['movie', 'Movies'], ['series', 'Series'], ['anime', 'Anime']] as [val, label]}
				<button class:active={filterType === val} on:click={() => setType(val)}>{label}</button>
			{/each}
		</div>
		<div class="sort-group">
			<button class:active={sortBy === 'rating'} on:click={() => setSort('rating')}>Rating{sortArrow('rating')}</button>
			<button class:active={sortBy === 'date'} on:click={() => setSort('date')}>Date{sortArrow('date')}</button>
			<button class:active={sortBy === 'title'} on:click={() => setSort('title')}>Title{sortArrow('title')}</button>
		</div>
	</div>

	{#if loading}
		<div class="loading">Loading...</div>
	{:else if ratings.length === 0}
		<div class="empty">
			<p>No ratings yet.</p>
			<p class="empty-sub">Rate titles from your library to see them here.</p>
		</div>
	{:else}
		<div class="ratings-list">
			{#each ratings as item (item.id)}
				<div class="rating-row" class:editing={editingId === item.id}>
					<div class="poster-thumb">
						{#if posterUrl(item)}
							<img src={posterUrl(item)} alt="" loading="lazy" />
						{:else}
							<div class="poster-placeholder">{(item.title || '?')[0]}</div>
						{/if}
					</div>

					<div class="info">
						<div class="title-row">
							{#if item.media_id}
								<a href="/library/{item.media_id}" class="title-link">{item.title || 'Unknown'}</a>
							{:else}
								<span class="title-text">{item.title || 'Unknown'}</span>
							{/if}
							{#if item.year}
								<span class="year">({item.year})</span>
							{/if}
							<span class="type-badge" style="background: {typeColor(item)}">{typeLabel(item)}</span>
						</div>

						{#if editingId === item.id}
							<div class="edit-form">
								<div class="edit-stars">
									<StarRating bind:value={editRating} />
									{#if editRating > 0}
										<span class="rating-num">{editRating}/5</span>
									{/if}
								</div>
								<textarea
									bind:value={editComment}
									placeholder="Your thoughts (optional)"
									rows="2"
								></textarea>
								<div class="edit-actions">
									<button class="btn-save" on:click={() => saveEdit(item)} disabled={saving || editRating < 1}>
										{saving ? 'Saving...' : 'Save'}
									</button>
									<button class="btn-cancel" on:click={cancelEdit}>Cancel</button>
								</div>
							</div>
						{:else}
							<div class="rating-display">
								<span class="stars-inline">
									{'★'.repeat(item.rating)}{'☆'.repeat(5 - item.rating)}
								</span>
								{#if item.comment}
									<p class="comment">{item.comment}</p>
								{/if}
							</div>
						{/if}
					</div>

					<div class="meta-col">
						<span class="time-ago">{timeAgo(item.updated_at)}</span>
						{#if editingId !== item.id}
							<div class="row-actions">
								<button class="act-btn" on:click={() => startEdit(item)} title="Edit">✎</button>
								<button class="act-btn act-delete" on:click={() => deleteRating(item)} title="Delete">✕</button>
							</div>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.page { max-width: 900px; }

	.page-header {
		display: flex;
		align-items: baseline;
		gap: 0.75rem;
		margin-bottom: 1rem;
	}
	.page-header h1 { font-size: 1.5rem; font-weight: 700; }
	.count { font-size: 0.85rem; color: var(--text-muted); }

	.controls {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-bottom: 1.25rem;
		flex-wrap: wrap;
	}

	.filter-group, .sort-group {
		display: flex;
		background: var(--bg-surface);
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
		border-radius: 8px;
		padding: 3px;
		gap: 2px;
	}
	.filter-group button, .sort-group button {
		padding: 0.35rem 0.75rem;
		border: none;
		border-radius: 6px;
		background: transparent;
		color: var(--text-secondary);
		font-size: 0.8rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.15s;
		white-space: nowrap;
	}
	.filter-group button:hover, .sort-group button:hover { color: var(--text-primary); }
	.filter-group button.active, .sort-group button.active {
		background: var(--bg-active);
		color: var(--text-primary);
	}

	.loading, .empty {
		text-align: center;
		padding: 3rem 1rem;
		color: var(--text-muted);
	}
	.empty p { font-size: 0.95rem; }
	.empty-sub { font-size: 0.8rem; color: var(--text-muted); margin-top: 0.35rem; }

	.ratings-list {
		display: flex;
		flex-direction: column;
		gap: 1px;
		background: var(--bg-elevated);
		border: 1px solid var(--border);
		border-radius: 10px;
		overflow: hidden;
	}

	.rating-row {
		display: flex;
		gap: 0.85rem;
		padding: 0.75rem 1rem;
		background: var(--bg-surface);
		align-items: flex-start;
		transition: background 0.15s;
	}
	.rating-row:hover { background: var(--bg-hover); }
	.rating-row.editing { background: var(--bg-hover); }

	.poster-thumb {
		width: 45px;
		min-width: 45px;
		height: 67px;
		border-radius: 5px;
		overflow: hidden;
		background: var(--bg-elevated);
		flex-shrink: 0;
	}
	.poster-thumb img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.poster-placeholder {
		width: 100%;
		height: 100%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 1.1rem;
		color: var(--text-muted);
		font-weight: 700;
		text-transform: uppercase;
	}

	.info {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.title-row {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		flex-wrap: wrap;
	}
	.title-link {
		font-weight: 600;
		font-size: 0.9rem;
		color: var(--text-primary);
		text-decoration: none;
	}
	.title-link:hover { color: var(--accent); }
	.title-text {
		font-weight: 600;
		font-size: 0.9rem;
		color: var(--text-primary);
	}
	.year {
		font-size: 0.8rem;
		color: var(--text-muted);
	}
	.type-badge {
		padding: 1px 6px;
		border-radius: 3px;
		font-size: 0.6rem;
		font-weight: 700;
		color: var(--text-inverse);
		text-transform: uppercase;
		flex-shrink: 0;
	}

	.rating-display {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}
	.stars-inline {
		font-size: 0.85rem;
		color: var(--gold);
		letter-spacing: 1px;
	}
	.comment {
		font-size: 0.8rem;
		color: var(--text-secondary);
		line-height: 1.4;
		margin: 0;
	}

	.edit-form {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		margin-top: 0.25rem;
	}
	.edit-stars {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.rating-num {
		font-size: 0.8rem;
		color: var(--text-secondary);
	}
	.edit-form textarea {
		width: 100%;
		padding: 0.45rem 0.65rem;
		background: var(--bg-input);
		border: 1px solid var(--border-subtle);
		border-radius: 6px;
		color: var(--text-primary);
		font-size: 0.8rem;
		resize: vertical;
		outline: none;
	}
	.edit-form textarea:focus { border-color: var(--accent); }
	.edit-actions {
		display: flex;
		gap: 0.5rem;
	}
	.btn-save {
		padding: 0.3rem 0.85rem;
		background: var(--accent);
		border: none;
		border-radius: 6px;
		color: var(--text-inverse);
		font-size: 0.78rem;
		font-weight: 600;
	}
	.btn-save:hover { background: var(--accent-hover); }
	.btn-save:disabled { opacity: 0.5; cursor: not-allowed; }
	.btn-cancel {
		padding: 0.3rem 0.85rem;
		background: transparent;
		border: 1px solid var(--border-subtle);
		border-radius: 6px;
		color: var(--text-secondary);
		font-size: 0.78rem;
		font-weight: 500;
	}
	.btn-cancel:hover { color: var(--text-primary); border-color: var(--border-strong); }

	.meta-col {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 0.4rem;
		min-width: 70px;
		flex-shrink: 0;
	}
	.time-ago {
		font-size: 0.72rem;
		color: var(--text-muted);
		white-space: nowrap;
	}
	.row-actions {
		display: flex;
		gap: 0.3rem;
		opacity: 0;
		transition: opacity 0.15s;
	}
	.rating-row:hover .row-actions { opacity: 1; }
	.act-btn {
		background: none;
		border: 1px solid var(--border-subtle);
		border-radius: 4px;
		color: var(--text-secondary);
		font-size: 0.75rem;
		padding: 0.15rem 0.4rem;
		cursor: pointer;
	}
	.act-btn:hover { color: var(--text-primary); border-color: var(--border-strong); }
	.act-delete:hover { color: var(--danger); border-color: var(--danger); }

	@media (max-width: 600px) {
		.controls { flex-direction: column; align-items: stretch; }
		.poster-thumb { width: 38px; min-width: 38px; height: 56px; }
		.meta-col { min-width: auto; }
		.row-actions { opacity: 1; }
	}
</style>
