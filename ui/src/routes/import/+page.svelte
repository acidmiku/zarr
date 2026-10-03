<script>
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';

	let importPath = '';
	let mediaType = 'movie';
	let scanning = false;
	let importing = false;
	let scanResults = [];
	let profiles = [];
	let selectedProfileId = 0;

	// Selection state
	let selectedItems = new Set();
	let selectAll = false;

	async function loadProfiles() {
		try {
			profiles = await api.getProfiles();
		} catch {
			profiles = [];
		}
	}
	onMount(loadProfiles);
	$: matchingProfiles = profiles.filter((p) =>
		mediaType === 'music' ? p.profile_type === 'music' : p.profile_type !== 'music'
	);
	$: if (selectedProfileId && !matchingProfiles.some((p) => p.id === selectedProfileId))
		selectedProfileId = 0;

	async function scan() {
		if (scanning) return;
		if (!importPath.trim()) {
			notify('Please enter a directory path', 'error');
			return;
		}
		scanning = true;
		scanResults = [];
		selectedItems = new Set();
		selectAll = false;
		try {
			scanResults = await api.importScan(importPath.trim(), mediaType);
			if (scanResults.length === 0) {
				notify('No importable content found in the specified directory', 'info');
			} else {
				// Auto-select items that have a match and aren't in library
				scanResults.forEach((item, i) => {
					if (item.match && !item.in_library) {
						selectedItems.add(i);
					}
				});
				selectedItems = selectedItems; // trigger reactivity
				selectAll = selectedItems.size > 0;
			}
		} catch (err) {
			notify(err?.message || 'Scan failed', 'error');
		}
		scanning = false;
	}

	function toggleItem(index) {
		if (selectedItems.has(index)) {
			selectedItems.delete(index);
		} else {
			selectedItems.add(index);
		}
		selectedItems = selectedItems;
		selectAll = selectedItems.size === importableCount;
	}

	function toggleSelectAll() {
		selectAll = !selectAll;
		selectedItems = new Set();
		if (selectAll) {
			scanResults.forEach((item, i) => {
				if (item.match && !item.in_library) {
					selectedItems.add(i);
				}
			});
		}
		selectedItems = selectedItems;
	}

	$: importableCount = scanResults.filter((r) => r.match && !r.in_library).length;

	async function executeImport() {
		if (importing || scanning) return;
		if (selectedItems.size === 0) {
			notify('No items selected for import', 'error');
			return;
		}

		importing = true;
		const items = [];
		selectedItems.forEach((i) => {
			const r = scanResults[i];
			if (!r.match) return;

			const item = {
				source_path: r.source_path,
				type: mediaType,
				quality_profile_id: selectedProfileId
			};

			if (mediaType === 'movie' && r.match.tmdb_id) {
				item.tmdb_id = r.match.tmdb_id;
			} else if ((mediaType === 'series' || mediaType === 'anime') && r.match.tmdb_id) {
				item.tmdb_id = r.match.tmdb_id;
			} else if (mediaType === 'music' && r.match.release_group_id) {
				item.release_group_id = r.match.release_group_id;
				item.artist_mbid = r.match.artist_id || '';
				item.artist_name = r.match.artist || '';
				item.album_title = r.match.title || '';
			}

			items.push(item);
		});

		try {
			const results = await api.importExecute(items);
			let imported = 0;
			let errors = 0;
			results.forEach((r) => {
				if (r.status === 'imported') imported++;
				else if (r.status === 'error') errors++;
			});

			if (imported > 0) {
				notify(`Imported ${imported} item${imported > 1 ? 's' : ''} successfully`, 'success');
			}
			if (errors > 0) {
				notify(`${errors} item${errors > 1 ? 's' : ''} failed to import`, 'error');
			}

			// Re-scan to update status
			await scan();
		} catch (err) {
			notify(err?.message || 'Import failed', 'error');
		}
		importing = false;
	}

	function matchTitle(item) {
		if (mediaType === 'music') {
			if (item.match) return `${item.match.artist} - ${item.match.title}`;
			return item.artist ? `${item.artist} - ${item.album}` : item.album || item.title;
		}
		if (item.match) return item.match.title;
		return item.title;
	}

	function matchYear(item) {
		if (item.match) {
			const y = item.match.year || '';
			return y.length >= 4 ? y.substring(0, 4) : y;
		}
		return item.year || '';
	}

	function matchPoster(item) {
		if (!item.match) return '';
		return item.match.poster_url || item.match.cover_url || '';
	}

	function fileCount(item) {
		return item.files ? item.files.length : 0;
	}

	const typeLabels = {
		movie: 'Movies',
		series: 'TV Series',
		anime: 'Anime',
		music: 'Music'
	};

	const typeIcons = {
		movie: '▶',
		series: '▤',
		anime: '◈',
		music: '♫'
	};
</script>

<svelte:head>
	<title>Import - Zarr</title>
</svelte:head>

<div class="page">
	<header class="page-header">
		<h1>Import Content</h1>
		<p class="subtitle">Import existing media files into your library</p>
	</header>

	<div class="import-config">
		<div class="config-row">
			<div class="path-input">
				<label for="import-path">Source directory</label>
				<input
					id="import-path"
					type="text"
					bind:value={importPath}
					disabled={scanning || importing}
					placeholder="/path/to/media/files"
					on:keydown={(e) => e.key === 'Enter' && scan()}
				/>
			</div>

			<div class="type-selector">
				<span class="connection-hint">Media type</span>
				<div class="type-buttons">
					{#each Object.entries(typeLabels) as [value, label]}
						<button
							class="type-btn"
							class:active={mediaType === value}
							disabled={scanning || importing}
							on:click={() => {
								mediaType = value;
								scanResults = [];
								selectedItems = new Set();
							}}
						>
							<span class="type-icon">{typeIcons[value]}</span>
							{label}
						</button>
					{/each}
				</div>
			</div>
		</div>

		<div class="config-row">
			<div class="profile-select">
				<label for="profile">Quality profile</label>
				<select id="profile" bind:value={selectedProfileId}>
					<option value={0}>Automatic (match media type)</option>
					{#each matchingProfiles as p}
						<option value={p.id}>{p.name}</option>
					{/each}
				</select>
			</div>

			<button
				class="scan-btn"
				on:click={scan}
				disabled={scanning || importing || !importPath.trim()}
			>
				{#if scanning}
					Scanning...
				{:else}
					Scan Directory
				{/if}
			</button>
		</div>
	</div>

	{#if scanResults.length > 0}
		<div class="results-header">
			<div class="results-info">
				<span class="results-count"
					>{scanResults.length} item{scanResults.length !== 1 ? 's' : ''} found</span
				>
				{#if importableCount > 0}
					<span class="results-matched">{importableCount} matched</span>
				{/if}
			</div>

			<div class="results-actions">
				<label class="select-all">
					<input type="checkbox" checked={selectAll} on:change={toggleSelectAll} />
					Select all matched
				</label>
				<button
					class="import-btn"
					on:click={executeImport}
					disabled={importing || selectedItems.size === 0}
				>
					{#if importing}
						Importing...
					{:else}
						Import {selectedItems.size} item{selectedItems.size !== 1 ? 's' : ''}
					{/if}
				</button>
			</div>
		</div>

		<div class="results-list">
			{#each scanResults as item, i}
				<div
					class="result-item"
					class:selected={selectedItems.has(i)}
					class:in-library={item.in_library}
					class:no-match={!item.match}
				>
					<div class="result-check">
						{#if item.match && !item.in_library}
							<input
								type="checkbox"
								checked={selectedItems.has(i)}
								on:change={() => toggleItem(i)}
							/>
						{:else if item.in_library}
							<span class="status-icon in-lib" title="Already in library">&#10003;</span>
						{:else}
							<span class="status-icon no-match" title="No match found">?</span>
						{/if}
					</div>

					{#if matchPoster(item)}
						<img
							class="result-poster"
							src={matchPoster(item).startsWith('http')
								? api.imageUrl(matchPoster(item))
								: matchPoster(item)}
							alt=""
							loading="lazy"
						/>
					{:else}
						<div class="result-poster placeholder">
							<span>{typeIcons[mediaType]}</span>
						</div>
					{/if}

					<div class="result-info">
						<div class="result-title">
							{matchTitle(item)}
							{#if matchYear(item)}
								<span class="result-year">({matchYear(item)})</span>
							{/if}
						</div>
						<div class="result-meta">
							<span class="result-files"
								>{fileCount(item)} file{fileCount(item) !== 1 ? 's' : ''}</span
							>
							{#if item.episode}
								<span class="result-episodes"
									>{item.episode} episode{item.episode !== 1 ? 's' : ''}</span
								>
							{/if}
							<span class="result-path" title={item.source_path}
								>{item.source_path.split('/').pop() || item.source_path.split('\\').pop()}</span
							>
						</div>
						{#if item.match?.overview}
							<p class="result-overview">
								{item.match.overview.substring(0, 120)}{item.match.overview.length > 120
									? '...'
									: ''}
							</p>
						{/if}
						{#if item.in_library}
							<span class="badge badge-in-library">In Library</span>
						{:else if !item.match}
							<span class="badge badge-no-match">No Match Found</span>
						{:else if item.match.rating}
							<span class="badge badge-rating">{item.match.rating.toFixed(1)}</span>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{:else if scanning}
		<div class="empty-state">
			<div class="spinner"></div>
			<p>Scanning directory...</p>
		</div>
	{:else}
		<div class="empty-state">
			<span class="empty-icon">↥</span>
			<p>Enter a directory path and click Scan to find importable content.</p>
			<p class="empty-hint">
				{#if mediaType === 'movie'}
					Expected structure: <code>Directory/Movie Name (Year)/movie.mkv</code>
				{:else if mediaType === 'series'}
					Expected structure: <code>Directory/Series Name/Season XX/S01E01.mkv</code>
				{:else if mediaType === 'anime'}
					Expected structure: <code>Directory/Anime Name/Season XX/S01E01.mkv</code>
				{:else}
					Expected structure: <code>Directory/Artist/Album/01 - Track.flac</code>
				{/if}
			</p>
		</div>
	{/if}
</div>

<style>
	.page {
		max-width: 960px;
	}

	.page-header {
		margin-bottom: 1.5rem;
	}
	.page-header h1 {
		font-family: var(--font-display);
		font-size: 1.75rem;
		font-weight: 800;
		letter-spacing: -0.03em;
	}
	.subtitle {
		color: var(--text-muted);
		font-size: 0.85rem;
		margin-top: 0.25rem;
	}

	/* Config section */
	.import-config {
		background: var(--glass-bg);
		backdrop-filter: blur(var(--glass-blur));
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-lg);
		padding: 1.25rem;
		margin-bottom: 1.5rem;
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.config-row {
		display: flex;
		gap: 1rem;
		align-items: flex-end;
	}

	label {
		display: block;
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--text-muted);
		margin-bottom: 0.4rem;
	}

	.path-input {
		flex: 1;
	}
	.path-input input {
		width: 100%;
		padding: 0.6rem 0.8rem;
		background: var(--bg-elevated);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--text-primary);
		font-family: var(--font-mono);
		font-size: 0.85rem;
	}
	.path-input input:focus {
		outline: none;
		border-color: var(--accent);
		box-shadow: 0 0 0 2px var(--accent-subtle);
	}

	.type-selector {
		flex-shrink: 0;
	}
	.type-buttons {
		display: flex;
		gap: 4px;
		background: var(--bg-elevated);
		border-radius: var(--radius-sm);
		padding: 3px;
		border: 1px solid var(--border);
	}
	.type-btn {
		padding: 0.4rem 0.65rem;
		font-size: 0.78rem;
		font-weight: 500;
		border: none;
		background: transparent;
		border-radius: calc(var(--radius-sm) - 2px);
		color: var(--text-muted);
		cursor: pointer;
		transition: all 0.15s;
		display: flex;
		align-items: center;
		gap: 0.3rem;
	}
	.type-btn:hover {
		color: var(--text-primary);
	}
	.type-btn.active {
		background: var(--accent);
		color: var(--text-inverse);
	}
	.type-icon {
		font-size: 0.85rem;
	}

	.profile-select {
		flex: 1;
	}
	.profile-select select {
		width: 100%;
		padding: 0.6rem 0.8rem;
		background: var(--bg-elevated);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--text-primary);
		font-size: 0.85rem;
	}

	.scan-btn {
		padding: 0.6rem 1.5rem;
		background: var(--accent);
		color: var(--text-inverse);
		border: none;
		border-radius: var(--radius-sm);
		font-weight: 600;
		font-size: 0.85rem;
		cursor: pointer;
		transition: all 0.15s;
		white-space: nowrap;
	}
	.scan-btn:hover:not(:disabled) {
		background: var(--accent-hover);
		box-shadow: 0 0 12px var(--accent-glow);
	}
	.scan-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	/* Results */
	.results-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1rem;
		flex-wrap: wrap;
		gap: 0.75rem;
	}
	.results-info {
		display: flex;
		gap: 0.75rem;
		align-items: center;
	}
	.results-count {
		font-weight: 600;
		font-size: 0.9rem;
	}
	.results-matched {
		font-size: 0.8rem;
		color: var(--success);
	}
	.results-actions {
		display: flex;
		align-items: center;
		gap: 1rem;
	}
	.select-all {
		display: flex !important;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.8rem;
		color: var(--text-muted);
		cursor: pointer;
		text-transform: none;
		letter-spacing: 0;
		font-weight: 400;
		margin: 0;
	}

	.import-btn {
		padding: 0.55rem 1.25rem;
		background: var(--success);
		color: #fff;
		border: none;
		border-radius: var(--radius-sm);
		font-weight: 600;
		font-size: 0.82rem;
		cursor: pointer;
		transition: all 0.15s;
	}
	.import-btn:hover:not(:disabled) {
		filter: brightness(1.15);
		box-shadow: 0 0 12px rgba(46, 160, 67, 0.3);
	}
	.import-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	/* Result items */
	.results-list {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.result-item {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.65rem 0.85rem;
		background: var(--glass-bg);
		border: 1px solid var(--glass-border);
		border-radius: var(--radius-md);
		transition: all 0.15s;
	}
	.result-item:hover {
		border-color: var(--border-hover);
	}
	.result-item.selected {
		border-color: var(--accent);
		background: var(--accent-subtle);
	}
	.result-item.in-library {
		opacity: 0.6;
	}
	.result-item.no-match {
		opacity: 0.7;
	}

	.result-check {
		width: 24px;
		flex-shrink: 0;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.result-check input[type='checkbox'] {
		accent-color: var(--accent);
		width: 16px;
		height: 16px;
		cursor: pointer;
	}
	.status-icon {
		font-size: 0.85rem;
		font-weight: 700;
	}
	.status-icon.in-lib {
		color: var(--success);
	}
	.status-icon.no-match {
		color: var(--text-muted);
	}

	.result-poster {
		width: 50px;
		height: 50px;
		border-radius: var(--radius-sm);
		object-fit: cover;
		flex-shrink: 0;
		background: var(--bg-elevated);
	}
	.result-poster.placeholder {
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--text-muted);
		font-size: 1.1rem;
	}

	.result-info {
		flex: 1;
		min-width: 0;
	}
	.result-title {
		font-weight: 600;
		font-size: 0.9rem;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.result-year {
		font-weight: 400;
		color: var(--text-muted);
		margin-left: 0.3rem;
	}
	.result-meta {
		display: flex;
		gap: 0.75rem;
		margin-top: 0.2rem;
		font-size: 0.75rem;
		color: var(--text-muted);
	}
	.result-path {
		max-width: 200px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.result-overview {
		margin-top: 0.3rem;
		font-size: 0.75rem;
		color: var(--text-muted);
		line-height: 1.4;
	}

	.badge {
		display: inline-block;
		padding: 0.15rem 0.45rem;
		border-radius: 4px;
		font-size: 0.65rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		margin-top: 0.3rem;
	}
	.badge-in-library {
		background: var(--success-bg);
		color: var(--success);
		border: 1px solid var(--success-border);
	}
	.badge-no-match {
		background: var(--bg-elevated);
		color: var(--text-muted);
		border: 1px solid var(--border);
	}
	.badge-rating {
		background: var(--accent-subtle);
		color: var(--accent);
		border: 1px solid var(--accent);
	}

	/* Empty state */
	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 4rem 2rem;
		text-align: center;
		color: var(--text-muted);
	}
	.empty-icon {
		font-size: 3rem;
		margin-bottom: 1rem;
		opacity: 0.3;
	}
	.empty-hint {
		font-size: 0.8rem;
		margin-top: 0.5rem;
		opacity: 0.7;
	}
	.empty-hint code {
		background: var(--bg-elevated);
		padding: 0.15rem 0.4rem;
		border-radius: 3px;
		font-size: 0.75rem;
	}

	.spinner {
		width: 32px;
		height: 32px;
		border: 3px solid var(--border);
		border-top-color: var(--accent);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
		margin-bottom: 1rem;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	@media (max-width: 768px) {
		.config-row {
			flex-direction: column;
		}
		.type-buttons {
			flex-wrap: wrap;
		}
		.results-header {
			flex-direction: column;
			align-items: flex-start;
		}
		.result-overview {
			display: none;
		}
	}
</style>
