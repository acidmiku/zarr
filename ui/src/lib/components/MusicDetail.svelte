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
		transition: transform 0.25s ease, box-shadow 0.25s ease;
	}

	.detail-cover:hover {
		transform: scale(1.03);
		box-shadow: var(--shadow-lg), 0 0 24px var(--accent-glow);
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
		transition: border-color 0.2s ease, box-shadow 0.2s ease;
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
		transition: background 0.2s ease, box-shadow 0.2s ease, transform 0.2s ease;
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
		box-shadow: var(--shadow-md), 0 0 16px var(--accent-glow);
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
