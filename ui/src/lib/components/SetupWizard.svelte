<script>
	import { createEventDispatcher } from 'svelte';
	import ConnectionSettings from './ConnectionSettings.svelte';
	import IndexerSettings from './IndexerSettings.svelte';
	import UsenetServers from './UsenetServers.svelte';
	const dispatch = createEventDispatcher();
	let connections;
	let finishing = false;
	async function finish() {
		if (finishing || !connections) return;
		finishing = true;
		if (await connections.save(true)) dispatch('complete');
		finishing = false;
	}
</script>

<main class="setup-page">
	<header>
		<span class="wordmark">zarr<span> / </span></span>
		<p class="eyebrow">A home for your obsessions</p>
		<h1>Less setup. More discovery.</h1>
		<p>
			Start with your library. Connect only what you need; everything can be changed in Settings.
		</p>
	</header>
	<ConnectionSettings setup bind:this={connections} />
	<details class="connection-section">
		<summary>Set up release sources now <span class="connection-status">Optional</span></summary
		><IndexerSettings /><UsenetServers />
	</details>
	<footer>
		<div>
			<strong>Ready when you are.</strong>
			<p>Quality profiles are already included. Download services can wait.</p>
		</div>
		<button class="connection-button primary" on:click={finish} disabled={finishing}
			>{finishing ? 'Opening your archive…' : 'Start using Zarr'}</button
		>
	</footer>
</main>

<style>
	.setup-page {
		width: min(880px, 100%);
		margin: auto;
		padding: 2.5rem 1.25rem;
	}
	header {
		margin-bottom: 2rem;
	}
	.wordmark {
		font-family: var(--font-display);
		font-size: 3rem;
		font-weight: 800;
		letter-spacing: -0.05em;
	}
	.wordmark span {
		color: var(--accent);
	}
	.eyebrow {
		text-transform: uppercase;
		letter-spacing: 0.15em;
		color: var(--accent);
		font-size: 0.7rem;
		margin: 0.8rem 0;
	}
	h1 {
		font: 800 clamp(2rem, 6vw, 3rem) var(--font-display);
		line-height: 1.05;
		margin-bottom: 1rem;
	}
	header > p:last-child,
	footer p {
		color: var(--text-secondary);
		line-height: 1.6;
		font-size: 0.88rem;
	}
	footer {
		display: flex;
		flex-wrap: wrap;
		gap: 1rem;
		justify-content: space-between;
		align-items: center;
		padding: 1rem 0;
	}
	footer p {
		margin-top: 0.3rem;
	}
	footer button {
		padding: 0.9rem 1.3rem;
	}
</style>
