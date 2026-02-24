<script>
	export let tool = '';

	const toolLabels = {
		search_mal: 'Searching MyAnimeList...',
		web_search: 'Searching the web...',
		show_recommendations: 'Preparing recommendations...',
		get_user_ratings: 'Checking your ratings...'
	};

	$: label = toolLabels[tool] || `Running ${tool}...`;
</script>

<div class="indicator">
	<div class="dot-pulse"></div>
	<span>{label}</span>
</div>

<style>
	.indicator {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.4rem 0.75rem;
		background: var(--bg-elevated);
		border: 1px solid var(--border-subtle);
		border-radius: 20px;
		font-size: 0.75rem;
		color: var(--text-secondary);
		margin: 0.5rem 0;
		transition: all 0.2s ease;
	}

	.dot-pulse {
		width: 6px;
		height: 6px;
		background: var(--accent);
		border-radius: 50%;
		animation: pulse 1s ease-in-out infinite;
	}

	@keyframes pulse {
		0%, 100% { opacity: 0.3; transform: scale(0.8); }
		50% { opacity: 1; transform: scale(1.2); }
	}
</style>
