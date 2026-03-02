<script>
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';
	import StatusBadge from './StatusBadge.svelte';

	let downloads = [];
	let interval;

	onMount(() => {
		loadDownloads();
		interval = setInterval(loadDownloads, 5000);
	});

	onDestroy(() => {
		clearInterval(interval);
	});

	async function loadDownloads() {
		try {
			downloads = await api.getDownloads();
		} catch {}
	}

	async function cancelDownload(id) {
		try {
			await api.cancelDownload(id);
			loadDownloads();
		} catch {}
	}

	async function retryDownload(id) {
		try {
			await api.retryDownload(id);
			loadDownloads();
		} catch {}
	}
</script>

<div class="queue">
	{#if downloads.length === 0}
		<div class="empty">No active downloads</div>
	{:else}
		{#each downloads as dl}
			<div class="download-item">
				<div class="dl-info">
					<div class="dl-title">
						{dl.media_title}
						<span class="dl-type-badge" class:torrent={dl.download_type === 'torrent'}>{dl.download_type === 'torrent' ? 'Torrent' : 'NZB'}</span>
					</div>
					<div class="dl-nzb">{dl.nzb_title}</div>
				</div>
				<div class="dl-progress">
					{#if dl.percentage}
						<div class="progress-bar">
							<div class="progress-fill" class:seeding={dl.status === 'seeding'} style="width: {dl.percentage}%"></div>
						</div>
						<span class="progress-text">{dl.percentage}%</span>
					{/if}
				</div>
				<div class="dl-meta">
					{#if dl.speed}<span class="dl-speed">{dl.speed}/s</span>{/if}
					{#if dl.time_left}<span class="dl-eta">{dl.time_left}</span>{/if}
					{#if dl.status === 'seeding' && dl.seed_ratio != null}
						<span class="dl-ratio">Ratio: {dl.seed_ratio.toFixed(2)}</span>
					{/if}
				</div>
				<StatusBadge status={dl.status} small />
				<div class="dl-actions">
					{#if dl.status === 'failed'}
						<button class="action-btn" on:click={() => retryDownload(dl.id)} title="Retry">↻</button>
					{/if}
					{#if dl.status !== 'imported' && dl.status !== 'completed'}
						<button class="action-btn danger" on:click={() => cancelDownload(dl.id)} title="Cancel">✕</button>
					{/if}
				</div>
			</div>
		{/each}
	{/if}
</div>

<style>
	.queue {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.empty {
		text-align: center;
		color: var(--text-muted);
		padding: 2rem;
	}

	.download-item {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 0.75rem 1rem;
		background: var(--bg-surface);
		border: 1px solid var(--border);
		border-radius: 8px;
	}

	.dl-info {
		flex: 1;
		min-width: 0;
	}

	.dl-title {
		font-weight: 500;
		font-size: 0.9rem;
		color: var(--text-primary);
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.dl-type-badge {
		background: var(--info-bg);
		color: var(--info);
		padding: 1px 5px;
		border-radius: 3px;
		font-size: 0.65rem;
		font-weight: 600;
		text-transform: uppercase;
		flex-shrink: 0;
	}

	.dl-type-badge.torrent {
		background: var(--success-bg);
		color: var(--success);
	}

	.dl-nzb {
		font-size: 0.75rem;
		color: var(--text-muted);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.dl-progress {
		width: 120px;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.progress-bar {
		flex: 1;
		height: 4px;
		background: var(--bg-elevated);
		border-radius: 2px;
		overflow: hidden;
	}

	.progress-fill {
		height: 100%;
		background: var(--accent);
		border-radius: 2px;
		transition: width 0.3s;
	}

	.progress-fill.seeding {
		background: var(--success);
	}

	.progress-text {
		font-size: 0.75rem;
		color: var(--text-secondary);
		min-width: 32px;
	}

	.dl-meta {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		font-size: 0.75rem;
		color: var(--text-secondary);
		gap: 2px;
	}

	.dl-speed {
		color: var(--accent);
	}

	.dl-ratio {
		color: var(--success);
		font-weight: 500;
	}

	.dl-actions {
		display: flex;
		gap: 0.25rem;
	}

	.action-btn {
		background: var(--bg-elevated);
		border: 1px solid var(--border-subtle);
		color: var(--text-secondary);
		width: 28px;
		height: 28px;
		border-radius: 4px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.85rem;
		cursor: pointer;
	}

	.action-btn:hover { background: var(--bg-hover); color: var(--text-primary); }
	.action-btn.danger:hover { background: var(--danger-bg); color: var(--danger); border-color: var(--danger-border); }
</style>
