<script>
	import { createEventDispatcher, onDestroy } from 'svelte';
	import { Search, X } from 'lucide-svelte';

	export let value = '';
	export let placeholder = 'Search movies, series, anime...';

	const dispatch = createEventDispatcher();

	let timeout;
	onDestroy(() => clearTimeout(timeout));
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
	<span class="search-icon"><Search size={18} /></span>
	<input
		type="search"
		aria-label={placeholder}
		bind:value
		on:input={onInput}
		on:keydown={onKeydown}
		{placeholder}
	/>
	{#if value}
		<button class="clear-btn" aria-label="Clear search" on:click={() => { value = ''; dispatch('search', ''); }}><X size={15} /></button>
	{/if}
</div>

<style>
	.search-bar {
		display: flex;
		align-items: center;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-md);
		padding: 0 1.25rem;
		transition: all 0.25s ease;
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
	}

	.search-bar:focus-within {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-subtle), var(--shadow-sm);
	}

	.search-icon {
		color: var(--text-muted);
		font-size: 1.15rem;
		margin-right: 0.6rem;
		transition: color 0.2s;
	}

	.search-bar:focus-within .search-icon {
		color: var(--accent);
	}

	input {
		flex: 1;
		min-width: 0;
		background: none;
		border: none;
		outline: none;
		color: var(--text-primary);
		padding: 0.85rem 0;
		font-size: 0.9rem;
		font-family: var(--font-body);
	}

	input::placeholder {
		color: var(--text-muted);
	}

	.clear-btn {
		background: none;
		border: none;
		color: var(--text-muted);
		font-size: 0.85rem;
		padding: 4px;
		transition: all 0.2s ease;
		border-radius: 4px;
	}

	.clear-btn:hover {
		color: var(--text-primary);
		background: var(--bg-hover);
	}
</style>
