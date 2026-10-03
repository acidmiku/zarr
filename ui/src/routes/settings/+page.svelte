<script>
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';
	import ConnectionSettings from '$lib/components/ConnectionSettings.svelte';
	import IndexerSettings from '$lib/components/IndexerSettings.svelte';
	import UsenetServers from '$lib/components/UsenetServers.svelte';
	import '$lib/components/connections.css';
	let profiles = [];
	let loading = true;
	let error = '';
	let busy = false;
	let scanning = false;
	let editing = null;
	let profile = defaults();
	function defaults() {
		return {
			name: '',
			profile_type: 'video',
			qualities: '["remux-1080p","bluray-1080p","web-1080p"]',
			language: 'en',
			tags: '{}',
			reject_patterns: '["cam","ts"]',
			upgrade_allowed: true
		};
	}
	onMount(loadProfiles);
	async function loadProfiles() {
		try {
			profiles = await api.getProfiles();
			error = '';
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	}
	function jsonText(value, fallback) {
		return typeof value === 'string' ? value : JSON.stringify(value ?? fallback);
	}
	function editProfile(value) {
		editing = value.id;
		profile = {
			...defaults(),
			...value,
			qualities: jsonText(value.qualities, []),
			tags: jsonText(value.tags, {}),
			reject_patterns: jsonText(value.reject_patterns, [])
		};
	}
	function resetProfile() {
		editing = null;
		profile = defaults();
	}
	function changeProfileType(event) {
		profile.profile_type = event.currentTarget.value;
		profile.qualities =
			profile.profile_type === 'music' ? '["flac","mp3-320"]' : defaults().qualities;
	}
	async function saveProfile() {
		if (busy) return;
		error = '';
		try {
			const data = {
				...profile,
				qualities: JSON.parse(profile.qualities),
				tags: JSON.parse(profile.tags),
				reject_patterns: JSON.parse(profile.reject_patterns)
			};
			if (
				!Array.isArray(data.qualities) ||
				!data.qualities.length ||
				data.qualities.some((q) => typeof q !== 'string')
			)
				throw new Error('Qualities must be a nonempty JSON array of names.');
			if (
				!Array.isArray(data.reject_patterns) ||
				data.reject_patterns.some((p) => typeof p !== 'string')
			)
				throw new Error('Reject patterns must be a JSON array of strings.');
			if (
				!data.tags ||
				Array.isArray(data.tags) ||
				typeof data.tags !== 'object' ||
				Object.values(data.tags).some((v) => typeof v !== 'number')
			)
				throw new Error('Tag bonuses must be a JSON object with numeric scores.');
			busy = true;
			if (editing !== null) await api.updateProfile(editing, data);
			else await api.createProfile(data);
			notify(editing !== null ? 'Profile updated' : 'Profile created', 'success');
			resetProfile();
			await loadProfiles();
		} catch (e) {
			error = e.message;
		} finally {
			busy = false;
		}
	}
	async function deleteProfile(value) {
		if (busy || !confirm(`Delete ${value.name}?`)) return;
		busy = true;
		try {
			await api.deleteProfile(value.id);
			if (editing === value.id) resetProfile();
			await loadProfiles();
		} catch (e) {
			error = e.message;
		} finally {
			busy = false;
		}
	}
	function qualityNames(value) {
		try {
			return (typeof value === 'string' ? JSON.parse(value) : value || []).join(' → ');
		} catch {
			return 'Invalid quality configuration';
		}
	}
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
	<section class="connection-section" aria-labelledby="profiles-heading">
		<h2 id="profiles-heading">Quality profiles</h2>
		<p class="connection-hint">
			Defaults are ready to use. Edit their preferences or create a separate profile for video or
			music.
		</p>
		{#if loading}<p role="status">Loading profiles…</p>{/if}
		{#each profiles as value}<div class="connection-item">
				<div>
					<strong>{value.name}</strong><span class="connection-status"
						>{value.profile_type || 'video'} · {value.language}</span
					>
					<p class="connection-hint">{qualityNames(value.qualities)}</p>
				</div>
				<div class="connection-actions">
					<button
						class="connection-button"
						disabled={busy}
						on:click={() => editProfile(value)}
						aria-label={`Edit profile ${value.name}`}>Edit</button
					><button
						class="connection-button danger"
						disabled={busy}
						on:click={() => deleteProfile(value)}
						aria-label={`Delete profile ${value.name}`}>Delete</button
					>
				</div>
			</div>{/each}
		<details class="connection-editor" open={editing !== null}>
			<summary>{editing !== null ? 'Edit quality profile' : 'Create quality profile'}</summary>
			<form on:submit|preventDefault={saveProfile}>
				<div class="connection-grid">
					<label>Profile name<input type="text" bind:value={profile.name} required /></label>
					<label
						>Profile media type<select
							bind:value={profile.profile_type}
							on:change={changeProfileType}
							><option value="video">Video</option><option value="music">Music</option></select
						></label
					>
					<label
						>Profile language<select bind:value={profile.language}
							><option value="en">English</option><option value="ja">Japanese</option><option
								value="ja-en">Japanese with English subtitles</option
							><option value="any">Any language</option></select
						></label
					>
					<label
						>Qualities (JSON array, best first)<input
							type="text"
							bind:value={profile.qualities}
							required
						/></label
					>
					<label
						>Tag bonuses (JSON object)<input
							type="text"
							bind:value={profile.tags}
							required
						/></label
					>
					<label
						>Reject patterns (JSON array)<input
							type="text"
							bind:value={profile.reject_patterns}
							required
						/></label
					>
				</div>
				<label class="connection-check"
					><input type="checkbox" bind:checked={profile.upgrade_allowed} /> Allow upgrades</label
				>
				<div class="connection-actions">
					<button class="connection-button primary" disabled={busy}
						>{editing !== null ? 'Save profile' : 'Create profile'}</button
					>{#if editing !== null}<button
							type="button"
							class="connection-button"
							on:click={resetProfile}>Cancel edit</button
						>{/if}
				</div>
			</form>
		</details>
		{#if error}<p class="connection-error" role="alert">{error}</p>{/if}
	</section>
	<section class="connection-section">
		<h2>Library maintenance</h2>
		<p class="connection-hint">Scan your media folder and match files already on disk.</p>
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
	summary {
		cursor: pointer;
		font-weight: 600;
	}
</style>
