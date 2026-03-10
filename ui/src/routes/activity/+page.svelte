<script>
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import DownloadQueue from '$lib/components/DownloadQueue.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';

	let activeTab = 'queue';
	let activities = [];
	let totalActivities = 0;
	let actPage = 1;

	onMount(() => {
		loadActivity();
	});

	async function loadActivity() {
		try {
			const data = await api.getActivity(actPage, 50);
			activities = data.items || [];
			totalActivities = data.total || 0;
		} catch {}
	}

	const actionIcons = {
		added: '+',
		searched: '⌕',
		grabbed: '↓',
		downloaded: '✓',
		imported: '✓',
		deleted: '✕',
		upgraded: '↑',
		failed: '!'
	};
</script>

<svelte:head>
	<title>Activity - Zarr</title>
</svelte:head>

<div class="page">
	<header class="page-header">
		<h1>Activity</h1>
	</header>

	<div class="tabs">
		<button class:active={activeTab === 'queue'} on:click={() => activeTab = 'queue'}>Queue</button>
		<button class:active={activeTab === 'history'} on:click={() => activeTab = 'history'}>History</button>
	</div>

	{#if activeTab === 'queue'}
		<DownloadQueue />
	{:else}
		<div class="history">
			{#if activities.length === 0}
				<div class="empty">No activity yet</div>
			{:else}
				{#each activities as act}
					<div class="activity-item">
						<div class="act-icon" class:act-success={['imported', 'downloaded', 'added'].includes(act.action)}
							class:act-warning={act.action === 'grabbed' || act.action === 'searched'}
							class:act-danger={act.action === 'failed' || act.action === 'deleted'}>
							{actionIcons[act.action] || '•'}
						</div>
						<div class="act-info">
							<div class="act-detail">
								<span class="act-action">{act.action}</span>
								{#if act.media_title}
									<span class="act-media">{act.media_title}</span>
								{/if}
								{#if act.download_type}
									<span class="act-type-badge" class:torrent={act.download_type === 'torrent'}>{act.download_type === 'torrent' ? 'Torrent' : 'NZB'}</span>
								{/if}
							</div>
							{#if act.details}
								<div class="act-details">{act.details}</div>
							{/if}
						</div>
						<div class="act-time">{formatTime(act.created_at)}</div>
					</div>
				{/each}
			{/if}

			{#if totalActivities > 50}
				<div class="pagination">
					<button disabled={actPage <= 1} on:click={() => { actPage--; loadActivity(); }}>Prev</button>
					<span>Page {actPage}</span>
					<button disabled={actPage * 50 >= totalActivities} on:click={() => { actPage++; loadActivity(); }}>Next</button>
				</div>
			{/if}
		</div>
	{/if}
</div>

<script context="module">
	function formatTime(dateStr) {
		if (!dateStr) return '';
		const d = new Date(dateStr);
		const now = new Date();
		const diff = now.getTime() - d.getTime();
		const mins = Math.floor(diff / 60000);
		if (mins < 1) return 'just now';
		if (mins < 60) return `${mins}m ago`;
		const hours = Math.floor(mins / 60);
		if (hours < 24) return `${hours}h ago`;
		const days = Math.floor(hours / 24);
		if (days < 7) return `${days}d ago`;
		return d.toLocaleDateString();
	}
</script>

<style>
	/* --- Stagger animation for list items --- */
	@keyframes stagger-in {
		from {
			opacity: 0;
			transform: translateY(8px);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}

	/* --- Page header --- */
	.page-header {
		margin-bottom: 1.5rem;
	}

	h1 {
		font-family: var(--font-display), sans-serif;
		font-size: 1.75rem;
		font-weight: 800;
		letter-spacing: -0.02em;
		color: var(--text-primary);
	}

	/* --- Tab selector: glass background with accent active state --- */
	.tabs {
		display: flex;
		gap: 0.25rem;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border-radius: var(--radius-md, 10px);
		padding: 4px;
		margin-bottom: 1.5rem;
		width: fit-content;
	}

	.tabs button {
		padding: 0.45rem 1.35rem;
		border: none;
		background: transparent;
		color: var(--text-secondary);
		border-radius: calc(var(--radius-md, 10px) - 2px);
		font-family: var(--font-display), sans-serif;
		font-size: 0.85rem;
		font-weight: 600;
		letter-spacing: -0.01em;
		cursor: pointer;
		transition: all 0.2s ease;
	}

	.tabs button:hover:not(.active) {
		color: var(--text-primary);
		background: rgba(255, 255, 255, 0.04);
	}

	.tabs button.active {
		background: var(--accent);
		color: var(--accent-contrast, #fff);
		box-shadow: 0 1px 4px rgba(var(--accent-rgb, 99, 102, 241), 0.3);
	}

	/* --- History list --- */
	.history {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	/* --- Activity items: glass panels with timeline accent border --- */
	.activity-item {
		display: flex;
		align-items: center;
		gap: 0.85rem;
		padding: 0.75rem 1rem;
		background: var(--glass-bg);
		border-radius: var(--radius-md, 10px);
		border: 1px solid var(--glass-border);
		border-left: 3px solid var(--border-subtle, rgba(255, 255, 255, 0.08));
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
		transition: all 0.2s ease;
		animation: stagger-in 0.35s ease both;
	}

	/* Stagger delay for each item */
	.activity-item:nth-child(1) { animation-delay: 0.00s; }
	.activity-item:nth-child(2) { animation-delay: 0.04s; }
	.activity-item:nth-child(3) { animation-delay: 0.08s; }
	.activity-item:nth-child(4) { animation-delay: 0.12s; }
	.activity-item:nth-child(5) { animation-delay: 0.16s; }
	.activity-item:nth-child(6) { animation-delay: 0.20s; }
	.activity-item:nth-child(7) { animation-delay: 0.24s; }
	.activity-item:nth-child(8) { animation-delay: 0.28s; }
	.activity-item:nth-child(9) { animation-delay: 0.32s; }
	.activity-item:nth-child(10) { animation-delay: 0.36s; }
	.activity-item:nth-child(n+11) { animation-delay: 0.40s; }

	.activity-item:hover {
		background: rgba(255, 255, 255, 0.06);
		border-left-color: var(--accent);
		transform: translateX(2px);
	}

	/* --- Status icons: semantic colors --- */
	.act-icon {
		width: 30px;
		height: 30px;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.85rem;
		font-weight: 700;
		background: var(--bg-elevated);
		color: var(--text-secondary);
		flex-shrink: 0;
		transition: transform 0.2s ease;
	}

	.activity-item:hover .act-icon {
		transform: scale(1.08);
	}

	.act-success {
		background: var(--success-bg);
		color: var(--success);
		box-shadow: 0 0 8px rgba(var(--success-rgb, 34, 197, 94), 0.15);
	}

	.act-warning {
		background: var(--info-bg);
		color: var(--info);
		box-shadow: 0 0 8px rgba(var(--info-rgb, 59, 130, 246), 0.15);
	}

	.act-danger {
		background: var(--danger-bg);
		color: var(--danger);
		box-shadow: 0 0 8px rgba(var(--danger-rgb, 239, 68, 68), 0.15);
	}

	/* --- Activity info --- */
	.act-info {
		flex: 1;
		min-width: 0;
	}

	.act-detail {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		flex-wrap: wrap;
	}

	.act-action {
		text-transform: capitalize;
		font-family: var(--font-display), sans-serif;
		font-weight: 700;
		letter-spacing: -0.01em;
	}

	.act-media {
		color: var(--accent);
		font-weight: 500;
	}

	/* --- Type badge: glass pill style --- */
	.act-type-badge {
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		color: var(--info);
		padding: 2px 8px;
		border-radius: 999px;
		font-size: 0.6rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		backdrop-filter: blur(8px);
		-webkit-backdrop-filter: blur(8px);
	}

	.act-type-badge.torrent {
		background: rgba(var(--success-rgb, 34, 197, 94), 0.1);
		border-color: rgba(var(--success-rgb, 34, 197, 94), 0.2);
		color: var(--success);
	}

	.act-details {
		font-size: 0.75rem;
		color: var(--text-muted);
		margin-top: 3px;
		line-height: 1.4;
	}

	.act-time {
		font-size: 0.7rem;
		color: var(--text-muted);
		flex-shrink: 0;
		font-variant-numeric: tabular-nums;
		opacity: 0.7;
		transition: opacity 0.2s ease;
	}

	.activity-item:hover .act-time {
		opacity: 1;
	}

	/* --- Empty state: centered icon + text --- */
	.empty {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		text-align: center;
		color: var(--text-muted);
		padding: 4rem 2rem;
		font-size: 0.95rem;
		gap: 0.5rem;
	}

	.empty::before {
		content: '○';
		font-size: 2.5rem;
		opacity: 0.25;
		display: block;
		margin-bottom: 0.25rem;
	}

	/* --- Pagination: glass pill buttons --- */
	.pagination {
		display: flex;
		justify-content: center;
		align-items: center;
		gap: 1rem;
		margin-top: 2rem;
	}

	.pagination button {
		padding: 0.45rem 1.15rem;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		color: var(--text-primary);
		border-radius: 999px;
		cursor: pointer;
		font-size: 0.8rem;
		font-weight: 600;
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
		transition: all 0.2s ease;
	}

	.pagination button:hover:not(:disabled) {
		background: var(--accent);
		color: var(--accent-contrast, #fff);
		border-color: var(--accent);
	}

	.pagination button:disabled {
		opacity: 0.25;
		cursor: default;
	}

	.pagination span {
		color: var(--text-secondary);
		font-size: 0.8rem;
		font-variant-numeric: tabular-nums;
	}
</style>
