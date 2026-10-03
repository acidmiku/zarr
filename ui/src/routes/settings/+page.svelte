<script>
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';
	import ConnectionSettings from '$lib/components/ConnectionSettings.svelte';
	import IndexerSettings from '$lib/components/IndexerSettings.svelte';
	import UsenetServers from '$lib/components/UsenetServers.svelte';
	import QualityProfiles from '$lib/components/QualityProfiles.svelte';
	import '$lib/components/connections.css';
	let scanning = false;
	async function triggerScan() {
		if (scanning) return;
		scanning = true;
		try {
			const result = await api.triggerScan();
			notify(
				`Scan complete: ${result.files_found} files found, ${result.files_matched} matched`,
				'success'
			);
		} catch (e) {
			notify(e.message, 'error');
		} finally {
			scanning = false;
		}
	}
</script>

<svelte:head><title>Settings - Zarr</title></svelte:head>
<div class="page settings-page">
	<h1>Settings</h1>
	<p class="connection-hint">
		Manage your library, credentials and connections here. No environment-file editing needed.
	</p>
	<ConnectionSettings />
	<IndexerSettings />
	<UsenetServers />
	<QualityProfiles />
	<section class="connection-section">
		<h2>Library maintenance</h2>
		<p class="connection-hint">Scan your media folder and match files already on disk. Rescans clear release provenance; automatic upgrades wait for a verified import.</p>
		<button class="connection-button" on:click={triggerScan} disabled={scanning}
			>{scanning ? 'Scanning…' : 'Scan library'}</button
		>
	</section>
</div>

<style>
	.settings-page {
		max-width: 1100px;
	}
	h1 {
		font: 800 2rem var(--font-display);
		margin-bottom: 0.6rem;
	}
</style>
