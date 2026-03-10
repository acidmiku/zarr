<script>
	import StatusBadge from './StatusBadge.svelte';

	export let tracks = [];

	$: maxDisc = Math.max(...tracks.map(t => t.disc_number || 1), 1);
	$: isMultiDisc = maxDisc > 1;

	$: groupedTracks = (() => {
		if (!isMultiDisc) return [{ disc: 1, tracks }];
		const groups = {};
		for (const t of tracks) {
			const d = t.disc_number || 1;
			if (!groups[d]) groups[d] = [];
			groups[d].push(t);
		}
		return Object.entries(groups)
			.sort(([a], [b]) => parseInt(a) - parseInt(b))
			.map(([disc, tracks]) => ({ disc: parseInt(disc), tracks }));
	})();

	function formatDuration(ms) {
		if (!ms) return '';
		const s = Math.floor(ms / 1000);
		const m = Math.floor(s / 60);
		const sec = s % 60;
		return `${m}:${String(sec).padStart(2, '0')}`;
	}
</script>

<div class="track-list">
	{#each groupedTracks as group}
		{#if isMultiDisc}
			<div class="disc-header">Disc {group.disc}</div>
		{/if}
		{#each group.tracks as track}
			<div class="track" class:available={track.status === 'available'}>
				<span class="track-num">{track.number}</span>
				<div class="track-info">
					<span class="track-title">{track.title}</span>
				</div>
				<span class="track-duration">{formatDuration(track.duration_ms)}</span>
				<div class="track-status">
					<StatusBadge status={track.status} small />
				</div>
			</div>
		{/each}
	{/each}
</div>

<style>
	.track-list {
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
	}

	.disc-header {
		font-family: var(--font-display);
		font-size: 0.8rem;
		font-weight: 700;
		color: var(--text-secondary);
		padding: 0.75rem 0.75rem 0.35rem;
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.track {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius-md);
		font-family: var(--font-body);
		font-size: 0.85rem;
		background: transparent;
		border: 1px solid transparent;
		transition: background 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
	}

	.track:hover {
		background: var(--glass-bg);
		border-color: var(--glass-border);
		box-shadow: var(--shadow-sm);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
	}

	.track.available {
		opacity: 1;
	}

	.track-num {
		width: 2rem;
		text-align: right;
		color: var(--accent);
		font-family: var(--font-display);
		font-size: 0.8rem;
		font-weight: 600;
		flex-shrink: 0;
	}

	.track-info {
		flex: 1;
		min-width: 0;
	}

	.track-title {
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		display: block;
		color: var(--text-primary);
	}

	.track-duration {
		color: var(--text-muted);
		font-family: var(--font-body);
		font-size: 0.75rem;
		flex-shrink: 0;
		min-width: 3rem;
		text-align: right;
	}

	.track-status {
		flex-shrink: 0;
	}
</style>
