<script>
	import { createEventDispatcher } from 'svelte';
	import { ArrowUpRight, Plus } from 'lucide-svelte';
	import Artwork from './Artwork.svelte';
	import { mediaTypeLabel } from '$lib/media';
	import StatusBadge from './StatusBadge.svelte';
	export let items = [];
	export let featured = null;
	export let discovery = false;
	const dispatch = createEventDispatcher();
	$: hero = featured || items[0];
	$: supporting = items.slice(1, 3);
	function select(item) {
		dispatch('select', item);
	}
</script>

{#if hero}
	<section
		class="showcase"
		aria-label={discovery ? 'Featured discoveries' : 'Collection highlights'}
	>
		<article class="feature cut-panel">
			<div class="feature-art">
				<Artwork src={hero.backdrop_url || hero.poster_url} title="" eager />
			</div>
			<div class="feature-shade"></div>
			<div class="feature-top">
				<span class="eyebrow">{discovery ? 'In the spotlight' : 'From your archive'}</span><span
					class="index-mark">01 / SELECTED</span
				>
			</div>
			<div class="feature-copy">
				<div class="feature-meta">
					{mediaTypeLabel(hero)}
					<span>/</span>
					{hero.year || 'Year unknown'}
				</div>
				<h2>{hero.title}</h2>
				{#if hero.overview}<p>{hero.overview.replace(/<[^>]*>/g, '')}</p>{/if}
				{#if hero.status}<StatusBadge status={hero.status} />{:else if hero.in_library}<span
						class="in-library">In your library</span
					>{/if}
				<div class="feature-actions">
					{#if discovery}<button class="archive-button" on:click={() => select(hero)}
							>{hero.in_library ? 'View details' : 'Add to library'}<Plus size={16} /></button
						>
					{:else}<a class="archive-button" href="/library/{hero.id}"
							>View details<ArrowUpRight size={17} /></a
						><a class="archive-button secondary" href="/library/{hero.id}?tab=releases"
							>Find releases</a
						>{/if}
				</div>
			</div>
		</article>
		{#each supporting as item}
			<article class="supporting cut-panel">
				<svelte:element
					this={discovery ? 'button' : 'a'}
					href={discovery ? undefined : `/library/${item.id}`}
					class="supporting-art"
					on:click={() => discovery && select(item)}
					aria-label={`View ${item.title}`}
				>
					<Artwork src={item.poster_url} title={item.title} eager />
				</svelte:element>
				<div class="supporting-info">
					<span class="eyebrow"
						>{mediaTypeLabel(item)} / {item.year ||
							'—'}</span
					>
					<h3>{item.title}</h3>
					{#if item.status}<StatusBadge status={item.status} small />{:else if item.in_library}<span
							class="in-library">In library</span
						>{/if}
					{#if discovery}<button class="tile-action" on:click={() => select(item)}
							>View details <ArrowUpRight size={14} /></button
						>{:else}<a class="tile-action" href="/library/{item.id}"
							>View details <ArrowUpRight size={14} /></a
						>{/if}
				</div>
			</article>
		{/each}
	</section>
{/if}

<style>
	.showcase {
		display: grid;
		grid-template-columns: minmax(0, 2.2fr) repeat(2, minmax(0, 1fr));
		gap: 14px;
	}
	.feature {
		min-height: 450px;
		overflow: hidden;
		display: flex;
		flex-direction: column;
		justify-content: space-between;
		color: #fff;
	}
	.feature-art,
	.feature-shade {
		position: absolute;
		inset: 0;
	}
	.feature-shade {
		background: linear-gradient(0deg, #090810 0%, #090810d9 18%, #09081030 75%, #09081055 100%);
	}
	.feature-top,
	.feature-copy {
		position: relative;
		padding: 24px;
	}
	.feature-top {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 12px;
	}
	.feature-top .eyebrow {
		color: #f1f0f7;
	}
	.index-mark {
		font: 0.6rem var(--font-mono);
		letter-spacing: 0.12em;
		color: #c7c1d6;
	}
	.feature-meta {
		font: 0.65rem var(--font-mono);
		text-transform: uppercase;
		letter-spacing: 0.12em;
		color: #e1dce9;
	}
	.feature-meta span {
		padding: 0 8px;
		color: #ff408c;
	}
	h2 {
		font-size: clamp(2.4rem, 4.2vw, 4.5rem);
		line-height: 0.95;
		text-transform: uppercase;
		margin: 12px 0 16px;
		max-width: 550px;
	}
	p {
		display: -webkit-box;
		-webkit-line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
		font-size: 0.8rem;
		line-height: 1.7;
		color: #d6d0df;
		margin-bottom: 16px;
		max-width: 420px;
	}
	.feature-actions {
		display: flex;
		gap: 10px;
		margin-top: 20px;
		flex-wrap: wrap;
	}
	.feature .secondary {
		color: #fff;
		background: #100e18c9;
		border-color: #6e657d;
	}
	.supporting {
		--panel-accent: var(--neon-cyan);
		min-width: 0;
		overflow: hidden;
		display: flex;
		flex-direction: column;
	}
	.supporting-art {
		display: block;
		width: 100%;
		aspect-ratio: 2/2.8;
		max-height: 320px;
		min-height: 0;
		overflow: hidden;
		border: 0;
		padding: 0;
		background: var(--bg-elevated);
	}
	.supporting-info {
		padding: 15px;
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 10px;
	}
	h3 {
		font-family: var(--font-body);
		font-size: 0.88rem;
		line-height: 1.35;
	}
	.tile-action {
		display: flex;
		justify-content: space-between;
		align-items: center;
		width: 100%;
		padding: 9px 10px;
		border: 1px solid var(--border-strong);
		background: transparent;
		color: var(--text-primary);
		font-size: 0.65rem;
		margin-top: auto;
	}
	.tile-action:hover {
		border-color: var(--neon-cyan);
		color: var(--neon-cyan);
	}
	.in-library {
		font-size: 0.68rem;
		color: var(--success);
	}
	@media (max-width: 1050px) {
		.feature {
			min-height: 400px;
		}
		.showcase {
			grid-template-columns: minmax(0, 1.8fr) repeat(2, minmax(0, 1fr));
		}
		.feature-top,
		.feature-copy {
			padding: 20px;
		}
		.index-mark {
			display: none;
		}
	}
	@media (max-width: 650px) {
		.showcase {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
		.feature {
			grid-column: 1/-1;
			min-height: 410px;
		}
		.supporting-art {
			aspect-ratio: 2/2.4;
		}
	}
</style>
