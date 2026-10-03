<script>
	import { Film, Tv, Cat, Disc3 } from 'lucide-svelte';
	export let mode = 'library';
	export let selected = 'all';
	const categories = [
		{ type: 'movie', title: 'Movies', description: 'New worlds. Cult classics.', icon: Film },
		{ type: 'series', title: 'Series', description: 'Every season. Every episode.', icon: Tv },
		{ type: 'anime', title: 'Anime', description: 'Beyond the ordinary.', icon: Cat },
		{ type: 'music', title: 'Music', description: 'Albums worth collecting.', icon: Disc3 }
	];
</script>

<nav class="archive-categories" aria-label="Media categories">
	{#each categories as category, i}
		<a
			class="category cut-panel"
			class:selected={selected === category.type}
			aria-current={selected === category.type ? 'page' : undefined}
			href={category.type === 'music'
				? `/music?tab=${mode === 'library' ? 'library' : 'discover'}`
				: `${mode === 'library' ? '/library' : '/'}?type=${category.type}`}
		>
			<span class="category-number">0{i + 1}</span><strong>{category.title}</strong
			><svelte:component this={category.icon} size={24} strokeWidth={1.5} /><span
				class="category-description">{category.description}</span
			>
		</a>
	{/each}
</nav>

<style>
	.archive-categories {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 14px;
		margin-bottom: 22px;
	}
	.category {
		display: flex;
		align-items: center;
		gap: 14px;
		min-width: 0;
		padding: 18px 20px;
		border-color: var(--border-strong);
		transition:
			border-color 0.15s,
			background 0.15s;
	}
	.category:nth-child(even) {
		--panel-accent: var(--neon-cyan);
	}
	.category:hover,
	.category.selected {
		border-color: var(--panel-accent, var(--accent));
		background: var(--accent-subtle);
	}
	.category-number {
		font-family: var(--font-display);
		font-size: 2rem;
		font-weight: 400;
		color: var(--panel-accent, var(--accent));
	}
	strong {
		font-family: var(--font-display);
		text-transform: uppercase;
		font-size: clamp(1.5rem, 2.5vw, 2.3rem);
		line-height: 1;
	}
	.category-description {
		color: var(--text-secondary);
		font-size: 0.66rem;
		line-height: 1.6;
		margin-left: auto;
		max-width: 100px;
	}
	.category :global(svg) {
		flex-shrink: 0;
		color: var(--text-muted);
	}
	@media (max-width: 1300px) {
		.category-description {
			display: none;
		}
		.category {
			padding: 14px;
			gap: 10px;
		}
		.category :global(svg) {
			margin-left: auto;
		}
	}
	@media (max-width: 600px) {
		.archive-categories {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			gap: 8px;
			margin-bottom: 16px;
		}
		strong {
			font-size: 1.6rem;
		}
		.category-number {
			font-size: 1.6rem;
		}
		.category {
			padding: 12px;
		}
	}
</style>
