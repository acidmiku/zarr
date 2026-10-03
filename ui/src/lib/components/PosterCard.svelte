<script>
	import Artwork from './Artwork.svelte';
	import StatusBadge from './StatusBadge.svelte';
	export let title = '';
	export let year = 0;
	export let posterUrl = '';
	export let rating = 0;
	export let status = '';
	export let inLibrary = false;
	export let anime = false;
	export let href = '';
</script>

<svelte:element
	this={href ? 'a' : 'button'}
	{href}
	type={href ? undefined : 'button'}
	class="poster-card"
	on:click
	aria-label={`View ${title}`}
>
	<div class="poster">
		<Artwork src={posterUrl} {title} />{#if rating > 0}<span class="rating"
				>★ {Number(rating).toFixed(1)}</span
			>{/if}{#if anime}<span class="anime">ANIME</span>{/if}
	</div>
	<div class="poster-info">
		<strong>{title}</strong>
		<div class="poster-meta">
			<span>{year || '—'}</span>{#if inLibrary && status}<StatusBadge
					{status}
					small
				/>{:else if inLibrary}<span class="owned">In library</span>{/if}
		</div>
	</div>
</svelte:element>

<style>
	.poster-card {
		display: block;
		width: 100%;
		padding: 0;
		min-width: 0;
		background: var(--bg-surface);
		border: 1px solid var(--border);
		color: var(--text-primary);
		text-align: left;
		transition:
			border-color 0.15s,
			transform 0.15s;
	}
	.poster-card:hover {
		border-color: var(--accent);
		transform: translateY(-3px);
	}
	.poster {
		position: relative;
		aspect-ratio: 2/3;
		overflow: hidden;
	}
	.rating {
		position: absolute;
		top: 10px;
		right: 10px;
		padding: 5px 7px;
		color: #fff;
		background: #0c0a12df;
		font-size: 0.66rem;
	}
	.anime {
		position: absolute;
		top: 10px;
		left: 10px;
		padding: 5px 7px;
		color: #66ede9;
		background: #0c0a12df;
		font: 0.55rem var(--font-mono);
		letter-spacing: 0.09em;
	}
	.poster-info {
		padding: 13px;
	}
	strong {
		display: block;
		font-size: 0.8rem;
		font-weight: 600;
		line-height: 1.4;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.poster-meta {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 6px;
		margin-top: 10px;
		font-size: 0.63rem;
		color: var(--text-muted);
		flex-wrap: wrap;
	}
	.owned {
		color: var(--success);
	}
</style>
