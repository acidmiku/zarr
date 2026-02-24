<script>
	import { createEventDispatcher } from 'svelte';

	export let value = '';
	export let placeholder = 'Search movies, series, anime...';

	const dispatch = createEventDispatcher();

	let timeout;
	function onInput() {
		clearTimeout(timeout);
		timeout = setTimeout(() => {
			dispatch('search', value);
		}, 400);
	}

	function onKeydown(e) {
		if (e.key === 'Enter') {
			clearTimeout(timeout);
			dispatch('search', value);
		}
	}
</script>

<div class="search-bar">
	<span class="search-icon">&#x2315;</span>
	<input
		type="text"
		bind:value
		on:input={onInput}
		on:keydown={onKeydown}
		{placeholder}
	/>
	{#if value}
		<button class="clear-btn" on:click={() => { value = ''; dispatch('search', ''); }}>&#x2715;</button>
	{/if}
</div>

<style>
	.search-bar {
		display: flex;
		align-items: center;
		background: var(--bg-input);
		border: 1px solid var(--border-subtle);
		border-radius: 10px;
		padding: 0 1rem;
		transition: border-color 0.15s;
		backdrop-filter: blur(8px);
		-webkit-backdrop-filter: blur(8px);
	}

	.search-bar:focus-within {
		border-color: var(--accent);
	}

	.search-icon {
		color: var(--text-muted);
		font-size: 1.2rem;
		margin-right: 0.5rem;
	}

	input {
		flex: 1;
		background: none;
		border: none;
		outline: none;
		color: var(--text-primary);
		padding: 0.75rem 0;
		font-size: 0.95rem;
	}

	input::placeholder {
		color: var(--text-muted);
	}

	.clear-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		font-size: 0.9rem;
		padding: 4px;
		transition: all 0.2s ease;
	}

	.clear-btn:hover {
		color: var(--text-secondary);
	}
</style>
