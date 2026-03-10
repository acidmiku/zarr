<script>
	export let value = 0;
	export let readonly = false;

	let hoverValue = 0;

	function handleClick(star) {
		if (readonly) return;
		value = value === star ? 0 : star;
	}
</script>

<div class="stars" class:readonly role="radiogroup" aria-label="Rating">
	{#each [1, 2, 3, 4, 5] as star}
		<button
			class="star"
			class:filled={star <= (hoverValue || value)}
			on:click={() => handleClick(star)}
			on:mouseenter={() => { if (!readonly) hoverValue = star; }}
			on:mouseleave={() => { hoverValue = 0; }}
			disabled={readonly}
			aria-label="{star} star{star !== 1 ? 's' : ''}"
		>
			★
		</button>
	{/each}
</div>

<style>
	.stars {
		display: inline-flex;
		align-items: center;
		gap: 0.15rem;
		padding: 0.3rem 0.6rem;
		background: var(--glass-bg);
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-sm);
	}

	.star {
		background: none;
		border: none;
		font-size: 1.5rem;
		color: var(--text-muted);
		cursor: pointer;
		padding: 0 0.05rem;
		transition: color 0.2s ease, transform 0.2s ease, text-shadow 0.2s ease;
		line-height: 1;
	}

	.star.filled {
		color: var(--gold);
		text-shadow: 0 0 8px var(--gold);
	}

	.star:hover:not(:disabled) {
		color: var(--gold);
		transform: scale(1.15);
		text-shadow: 0 0 10px var(--gold);
	}

	.star:disabled {
		cursor: default;
	}

	.readonly .star {
		cursor: default;
	}

	.readonly .star:hover:not(:disabled) {
		transform: none;
	}
</style>
