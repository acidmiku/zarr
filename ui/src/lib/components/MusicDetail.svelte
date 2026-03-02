<script>
	import { createEventDispatcher, onMount } from 'svelte';
	import { api } from '$lib/api';

	export let item = null;
	export let profiles = [];

	const dispatch = createEventDispatcher();

	let selectedProfile = (() => {
		const match = profiles.find(p => p.profile_type === 'music');
		return match?.id || profiles[0]?.id || 1;
	})();
	let adding = false;
	let albumDetail = null;
	let loadingDetail = false;

	onMount(async () => {
		// If we have a MusicBrainz ID, fetch track details
		const rgid = item?.id || item?.mbid;
		if (rgid) {
			loadingDetail = true;
			try {
				albumDetail = await api.musicAlbum(rgid);
			} catch {}
			loadingDetail = false;
		}
	});

	$: coverUrl = item?.cover_url || item?.image_url
		? api.imageUrl(item.cover_url || item.image_url)
		: '';

	$: artistName = item?.artist || item?.artist_name || '';
	$: albumTitle = item?.title || item?.name || '';
	$: year = item?.year ? (typeof item.year === 'string' ? item.year.slice(0, 4) : item.year) : '';

	$: trackList = albumDetail?.release?.media?.flatMap(m =>
		(m.tracks || []).map(t => ({
			...t,
			disc: m.position
		}))
	) || [];

	async function addToLibrary() {
		adding = true;
		try {
			const rgid = item?.id || item?.mbid;
			const payload = {
				release_group_id: rgid,
				artist_mbid: item?.artist_id || item?.artist_mbid || '',
				artist_name: artistName,
				album_title: albumTitle,
				year: parseInt(year) || 0,
				album_type: item?.type || 'Album',
				quality_profile_id: selectedProfile,
				image_url: item?.cover_url || item?.image_url || ''
			};

			await api.addMusicToLibrary(payload);
			dispatch('added');
		} catch (e) {
			dispatch('error', e.message);
		}
		adding = false;
	}

	function formatDuration(ms) {
		if (!ms) return '';
		const s = Math.floor(ms / 1000);
		const m = Math.floor(s / 60);
		const sec = s % 60;
		return `${m}:${String(sec).padStart(2, '0')}`;
	}
</script>

<div class="detail" on:click|stopPropagation on:keydown|stopPropagation role="dialog">
	<div class="detail-content">
		<div class="detail-header">
			{#if coverUrl}
				<img class="detail-cover" src={coverUrl} alt={albumTitle}
					on:error={(e) => e.target.style.display = 'none'} />
			{:else}
				<div class="detail-cover cover-fallback">{(albumTitle || '?')[0]}</div>
			{/if}
			<div class="detail-info">
				<h2>{albumTitle}</h2>
				<div class="meta-row">
					{#if artistName}<span class="artist">{artistName}</span>{/if}
					{#if year}<span class="year">{year}</span>{/if}
					{#if item?.type}<span class="type-tag">{item.type}</span>{/if}
					{#if item?.in_library}<span class="lib-tag">IN LIBRARY</span>{/if}
				</div>

				{#if trackList.length > 0}
					<div class="track-count">{trackList.length} tracks</div>
				{/if}

				<div class="actions">
					{#if !item?.in_library}
						<div class="add-row">
							<select bind:value={selectedProfile}>
								{#each profiles.filter(p => p.profile_type === 'music') as p}
									<option value={p.id}>{p.name}</option>
								{/each}
							</select>
							<button class="btn btn-primary" on:click={addToLibrary} disabled={adding}>
								{adding ? 'Adding...' : 'Add to Library'}
							</button>
						</div>
					{:else}
						<a href="/music/{item.library_id}" class="btn btn-secondary">View in Library</a>
					{/if}
				</div>
			</div>
		</div>

		{#if loadingDetail}
			<div class="loading-tracks">Loading tracks...</div>
		{:else if trackList.length > 0}
			<div class="tracklist-preview">
				<h3>Tracklist</h3>
				<div class="tracks">
					{#each trackList as track}
						<div class="track-row">
							<span class="track-num">{track.position}</span>
							<span class="track-title">{track.title}</span>
							<span class="track-dur">{formatDuration(track.length || track.recording?.length)}</span>
						</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>
</div>

<style>
	.detail {
		position: relative;
		background: var(--bg-surface);
		border-radius: 12px;
		overflow: hidden;
	}

	.detail-content {
		padding: 1.5rem;
	}

	.detail-header {
		display: flex;
		gap: 1.5rem;
	}

	.detail-cover {
		width: 180px;
		height: 180px;
		border-radius: 8px;
		box-shadow: var(--shadow-md);
		flex-shrink: 0;
		object-fit: cover;
	}

	.cover-fallback {
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--bg-elevated);
		font-size: 3rem;
		font-weight: 700;
		color: var(--text-muted);
	}

	.detail-info { flex: 1; min-width: 0; }

	h2 {
		font-size: 1.5rem;
		font-weight: 700;
		margin-bottom: 0.5rem;
	}

	.meta-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.75rem;
		flex-wrap: wrap;
	}

	.artist { font-weight: 600; font-size: 0.9rem; }
	.year { font-size: 0.85rem; color: var(--text-secondary); }

	.type-tag {
		background: var(--bg-elevated);
		padding: 2px 8px;
		border-radius: 4px;
		font-size: 0.7rem;
		text-transform: uppercase;
		color: var(--text-secondary);
	}

	.lib-tag {
		background: var(--accent);
		color: var(--text-inverse);
		padding: 2px 8px;
		border-radius: 4px;
		font-size: 0.65rem;
		font-weight: 700;
	}

	.track-count {
		font-size: 0.8rem;
		color: var(--text-muted);
		margin-bottom: 0.75rem;
	}

	.actions { display: flex; gap: 0.75rem; align-items: center; }

	.add-row { display: flex; gap: 0.5rem; align-items: center; }

	select {
		background: var(--bg-input);
		border: 1px solid var(--border-subtle);
		color: var(--text-primary);
		padding: 0.5rem 0.75rem;
		border-radius: 6px;
		font-size: 0.85rem;
	}

	.btn {
		padding: 0.5rem 1.25rem;
		border-radius: 6px;
		font-size: 0.85rem;
		font-weight: 600;
		border: none;
		display: inline-flex;
		align-items: center;
		text-decoration: none;
		cursor: pointer;
	}

	.btn-primary { background: var(--accent); color: var(--text-inverse); }
	.btn-primary:hover:not(:disabled) { background: var(--accent-hover); }
	.btn-primary:disabled { opacity: 0.5; }
	.btn-secondary { background: var(--bg-elevated); color: var(--text-primary); }

	.loading-tracks {
		text-align: center;
		color: var(--text-muted);
		padding: 1rem;
		font-size: 0.85rem;
	}

	.tracklist-preview {
		margin-top: 1.25rem;
		border-top: 1px solid var(--border);
		padding-top: 1rem;
	}

	h3 {
		font-size: 0.9rem;
		font-weight: 600;
		margin-bottom: 0.5rem;
		color: var(--text-secondary);
	}

	.tracks {
		max-height: 300px;
		overflow-y: auto;
	}

	.track-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.35rem 0.5rem;
		font-size: 0.8rem;
		border-radius: 4px;
	}

	.track-row:hover {
		background: var(--bg-hover);
	}

	.track-num {
		width: 2rem;
		text-align: right;
		color: var(--text-muted);
		font-size: 0.75rem;
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
		font-size: 0.75rem;
		flex-shrink: 0;
	}

	@media (max-width: 600px) {
		.detail-header {
			flex-direction: column;
			align-items: center;
			text-align: center;
		}
		.detail-cover { width: 150px; height: 150px; }
		.meta-row { justify-content: center; }
	}
</style>
