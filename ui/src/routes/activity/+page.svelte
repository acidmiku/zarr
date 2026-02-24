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
	.page-header { margin-bottom: 1rem; }
	h1 { font-size: 1.5rem; font-weight: 700; }

	.tabs {
		display: flex;
		gap: 0.25rem;
		background: var(--bg-surface);
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
		border-radius: 8px;
		padding: 3px;
		margin-bottom: 1.25rem;
		width: fit-content;
	}

	.tabs button {
		padding: 0.4rem 1.25rem;
		border: none;
		background: transparent;
		color: var(--text-secondary);
		border-radius: 6px;
		font-size: 0.85rem;
		font-weight: 500;
		cursor: pointer;
	}

	.tabs button.active {
		background: var(--bg-active);
		color: var(--text-primary);
	}

	.history {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.activity-item {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.65rem 1rem;
		background: var(--bg-surface);
		border-radius: 6px;
		border: 1px solid var(--border);
	}

	.act-icon {
		width: 28px;
		height: 28px;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.85rem;
		font-weight: 700;
		background: var(--bg-elevated);
		color: var(--text-secondary);
		flex-shrink: 0;
	}

	.act-success { background: var(--success-bg); color: var(--success); }
	.act-warning { background: var(--info-bg); color: var(--info); }
	.act-danger { background: var(--danger-bg); color: var(--danger); }

	.act-info { flex: 1; min-width: 0; }

	.act-detail {
		display: flex;
		gap: 0.5rem;
		font-size: 0.875rem;
	}

	.act-action {
		text-transform: capitalize;
		font-weight: 500;
	}

	.act-media {
		color: var(--accent);
	}

	.act-details {
		font-size: 0.75rem;
		color: var(--text-muted);
		margin-top: 2px;
	}

	.act-time {
		font-size: 0.75rem;
		color: var(--text-muted);
		flex-shrink: 0;
	}

	.empty {
		text-align: center;
		color: var(--text-muted);
		padding: 3rem;
	}

	.pagination {
		display: flex;
		justify-content: center;
		align-items: center;
		gap: 1rem;
		margin-top: 1.5rem;
	}

	.pagination button {
		padding: 0.4rem 1rem;
		background: var(--bg-elevated);
		border: 1px solid var(--border-subtle);
		color: var(--text-primary);
		border-radius: 6px;
		cursor: pointer;
	}

	.pagination button:disabled { opacity: 0.3; cursor: default; }
	.pagination span { color: var(--text-secondary); font-size: 0.85rem; }
</style>
