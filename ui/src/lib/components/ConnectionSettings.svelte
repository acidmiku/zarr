<script>
	import { onMount, createEventDispatcher } from 'svelte';
	import { api } from '$lib/api';
	import './connections.css';

	export let setup = false;
	const dispatch = createEventDispatcher();
	let settings = {};
	let form = {};
	let secrets = {};
	let loading = true;
	let saving = false;
	let error = '';
	let saved = false;
	let testing = {};
	let results = {};
	let models = [];

	onMount(load);
	async function load() {
		loading = true;
		error = '';
		try {
			settings = await api.getSettings();
			form = {
				media_root: settings.media_root || '/data/media',
				proxy: settings.proxy || '',
				sabnzbd_url: settings.sabnzbd_url || 'http://sabnzbd:8080',
				qbittorrent_enabled: settings.qbittorrent_enabled === true,
				qbittorrent_url: settings.qbittorrent_url || 'http://qbittorrent:9090',
				qbittorrent_username: settings.qbittorrent_username || 'admin',
				torrent_seed_time_hours: Number(settings.torrent_seed_time_hours ?? 24),
				torrent_remove_after_seed: settings.torrent_remove_after_seed !== false,
				openrouter_model: settings.openrouter_model || 'moonshotai/kimi-k3',
				openrouter_reasoning_effort: settings.openrouter_reasoning_effort || 'high',
				ai_personality_preset: settings.ai_personality_preset || 'default',
				ai_personality_custom: settings.ai_personality_custom || ''
			};
			secrets = {};
			api
				.aiModels()
				.then((value) => {
					models = Array.isArray(value) ? value : [];
				})
				.catch(() => {});
		} catch (e) {
			error = e.message;
		}
		loading = false;
	}

	export async function save(complete = false) {
		if (loading || saving) return false;
		error = '';
		saved = false;
		if (!form.media_root?.trim()) {
			error = 'Choose a media folder before saving.';
			return false;
		}
		if (
			!Number.isInteger(Number(form.torrent_seed_time_hours)) ||
			Number(form.torrent_seed_time_hours) < 0 ||
			Number(form.torrent_seed_time_hours) > 87600
		) {
			error = 'Seed time must be a whole number from 0 to 87600 hours.';
			return false;
		}
		saving = true;
		try {
			const data = { ...form, torrent_seed_time_hours: String(form.torrent_seed_time_hours) };
			for (const [key, value] of Object.entries(secrets)) if (value) data[key] = value;
			if (complete) data.setup_complete = true;
			await api.updateSettings(data);
			// Commit success is independent of any subsequent status fetch.
			for (const [key, value] of Object.entries(secrets)) {
				if (value) {
					const flag =
						key === 'qbittorrent_password'
							? 'qbittorrent_configured'
							: key.replace('_api_key', '_configured');
					settings[flag] = true;
				}
			}
			settings = { ...settings, ...form };
			secrets = {};
			saved = true;
			results = {};
			dispatch('saved', { complete, settings });
			return true;
		} catch (e) {
			error = e.message;
			return false;
		} finally {
			saving = false;
		}
	}

	async function test(kind) {
		testing = { ...testing, [kind]: true };
		results = { ...results, [kind]: null };
		try {
			let response;
			if (kind === 'tmdb')
				response = await api.testTMDB({ api_key: secrets.tmdb_api_key || undefined });
			if (kind === 'sab')
				response = await api.testSABnzbd({
					url: form.sabnzbd_url,
					api_key: secrets.sabnzbd_api_key || undefined
				});
			if (kind === 'qbt')
				response = await api.testQBittorrent({
					url: form.qbittorrent_url,
					username: form.qbittorrent_username,
					password: secrets.qbittorrent_password || undefined
				});
			if (kind === 'ai')
				response = await api.testOpenRouter({
					api_key: secrets.openrouter_api_key || undefined,
					model: form.openrouter_model,
					reasoning_effort: form.openrouter_reasoning_effort
				});
			results = {
				...results,
				[kind]: {
					ok: response.success,
					message: response.success
						? 'Connection successful'
						: response.error || 'Connection failed'
				}
			};
		} catch (e) {
			results = { ...results, [kind]: { ok: false, message: e.message } };
		} finally {
			testing = { ...testing, [kind]: false };
		}
	}
</script>

<div class="connection-settings">
	{#if loading}
		<p role="status">Loading connections…</p>
	{:else if !form.media_root}
		<p class="connection-error" role="alert">{error}</p>
		<button class="connection-button" on:click={load}>Retry loading settings</button>
	{:else}
		<form
			on:submit|preventDefault={() => save()}
			on:input={() => {
				saved = false;
			}}
		>
			<section class="connection-section">
				<h2>{setup ? 'Your library starts here' : 'Library & metadata'}</h2>
				<p class="connection-hint">
					Add a TMDB key for movie and TV discovery. You can start without one and connect services
					later.
				</p>
				<div class="connection-grid">
					<label
						>TMDB API key <span class="connection-status"
							>{settings.tmdb_configured ? 'Saved' : 'Optional'}</span
						><input
							type="password"
							autocomplete="new-password"
							bind:value={secrets.tmdb_api_key}
							placeholder={settings.tmdb_configured
								? 'Leave blank to keep saved key'
								: 'Enter TMDB API key'}
						/></label
					>
					<label
						>Media folder<input type="text" bind:value={form.media_root} required /><span
							class="connection-hint">A folder accessible inside the Zarr container.</span
						></label
					>
				</div>
				<div class="connection-actions">
					<button
						type="button"
						class="connection-button"
						on:click={() => test('tmdb')}
						disabled={testing.tmdb}>{testing.tmdb ? 'Testing…' : 'Test TMDB'}</button
					><a href="https://www.themoviedb.org/settings/api" target="_blank" rel="noreferrer"
						>Get a TMDB key ↗</a
					>
				</div>
				{#if results.tmdb}<p
						role="status"
						class:connection-error={!results.tmdb.ok}
						class:connection-success={results.tmdb.ok}
					>
						{results.tmdb.message}
					</p>{/if}
			</section>

			<details class="connection-section" open={!setup}>
				<summary
					>AI assistant <span class="connection-status"
						>{settings.openrouter_configured ? 'Connected' : 'Optional'}</span
					></summary
				>
				<p class="connection-hint">
					Personal recommendations through OpenRouter. Your model ID stays editable even if it is
					absent from the model list.
				</p>
				<div class="connection-grid">
					<label
						>OpenRouter API key <span class="connection-status"
							>{settings.openrouter_configured ? 'Saved' : ''}</span
						><input
							type="password"
							autocomplete="new-password"
							bind:value={secrets.openrouter_api_key}
							placeholder={settings.openrouter_configured
								? 'Leave blank to keep saved key'
								: 'Enter OpenRouter API key'}
						/></label
					>
					<label
						>OpenRouter model<input
							type="text"
							bind:value={form.openrouter_model}
							list="connection-models"
							placeholder="moonshotai/kimi-k3"
						/></label
					>
					<datalist id="connection-models"
						>{#each models as model}<option value={model.id}>{model.name || model.id}</option
							>{/each}</datalist
					>
					<label
						>Reasoning effort<select bind:value={form.openrouter_reasoning_effort}
							><option value="high">High</option><option value="medium">Medium</option><option
								value="low">Low</option
							><option value="none">None</option></select
						></label
					>
				</div>
				<div class="connection-actions">
					<button
						type="button"
						class="connection-button"
						on:click={() => test('ai')}
						disabled={testing.ai}>{testing.ai ? 'Testing…' : 'Test OpenRouter'}</button
					>
				</div>
				{#if results.ai}<p
						role="status"
						class:connection-error={!results.ai.ok}
						class:connection-success={results.ai.ok}
					>
						{results.ai.message}
					</p>{/if}
				{#if !setup}<div class="connection-grid">
						<label
							>Personality<select bind:value={form.ai_personality_preset}
								>{#each ['default', 'concise', 'quirky', 'sexy', 'academic', 'custom'] as preset}<option
										value={preset}>{preset}</option
									>{/each}</select
							></label
						>{#if form.ai_personality_preset === 'custom'}<label
								>Custom personality<textarea rows="3" bind:value={form.ai_personality_custom}
								></textarea></label
							>{/if}
					</div>{/if}
			</details>

			<details class="connection-section" open={!setup}>
				<summary>Download clients <span class="connection-status">Optional</span></summary>
				<p class="connection-hint">
					Choose Usenet, torrents, or both. Docker Compose service URLs work between containers;
					localhost refers to the Zarr container.
				</p>
				<h3>SABnzbd · Usenet</h3>
				<div class="connection-grid">
					<label
						>SABnzbd URL<input
							type="url"
							bind:value={form.sabnzbd_url}
							placeholder="http://sabnzbd:8080"
						/></label
					>
					<label
						>SABnzbd API key <span class="connection-status"
							>{settings.sabnzbd_configured ? 'Saved' : ''}</span
						><input
							type="password"
							autocomplete="new-password"
							bind:value={secrets.sabnzbd_api_key}
							placeholder={settings.sabnzbd_configured
								? 'Leave blank to keep saved key'
								: 'From SABnzbd configuration'}
						/></label
					>
				</div>
				<div class="connection-actions">
					<button
						type="button"
						class="connection-button"
						on:click={() => test('sab')}
						disabled={testing.sab}>{testing.sab ? 'Testing…' : 'Test SABnzbd'}</button
					>
				</div>
				{#if results.sab}<p
						role="status"
						class:connection-error={!results.sab.ok}
						class:connection-success={results.sab.ok}
					>
						{results.sab.message}
					</p>{/if}
				<h3>qBittorrent · Torrents</h3>
				<label class="connection-check"
					><input type="checkbox" bind:checked={form.qbittorrent_enabled} /> Enable qBittorrent</label
				>
				{#if form.qbittorrent_enabled}
					<div class="connection-grid">
						<label>qBittorrent URL<input type="url" bind:value={form.qbittorrent_url} /></label>
						<label
							>qBittorrent username<input
								type="text"
								autocomplete="username"
								bind:value={form.qbittorrent_username}
							/></label
						>
						<label
							>qBittorrent password <span class="connection-status"
								>{settings.qbittorrent_configured ? 'Saved' : ''}</span
							><input
								type="password"
								autocomplete="new-password"
								bind:value={secrets.qbittorrent_password}
								placeholder={settings.qbittorrent_configured
									? 'Leave blank to keep saved password'
									: 'Enter password'}
							/></label
						>
						<label
							>Seed time (hours)<input
								type="number"
								min="0"
								max="87600"
								step="1"
								bind:value={form.torrent_seed_time_hours}
							/></label
						>
					</div>
					<label class="connection-check"
						><input type="checkbox" bind:checked={form.torrent_remove_after_seed} /> Remove completed
						torrents after seeding</label
					>
					<div class="connection-actions">
						<button
							type="button"
							class="connection-button"
							on:click={() => test('qbt')}
							disabled={testing.qbt}>{testing.qbt ? 'Testing…' : 'Test qBittorrent'}</button
						>
					</div>
					{#if results.qbt}<p
							role="status"
							class:connection-error={!results.qbt.ok}
							class:connection-success={results.qbt.ok}
						>
							{results.qbt.message}
						</p>{/if}
				{/if}
			</details>

			<details class="connection-section">
				<summary>Music, web search & network</summary>
				<div class="connection-grid">
					<label
						>Last.fm API key <span class="connection-status"
							>{settings.lastfm_configured ? 'Saved' : 'Optional'}</span
						><input
							type="password"
							autocomplete="new-password"
							bind:value={secrets.lastfm_api_key}
							placeholder="Leave blank to keep saved key"
						/></label
					>
					<label
						>Brave Search API key <span class="connection-status"
							>{settings.brave_configured ? 'Saved' : 'Optional'}</span
						><input
							type="password"
							autocomplete="new-password"
							bind:value={secrets.brave_api_key}
							placeholder="Leave blank to keep saved key"
						/></label
					>
					<label
						>Proxy URL<input
							type="text"
							bind:value={form.proxy}
							placeholder="socks5://host.docker.internal:7897"
						/><span class="connection-hint"
							>Save before testing connections. Leave empty for a direct connection.</span
						></label
					>
				</div>
			</details>
			{#if error}<p class="connection-error" role="alert">{error}</p>{/if}
			{#if saved}<p class="connection-success" role="status">
					Connections saved. Existing keys were kept unless you supplied replacements.
				</p>{/if}
			<div class="connection-actions">
				<button type="submit" class="connection-button primary" disabled={saving}
					>{saving ? 'Saving…' : 'Save connections'}</button
				><span class="connection-hint"
					>Keys are stored on your server. Blank fields keep saved credentials.</span
				>
			</div>
		</form>
	{/if}
</div>
