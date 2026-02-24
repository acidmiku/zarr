<script>
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';

	// Settings
	let settings = {};
	let tmdbKey = '';
	let sabUrl = '';
	let sabKey = '';
	let proxy = '';
	let mediaRoot = '';

	// AI settings
	let orKey = '';
	let orModel = '';
	let braveKey = '';
	let aiPreset = 'default';
	let aiCustom = '';
	let aiModels = [];
	let modelSearch = '';
	let testingOR = false;
	let orTestResult = null;

	// Indexers
	let indexers = [];
	let newIndexer = { name: '', url: '', api_key: '', priority: 0, enabled: true };

	// Profiles
	let profiles = [];
	let newProfile = {
		name: '', qualities: '["remux-1080p","bluray-1080p","web-1080p"]',
		language: 'en', tags: '{}', reject_patterns: '["cam","ts"]', upgrade_allowed: true
	};

	let editingProfile = null;

	onMount(async () => {
		await loadAll();
	});

	async function loadAll() {
		try {
			settings = await api.getSettings();
			tmdbKey = '';
			sabUrl = settings.sabnzbd_url || '';
			sabKey = '';
			proxy = settings.proxy || '';
			mediaRoot = settings.media_root || '/data/media';

			// AI settings
			orKey = '';
			orModel = settings.openrouter_model || 'anthropic/claude-sonnet-4-20250514';
			braveKey = '';
			aiPreset = settings.ai_personality_preset || 'default';
			aiCustom = settings.ai_personality_custom || '';

			indexers = await api.getIndexers();
			profiles = await api.getProfiles();

			// Load AI models if configured
			if (settings.openrouter_configured) {
				loadModels();
			}
		} catch (e) {
			notify(e.message, 'error');
		}
	}

	async function loadModels() {
		try {
			aiModels = await api.aiModels();
		} catch {
			aiModels = [];
		}
	}

	async function saveSettings() {
		try {
			const data = {};
			if (tmdbKey) data.tmdb_api_key = tmdbKey;
			if (sabUrl) data.sabnzbd_url = sabUrl;
			if (sabKey) data.sabnzbd_api_key = sabKey;
			data.proxy = proxy;
			data.media_root = mediaRoot;

			// AI settings
			if (orKey) data.openrouter_api_key = orKey;
			if (orModel) data.openrouter_model = orModel;
			if (braveKey) data.brave_api_key = braveKey;
			data.ai_personality_preset = aiPreset;
			data.ai_personality_custom = aiCustom;

			await api.updateSettings(data);
			notify('Settings saved', 'success');
			settings = await api.getSettings();
			tmdbKey = '';
			sabKey = '';
			orKey = '';
			braveKey = '';

			if (settings.openrouter_configured && aiModels.length === 0) {
				loadModels();
			}
		} catch (e) {
			notify(e.message, 'error');
		}
	}

	async function testOpenRouter() {
		testingOR = true;
		orTestResult = null;
		try {
			const result = await api.testOpenRouter(orKey || undefined);
			orTestResult = result.success ? 'success' : result.error;
			if (result.success) {
				loadModels();
			}
		} catch (e) {
			orTestResult = e.message;
		}
		testingOR = false;
	}

	$: filteredModels = aiModels.filter(m =>
		!modelSearch || m.id.toLowerCase().includes(modelSearch.toLowerCase()) || m.name.toLowerCase().includes(modelSearch.toLowerCase())
	).slice(0, 50);

	// Indexer operations
	async function addIndexer() {
		if (!newIndexer.name || !newIndexer.url || !newIndexer.api_key) {
			notify('Fill in all indexer fields', 'error');
			return;
		}
		try {
			await api.createIndexer(newIndexer);
			notify('Indexer added', 'success');
			newIndexer = { name: '', url: '', api_key: '', priority: 0, enabled: true };
			indexers = await api.getIndexers();
		} catch (e) {
			notify(e.message, 'error');
		}
	}

	async function deleteIndexer(id) {
		if (!confirm('Delete this indexer?')) return;
		try {
			await api.deleteIndexer(id);
			indexers = await api.getIndexers();
			notify('Indexer deleted', 'success');
		} catch (e) {
			notify(e.message, 'error');
		}
	}

	async function testIndexer(id) {
		try {
			const result = await api.testIndexer(id);
			if (result.success) {
				notify('Connection successful!', 'success');
			} else {
				notify('Test failed: ' + result.error, 'error');
			}
		} catch (e) {
			notify(e.message, 'error');
		}
	}

	async function toggleIndexer(idx) {
		try {
			await api.updateIndexer(idx.id, { ...idx, enabled: !idx.enabled });
			indexers = await api.getIndexers();
		} catch (e) {
			notify(e.message, 'error');
		}
	}

	// Profile operations
	async function addProfile() {
		if (!newProfile.name) {
			notify('Profile name required', 'error');
			return;
		}
		try {
			await api.createProfile({
				name: newProfile.name,
				qualities: JSON.parse(newProfile.qualities),
				language: newProfile.language,
				tags: JSON.parse(newProfile.tags),
				reject_patterns: JSON.parse(newProfile.reject_patterns),
				upgrade_allowed: newProfile.upgrade_allowed
			});
			notify('Profile created', 'success');
			newProfile = {
				name: '', qualities: '["remux-1080p","bluray-1080p","web-1080p"]',
				language: 'en', tags: '{}', reject_patterns: '["cam","ts"]', upgrade_allowed: true
			};
			profiles = await api.getProfiles();
		} catch (e) {
			notify(e.message, 'error');
		}
	}

	async function deleteProfile(id) {
		if (!confirm('Delete this quality profile?')) return;
		try {
			await api.deleteProfile(id);
			profiles = await api.getProfiles();
			notify('Profile deleted', 'success');
		} catch (e) {
			notify(e.message, 'error');
		}
	}

	async function triggerScan() {
		try {
			const result = await api.triggerScan();
			notify(`Scan complete: ${result.files_found} files found, ${result.files_matched} matched`, 'success');
		} catch (e) {
			notify(e.message, 'error');
		}
	}
</script>

<svelte:head>
	<title>Settings - Zarr</title>
</svelte:head>

<div class="page">
	<h1>Settings</h1>

	<!-- General Settings -->
	<section class="section">
		<h2>General</h2>
		<div class="form-grid">
			<div class="field">
				<label>TMDB API Key</label>
				<input type="password" bind:value={tmdbKey}
					placeholder={settings.tmdb_configured ? '••••••••' : 'Enter TMDB API key'} />
			</div>
			<div class="field">
				<label>Proxy (optional)</label>
				<input type="text" bind:value={proxy} placeholder="socks5://host:port" />
			</div>
			<div class="field">
				<label>Media Root</label>
				<input type="text" bind:value={mediaRoot} />
			</div>
		</div>
	</section>

	<!-- SABnzbd -->
	<section class="section">
		<h2>SABnzbd</h2>
		<div class="form-grid">
			<div class="field">
				<label>URL</label>
				<input type="text" bind:value={sabUrl} />
			</div>
			<div class="field">
				<label>API Key</label>
				<input type="password" bind:value={sabKey}
					placeholder={settings.sabnzbd_configured ? '••••••••' : 'Enter SABnzbd API key'} />
			</div>
		</div>
	</section>

	<!-- Indexers -->
	<section class="section">
		<h2>Indexers</h2>
		<div class="item-list">
			{#each indexers as idx}
				<div class="list-item">
					<div class="item-info">
						<span class="item-name">{idx.name}</span>
						<span class="item-url">{idx.url}</span>
					</div>
					<div class="item-actions">
						<button class="toggle" class:enabled={idx.enabled} on:click={() => toggleIndexer(idx)}>
							{idx.enabled ? 'ON' : 'OFF'}
						</button>
						<button class="btn btn-small" on:click={() => testIndexer(idx.id)}>Test</button>
						<button class="btn btn-small btn-danger" on:click={() => deleteIndexer(idx.id)}>Delete</button>
					</div>
				</div>
			{/each}
		</div>

		<div class="add-form">
			<h3>Add Indexer</h3>
			<div class="form-row">
				<input type="text" bind:value={newIndexer.name} placeholder="Name" />
				<input type="text" bind:value={newIndexer.url} placeholder="Newznab API URL" />
				<input type="text" bind:value={newIndexer.api_key} placeholder="API Key" />
				<button class="btn btn-primary" on:click={addIndexer}>Add</button>
			</div>
		</div>
	</section>

	<!-- Quality Profiles -->
	<section class="section">
		<h2>Quality Profiles</h2>
		<div class="item-list">
			{#each profiles as p}
				<div class="list-item profile-item">
					<div class="item-info">
						<span class="item-name">{p.name}</span>
						<span class="item-meta">
							{p.language} |
							{JSON.stringify(p.qualities)}
						</span>
					</div>
					<div class="item-actions">
						<button class="btn btn-small btn-danger" on:click={() => deleteProfile(p.id)}>Delete</button>
					</div>
				</div>
			{/each}
		</div>

		<div class="add-form">
			<h3>Add Profile</h3>
			<div class="profile-form">
				<div class="field">
					<label>Name</label>
					<input type="text" bind:value={newProfile.name} placeholder="Profile name" />
				</div>
				<div class="field">
					<label>Language</label>
					<select bind:value={newProfile.language}>
						<option value="en">English</option>
						<option value="ja-en">Japanese (w/ English subs)</option>
					</select>
				</div>
				<div class="field">
					<label>Qualities (JSON array, ordered by preference)</label>
					<input type="text" bind:value={newProfile.qualities} />
				</div>
				<div class="field">
					<label>Tag bonuses (JSON object)</label>
					<input type="text" bind:value={newProfile.tags} />
				</div>
				<div class="field">
					<label>Reject patterns (JSON array)</label>
					<input type="text" bind:value={newProfile.reject_patterns} />
				</div>
				<div class="field">
					<label>
						<input type="checkbox" bind:checked={newProfile.upgrade_allowed} />
						Allow upgrades
					</label>
				</div>
				<button class="btn btn-primary" on:click={addProfile}>Create Profile</button>
			</div>
		</div>
	</section>

	<!-- AI Assistant -->
	<section class="section">
		<h2>AI Assistant</h2>
		<div class="ai-form">
			<div class="field">
				<label>OpenRouter API Key</label>
				<div class="key-row">
					<input type="password" bind:value={orKey}
						placeholder={settings.openrouter_configured ? '••••••••' : 'Enter OpenRouter API key'} />
					<button class="btn btn-small" on:click={testOpenRouter} disabled={testingOR}>
						{testingOR ? '...' : 'Test'}
					</button>
					{#if orTestResult === 'success'}
						<span class="test-ok">OK</span>
					{:else if orTestResult}
						<span class="test-fail">{orTestResult}</span>
					{/if}
				</div>
			</div>

			<div class="field">
				<label>Model</label>
				{#if aiModels.length > 0}
					<input type="text" bind:value={modelSearch} placeholder="Search models..." class="model-search" />
					<select bind:value={orModel} class="model-select">
						{#each filteredModels as m}
							<option value={m.id}>{m.name} ({m.context_length?.toLocaleString()} ctx)</option>
						{/each}
					</select>
				{:else}
					<input type="text" bind:value={orModel} placeholder="e.g. anthropic/claude-sonnet-4-20250514" />
				{/if}
			</div>

			<div class="field">
				<label>Brave Search API Key (optional, for web search)</label>
				<input type="password" bind:value={braveKey}
					placeholder={settings.brave_configured ? '••••••••' : 'Enter Brave API key'} />
			</div>

			<div class="personality-section">
				<h3>Personality</h3>
				<div class="field">
					<label>Preset</label>
					<select bind:value={aiPreset}>
						<option value="default">Default</option>
						<option value="concise">Concise</option>
						<option value="quirky">Quirky</option>
						<option value="sexy">Sexy</option>
						<option value="academic">Academic</option>
						<option value="custom">Custom</option>
					</select>
				</div>

				{#if aiPreset === 'custom'}
					<div class="field">
						<label>Custom prompt</label>
						<textarea bind:value={aiCustom} rows="3"
							placeholder="Describe how the assistant should behave. This text becomes part of its personality."></textarea>
					</div>
				{/if}
			</div>
		</div>
	</section>

	<!-- System -->
	<section class="section">
		<h2>System</h2>
		<button class="btn btn-secondary" on:click={triggerScan}>Scan Library</button>
	</section>

	<button class="btn btn-primary save-btn" on:click={saveSettings}>Save Settings</button>
</div>

<style>
	h1 { font-size: 1.5rem; font-weight: 700; margin-bottom: 1.5rem; }

	.section {
		background: var(--glass-bg);
		backdrop-filter: blur(var(--glass-blur));
		-webkit-backdrop-filter: blur(var(--glass-blur));
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-md);
		padding: 1.25rem;
		margin-bottom: 1.25rem;
	}

	h2 {
		font-size: 1.05rem;
		font-weight: 600;
		margin-bottom: 1rem;
	}

	h3 {
		font-size: 0.9rem;
		color: var(--text-secondary);
		margin-bottom: 0.75rem;
		margin-top: 1rem;
	}

	.form-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
		gap: 0.75rem;
	}

	.field label {
		display: block;
		font-size: 0.8rem;
		color: var(--text-secondary);
		margin-bottom: 0.25rem;
	}

	input[type="text"], input[type="password"], select {
		width: 100%;
		padding: 0.55rem 0.75rem;
		background: var(--bg-input);
		border: 1px solid var(--border-subtle);
		border-radius: var(--radius-sm);
		color: var(--text-primary);
		font-size: 0.85rem;
		outline: none;
		transition: border-color 0.2s;
	}

	input:focus, select:focus {
		border-color: var(--accent);
		box-shadow: 0 0 0 2px var(--accent-subtle);
	}

	input[type="checkbox"] { margin-right: 0.5rem; }

	.btn {
		padding: 0.5rem 1rem;
		border-radius: var(--radius-sm);
		font-size: 0.85rem;
		font-weight: 600;
		border: none;
		cursor: pointer;
		transition: all 0.2s;
	}

	.btn-primary { background: var(--accent); color: var(--text-inverse); }
	.btn-primary:hover { background: var(--accent-hover); box-shadow: var(--shadow-glow); }
	.btn-secondary { background: var(--bg-elevated); color: var(--text-secondary); border: 1px solid var(--border); }
	.btn-secondary:hover { color: var(--text-primary); background: var(--bg-hover); }
	.btn-small { padding: 0.3rem 0.6rem; font-size: 0.8rem; }
	.btn-danger { background: var(--danger-bg); color: var(--danger); border: 1px solid var(--danger-border); }
	.btn-danger:hover { background: var(--danger); color: var(--text-inverse); }

	.save-btn { margin-bottom: 1.25rem; }

	.item-list {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.list-item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.65rem 0.85rem;
		background: var(--bg-surface);
		border-radius: var(--radius-sm);
		border: 1px solid var(--border);
	}

	.item-info {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.item-name { font-weight: 500; font-size: 0.9rem; }
	.item-url, .item-meta { font-size: 0.75rem; color: var(--text-muted); }

	.item-actions {
		display: flex;
		gap: 0.4rem;
		align-items: center;
	}

	.toggle {
		padding: 0.2rem 0.6rem;
		border: 1px solid var(--border-subtle);
		border-radius: 4px;
		font-size: 0.7rem;
		font-weight: 700;
		cursor: pointer;
		background: var(--bg-elevated);
		color: var(--text-secondary);
		transition: all 0.2s;
	}

	.toggle.enabled {
		background: var(--success-bg);
		border-color: var(--success-border);
		color: var(--success);
	}

	.form-row {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.form-row input { flex: 1; min-width: 120px; }

	.profile-form {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.profile-form .field { max-width: 500px; }

	/* AI settings */
	.ai-form {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		max-width: 500px;
	}

	.key-row {
		display: flex;
		gap: 0.5rem;
		align-items: center;
	}

	.key-row input { flex: 1; }

	.test-ok { color: var(--success); font-size: 0.8rem; font-weight: 600; }
	.test-fail { color: var(--danger); font-size: 0.75rem; max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

	.model-search { margin-bottom: 0.35rem; }
	.model-select { width: 100%; }

	textarea {
		width: 100%;
		padding: 0.55rem 0.75rem;
		background: var(--bg-input);
		border: 1px solid var(--border-subtle);
		border-radius: var(--radius-sm);
		color: var(--text-primary);
		font-size: 0.85rem;
		outline: none;
		resize: vertical;
		transition: border-color 0.2s;
	}

	textarea:focus {
		border-color: var(--accent);
		box-shadow: 0 0 0 2px var(--accent-subtle);
	}

	.personality-section {
		border-top: 1px solid var(--border);
		padding-top: 0.75rem;
		margin-top: 0.25rem;
	}

	.personality-section h3 { margin-top: 0; }
</style>
