<script>
	import { createEventDispatcher, onMount } from 'svelte';
	import { api } from '$lib/api';
	import { musicArtwork, releaseGroupId } from '$lib/music';
	import Artwork from './Artwork.svelte';
	import MusicAcquisition from './MusicAcquisition.svelte';
	export let item = null;
	export let profiles = [];
	const dispatch = createEventDispatcher();
	let selectedProfile = 0;
	let adding = false;
	let monitored = false;
	let albumDetail = null;
	let loadingDetail = false;
	let detailError = '';
	let actionError = '';
	let savedAlbum = null;
	let selectedEdition = '';
	let detailRequest = 0;
	let disposed = false;
	$: artistName = item?.artist || item?.artist_name || '';
	$: albumTitle = item?.title || item?.name || '';
	$: year = String(item?.year || '').slice(0, 4);
	$: trackList =
		albumDetail?.release?.media?.flatMap((m) =>
			(m.tracks || []).map((t) => ({ ...t, disc: m.position }))
		) || [];
	$: editions = Array.isArray(albumDetail?.editions) ? albumDetail.editions : [];
	$: canSave = !adding && !loadingDetail && !detailError && !!albumDetail?.release?.id;
	onMount(() => {
		loadDetail();
		if (item?.in_library && item?.library_id) {
			api
				.getMusicLibraryItem(item.library_id)
				.then((data) => {
					if (!disposed) savedAlbum = data.album;
				})
				.catch((e) => {
					if (!disposed) actionError = e.message;
				});
		}
		return () => {
			disposed = true;
			detailRequest++;
		};
	});
	async function loadDetail(edition = '') {
		if (edition) selectedEdition = edition;
		const rgid = releaseGroupId(item);
		if (!rgid) {
			detailError = 'This album could not be identified in the catalog.';
			return;
		}
		const request = ++detailRequest;
		loadingDetail = true;
		detailError = '';
		try {
			const data = await api.musicAlbum(rgid, edition);
			if (disposed || request !== detailRequest) return;
			if (!data.release?.id) throw new Error('No complete edition is available.');
			if (edition && data.release.id !== edition)
				throw new Error('The selected edition could not be loaded.');
			albumDetail = { ...data, editions: data.editions || albumDetail?.editions || [] };
			selectedEdition = data.release?.id || edition;
		} catch (e) {
			if (!disposed && request === detailRequest)
				detailError = `Track details are unavailable: ${e.message}`;
		} finally {
			if (request === detailRequest) loadingDetail = false;
		}
	}
	async function save(downloadNow = false) {
		if (!canSave) return;
		adding = true;
		actionError = '';
		try {
			const payload = {
				release_group_id: releaseGroupId(item),
				artist_mbid: item?.artist_id || item?.artist_mbid || '',
				artist_name: artistName,
				album_title: albumTitle,
				year: parseInt(year) || 0,
				album_type: item?.type || 'Album',
				quality_profile_id: selectedProfile,
				image_url: item?.cover_url || item?.image_url || '',
				monitored,
				download_now: downloadNow
			};
			if (selectedEdition) payload.release_id = selectedEdition;
			const result = await api.addMusicToLibrary(payload);
			if (disposed) return;
			if (!result?.id)
				throw new Error(
					'Album was saved, but its library ID was not returned. Refresh the library before trying again.'
				);
			savedAlbum = {
				...item,
				id: result.id,
				monitored,
				acquisition: { status: downloadNow ? 'queued' : 'saved' }
			};
			try {
				const data = await api.getMusicLibraryItem(result.id);
				if (!disposed && data.album) savedAlbum = data.album;
			} catch {
				actionError = 'Album saved. Status could not refresh yet; open the library to check it.';
			}
			dispatch('added', { id: result.id, downloadNow, album: savedAlbum });
		} catch (e) {
			if (!disposed) actionError = e.message;
		} finally {
			adding = false;
		}
	}
	function editionLabel(edition) {
		const tracks = (edition.media || []).reduce(
			(sum, disc) => sum + (disc['track-count'] || disc.track_count || disc.tracks?.length || 0),
			0
		);
		return (
			[
				edition.title,
				edition.date || edition.country,
				edition.country && edition.date ? edition.country : '',
				tracks ? `${tracks} tracks` : ''
			]
				.filter(Boolean)
				.join(' · ') || edition.id
		);
	}
	function formatDuration(ms) {
		if (!ms) return '';
		const seconds = Math.floor(ms / 1000);
		return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`;
	}
</script>

<div
	class="detail"
	on:click|stopPropagation
	on:keydown|stopPropagation
	role="dialog"
	aria-label={albumTitle}
>
	<div class="detail-content">
		<div class="detail-header">
			<div class="detail-cover"><Artwork src={musicArtwork(item)} title={albumTitle} eager /></div>
			<div class="detail-info">
				<h2>{albumTitle}</h2>
				<div class="meta-row">
					<span class="artist">{artistName}</span>{#if year}<span class="year">{year}</span
						>{/if}{#if item?.type}<span class="type-tag">{item.type}</span>{/if}
				</div>
				{#if item?.reason}<p class="album-reason">{item.reason}</p>{/if}
				{#if savedAlbum}
					<MusicAcquisition
						bind:album={savedAlbum}
						on:updated={(e) => dispatch('updated', e.detail)}
					/>
					<a href="/music/{savedAlbum.id}" class="btn btn-secondary">View in Library</a>
				{:else if item?.in_library}
					<p>Saved in your library.</p>
					{#if item.library_id}<a href="/music/{item.library_id}" class="btn btn-secondary"
							>View in Library</a
						>{/if}
				{:else}
					<p class="save-hint">Save albums you want to return to. Download when you are ready.</p>
					<label class="profile-label"
						>Download quality<select bind:value={selectedProfile} disabled={adding}
							><option value={0}>Default music profile</option
							>{#each profiles.filter((p) => p.profile_type === 'music') as p}<option value={p.id}
									>{p.name}</option
								>{/each}</select
						></label
					>
					<label class="monitor-label"
						><input type="checkbox" bind:checked={monitored} disabled={adding} /> Monitor album</label
					>
					<p class="save-hint">
						Monitoring automatically searches for a matching release. Leave it off to only save.
					</p>
					<div class="actions">
						<button class="btn btn-primary" on:click={() => save(false)} disabled={!canSave}
							>{adding ? 'Saving…' : 'Save to library'}</button
						><button class="btn btn-secondary" on:click={() => save(true)} disabled={!canSave}
							>Download now</button
						>
					</div>
				{/if}
				{#if actionError}<p class="action-error" role="alert">{actionError}</p>{/if}
			</div>
		</div>
		{#if editions.length > 1 && !savedAlbum && !item?.in_library}<label class="edition-label"
				>Edition<select
					aria-label="Edition"
					value={selectedEdition}
					disabled={loadingDetail || adding}
					on:change={(e) => loadDetail(e.currentTarget.value)}
					>{#each editions as edition}<option value={edition.id}>{editionLabel(edition)}</option
						>{/each}</select
				></label
			>{/if}
		{#if loadingDetail}<div class="loading-tracks">Loading tracks…</div>{:else if detailError}<p
				class="save-hint"
			>
				{detailError} Load a complete edition before saving or downloading.
			</p>
			<button class="btn btn-secondary" on:click={() => loadDetail(selectedEdition)}
				>Retry album details</button
			>{:else if trackList.length}
			<div class="tracklist-preview">
				<h3>Tracklist · {trackList.length} tracks</h3>
				<div class="tracks">
					{#each trackList as track}<div class="track-row">
							<span class="track-num">{track.disc > 1 ? `${track.disc}.` : ''}{track.position}</span
							><span class="track-title">{track.title}</span><span class="track-dur"
								>{formatDuration(track.length || track.recording?.length)}</span
							>
						</div>{/each}
				</div>
			</div>
		{/if}
	</div>
</div>

<style>
	.profile-label,
	.edition-label {
		display: grid;
		gap: 0.4rem;
		font-size: 0.8rem;
		color: var(--text-secondary);
		margin: 1rem 0;
	}
	.profile-label select,
	.edition-label select {
		min-width: 0;
		width: 100%;
	}
	.monitor-label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.8rem;
	}
	.monitor-label input {
		accent-color: var(--accent);
	}
	.save-hint,
	.album-reason {
		font-size: 0.8rem;
		line-height: 1.6;
		color: var(--text-secondary);
		margin: 0.5rem 0;
	}
	.action-error {
		font-size: 0.8rem;
		line-height: 1.6;
		color: var(--danger);
	}
	.actions {
		flex-wrap: wrap;
		margin-top: 0.8rem;
	}

	/* ── Container ── */
	.detail {
		position: relative;
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-xl);
		box-shadow: var(--shadow-lg);
		overflow: hidden;
	}

	.detail-content {
		padding: 1.75rem;
	}

	/* ── Header layout ── */
	.detail-header {
		display: flex;
		gap: 1.75rem;
	}

	/* ── Cover art ── */
	.detail-cover {
		width: 200px;
		height: 200px;
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg), var(--shadow-glow);
		flex-shrink: 0;
		object-fit: cover;
		transition:
			transform 0.25s ease,
			box-shadow 0.25s ease;
	}

	.detail-cover:hover {
		transform: scale(1.03);
		box-shadow:
			var(--shadow-lg),
			0 0 24px var(--accent-glow);
	}

	.cover-fallback {
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		font-family: var(--font-display);
		font-size: 3.5rem;
		font-weight: 800;
		letter-spacing: -0.02em;
		color: var(--text-muted);
	}

	/* ── Info panel ── */
	.detail-info {
		flex: 1;
		min-width: 0;
	}

	h2 {
		font-family: var(--font-display);
		font-size: 1.6rem;
		font-weight: 800;
		letter-spacing: -0.02em;
		color: var(--text-primary);
		margin-bottom: 0.5rem;
		line-height: 1.2;
	}

	/* ── Meta row ── */
	.meta-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.75rem;
		flex-wrap: wrap;
	}

	.artist {
		font-family: var(--font-body);
		font-weight: 600;
		font-size: 0.95rem;
		color: var(--text-primary);
	}

	.year {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--text-secondary);
	}

	.type-tag {
		background: var(--accent-subtle);
		padding: 3px 10px;
		border-radius: var(--radius-sm);
		font-family: var(--font-display);
		font-size: 0.65rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--accent);
	}

	.lib-tag {
		background: var(--accent);
		color: var(--text-inverse);
		padding: 3px 10px;
		border-radius: var(--radius-sm);
		font-family: var(--font-display);
		font-size: 0.65rem;
		font-weight: 700;
		letter-spacing: 0.05em;
		box-shadow: 0 0 12px var(--accent-glow);
	}

	/* ── Track count ── */
	.track-count {
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--text-muted);
		margin-bottom: 0.75rem;
	}

	/* ── Actions ── */
	.actions {
		display: flex;
		gap: 0.75rem;
		align-items: center;
	}

	.add-row {
		display: flex;
		gap: 0.5rem;
		align-items: center;
	}

	select {
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		color: var(--text-primary);
		font-family: var(--font-body);
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius-md);
		font-size: 0.85rem;
		transition:
			border-color 0.2s ease,
			box-shadow 0.2s ease;
		outline: none;
	}

	select:focus {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	/* ── Buttons ── */
	.btn {
		padding: 0.55rem 1.35rem;
		border-radius: var(--radius-md);
		font-family: var(--font-display);
		font-size: 0.85rem;
		font-weight: 700;
		letter-spacing: -0.01em;
		border: none;
		display: inline-flex;
		align-items: center;
		text-decoration: none;
		cursor: pointer;
		transition:
			background 0.2s ease,
			box-shadow 0.2s ease,
			transform 0.2s ease;
		outline: none;
	}

	.btn:focus-visible {
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	.btn-primary {
		background: var(--accent);
		color: var(--text-inverse);
		box-shadow: var(--shadow-sm);
	}

	.btn-primary:hover:not(:disabled) {
		background: var(--accent-hover);
		box-shadow:
			var(--shadow-md),
			0 0 16px var(--accent-glow);
		transform: translateY(-1px);
	}

	.btn-primary:active:not(:disabled) {
		transform: translateY(0);
	}

	.btn-primary:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.btn-secondary {
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		color: var(--text-primary);
	}

	.btn-secondary:hover {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	/* ── Loading state ── */
	.loading-tracks {
		text-align: center;
		color: var(--text-muted);
		font-family: var(--font-body);
		padding: 1.25rem;
		font-size: 0.85rem;
	}

	/* ── Tracklist section ── */
	.tracklist-preview {
		margin-top: 1.5rem;
		border-top: 1px solid var(--glass-border);
		padding-top: 1.25rem;
	}

	h3 {
		font-family: var(--font-display);
		font-size: 0.85rem;
		font-weight: 700;
		letter-spacing: -0.02em;
		text-transform: uppercase;
		margin-bottom: 0.6rem;
		color: var(--text-secondary);
	}

	.tracks {
		max-height: 300px;
		overflow-y: auto;
		scrollbar-width: thin;
	}

	.track-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.45rem 0.6rem;
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--text-primary);
		border-radius: var(--radius-sm);
		transition: background 0.2s ease;
	}

	.track-row:hover {
		background: var(--accent-subtle);
	}

	.track-num {
		width: 2rem;
		text-align: right;
		color: var(--text-muted);
		font-family: var(--font-display);
		font-size: 0.75rem;
		font-weight: 600;
		flex-shrink: 0;
	}

	.track-title {
		flex: 1;
		min-width: 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.track-dur {
		color: var(--text-muted);
		font-family: var(--font-body);
		font-size: 0.75rem;
		flex-shrink: 0;
	}

	/* ── Responsive ── */
	@media (max-width: 600px) {
		.detail-header {
			flex-direction: column;
			align-items: center;
			text-align: center;
		}
		.detail-cover {
			width: 160px;
			height: 160px;
		}
		.meta-row {
			justify-content: center;
		}
		.actions {
			justify-content: center;
		}
		.add-row {
			flex-direction: column;
			width: 100%;
		}
		select {
			width: 100%;
		}
		.btn {
			width: 100%;
			justify-content: center;
		}
	}
</style>
