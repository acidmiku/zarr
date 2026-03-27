<script>
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';
	import StatusBadge from './StatusBadge.svelte';

	export let mediaId;
	export let seasons = [];
	export let onViewReleases = null;

	let expandedSeasons = new Set();

	function toggleSeason(num) {
		if (expandedSeasons.has(num)) {
			expandedSeasons.delete(num);
		} else {
			expandedSeasons.add(num);
		}
		expandedSeasons = expandedSeasons;
	}

	async function searchEpisode(epId) {
		try {
			await api.searchEpisode(mediaId, epId);
			notify('Search triggered', 'success');
		} catch (e) {
			notify(e.message, 'error');
		}
	}

	async function resetEpisode(epId) {
		try {
			await api.resetEpisode(mediaId, epId);
			// Update local state
			for (const season of seasons) {
				const ep = season.episodes?.find(e => e.id === epId);
				if (ep) {
					ep.status = 'wanted';
					seasons = seasons;
					break;
				}
			}
			notify('Episode reset to wanted', 'success');
		} catch (e) {
			notify(e.message, 'error');
		}
	}

	async function deleteFile(epId) {
		if (!confirm('Delete this episode file?')) return;
		try {
			await api.deleteEpisodeFile(mediaId, epId);
			// Update local state
			for (const season of seasons) {
				const ep = season.episodes?.find(e => e.id === epId);
				if (ep) {
					ep.file_path = '';
					ep.status = 'wanted';
					seasons = seasons;
					break;
				}
			}
			notify('File deleted', 'success');
		} catch (e) {
			notify(e.message, 'error');
		}
	}

	function formatAirDate(dateStr) {
		if (!dateStr) return '';
		const d = new Date(dateStr);
		if (isNaN(d.getTime())) return dateStr; // fallback to original if invalid
		return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
	}
</script>

<div class="episode-list">
	{#each seasons as season}
		<div class="season">
			<button class="season-header" on:click={() => toggleSeason(season.number)}>
				<span class="expand-icon">{expandedSeasons.has(season.number) ? '▾' : '▸'}</span>
				<span class="season-title">{season.title || `Season ${season.number}`}</span>
				<span class="ep-count">{season.episodes?.length || 0} episodes</span>
				<span class="season-stats">
					{season.episodes?.filter(e => e.status === 'available').length || 0} / {season.episodes?.length || 0}
				</span>
			</button>

			{#if expandedSeasons.has(season.number)}
				<div class="episodes">
					{#each season.episodes || [] as ep}
						<div class="episode" class:special={ep.episode_type !== 'standard'}>
							<div class="ep-num">
								{#if ep.absolute_number}
									<span class="abs-num">#{ep.absolute_number}</span>
								{/if}
								E{String(ep.number).padStart(2, '0')}
							</div>
							<div class="ep-info">
								<div class="ep-title">
									{ep.title || `Episode ${ep.number}`}
									{#if ep.episode_type !== 'standard'}
										<StatusBadge status={ep.episode_type} small />
									{/if}
								</div>
								{#if ep.air_date}
									<div class="ep-date">{formatAirDate(ep.air_date)}</div>
								{/if}
							</div>
							<div class="ep-status">
								<StatusBadge status={ep.status} small />
							</div>
							<div class="ep-actions">
								{#if onViewReleases}
									<button class="action-btn" title="Browse releases" on:click={() => onViewReleases(ep.id, `S${String(season.number).padStart(2,'0')}E${String(ep.number).padStart(2,'0')}`)}>☰</button>
								{/if}
								{#if ep.status === 'wanted' || ep.status === 'searching'}
									<button class="action-btn" title="Auto search" on:click={() => searchEpisode(ep.id)}>⌕</button>
								{/if}
								{#if ep.status === 'downloading'}
									<button class="action-btn" title="Search again" on:click={() => searchEpisode(ep.id)}>⌕</button>
									<button class="action-btn danger" title="Cancel and reset" on:click={() => resetEpisode(ep.id)}>↺</button>
								{/if}
								{#if ep.file_path}
									<button class="action-btn danger" title="Delete file" on:click={() => deleteFile(ep.id)}>✕</button>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/each}
</div>

<style>
	.episode-list {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}

	.season-header {
		width: 100%;
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.75rem 1rem;
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-family: var(--font-display);
		font-size: 0.9rem;
		font-weight: 700;
		letter-spacing: -0.02em;
		cursor: pointer;
		text-align: left;
		transition: background 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
		box-shadow: var(--shadow-sm);
	}

	.season-header:hover {
		background: var(--accent-subtle);
		border-color: var(--accent);
		box-shadow: var(--shadow-md);
	}

	.expand-icon {
		color: var(--text-muted);
		width: 1rem;
		transition: color 0.2s ease;
	}

	.season-header:hover .expand-icon {
		color: var(--accent);
	}

	.season-title {
		font-weight: 700;
		flex: 1;
	}

	.ep-count, .season-stats {
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-weight: 400;
		color: var(--text-muted);
	}

	.episodes {
		margin: 0.375rem 0 0.5rem 1rem;
		border-left: 2px solid var(--glass-border);
		padding-left: 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.episode {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.5rem 0.75rem;
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		border: 1px solid transparent;
		border-radius: var(--radius-sm);
		font-family: var(--font-body);
		font-size: 0.85rem;
		transition: background 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
	}

	.episode:hover {
		background: var(--accent-subtle);
		border-color: var(--glass-border);
		box-shadow: var(--shadow-sm);
	}

	.episode.special {
		opacity: 0.8;
	}

	.ep-num {
		width: 3.5rem;
		color: var(--text-muted);
		font-family: var(--font-body);
		font-size: 0.8rem;
		flex-shrink: 0;
		text-align: right;
	}

	.abs-num {
		color: var(--accent);
		margin-right: 4px;
		font-weight: 600;
	}

	.ep-info {
		flex: 1;
		min-width: 0;
	}

	.ep-title {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		color: var(--text-primary);
		font-family: var(--font-body);
	}

	.ep-date {
		font-size: 0.75rem;
		color: var(--text-muted);
		font-family: var(--font-body);
	}

	.ep-status {
		flex-shrink: 0;
	}

	.ep-actions {
		display: flex;
		gap: 0.25rem;
		flex-shrink: 0;
	}

	.action-btn {
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		color: var(--text-secondary);
		width: 28px;
		height: 28px;
		border-radius: var(--radius-sm);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.85rem;
		cursor: pointer;
		transition: background 0.2s ease, color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
	}

	.action-btn:hover {
		background: var(--accent);
		color: var(--text-inverse);
		border-color: var(--accent);
		box-shadow: var(--shadow-sm);
	}

	.action-btn.danger:hover {
		background: var(--danger-bg);
		color: var(--danger);
		border-color: var(--danger-border);
	}
</style>
