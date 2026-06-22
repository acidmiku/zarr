<script>
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';
	import StatusBadge from './StatusBadge.svelte';

	let downloads = [];
	let interval;
	let clearing = false;

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

	async function clearFailed() {
		clearing = true;
		try {
			const res = await api.clearFailedDownloads();
			notify(`Cleared ${res?.cleared ?? 0} failed`, 'success');
			loadDownloads();
		} catch {
			notify('Failed to clear', 'error');
		} finally {
			clearing = false;
		}
	}

	$: failedCount = downloads.filter(d => d.status === 'failed').length;
</script>

<div class="queue">
	{#if failedCount > 0}
		<div class="queue-toolbar">
			<button class="clear-btn" disabled={clearing} on:click={clearFailed}>
				Clear {failedCount} failed
			</button>
		</div>
	{/if}

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
		font-family: var(--font-body);
		padding: 2rem;
	}

	.queue-toolbar {
		display: flex;
		justify-content: flex-end;
		margin-bottom: 0.25rem;
	}

	.clear-btn {
		background: transparent;
		border: 1px solid var(--danger-border);
		color: var(--danger);
		padding: 0.35rem 0.85rem;
		border-radius: var(--radius-sm);
		font-family: var(--font-body);
		font-size: 0.78rem;
		font-weight: 600;
		letter-spacing: -0.005em;
		cursor: pointer;
		transition: background 0.15s ease, color 0.15s ease;
	}

	.clear-btn:hover:not(:disabled) {
		background: var(--danger-bg);
	}

	.clear-btn:disabled {
		opacity: 0.5;
		cursor: default;
	}

	.download-item {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 0.75rem 1rem;
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-sm);
		transition: background 0.2s ease, border-color 0.2s ease, box-shadow 0.25s ease;
	}

	.download-item:hover {
		background: var(--accent-subtle);
		border-color: var(--accent);
		box-shadow: var(--shadow-md);
	}

	.dl-info {
		flex: 1;
		min-width: 0;
	}

	.dl-title {
		font-family: var(--font-display);
		font-weight: 700;
		font-size: 0.9rem;
		letter-spacing: -0.02em;
		color: var(--text-primary);
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.dl-type-badge {
		background: var(--accent-subtle);
		color: var(--accent);
		padding: 1px 6px;
		border-radius: var(--radius-sm);
		font-family: var(--font-display);
		font-size: 0.65rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.02em;
		flex-shrink: 0;
	}

	.dl-type-badge.torrent {
		background: var(--status-available);
		color: var(--text-inverse);
	}

	.dl-nzb {
		font-family: var(--font-body);
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
		background: var(--glass-border);
		border-radius: var(--radius-sm);
		overflow: hidden;
	}

	.progress-fill {
		height: 100%;
		background: var(--accent);
		border-radius: var(--radius-sm);
		box-shadow: 0 0 8px var(--accent-glow);
		transition: width 0.25s ease;
	}

	.progress-fill.seeding {
		background: var(--status-available);
		box-shadow: 0 0 8px var(--status-available);
	}

	.progress-text {
		font-family: var(--font-body);
		font-size: 0.75rem;
		color: var(--text-secondary);
		min-width: 32px;
	}

	.dl-meta {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		font-family: var(--font-body);
		font-size: 0.75rem;
		color: var(--text-secondary);
		gap: 2px;
	}

	.dl-speed {
		color: var(--accent);
		font-weight: 600;
	}

	.dl-ratio {
		color: var(--status-available);
		font-weight: 600;
	}

	.dl-actions {
		display: flex;
		gap: 0.25rem;
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
