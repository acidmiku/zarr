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
	/* ---- Layout ---- */
	.detail-page {
		position: relative;
		animation: fadeSlideUp 0.45s cubic-bezier(0.16, 1, 0.3, 1) both;
	}

	@keyframes fadeSlideUp {
		from { opacity: 0; transform: translateY(14px); }
		to   { opacity: 1; transform: translateY(0); }
	}

	.content {
		position: relative;
		z-index: 1;
	}

	/* ---- Back link ---- */
	.back-link {
		display: inline-flex;
		align-items: center;
		color: var(--text-secondary);
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-weight: 500;
		letter-spacing: 0.01em;
		margin-bottom: 1.5rem;
		transition: color 0.2s ease;
	}

	.back-link:hover {
		color: var(--accent);
	}

	/* ---- Header — hero section ---- */
	.header {
		display: flex;
		gap: 2.25rem;
		margin-bottom: 2.5rem;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-xl);
		padding: 1.75rem;
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		box-shadow: var(--shadow-md);
	}

	/* ---- Cover art ---- */
	.cover-art {
		width: 260px;
		height: 260px;
		border-radius: var(--radius-lg);
		overflow: hidden;
		flex-shrink: 0;
		position: relative;
		background: linear-gradient(135deg, var(--bg-elevated), var(--bg-surface));
		box-shadow: var(--shadow-lg), 0 0 0 1px var(--glass-border);
		transition: box-shadow 0.25s ease;
	}

	.cover-art:hover {
		box-shadow: var(--shadow-lg), var(--shadow-glow);
	}

	.cover-art img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		position: absolute;
		top: 0;
		left: 0;
		transition: transform 0.25s ease;
	}

	.cover-art:hover img {
		transform: scale(1.03);
	}

	.cover-art .fallback {
		z-index: 0;
	}

	.cover-placeholder {
		width: 100%;
		height: 100%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-family: var(--font-display);
		font-size: 4.5rem;
		font-weight: 800;
		color: var(--text-muted);
		background: linear-gradient(135deg, var(--bg-elevated), var(--bg-surface));
		letter-spacing: -0.04em;
	}

	/* ---- Info column ---- */
	.info {
		flex: 1;
		display: flex;
		flex-direction: column;
		justify-content: center;
		min-width: 0;
	}

	h1 {
		font-family: var(--font-display);
		font-size: 2rem;
		font-weight: 800;
		letter-spacing: -0.02em;
		line-height: 1.15;
		color: var(--text-primary);
		margin-bottom: 0.6rem;
	}

	/* ---- Meta row ---- */
	.meta-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.75rem;
		color: var(--text-secondary);
		font-family: var(--font-body);
		font-size: 0.9rem;
		flex-wrap: wrap;
	}

	.artist-name {
		font-weight: 600;
		color: var(--text-primary);
	}

	.type-tag {
		background: var(--accent-subtle);
		color: var(--accent);
		padding: 2px 10px;
		border-radius: var(--radius-sm);
		font-family: var(--font-body);
		font-size: 0.65rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}

	/* ---- User rating ---- */
	.user-rating {
		color: var(--gold);
		font-size: 1rem;
		margin-bottom: 0.6rem;
		letter-spacing: 0.04em;
	}

	.rating-comment {
		color: var(--text-secondary);
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-style: italic;
		opacity: 0.85;
	}

	/* ---- Meta details ---- */
	.meta-details {
		display: flex;
		gap: 1rem;
		font-family: var(--font-body);
		font-size: 0.75rem;
		color: var(--text-muted);
		margin-bottom: 1.25rem;
		letter-spacing: 0.01em;
	}

	/* ---- Toolbar ---- */
	.toolbar {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
		align-items: center;
	}

	.btn {
		padding: 0.5rem 1.1rem;
		border-radius: var(--radius-sm);
		font-family: var(--font-body);
		font-size: 0.82rem;
		font-weight: 600;
		border: none;
		cursor: pointer;
		transition: all 0.2s ease;
		letter-spacing: 0.01em;
	}

	.btn:focus-visible {
		outline: none;
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	.btn-primary {
		background: var(--accent);
		color: var(--text-inverse);
		box-shadow: 0 0 12px color-mix(in srgb, var(--accent) 30%, transparent);
	}

	.btn-primary:hover {
		background: var(--accent-hover);
		box-shadow: 0 0 20px color-mix(in srgb, var(--accent) 45%, transparent);
		transform: translateY(-1px);
	}

	.btn-secondary {
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		color: var(--text-secondary);
		backdrop-filter: blur(8px);
		-webkit-backdrop-filter: blur(8px);
	}

	.btn-secondary:hover {
		background: var(--bg-hover);
		color: var(--text-primary);
		border-color: var(--accent-subtle);
		transform: translateY(-1px);
	}

	.btn-danger {
		background: var(--danger-bg);
		color: var(--danger);
		border: 1px solid var(--danger-border);
	}

	.btn-danger:hover {
		border-color: var(--danger);
		transform: translateY(-1px);
	}

	.btn-small {
		padding: 0.3rem 0.85rem;
		font-family: var(--font-body);
		font-size: 0.78rem;
		font-weight: 600;
		background: var(--accent);
		color: var(--text-inverse);
		border: none;
		border-radius: var(--radius-sm);
		cursor: pointer;
		transition: all 0.2s ease;
	}

	.btn-small:hover {
		background: var(--accent-hover);
		box-shadow: 0 0 12px color-mix(in srgb, var(--accent) 40%, transparent);
		transform: translateY(-1px);
	}

	.btn-small:focus-visible {
		outline: none;
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	/* ---- Select ---- */
	select {
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		color: var(--text-primary);
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius-sm);
		font-family: var(--font-body);
		font-size: 0.82rem;
		backdrop-filter: blur(8px);
		-webkit-backdrop-filter: blur(8px);
		cursor: pointer;
		transition: border-color 0.2s ease;
	}

	select:hover {
		border-color: var(--accent-subtle);
	}

	select:focus-visible {
		outline: none;
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	/* ---- Tracks section ---- */
	.tracks-section {
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-lg);
		padding: 1.5rem;
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		box-shadow: var(--shadow-sm);
	}

	.tracks-section h2 {
		font-family: var(--font-display);
		font-size: 1.1rem;
		font-weight: 700;
		letter-spacing: -0.02em;
		margin-bottom: 1rem;
		color: var(--text-primary);
	}

	/* ---- Error banner ---- */
	.error-banner {
		background: var(--danger-bg);
		border: 1px solid var(--danger-border);
		color: var(--danger);
		padding: 0.65rem 1rem;
		border-radius: var(--radius-sm);
		font-family: var(--font-body);
		font-size: 0.82rem;
		margin-top: 0.75rem;
		backdrop-filter: blur(8px);
		-webkit-backdrop-filter: blur(8px);
	}

	/* ---- Loading & empty states ---- */
	.loading, .empty {
		text-align: center;
		color: var(--text-muted);
		font-family: var(--font-body);
		padding: 3rem;
		font-size: 0.9rem;
	}

	/* ---- Modal overlay ---- */
	.modal-overlay {
		position: fixed;
		inset: 0;
		background: var(--bg-overlay);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 200;
		padding: 2rem;
		animation: fadeIn 0.2s ease both;
	}

	@keyframes fadeIn {
		from { opacity: 0; }
		to   { opacity: 1; }
	}

	@keyframes scaleIn {
		from { opacity: 0; transform: scale(0.96) translateY(8px); }
		to   { opacity: 1; transform: scale(1) translateY(0); }
	}

	/* ---- Releases modal ---- */
	.releases-modal {
		width: 100%;
		max-width: 820px;
		max-height: 80vh;
		overflow-y: auto;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-xl);
		padding: 1.75rem;
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		box-shadow: var(--shadow-lg);
		animation: scaleIn 0.25s cubic-bezier(0.16, 1, 0.3, 1) both;
	}

	.releases-modal h2 {
		font-family: var(--font-display);
		font-weight: 700;
		letter-spacing: -0.02em;
		margin-bottom: 1.25rem;
		font-size: 1.2rem;
	}

	.releases-list {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.release {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 0.85rem 1rem;
		background: var(--bg-hover);
		border-radius: var(--radius-md);
		border: 1px solid transparent;
		transition: all 0.2s ease;
	}

	.release:hover {
		border-color: var(--glass-border);
		background: var(--bg-active);
		box-shadow: var(--shadow-sm);
	}

	.release.rejected {
		opacity: 0.35;
	}

	.rel-info {
		flex: 1;
		min-width: 0;
	}

	.rel-title {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-weight: 500;
		color: var(--text-primary);
		word-break: break-all;
		line-height: 1.4;
	}

	.rel-meta {
		display: flex;
		gap: 0.6rem;
		font-family: var(--font-body);
		font-size: 0.72rem;
		color: var(--text-muted);
		margin-top: 0.3rem;
		flex-wrap: wrap;
		align-items: center;
	}

	.rel-quality {
		color: var(--accent);
		font-weight: 600;
	}

	.rel-type-badge {
		background: var(--info-bg);
		color: var(--info);
		padding: 1px 7px;
		border-radius: var(--radius-sm);
		font-family: var(--font-body);
		font-size: 0.65rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}

	.rel-type-badge.torrent {
		background: var(--success-bg);
		color: var(--success);
	}

	.rel-seeders {
		color: var(--success);
		font-weight: 600;
	}

	.reject-reason {
		font-family: var(--font-body);
		font-size: 0.7rem;
		color: var(--danger);
		margin-top: 3px;
		opacity: 0.9;
	}

	.rel-score {
		font-family: var(--font-display);
		font-size: 1.15rem;
		font-weight: 800;
		color: var(--accent);
		min-width: 44px;
		text-align: center;
		letter-spacing: -0.02em;
	}

	/* ---- Rating modal ---- */
	.rating-modal {
		width: 100%;
		max-width: 480px;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-xl);
		padding: 1.75rem;
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		box-shadow: var(--shadow-lg);
		animation: scaleIn 0.25s cubic-bezier(0.16, 1, 0.3, 1) both;
	}

	.rating-modal h2 {
		font-family: var(--font-display);
		font-weight: 700;
		letter-spacing: -0.02em;
		margin-bottom: 1.25rem;
		font-size: 1.15rem;
	}

	.rating-input {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 1.25rem;
	}

	.rating-label {
		font-family: var(--font-body);
		font-size: 0.9rem;
		color: var(--text-secondary);
	}

	.comment-input {
		margin-bottom: 1.25rem;
	}

	.comment-input label {
		display: block;
		font-family: var(--font-display);
		font-size: 0.78rem;
		font-weight: 600;
		color: var(--text-secondary);
		margin-bottom: 0.4rem;
		letter-spacing: 0.01em;
	}

	.comment-input textarea {
		width: 100%;
		background: var(--bg-input);
		border: 1px solid var(--glass-border);
		color: var(--text-primary);
		border-radius: var(--radius-md);
		padding: 0.7rem 0.85rem;
		resize: vertical;
		font-family: var(--font-body);
		font-size: 0.85rem;
		line-height: 1.5;
		transition: border-color 0.2s ease, box-shadow 0.2s ease;
	}

	.comment-input textarea:focus {
		outline: none;
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	.modal-actions {
		display: flex;
		gap: 0.5rem;
	}

	/* ---- Responsive ---- */
	@media (max-width: 768px) {
		.header {
			flex-direction: column;
			align-items: center;
			text-align: center;
			padding: 1.25rem;
			border-radius: var(--radius-lg);
		}

		.cover-art {
			width: 200px;
			height: 200px;
		}

		.info {
			align-items: center;
		}

		.meta-row {
			justify-content: center;
		}

		.toolbar {
			justify-content: center;
		}

		.tracks-section {
			padding: 1rem;
			border-radius: var(--radius-md);
		}

		.releases-modal,
		.rating-modal {
			border-radius: var(--radius-lg);
			padding: 1.25rem;
		}
	}
</style>
