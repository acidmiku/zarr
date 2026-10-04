<script>
	import { api } from '$lib/api';
	import { Image } from 'lucide-svelte';
	export let src = '';
	export let title = '';
	export let eager = false;
	export let compact = false;
	let failed = false;
	$: {
		src;
		failed = false;
	}
	$: url = src ? (src.startsWith('/api/') ? src : api.imageUrl(src)) : '';
</script>

{#if url && !failed}
	<img src={url} alt={title} loading={eager ? 'eager' : 'lazy'} on:error={() => (failed = true)} />
{:else}
	<div class="artwork-placeholder" class:compact aria-label={title || 'No artwork'}>
		<Image size={compact ? 20 : 28} strokeWidth={1} />
		{#if !compact}<span>{title || 'Artwork unavailable'}</span>{/if}
	</div>
{/if}

<style>
	img {
		display: block;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.artwork-placeholder {
		width: 100%;
		height: 100%;
		min-height: 80px;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 16px;
		padding: 20px;
		text-align: center;
		background: var(--bg-elevated);
		color: var(--text-muted);
	}
	span {
		font-family: var(--font-display);
		text-transform: uppercase;
		font-size: 1.4rem;
		line-height: 1.1;
	}
	.artwork-placeholder.compact {
		min-height: 0;
		padding: 8px;
	}
</style>
