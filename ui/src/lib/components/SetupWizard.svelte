<script>
	import { createEventDispatcher } from 'svelte';
	import { api } from '$lib/api';

	const dispatch = createEventDispatcher();

	let step = 1;
	const totalSteps = 6;
	let error = '';

	// Step 1: TMDB
	let tmdbKey = '';

	// Step 2: SABnzbd
	let sabUrl = 'http://sabnzbd:8080';
	let sabKey = '';

	// Step 3: Proxy
	let proxy = '';

	// Step 4: Media root
	let mediaRoot = '/data/media';

	// Step 5: Indexer
	let indexerName = '';
	let indexerUrl = '';
	let indexerKey = '';

	// Step 6: Quality profile
	let selectedProfile = 'defaults';

	let saving = false;

	async function nextStep() {
		error = '';
		if (step === 1 && !tmdbKey) {
			error = 'TMDB API key is required';
			return;
		}
		if (step < totalSteps) {
			step++;
			return;
		}

		// Final step — save everything
		saving = true;
		try {
			await api.updateSettings({
				tmdb_api_key: tmdbKey,
				sabnzbd_url: sabUrl,
				sabnzbd_api_key: sabKey,
				proxy: proxy || '',
				media_root: mediaRoot
			});

			if (indexerName && indexerUrl && indexerKey) {
				await api.createIndexer({
					name: indexerName,
					url: indexerUrl,
					api_key: indexerKey,
					priority: 0,
					enabled: true
				});
			}

			// Default profiles are already seeded in the database.
			// If user chose 'defaults', nothing extra to do.
			// The three profiles (HD Series, HD Movies, Anime) are ready to use.

			await api.updateSettings({ setup_complete: true });
			dispatch('complete');
		} catch (e) {
			error = e.message;
		}
		saving = false;
	}

	function prevStep() {
		if (step > 1) step--;
	}
</script>

<div class="wizard-overlay">
	<div class="wizard">
		<div class="wizard-header">
			<span class="wizard-logo">⚒</span>
			<h1>Zarr Setup</h1>
			<p class="wizard-sub">Let's get your media library manager configured.</p>
		</div>

		<div class="progress-dots">
			{#each Array.from({length: totalSteps}, (_, i) => i + 1) as s}
				<div class="dot" class:active={s === step} class:done={s < step}></div>
			{/each}
		</div>

		<div class="wizard-body">
			{#if step === 1}
				<div class="step">
					<h2>TMDB API Key</h2>
					<p>Required for movie and TV metadata. Get one free at <a href="https://www.themoviedb.org/settings/api" target="_blank" rel="noreferrer">themoviedb.org</a></p>
					<input type="text" bind:value={tmdbKey} placeholder="Enter your TMDB API key" />
				</div>
			{:else if step === 2}
				<div class="step">
					<h2>SABnzbd Connection</h2>
					<p>SABnzbd handles Usenet downloads. If using Docker Compose, the default URL works.</p>
					<label>URL</label>
					<input type="text" bind:value={sabUrl} placeholder="http://sabnzbd:8080" />
					<label>API Key</label>
					<input type="text" bind:value={sabKey} placeholder="SABnzbd API key (find in SABnzbd settings)" />
				</div>
			{:else if step === 3}
				<div class="step">
					<h2>Proxy (Optional)</h2>
					<p>Route external API calls through a SOCKS5 or HTTP proxy. Leave empty for direct connection.</p>
					<input type="text" bind:value={proxy} placeholder="socks5://host:port" />
				</div>
			{:else if step === 4}
				<div class="step">
					<h2>Media Root Directory</h2>
					<p>Where your organized media files will be stored.</p>
					<input type="text" bind:value={mediaRoot} placeholder="/data/media" />
				</div>
			{:else if step === 5}
				<div class="step">
					<h2>Add an Indexer (Optional)</h2>
					<p>Add a Newznab-compatible Usenet indexer. You can add more later in Settings.</p>
					<label>Name</label>
					<input type="text" bind:value={indexerName} placeholder="e.g. NZBGeek" />
					<label>Newznab API URL</label>
					<input type="text" bind:value={indexerUrl} placeholder="https://api.nzbgeek.info/api" />
					<label>API Key</label>
					<input type="text" bind:value={indexerKey} placeholder="Your indexer API key" />
				</div>
			{:else if step === 6}
				<div class="step">
					<h2>Quality Profiles</h2>
					<p>Zarr comes with three pre-configured quality profiles. You can customize them later in Settings.</p>
					<div class="profile-list">
						<div class="profile-card">
							<div class="profile-name">HD Series</div>
							<div class="profile-desc">For TV shows. Prefers BluRay 1080p, accepts WEB 1080p/720p. English audio.</div>
						</div>
						<div class="profile-card">
							<div class="profile-name">HD Movies</div>
							<div class="profile-desc">For movies. Prefers Remux/BluRay 1080p, accepts WEB 1080p. IMAX Enhanced bonus. English audio.</div>
						</div>
						<div class="profile-card">
							<div class="profile-name">Anime</div>
							<div class="profile-desc">For anime. Japanese audio with English subs. Bonuses for 10bit, dual-audio, uncensored.</div>
						</div>
					</div>
					<p class="hint">These profiles are ready to use. You can create custom profiles in Settings anytime.</p>
				</div>
			{/if}
		</div>

		{#if error}
			<div class="error">{error}</div>
		{/if}

		<div class="wizard-footer">
			{#if step > 1}
				<button class="btn btn-secondary" on:click={prevStep}>Back</button>
			{:else}
				<div></div>
			{/if}
			<button class="btn btn-primary" on:click={nextStep} disabled={saving}>
				{#if step === totalSteps}
					{saving ? 'Saving...' : 'Finish Setup'}
				{:else}
					Next
				{/if}
			</button>
		</div>
	</div>
</div>

<style>
	.wizard-overlay {
		position: fixed;
		inset: 0;
		background: var(--bg-base);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1000;
	}

	.wizard {
		width: 100%;
		max-width: 520px;
		background: var(--glass-bg);
		backdrop-filter: blur(var(--glass-blur));
		-webkit-backdrop-filter: blur(var(--glass-blur));
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-xl);
		padding: 2rem;
		box-shadow: var(--shadow-lg);
	}

	.wizard-header {
		text-align: center;
		margin-bottom: 1.5rem;
	}

	.wizard-logo {
		font-size: 2.5rem;
		display: block;
		margin-bottom: 0.5rem;
	}

	h1 {
		font-size: 1.5rem;
		font-weight: 700;
		font-family: var(--font-display);
		background: linear-gradient(135deg, var(--accent), var(--accent-hover));
		-webkit-background-clip: text;
		-webkit-text-fill-color: transparent;
		background-clip: text;
	}

	.wizard-sub {
		color: var(--text-secondary);
		font-size: 0.9rem;
		margin-top: 0.25rem;
	}

	.progress-dots {
		display: flex;
		justify-content: center;
		gap: 0.5rem;
		margin-bottom: 2rem;
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--border);
		transition: all 0.2s;
	}

	.dot.active { background: var(--accent); box-shadow: 0 0 8px var(--accent-glow); }
	.dot.done { background: var(--success); }

	.wizard-body {
		min-height: 200px;
	}

	.step h2 {
		font-size: 1.1rem;
		margin-bottom: 0.5rem;
		font-family: var(--font-display);
		color: var(--text-primary);
	}

	.step p {
		color: var(--text-secondary);
		font-size: 0.875rem;
		margin-bottom: 1rem;
		line-height: 1.4;
	}

	.step a {
		color: var(--accent);
	}
	.step a:hover {
		text-decoration: underline;
	}

	label {
		display: block;
		font-size: 0.8rem;
		color: var(--text-secondary);
		margin-bottom: 0.25rem;
		margin-top: 0.75rem;
		font-family: var(--font-body);
	}

	input {
		width: 100%;
		padding: 0.65rem 0.85rem;
		background: var(--bg-input);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--text-primary);
		font-size: 0.9rem;
		outline: none;
		font-family: var(--font-body);
		transition: border-color 0.2s ease, box-shadow 0.2s ease;
	}

	input:focus {
		border-color: var(--accent);
		box-shadow: 0 0 0 2px var(--accent-subtle);
	}

	.profile-list {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		margin-bottom: 0.75rem;
	}

	.profile-card {
		background: var(--bg-surface);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		padding: 0.75rem 1rem;
		transition: border-color 0.2s ease;
	}

	.profile-card:hover {
		border-color: var(--border-strong);
	}

	.profile-name {
		font-weight: 600;
		font-size: 0.9rem;
		margin-bottom: 0.25rem;
		color: var(--text-primary);
		font-family: var(--font-display);
	}

	.profile-desc {
		font-size: 0.8rem;
		color: var(--text-secondary);
		line-height: 1.3;
	}

	.hint {
		font-size: 0.8rem;
		color: var(--text-muted);
		font-style: italic;
	}

	.error {
		background: var(--danger-bg);
		border: 1px solid var(--danger-border);
		color: var(--danger);
		padding: 0.5rem 0.75rem;
		border-radius: var(--radius-sm);
		font-size: 0.85rem;
		margin-top: 1rem;
	}

	.wizard-footer {
		display: flex;
		justify-content: space-between;
		margin-top: 1.5rem;
	}

	.btn {
		padding: 0.6rem 1.5rem;
		border-radius: var(--radius-sm);
		font-size: 0.9rem;
		font-weight: 600;
		border: none;
		cursor: pointer;
		font-family: var(--font-body);
		transition: all 0.2s ease;
	}

	.btn-primary {
		background: var(--accent);
		color: var(--text-inverse);
	}

	.btn-primary:hover:not(:disabled) { background: var(--accent-hover); box-shadow: var(--shadow-glow); }
	.btn-primary:disabled { opacity: 0.5; cursor: default; }

	.btn-secondary {
		background: var(--bg-elevated);
		border: 1px solid var(--border);
		color: var(--text-secondary);
	}

	.btn-secondary:hover { background: var(--bg-hover); color: var(--text-primary); }
</style>
