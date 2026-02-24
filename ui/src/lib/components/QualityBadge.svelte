<script>
	export let quality = '';

	const qualityColors = {
		'remux-2160p': 'var(--gold)',
		'remux-1080p': 'var(--gold)',
		'bluray-2160p': 'var(--info)',
		'bluray-1080p': 'var(--info)',
		'web-2160p': 'var(--success)',
		'web-1080p': 'var(--success)',
		'web-720p': 'var(--success)',
		'hdtv-1080p': 'var(--text-secondary)',
		'hdtv-720p': 'var(--text-muted)'
	};

	$: color = qualityColors[quality] || 'var(--text-muted)';
</script>

{#if quality}
	<span class="quality-badge" style="color: {color}; border-color: {color}">
		{quality}
	</span>
{/if}

<style>
	.quality-badge {
		display: inline-block;
		padding: 1px 6px;
		border: 1px solid;
		border-radius: 3px;
		font-size: 0.7rem;
		font-weight: 600;
		font-family: monospace;
		background: var(--bg-elevated);
		transition: all 0.2s ease;
	}
</style>
