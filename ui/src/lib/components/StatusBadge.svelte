<script>
	export let status = '';
	export let small = false;

	const colors = {
		wanted: { bg: 'var(--status-wanted-bg)', border: 'var(--status-wanted-border)', text: 'var(--status-wanted)' },
		searching: { bg: 'var(--status-searching-bg)', border: 'var(--status-searching-border)', text: 'var(--status-searching)' },
		downloading: { bg: 'var(--status-searching-bg)', border: 'var(--status-searching-border)', text: 'var(--status-searching)' },
		available: { bg: 'var(--status-available-bg)', border: 'var(--status-available-border)', text: 'var(--status-available)' },
		unavailable: { bg: 'var(--bg-elevated)', border: 'var(--border)', text: 'var(--text-secondary)' },
		standard: { bg: 'var(--bg-elevated)', border: 'var(--border)', text: 'var(--text-secondary)' },
		special: { bg: 'var(--bg-elevated)', border: 'var(--border)', text: 'var(--text-secondary)' },
		ova: { bg: 'var(--bg-elevated)', border: 'var(--border)', text: 'var(--text-secondary)' },
		ona: { bg: 'var(--bg-elevated)', border: 'var(--border)', text: 'var(--text-secondary)' },
		recap: { bg: 'var(--bg-elevated)', border: 'var(--border)', text: 'var(--text-secondary)' },
		queued: { bg: 'var(--bg-elevated)', border: 'var(--border)', text: 'var(--text-secondary)' },
		extracting: { bg: 'var(--status-searching-bg)', border: 'var(--status-searching-border)', text: 'var(--status-searching)' },
		completed: { bg: 'var(--status-available-bg)', border: 'var(--status-available-border)', text: 'var(--status-available)' },
		failed: { bg: 'var(--danger-bg)', border: 'var(--danger-border)', text: 'var(--danger)' },
		imported: { bg: 'var(--status-available-bg)', border: 'var(--status-available-border)', text: 'var(--status-available)' }
	};

	$: style = colors[status] || colors.unavailable;
</script>

<span
	class="badge"
	class:small
	style="background: {style.bg}; border-color: {style.border}; color: {style.text}"
>
	{status}
</span>

<style>
	.badge {
		display: inline-block;
		padding: 2px 8px;
		border-radius: 4px;
		font-size: 0.7rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.5px;
		border: 1px solid;
		transition: all 0.2s ease;
	}

	.small {
		padding: 1px 5px;
		font-size: 0.6rem;
	}
</style>
