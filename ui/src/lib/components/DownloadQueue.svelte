<script>
	import { RotateCw, X, Download } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { downloads, notify } from '$lib/stores/app';
	import { queueError, queueLoaded, refreshDownloads, progress } from '$lib/stores/queue';
	import StatusBadge from './StatusBadge.svelte';
	export let compact = false;
	let pending = new Set();
	let clearing = false;
	$: failedCount = $downloads.filter((d) => d.status === 'failed').length;
	$: visible = compact ? $downloads.slice(0, 3) : $downloads;
	async function act(id, action) {
		if (pending.has(id)) return;
		pending = new Set([...pending, id]);
		try {
			await (action === 'retry' ? api.retryDownload(id) : api.cancelDownload(id));
			await refreshDownloads();
		} catch (error) {
			notify(error.message || 'Download action failed', 'error');
		} finally {
			pending.delete(id);
			pending = new Set(pending);
		}
	}
	async function clearFailed() {
		clearing = true;
		try {
			const result = await api.clearFailedDownloads();
			notify(`Cleared ${result?.cleared ?? 0} failed downloads`);
			await refreshDownloads();
		} catch (error) {
			notify(error.message || 'Could not clear downloads', 'error');
		} finally {
			clearing = false;
		}
	}
</script>

<div class="download-queue" class:compact>
	{#if $queueError}<div class="queue-error" role="alert">
			<span>Downloads unavailable. {$queueError}</span><button on:click={refreshDownloads}
				>Retry</button
			>
		</div>{/if}
	{#if failedCount > 0 && !compact}<div class="queue-toolbar">
			<button class="archive-button secondary" disabled={clearing} on:click={clearFailed}
				>Clear {failedCount} failed</button
			>
		</div>{/if}
	{#if !$queueLoaded}<p class="queue-empty">Loading downloads…</p>
	{:else if !$downloads.length && !$queueError}<div class="queue-empty">
			<Download size={24} strokeWidth={1} /><span>All quiet. Your next discovery starts it.</span><a
				href="/">Discover media <span aria-hidden="true">↗</span></a
			>
		</div>
	{:else}{#each visible as dl (dl.id)}
			<div class="download-row" class:failed={dl.status === 'failed'}>
				<div class="download-symbol"><Download size={19} /></div>
				<div class="download-body">
					<div class="download-top">
						<a
							href={dl.album_id
								? `/music/${dl.album_id}`
								: dl.media_item_id
									? `/library/${dl.media_item_id}`
									: '/activity'}>{dl.media_title || dl.nzb_title || 'Untitled download'}</a
						><span class="client" class:torrent={dl.download_type === 'torrent'}
							>{dl.download_type === 'torrent' ? 'qBittorrent' : 'SABnzbd'}</span
						>
					</div>
					{#if !compact}<p class="release-name" title={dl.nzb_title}>{dl.nzb_title}</p>{/if}
					<div class="download-progress">
						<progress
							max="100"
							value={progress(dl.percentage)}
							aria-label={`${dl.media_title || 'Download'} progress`}
						></progress><span
							>{dl.percentage != null && dl.percentage !== ''
								? `${progress(dl.percentage)}%`
								: '—'}</span
						>
					</div>
					<div class="download-bottom">
						<StatusBadge status={dl.status} small /><span
							>{dl.quality || ''}{dl.speed ? ` · ${dl.speed}/s` : ''}{dl.time_left
								? ` · ${dl.time_left} left`
								: ''}{dl.status === 'seeding' && dl.seed_ratio != null
								? ` · Ratio ${Number(dl.seed_ratio).toFixed(2)}`
								: ''}</span
						>
					</div>
				</div>
				<div class="download-actions">
					{#if dl.status === 'failed'}<button
							disabled={pending.has(dl.id)}
							aria-label={`Retry ${dl.media_title}`}
							title="Retry download"
							on:click={() => act(dl.id, 'retry')}><RotateCw size={16} /></button
						>{/if}{#if !['imported', 'completed'].includes(dl.status)}<button
							disabled={pending.has(dl.id)}
							aria-label={`Cancel ${dl.media_title}`}
							title="Cancel download"
							on:click={() => act(dl.id, 'cancel')}><X size={16} /></button
						>{/if}
				</div>
			</div>
		{/each}{/if}
	{#if compact && $downloads.length > 3}<a class="queue-more" href="/activity"
			>View all {$downloads.length} downloads <span aria-hidden="true">↗</span></a
		>{/if}
</div>

<style>
	.download-queue {
		display: grid;
		gap: 12px;
		min-width: 0;
	}
	.download-row {
		display: flex;
		gap: 15px;
		align-items: center;
		border: 1px solid var(--border);
		background: var(--bg-surface);
		padding: 20px;
		min-width: 0;
	}
	.download-symbol {
		width: 44px;
		height: 52px;
		background: var(--accent-subtle);
		border: 1px solid var(--border);
		color: var(--neon-cyan);
		display: grid;
		place-items: center;
		flex-shrink: 0;
	}
	.download-body {
		min-width: 0;
		flex: 1;
	}
	.download-top {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 12px;
	}
	.download-top a {
		font-size: 0.78rem;
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.download-top a:hover {
		color: var(--accent);
	}
	.client {
		color: var(--warning);
		border: 1px solid var(--warning-border);
		padding: 4px 6px;
		font: 0.55rem var(--font-mono);
		flex-shrink: 0;
	}
	.client.torrent {
		color: var(--neon-cyan);
		border-color: var(--info-border);
	}
	.release-name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 0.68rem;
		margin-top: 8px;
		color: var(--text-muted);
	}
	.download-progress {
		display: flex;
		gap: 10px;
		align-items: center;
		margin: 12px 0 9px;
		font: 0.6rem var(--font-mono);
		color: var(--text-secondary);
	}
	progress {
		appearance: none;
		flex: 1;
		width: 100%;
		min-width: 0;
		height: 5px;
		background: var(--border);
		border: 0;
		accent-color: var(--neon-cyan);
	}
	progress::-webkit-progress-bar {
		background: var(--border);
	}
	progress::-webkit-progress-value {
		background: var(--neon-cyan);
	}
	progress::-moz-progress-bar {
		background: var(--neon-cyan);
	}
	.download-row:nth-child(even) progress::-webkit-progress-value {
		background: var(--accent);
	}
	.failed progress::-webkit-progress-value {
		background: var(--danger);
	}
	.download-bottom {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		font-size: 0.6rem;
		color: var(--text-muted);
	}
	.download-actions {
		display: flex;
		gap: 5px;
	}
	.download-actions button {
		display: grid;
		place-items: center;
		width: 30px;
		height: 30px;
		border: 1px solid var(--border-strong);
		background: var(--bg-elevated);
		color: var(--text-secondary);
	}
	.download-actions button:hover {
		color: var(--accent);
		border-color: var(--accent);
	}
	.queue-empty {
		display: flex;
		align-items: center;
		justify-content: center;
		flex-direction: column;
		gap: 15px;
		padding: 36px 20px;
		color: var(--text-muted);
		font-size: 0.78rem;
		text-align: center;
	}
	.queue-empty a,
	.queue-more {
		color: var(--neon-cyan);
		font-size: 0.7rem;
	}
	.queue-error {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 12px;
		padding: 15px;
		color: var(--danger);
		background: var(--danger-bg);
		font-size: 0.75rem;
	}
	.queue-error button {
		padding: 8px 12px;
		background: transparent;
		border: 1px solid var(--danger);
		color: var(--danger);
	}
	.queue-toolbar {
		display: flex;
		justify-content: flex-end;
	}
	.compact {
		gap: 0;
	}
	.compact .download-row {
		background: none;
		border: 0;
		border-bottom: 1px solid var(--border);
		padding: 17px 0;
		gap: 10px;
	}
	.compact .download-symbol {
		display: none;
	}
	.compact .download-top {
		gap: 8px;
	}
	.compact .download-top a {
		font-size: 0.71rem;
	}
	.compact .download-actions {
		flex-direction: column;
	}
	.compact .download-actions button {
		width: 26px;
		height: 26px;
	}
	.queue-more {
		padding-top: 16px;
	}
	@media (max-width: 600px) {
		.download-row {
			padding: 14px;
			gap: 10px;
		}
		.download-symbol {
			display: none;
		}
		.download-top {
			flex-wrap: wrap;
			gap: 7px;
		}
		.download-actions {
			flex-direction: column;
		}
		.download-top a {
			white-space: normal;
		}
		.download-bottom {
			justify-content: flex-start;
		}
	}
</style>
