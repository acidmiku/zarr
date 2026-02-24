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
		gap: 2px;
	}

	.star {
		background: none;
		border: none;
		font-size: 1.5rem;
		color: var(--text-dim);
		cursor: pointer;
		padding: 0;
		transition: color 0.1s;
		line-height: 1;
	}

	.star.filled {
		color: var(--gold);
	}

	.star:hover:not(:disabled) {
		color: var(--gold);
	}

	.star:disabled {
		cursor: default;
	}

	.readonly .star {
		cursor: default;
	}
</style>
