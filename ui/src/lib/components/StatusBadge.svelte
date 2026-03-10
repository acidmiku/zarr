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
		imported: { bg: 'var(--status-available-bg)', border: 'var(--status-available-border)', text: 'var(--status-available)' },
		seeding: { bg: 'var(--success-bg)', border: 'var(--success)', text: 'var(--success)' }
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
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		padding: 0.2rem 0.65rem;
		border-radius: 9999px;
		font-family: var(--font-display);
		font-size: 0.7rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.5px;
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		color: var(--text-primary);
		box-shadow: var(--shadow-sm);
		transition: all 0.2s ease;
		position: relative;
		padding-left: 1.25rem;
	}

	.badge::before {
		content: '';
		position: absolute;
		left: 0.5rem;
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: currentColor;
		box-shadow: 0 0 6px currentColor;
	}

	.small {
		padding: 0.1rem 0.5rem 0.1rem 1.1rem;
		font-size: 0.6rem;
	}

	.small::before {
		width: 5px;
		height: 5px;
		left: 0.4rem;
	}
</style>
