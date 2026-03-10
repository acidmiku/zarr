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

	function itemKey(item) {
		return item.album_id ? `music-${item.album_id}` : item.id;
	}

	function startEdit(item) {
		editingId = itemKey(item);
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
			if (item.media_type === 'music' && item.album_id) {
				await api.rateMusicAlbum(item.album_id, { rating: editRating, comment: editComment });
			} else {
				await api.upsertRating({
					tmdb_id: item.tmdb_id,
					media_type: item.media_type,
					rating: editRating,
					comment: editComment
				});
			}
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
			if (item.media_type === 'music' && item.album_id) {
				await api.rateMusicAlbum(item.album_id, { rating: 0, comment: '' });
			} else {
				await api.deleteRating(item.tmdb_id, item.media_type);
			}
			notify('Rating removed', 'success');
			await loadRatings();
		} catch (e) {
			notify(e.message, 'error');
		}
	}

	function typeLabel(item) {
		if (item.media_type === 'music') return 'Music';
		if (item.anime) return 'Anime';
		if (item.media_type === 'movie') return 'Movie';
		return 'Series';
	}

	function typeColor(item) {
		if (item.media_type === 'music') return '#1db954';
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
		if (item.media_type === 'music' && item.release_group_id) {
			return api.musicCoverUrl(item.release_group_id);
		}
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
			{#each [['all', 'All'], ['movie', 'Movies'], ['series', 'Series'], ['anime', 'Anime'], ['music', 'Music']] as [val, label]}
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
			{#each ratings as item (item.album_id ? `music-${item.album_id}` : item.id)}
				<div class="rating-row" class:editing={editingId === itemKey(item)}>
					<div class="poster-thumb">
						{#if posterUrl(item)}
							<img src={posterUrl(item)} alt="" loading="lazy" />
						{:else}
							<div class="poster-placeholder">{(item.title || '?')[0]}</div>
						{/if}
					</div>

					<div class="info">
						<div class="title-row">
							{#if item.media_type === 'music' && item.album_id}
								<a href="/music/{item.album_id}" class="title-link">{item.title || 'Unknown'}</a>
							{:else if item.media_id}
								<a href="/library/{item.media_id}" class="title-link">{item.title || 'Unknown'}</a>
							{:else}
								<span class="title-text">{item.title || 'Unknown'}</span>
							{/if}
							{#if item.year}
								<span class="year">({item.year})</span>
							{/if}
							<span class="type-badge" style="background: {typeColor(item)}">{typeLabel(item)}</span>
						</div>

						{#if editingId === itemKey(item)}
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
		margin-bottom: 1.25rem;
	}
	.page-header h1 {
		font-family: var(--font-display);
		font-size: 1.75rem;
		font-weight: 800;
		letter-spacing: -0.02em;
	}
	.count {
		font-size: 0.85rem;
		color: var(--text-muted);
		font-weight: 500;
	}

	.controls {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-bottom: 1.5rem;
		flex-wrap: wrap;
	}

	.filter-group, .sort-group {
		display: flex;
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-sm);
		padding: 3px;
		gap: 2px;
	}
	.filter-group button, .sort-group button {
		padding: 0.4rem 0.8rem;
		border: none;
		border-radius: 6px;
		background: transparent;
		color: var(--text-secondary);
		font-size: 0.8rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.2s ease;
		white-space: nowrap;
	}
	.filter-group button:hover, .sort-group button:hover {
		color: var(--text-primary);
		background: var(--glass-border);
	}
	.filter-group button.active, .sort-group button.active {
		background: var(--accent);
		color: var(--text-inverse);
		font-weight: 600;
	}

	.loading {
		text-align: center;
		padding: 3rem 1rem;
		color: var(--text-muted);
	}
	.empty {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		text-align: center;
		padding: 4rem 1rem;
		color: var(--text-muted);
	}
	.empty p {
		font-size: 1rem;
		font-family: var(--font-display);
		font-weight: 700;
		letter-spacing: -0.02em;
	}
	.empty-sub {
		font-size: 0.8rem;
		color: var(--text-muted);
		margin-top: 0.35rem;
		font-family: inherit;
		font-weight: 400;
	}

	.ratings-list {
		display: flex;
		flex-direction: column;
		gap: 1px;
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}

	.rating-row {
		display: flex;
		gap: 0.85rem;
		padding: 0.75rem 1rem;
		background: var(--glass-bg);
		align-items: flex-start;
		transition: background 0.2s ease;
	}
	.rating-row:hover {
		background: var(--glass-border);
	}
	.rating-row.editing {
		background: var(--glass-border);
	}

	.poster-thumb {
		width: 45px;
		min-width: 45px;
		height: 67px;
		border-radius: var(--radius-sm);
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
		font-family: var(--font-display);
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
		font-family: var(--font-display);
		font-weight: 600;
		font-size: 0.9rem;
		color: var(--text-primary);
		text-decoration: none;
		letter-spacing: -0.01em;
		transition: color 0.2s ease;
	}
	.title-link:hover { color: var(--accent); }
	.title-text {
		font-family: var(--font-display);
		font-weight: 600;
		font-size: 0.9rem;
		color: var(--text-primary);
		letter-spacing: -0.01em;
	}
	.year {
		font-size: 0.8rem;
		color: var(--text-muted);
	}
	.type-badge {
		padding: 2px 7px;
		border-radius: 4px;
		font-size: 0.6rem;
		font-weight: 700;
		color: var(--text-inverse);
		text-transform: uppercase;
		flex-shrink: 0;
		letter-spacing: 0.03em;
	}

	.rating-display {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}
	.stars-inline {
		font-size: 0.9rem;
		color: var(--gold);
		letter-spacing: 1.5px;
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
		padding: 0.5rem 0.7rem;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-sm);
		color: var(--text-primary);
		font-size: 0.8rem;
		resize: vertical;
		outline: none;
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
		transition: border-color 0.2s ease, box-shadow 0.2s ease;
	}
	.edit-form textarea:focus {
		border-color: var(--accent);
		box-shadow: 0 0 0 2px rgba(var(--accent-rgb, 99, 102, 241), 0.15);
	}
	.edit-actions {
		display: flex;
		gap: 0.5rem;
	}
	.btn-save {
		padding: 0.35rem 0.9rem;
		background: var(--accent);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--text-inverse);
		font-size: 0.78rem;
		font-weight: 600;
		cursor: pointer;
		transition: background 0.2s ease, transform 0.1s ease;
	}
	.btn-save:hover {
		background: var(--accent-hover);
		transform: translateY(-1px);
	}
	.btn-save:disabled { opacity: 0.5; cursor: not-allowed; transform: none; }
	.btn-cancel {
		padding: 0.35rem 0.9rem;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-sm);
		color: var(--text-secondary);
		font-size: 0.78rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.2s ease;
	}
	.btn-cancel:hover {
		color: var(--text-primary);
		border-color: var(--border-strong);
		background: var(--glass-border);
	}

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
		gap: 0.35rem;
		opacity: 0;
		transition: opacity 0.2s ease;
	}
	.rating-row:hover .row-actions { opacity: 1; }
	.act-btn {
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: 5px;
		color: var(--text-secondary);
		font-size: 0.75rem;
		padding: 0.2rem 0.45rem;
		cursor: pointer;
		transition: all 0.2s ease;
	}
	.act-btn:hover {
		color: var(--text-primary);
		border-color: var(--border-strong);
		background: var(--glass-border);
	}
	.act-delete:hover {
		color: var(--danger);
		border-color: var(--danger);
		background: rgba(239, 68, 68, 0.08);
	}

	@media (max-width: 600px) {
		.controls { flex-direction: column; align-items: stretch; }
		.poster-thumb { width: 38px; min-width: 38px; height: 56px; }
		.meta-col { min-width: auto; }
		.row-actions { opacity: 1; }
	}
</style>
