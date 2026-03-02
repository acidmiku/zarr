<script>
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';
	import TrackList from '$lib/components/TrackList.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import StarRating from '$lib/components/StarRating.svelte';

	let album = null;
	let tracks = [];
	let profiles = [];
	let loading = true;
	let error = '';

	// Releases modal
	let showReleases = false;
	let releases = [];
	let searchingReleases = false;

	// Rating
	let showRatingModal = false;
	let userRating = 0;
	let userComment = '';
	let savingRating = false;

	$: id = parseInt($page.params.id);

	onMount(async () => {
		await loadAlbum();
		profiles = await api.getProfiles();
		loading = false;
	});

	async function loadAlbum() {
		try {
			const data = await api.getMusicLibraryItem(id);
			album = data.album;
			tracks = data.tracks || [];
			if (album.rating) {
				userRating = album.rating;
				userComment = album.rating_comment || '';
			}
		} catch (e) {
			notify('Failed to load album', 'error');
		}
	}

	async function searchAlbum() {
		showReleases = true;
		searchingReleases = true;
		error = '';
		try {
			releases = await api.searchMusicAlbum(id);
			if (!Array.isArray(releases)) releases = [];
		} catch (e) {
			error = e.message;
			releases = [];
		}
		searchingReleases = false;
	}

	async function grabRelease(rel) {
		error = '';
		try {
			const data = {
				release_url: rel.nzb_url,
				album_id: id
			};
			if (rel.download_type === 'torrent') {
				data.download_type = 'torrent';
				data.topic_id = rel.topic_id;
			}
			await api.grabMusicRelease(data);
			notify('Release grabbed!', 'success');
			showReleases = false;
			await loadAlbum();
		} catch (e) {
			error = e.message;
		}
	}

	async function deleteAlbum() {
		if (!confirm('Remove this album from library?')) return;
		try {
			await api.deleteMusicLibraryItem(id);
			notify('Album removed', 'success');
			goto('/music');
		} catch (e) {
			error = e.message;
		}
	}

	async function saveRating() {
		savingRating = true;
		try {
			await api.rateMusicAlbum(id, { rating: userRating, comment: userComment });
			album.rating = userRating;
			album.rating_comment = userComment;
			notify('Rating saved', 'success');
			showRatingModal = false;
		} catch (e) {
			error = e.message;
		}
		savingRating = false;
	}

	$: coverSrc = album?.release_group_id ? api.musicCoverUrl(album.release_group_id) : '';
	$: imageSrc = album?.image_url ? api.imageUrl(album.image_url) : '';
</script>

<svelte:head>
	<title>{album?.title || 'Loading'} - Music - Zarr</title>
</svelte:head>

{#if loading}
	<div class="loading">Loading...</div>
{:else if album}
	<div class="detail-page">
		<div class="content">
			<a href="/music" class="back-link">← Music Library</a>

			<div class="header">
				<div class="cover-art">
					{#if coverSrc}
						<img src={coverSrc} alt={album.title} on:error={(e) => e.target.style.display = 'none'} />
					{/if}
					{#if imageSrc}
						<img src={imageSrc} alt={album.title} class="fallback" />
					{/if}
					<div class="cover-placeholder">{(album.title || '?')[0]}</div>
				</div>
				<div class="info">
					<h1>{album.title}</h1>
					<div class="meta-row">
						<span class="artist-name">{album.artist_name}</span>
						{#if album.year}<span>{album.year}</span>{/if}
						{#if album.album_type}<span class="type-tag">{album.album_type}</span>{/if}
						<StatusBadge status={album.status} />
					</div>

					{#if album.rating}
						<div class="user-rating">
							{'★'.repeat(album.rating)}{'☆'.repeat(5 - album.rating)}
							{#if album.rating_comment}
								<span class="rating-comment">— {album.rating_comment}</span>
							{/if}
						</div>
					{/if}

					<div class="meta-details">
						<span>{album.track_count} tracks</span>
						{#if album.release_group_id}<span>MBID: {album.release_group_id.slice(0, 8)}...</span>{/if}
					</div>

					<div class="toolbar">
						<button class="btn btn-primary" on:click={searchAlbum}>Search</button>
						<button class="btn btn-secondary" on:click={() => { showReleases = true; searchAlbum(); }}>View Releases</button>
						<button class="btn btn-secondary" on:click={() => showRatingModal = true}>
							{album.rating ? 'Edit Rating' : 'Rate'}
						</button>
						<select value={album.quality_profile_id}>
							{#each profiles.filter(p => p.profile_type === 'music') as p}
								<option value={p.id}>{p.name}</option>
							{/each}
						</select>
						<button class="btn btn-danger" on:click={deleteAlbum}>Delete</button>
					</div>

					{#if error}
						<div class="error-banner">{error}</div>
					{/if}
				</div>
			</div>

			{#if tracks.length > 0}
				<div class="tracks-section">
					<h2>Tracks</h2>
					<TrackList {tracks} />
				</div>
			{/if}
		</div>
	</div>

	{#if showReleases}
		<!-- svelte-ignore a11y-click-events-have-key-events -->
		<div class="modal-overlay" on:click={() => showReleases = false} role="presentation">
			<div class="modal releases-modal" on:click|stopPropagation on:keydown|stopPropagation role="dialog">
				<h2>Available Releases</h2>
				{#if searchingReleases}
					<div class="loading">Searching indexers...</div>
				{:else if releases.length === 0}
					<div class="empty">No releases found</div>
				{:else}
					<div class="releases-list">
						{#each releases.sort((a, b) => b.score - a.score) as rel}
							<div class="release" class:rejected={!rel.acceptable}>
								<div class="rel-info">
									<div class="rel-title">{rel.title}</div>
									<div class="rel-meta">
										<span class="rel-type-badge" class:torrent={rel.download_type === 'torrent'}>{rel.download_type === 'torrent' ? 'Torrent' : 'NZB'}</span>
										{#if rel.quality}<span class="rel-quality">{rel.quality}</span>{/if}
										{#if rel.indexer}<span>via {rel.indexer}</span>{/if}
										{#if rel.size}<span>{(rel.size / 1024 / 1024).toFixed(0)} MB</span>{/if}
										{#if rel.download_type === 'torrent' && rel.seeders != null}
											<span class="rel-seeders">{rel.seeders}S / {rel.leechers || 0}L</span>
										{/if}
									</div>
									{#if !rel.acceptable && rel.reject_reason}
										<div class="reject-reason">{rel.reject_reason}</div>
									{/if}
								</div>
								<div class="rel-score">{rel.score || 0}</div>
								{#if rel.acceptable}
									<button class="btn btn-small" on:click={() => grabRelease(rel)}>Grab</button>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	{/if}

	{#if showRatingModal}
		<!-- svelte-ignore a11y-click-events-have-key-events -->
		<div class="modal-overlay" on:click={() => showRatingModal = false} role="presentation">
			<div class="modal rating-modal" on:click|stopPropagation on:keydown|stopPropagation role="dialog">
				<h2>{album.rating ? 'Edit Rating' : 'Rate Album'}</h2>
				<div class="rating-input">
					<StarRating bind:value={userRating} />
					{#if userRating > 0}
						<span class="rating-label">{userRating}/5</span>
					{/if}
				</div>
				<div class="comment-input">
					<label for="rating-comment">Comment (optional)</label>
					<textarea id="rating-comment" bind:value={userComment} placeholder="What did you think?" rows="3"></textarea>
				</div>
				<div class="modal-actions">
					<button class="btn btn-primary" on:click={saveRating} disabled={savingRating}>
						{savingRating ? 'Saving...' : 'Save Rating'}
					</button>
					<button class="btn btn-secondary" on:click={() => showRatingModal = false}>Cancel</button>
				</div>
			</div>
		</div>
	{/if}
{/if}

<style>
	.detail-page { position: relative; }

	.content { position: relative; z-index: 1; }

	.back-link {
		display: inline-block;
		color: var(--accent);
		font-size: 0.85rem;
		margin-bottom: 1rem;
	}

	.header {
		display: flex;
		gap: 2rem;
		margin-bottom: 2rem;
	}

	.cover-art {
		width: 250px;
		height: 250px;
		border-radius: 10px;
		overflow: hidden;
		flex-shrink: 0;
		position: relative;
		background: var(--bg-elevated);
		box-shadow: var(--shadow-lg);
	}

	.cover-art img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		position: absolute;
		top: 0;
		left: 0;
	}

	.cover-art .fallback { z-index: 0; }

	.cover-placeholder {
		width: 100%;
		height: 100%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 4rem;
		font-weight: 700;
		color: var(--text-muted);
	}

	.info { flex: 1; }

	h1 { font-size: 1.75rem; font-weight: 700; margin-bottom: 0.5rem; }

	.meta-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.75rem;
		color: var(--text-secondary);
		font-size: 0.9rem;
	}

	.artist-name { font-weight: 600; color: var(--text-primary); }

	.type-tag {
		background: var(--bg-elevated);
		padding: 2px 8px;
		border-radius: 4px;
		font-size: 0.7rem;
		text-transform: uppercase;
	}

	.user-rating {
		color: var(--gold);
		font-size: 0.9rem;
		margin-bottom: 0.5rem;
	}

	.rating-comment {
		color: var(--text-secondary);
		font-size: 0.8rem;
		font-style: italic;
	}

	.meta-details {
		display: flex;
		gap: 1rem;
		font-size: 0.75rem;
		color: var(--text-muted);
		margin-bottom: 1rem;
	}

	.toolbar {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
		align-items: center;
	}

	.btn {
		padding: 0.5rem 1rem;
		border-radius: 6px;
		font-size: 0.85rem;
		font-weight: 600;
		border: none;
		cursor: pointer;
	}

	.btn-primary { background: var(--accent); color: var(--text-inverse); }
	.btn-primary:hover { background: var(--accent-hover); }
	.btn-secondary { background: var(--bg-elevated); border: 1px solid var(--border); color: var(--text-secondary); }
	.btn-secondary:hover { background: var(--bg-hover); color: var(--text-primary); }
	.btn-danger { background: var(--danger-bg); color: var(--danger); border: 1px solid var(--danger-border); }
	.btn-small { padding: 0.3rem 0.75rem; font-size: 0.8rem; background: var(--accent); color: var(--text-inverse); border: none; border-radius: 4px; }

	select {
		background: var(--bg-input);
		border: 1px solid var(--border-subtle);
		color: var(--text-primary);
		padding: 0.5rem 0.75rem;
		border-radius: 6px;
		font-size: 0.85rem;
	}

	.tracks-section h2 {
		font-size: 1.1rem;
		margin-bottom: 0.75rem;
	}

	.error-banner {
		background: var(--danger-bg);
		border: 1px solid var(--danger-border);
		color: var(--danger);
		padding: 0.6rem 1rem;
		border-radius: 6px;
		font-size: 0.85rem;
		margin-top: 0.75rem;
	}

	.loading, .empty {
		text-align: center;
		color: var(--text-muted);
		padding: 2rem;
	}

	.modal-overlay {
		position: fixed;
		inset: 0;
		background: var(--bg-overlay);
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 200;
		padding: 2rem;
	}

	.releases-modal {
		width: 100%;
		max-width: 800px;
		max-height: 80vh;
		overflow-y: auto;
		background: var(--bg-surface);
		border: 1px solid var(--border);
		border-radius: 12px;
		padding: 1.5rem;
	}

	.releases-modal h2 { margin-bottom: 1rem; }

	.releases-list { display: flex; flex-direction: column; gap: 0.5rem; }

	.release {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 0.75rem;
		background: var(--bg-surface);
		border-radius: 6px;
		border: 1px solid var(--border);
	}

	.release.rejected { opacity: 0.4; }
	.rel-info { flex: 1; min-width: 0; }
	.rel-title { font-size: 0.85rem; word-break: break-all; }
	.rel-meta { display: flex; gap: 0.5rem; font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem; }
	.rel-quality { color: var(--accent); }
	.rel-type-badge {
		background: var(--info-bg);
		color: var(--info);
		padding: 1px 6px;
		border-radius: 3px;
		font-size: 0.7rem;
		font-weight: 600;
		text-transform: uppercase;
	}
	.rel-type-badge.torrent {
		background: var(--success-bg);
		color: var(--success);
	}
	.rel-seeders { color: var(--success); font-weight: 500; }
	.reject-reason { font-size: 0.7rem; color: var(--danger); margin-top: 2px; }
	.rel-score { font-size: 1.1rem; font-weight: 700; color: var(--accent); min-width: 40px; text-align: center; }

	.rating-modal {
		width: 100%;
		max-width: 500px;
		background: var(--bg-surface);
		border: 1px solid var(--border);
		border-radius: 12px;
		padding: 1.5rem;
	}

	.rating-modal h2 { margin-bottom: 1rem; font-size: 1.1rem; }
	.rating-input { display: flex; align-items: center; gap: 0.75rem; margin-bottom: 1rem; }
	.rating-label { font-size: 0.9rem; color: var(--text-secondary); }

	.comment-input { margin-bottom: 1rem; }
	.comment-input label { display: block; font-size: 0.8rem; color: var(--text-secondary); margin-bottom: 0.35rem; }
	.comment-input textarea {
		width: 100%;
		background: var(--bg-input);
		border: 1px solid var(--border-subtle);
		color: var(--text-primary);
		border-radius: 6px;
		padding: 0.6rem 0.75rem;
		resize: vertical;
		font-size: 0.85rem;
		font-family: inherit;
	}

	.modal-actions { display: flex; gap: 0.5rem; }

	@media (max-width: 768px) {
		.header {
			flex-direction: column;
			align-items: center;
			text-align: center;
		}
		.cover-art { width: 200px; height: 200px; }
		.toolbar { justify-content: center; }
	}
</style>
