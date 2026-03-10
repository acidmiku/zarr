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

	// Music settings
	let lastfmKey = '';

	// qBittorrent settings
	let qbtEnabled = false;
	let qbtUrl = '';
	let qbtUsername = '';
	let qbtPassword = '';
	let seedTimeHours = '24';
	let removeAfterSeed = true;
	let testingQBT = false;
	let qbtTestResult = null;

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
	let newIndexerType = 'newznab';
	let newIndexer = { name: '', url: '', api_key: '', priority: 0, enabled: true, type: 'newznab', username: '', password: '', content_types: ['movie', 'series', 'anime', 'music'] };

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

			// Music settings
			lastfmKey = '';

			// qBittorrent settings
			qbtEnabled = settings.qbittorrent_enabled || false;
			qbtUrl = settings.qbittorrent_url || 'http://qbittorrent:8080';
			qbtUsername = settings.qbittorrent_username || 'admin';
			qbtPassword = '';
			seedTimeHours = settings.torrent_seed_time_hours || '24';
			removeAfterSeed = settings.torrent_remove_after_seed !== false;

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

			// Music settings
			if (lastfmKey) data.lastfm_api_key = lastfmKey;

			// qBittorrent settings
			data.qbittorrent_enabled = qbtEnabled;
			data.qbittorrent_url = qbtUrl;
			data.qbittorrent_username = qbtUsername;
			if (qbtPassword) data.qbittorrent_password = qbtPassword;
			data.torrent_seed_time_hours = seedTimeHours;
			data.torrent_remove_after_seed = removeAfterSeed;

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
			qbtPassword = '';
			orKey = '';
			lastfmKey = '';
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

	async function testQBittorrent() {
		testingQBT = true;
		qbtTestResult = null;
		try {
			const result = await api.testQBittorrent({
				url: qbtUrl,
				username: qbtUsername,
				password: qbtPassword || undefined
			});
			qbtTestResult = result.success ? 'success' : result.error;
		} catch (e) {
			qbtTestResult = e.message;
		}
		testingQBT = false;
	}

	$: filteredModels = aiModels.filter(m =>
		!modelSearch || m.id.toLowerCase().includes(modelSearch.toLowerCase()) || m.name.toLowerCase().includes(modelSearch.toLowerCase())
	).slice(0, 50);

	// Indexer operations
	function resetNewIndexer() {
		newIndexer = {
			name: '', url: newIndexerType === 'rutracker' ? 'https://rutracker.org' : '',
			api_key: '', priority: 0, enabled: true,
			type: newIndexerType, username: '', password: '',
			content_types: ['movie', 'series', 'anime', 'music']
		};
	}

	$: if (newIndexerType) resetNewIndexer();

	async function addIndexer() {
		if (!newIndexer.name || !newIndexer.url) {
			notify('Fill in required indexer fields', 'error');
			return;
		}
		if (newIndexer.type === 'newznab' && !newIndexer.api_key) {
			notify('API key required for Newznab indexers', 'error');
			return;
		}
		if (newIndexer.type === 'rutracker' && (!newIndexer.username || !newIndexer.password)) {
			notify('Username and password required for Rutracker', 'error');
			return;
		}
		try {
			await api.createIndexer(newIndexer);
			notify('Indexer added', 'success');
			resetNewIndexer();
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

	function toggleContentType(ct) {
		if (newIndexer.content_types.includes(ct)) {
			newIndexer.content_types = newIndexer.content_types.filter(t => t !== ct);
		} else {
			newIndexer.content_types = [...newIndexer.content_types, ct];
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

	<!-- Downloaders -->
	<section class="section">
		<h2>Downloaders</h2>

		<h3>Usenet (SABnzbd)</h3>
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

		<h3>Torrent (qBittorrent)</h3>
		<div class="form-grid">
			<div class="field">
				<label>
					<input type="checkbox" bind:checked={qbtEnabled} />
					Enable qBittorrent
				</label>
			</div>
		</div>
		{#if qbtEnabled}
			<div class="form-grid" style="margin-top: 0.5rem">
				<div class="field">
					<label>URL</label>
					<input type="text" bind:value={qbtUrl} placeholder="http://qbittorrent:8080" />
				</div>
				<div class="field">
					<label>Username</label>
					<input type="text" bind:value={qbtUsername} placeholder="admin" />
				</div>
				<div class="field">
					<label>Password</label>
					<div class="key-row">
						<input type="password" bind:value={qbtPassword}
							placeholder={settings.qbittorrent_configured ? '••••••••' : 'Enter password'} />
						<button class="btn btn-small" on:click={testQBittorrent} disabled={testingQBT}>
							{testingQBT ? '...' : 'Test'}
						</button>
						{#if qbtTestResult === 'success'}
							<span class="test-ok">OK</span>
						{:else if qbtTestResult}
							<span class="test-fail">{qbtTestResult}</span>
						{/if}
					</div>
				</div>
			</div>
			<div class="form-grid" style="margin-top: 0.5rem">
				<div class="field">
					<label>Seed time (hours)</label>
					<input type="text" bind:value={seedTimeHours} placeholder="24" />
				</div>
				<div class="field">
					<label>
						<input type="checkbox" bind:checked={removeAfterSeed} />
						Remove torrent after seeding
					</label>
				</div>
			</div>
		{/if}
	</section>

	<!-- Music -->
	<section class="section">
		<h2>Music</h2>
		<div class="form-grid">
			<div class="field">
				<label>Last.fm API Key</label>
				<input type="password" bind:value={lastfmKey}
					placeholder={settings.lastfm_configured ? '••••••••' : 'Enter Last.fm API key'} />
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
						<span class="item-name">
							{idx.name}
							<span class="type-badge" class:torrent={idx.type === 'rutracker'}>
								{idx.type === 'rutracker' ? 'Rutracker' : 'Newznab'}
							</span>
						</span>
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
			<div class="field" style="max-width: 200px; margin-bottom: 0.5rem">
				<label>Type</label>
				<select bind:value={newIndexerType}>
					<option value="newznab">Newznab</option>
					<option value="rutracker">Rutracker</option>
				</select>
			</div>

			{#if newIndexerType === 'newznab'}
				<div class="form-row">
					<input type="text" bind:value={newIndexer.name} placeholder="Name" />
					<input type="text" bind:value={newIndexer.url} placeholder="Newznab API URL" />
					<input type="text" bind:value={newIndexer.api_key} placeholder="API Key" />
					<button class="btn btn-primary" on:click={addIndexer}>Add</button>
				</div>
			{:else}
				<div class="form-row">
					<input type="text" bind:value={newIndexer.name} placeholder="Name (e.g. Rutracker)" />
					<input type="text" bind:value={newIndexer.url} placeholder="https://rutracker.org" />
				</div>
				<div class="form-row" style="margin-top: 0.5rem">
					<input type="text" bind:value={newIndexer.username} placeholder="Username" />
					<input type="password" bind:value={newIndexer.password} placeholder="Password" />
					<button class="btn btn-primary" on:click={addIndexer}>Add</button>
				</div>
			{/if}

			<div class="content-types" style="margin-top: 0.5rem">
				<label style="font-size: 0.8rem; color: var(--text-secondary)">Content types:</label>
				<div class="ct-row">
					{#each ['movie', 'series', 'anime', 'music'] as ct}
						<label class="ct-label">
							<input type="checkbox" checked={newIndexer.content_types.includes(ct)}
								on:change={() => toggleContentType(ct)} />
							{ct}
						</label>
					{/each}
				</div>
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
	h1 {
		font-family: var(--font-display);
		font-size: 1.75rem;
		font-weight: 800;
		letter-spacing: -0.02em;
		margin-bottom: 1.5rem;
	}

	.section {
		background: var(--glass-bg);
		backdrop-filter: blur(var(--glass-blur));
		-webkit-backdrop-filter: blur(var(--glass-blur));
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-lg);
		padding: 1.25rem;
		margin-bottom: 1.25rem;
	}

	h2 {
		font-family: var(--font-display);
		font-size: 1.1rem;
		font-weight: 700;
		letter-spacing: -0.02em;
		margin-bottom: 1rem;
	}

	h3 {
		font-family: var(--font-display);
		font-size: 0.85rem;
		font-weight: 700;
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

	input[type="text"], input[type="password"], input[type="number"], select {
		width: 100%;
		padding: 0.55rem 0.75rem;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-sm);
		color: var(--text-primary);
		font-size: 0.85rem;
		outline: none;
		transition: border-color 0.2s, box-shadow 0.2s;
	}

	input:focus, select:focus {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	input[type="checkbox"] {
		margin-right: 0.5rem;
		accent-color: var(--accent);
	}

	.btn {
		padding: 0.5rem 1rem;
		border-radius: var(--radius-sm);
		font-family: var(--font-display);
		font-size: 0.85rem;
		font-weight: 700;
		border: none;
		cursor: pointer;
		transition: all 0.25s ease;
	}

	.btn-primary {
		background: var(--accent);
		color: var(--text-inverse);
	}
	.btn-primary:hover {
		background: var(--accent-hover);
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	.btn-secondary {
		background: var(--glass-bg);
		color: var(--text-secondary);
		border: 1px solid var(--glass-border);
	}
	.btn-secondary:hover {
		color: var(--text-primary);
		border-color: var(--accent);
		background: var(--glass-bg);
	}

	.btn-small { padding: 0.3rem 0.6rem; font-size: 0.8rem; }

	.btn-danger {
		background: var(--danger-bg);
		color: var(--danger);
		border: 1px solid var(--danger-border);
	}
	.btn-danger:hover {
		background: var(--danger);
		color: var(--text-inverse);
		border-color: var(--danger);
	}

	.save-btn {
		margin-bottom: 1.25rem;
		padding: 0.65rem 1.5rem;
		font-size: 0.95rem;
		font-weight: 800;
		letter-spacing: -0.01em;
	}
	.save-btn:hover {
		box-shadow: 0 0 16px var(--accent-subtle), 0 0 0 3px var(--accent-subtle);
	}

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
		background: var(--glass-bg);
		border-radius: var(--radius-sm);
		border: 1px solid var(--glass-border);
		transition: border-color 0.2s ease, background 0.2s ease;
	}

	.list-item:hover {
		border-color: var(--accent-subtle);
	}

	.item-info {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.item-name {
		font-family: var(--font-display);
		font-weight: 600;
		font-size: 0.9rem;
		display: flex;
		align-items: center;
		gap: 0.4rem;
	}
	.item-url, .item-meta { font-size: 0.75rem; color: var(--text-muted); }

	.type-badge {
		font-size: 0.65rem;
		font-weight: 700;
		padding: 0.15rem 0.45rem;
		border-radius: var(--radius-sm);
		background: var(--accent-subtle);
		color: var(--accent);
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.type-badge.torrent {
		background: #2d1f4e;
		color: #b388ff;
	}

	.item-actions {
		display: flex;
		gap: 0.4rem;
		align-items: center;
	}

	.toggle {
		padding: 0.25rem 0.65rem;
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-sm);
		font-size: 0.7rem;
		font-weight: 700;
		cursor: pointer;
		background: var(--glass-bg);
		color: var(--text-secondary);
		transition: all 0.25s ease;
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

	.content-types {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.ct-row {
		display: flex;
		gap: 1rem;
		flex-wrap: wrap;
	}

	.ct-label {
		display: flex;
		align-items: center;
		font-size: 0.8rem;
		color: var(--text-primary);
		text-transform: capitalize;
	}

	.profile-form {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.profile-form .field { max-width: 500px; }

	.profile-item {
		transition: border-color 0.25s ease;
	}
	.profile-item:hover {
		border-color: var(--accent-subtle);
	}

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

	.test-ok {
		color: var(--success);
		font-size: 0.8rem;
		font-weight: 600;
		display: flex;
		align-items: center;
		gap: 0.3rem;
	}
	.test-ok::before {
		content: '';
		display: inline-block;
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--success);
	}

	.test-fail {
		color: var(--danger);
		font-size: 0.75rem;
		max-width: 200px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		display: flex;
		align-items: center;
		gap: 0.3rem;
	}
	.test-fail::before {
		content: '';
		display: inline-block;
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--danger);
		flex-shrink: 0;
	}

	.model-search { margin-bottom: 0.35rem; }
	.model-select { width: 100%; }

	textarea {
		width: 100%;
		padding: 0.55rem 0.75rem;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-sm);
		color: var(--text-primary);
		font-size: 0.85rem;
		outline: none;
		resize: vertical;
		transition: border-color 0.2s, box-shadow 0.2s;
	}

	textarea:focus {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-subtle);
	}

	.personality-section {
		border-top: 1px solid var(--glass-border);
		padding-top: 0.75rem;
		margin-top: 0.25rem;
	}

	.personality-section h3 { margin-top: 0; }
</style>
