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
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		border-radius: 9999px;
		font-family: var(--font-body);
		font-size: 0.75rem;
		color: var(--text-secondary);
		margin: 0.5rem 0;
		box-shadow: var(--shadow-sm);
		transition: all 0.2s ease;
	}

	.dot-pulse {
		width: 6px;
		height: 6px;
		background: var(--accent);
		border-radius: 50%;
		box-shadow: 0 0 6px var(--accent-glow);
		animation: pulse 1.2s ease-in-out infinite;
	}

	@keyframes pulse {
		0%, 100% {
			opacity: 0.3;
			transform: scale(0.8);
			box-shadow: 0 0 4px var(--accent-glow);
		}
		50% {
			opacity: 1;
			transform: scale(1.2);
			box-shadow: 0 0 12px var(--accent-glow);
		}
	}
</style>
