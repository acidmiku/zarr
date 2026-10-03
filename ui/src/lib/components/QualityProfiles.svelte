<script>
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { notify } from '$lib/stores/app';
	import { scoringConfig } from '$lib/profiles';

	let profiles = [];
	let presets = [];
	let loading = true;
	let error = '';
	let presetError = '';
	let busy = false;
	let editing = null;
	let profile = defaults();
	let config = {};
	let groups = [];
	let formatFilter = '';
	$: preset = presets.find((value) => value.id === config.preset);
	$: formats = (preset?.formats || []).filter((format) =>
		format.name.toLowerCase().includes(formatFilter.toLowerCase())
	);

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
	function jsonText(value, fallback) {
		return typeof value === 'string' ? value : JSON.stringify(value ?? fallback);
	}
	function clone(value) {
		return JSON.parse(JSON.stringify(value));
	}
	onMount(async () => {
		await Promise.all([loadProfiles(), loadPresets()]);
		loading = false;
	});
	async function loadProfiles() {
		try {
			profiles = await api.getProfiles();
		} catch (e) {
			error = e.message;
		}
	}
	async function loadPresets() {
		try {
			const result = await api.getProfilePresets();
			presets = Array.isArray(result) ? result : [];
		} catch (e) {
			presetError = `Anime presets could not load: ${e.message}`;
		}
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
		config = clone(scoringConfig(value.scoring_config));
		if (config.preset) config.dual_audio ||= 'optional';
		groups = (config.quality_groups || []).map((group) => group.join(', '));
		formatFilter = '';
		error = '';
	}
	function resetProfile() {
		editing = null;
		profile = defaults();
		config = {};
		groups = [];
		formatFilter = '';
		error = '';
	}
	function changeProfileType(event) {
		profile.profile_type = event.currentTarget.value;
		profile.qualities =
			profile.profile_type === 'music' ? '["flac","mp3-320"]' : defaults().qualities;
		if (profile.profile_type === 'music') {
			config = {};
			groups = [];
			profile.language = 'any';
		}
	}
	function choosePreset(event) {
		const selected = presets.find((value) => value.id === event.currentTarget.value);
		if (!selected) {
			config = {};
			groups = [];
			return;
		}
		config = clone(selected.config);
		config.dual_audio ||= 'optional';
		groups = config.quality_groups.map((group) => group.join(', '));
		profile.qualities = JSON.stringify(selected.qualities);
		profile.language = 'any';
		profile.tags = '{}';
		profile.reject_patterns = '[]';
		if (!profile.name) {
			let name = selected.name;
			let suffix = 1;
			while (profiles.some((value) => value.name === name)) {
				name = `${selected.name} (custom${suffix > 1 ? ` ${suffix}` : ''})`;
				suffix += 1;
			}
			profile.name = name;
		}
	}
	function setFormatScore(id, event) {
		const value = event.currentTarget.value;
		const scores = { ...(config.format_scores || {}) };
		if (value === '') delete scores[id];
		else scores[id] = Number(value);
		config = { ...config, format_scores: scores };
	}
	function resetFormatScore(id) {
		const scores = { ...(config.format_scores || {}) };
		delete scores[id];
		config = { ...config, format_scores: scores };
	}
	function moveGroup(index, offset) {
		const reordered = [...groups];
		[reordered[index], reordered[index + offset]] = [reordered[index + offset], reordered[index]];
		groups = reordered;
	}
	async function saveProfile() {
		if (busy) return;
		error = '';
		try {
			const scoring = config.preset
				? {
						...config,
						quality_groups: groups.map((group) =>
							group
								.split(',')
								.map((quality) => quality.trim())
								.filter(Boolean)
						)
					}
				: {};
			if (
				scoring.preset &&
				(!scoring.quality_groups.length || scoring.quality_groups.some((group) => !group.length))
			)
				throw new Error('Each quality group needs at least one quality.');
			const data = {
				...profile,
				qualities: scoring.preset ? scoring.quality_groups.flat() : JSON.parse(profile.qualities),
				tags: JSON.parse(profile.tags),
				reject_patterns: JSON.parse(profile.reject_patterns),
				scoring_config: scoring
			};
			if (
				!Array.isArray(data.qualities) ||
				!data.qualities.length ||
				data.qualities.some((quality) => typeof quality !== 'string' || !quality.trim())
			)
				throw new Error('Qualities must be a nonempty JSON array of names.');
			if (
				!Array.isArray(data.reject_patterns) ||
				data.reject_patterns.some((pattern) => typeof pattern !== 'string')
			)
				throw new Error('Reject patterns must be a JSON array of strings.');
			if (
				!data.tags ||
				Array.isArray(data.tags) ||
				typeof data.tags !== 'object' ||
				Object.values(data.tags).some((score) => !Number.isInteger(score))
			)
				throw new Error('Tag bonuses must be a JSON object with integer scores.');
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
		error = '';
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
		const scoring = scoringConfig(value.scoring_config);
		if (scoring.preset)
			return (scoring.quality_groups || []).map((group) => group.join(' / ')).join(' → ');
		try {
			return (
				typeof value.qualities === 'string' ? JSON.parse(value.qualities) : value.qualities || []
			).join(' → ');
		} catch {
			return 'Invalid quality configuration';
		}
	}
</script>

<section class="connection-section quality-profiles" aria-labelledby="profiles-heading">
	<h2 id="profiles-heading">Quality profiles</h2>
	<p class="connection-hint">
		Choose an anime preset for quality groups and release-format scoring, or keep a custom video or
		music profile.
	</p>
	{#if loading}<p role="status">Loading profiles…</p>{/if}
	{#each profiles as value}
		<div class="connection-item">
			<div class="profile-description">
				<strong>{value.name}</strong><span class="connection-status"
					>{scoringConfig(value.scoring_config).preset
						? 'TRaSH anime'
						: `${value.profile_type || 'video'} · ${value.language}`}</span
				>
				<p class="connection-hint">{qualityNames(value)}</p>
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
		</div>
	{/each}
	<details class="connection-editor" open={editing !== null}>
		<summary>{editing !== null ? 'Edit quality profile' : 'Create quality profile'}</summary>
		<form on:submit|preventDefault={saveProfile}>
			<div class="connection-grid">
				<label>Profile name<input type="text" bind:value={profile.name} required /></label>
				<label
					>Profile media type<select bind:value={profile.profile_type} on:change={changeProfileType}
						><option value="video">Video</option><option value="music">Music</option></select
					></label
				>
				{#if profile.profile_type !== 'music'}<label
						>Scoring preset<select value={config.preset || ''} on:change={choosePreset}
							><option value="">Custom / legacy</option>{#each presets as value}<option
									value={value.id}>{value.name}</option
								>{/each}{#if config.preset && !preset}<option value={config.preset}
									>{config.preset} (saved)</option
								>{/if}</select
						></label
					>{/if}
			</div>
			{#if config.preset}
				<div class="preset-summary">
					<p>
						Quality groups rank first. Custom formats choose the preferred release within a quality
						group. Minimum scores apply at every quality.
					</p>
					{#if preset}<p class="connection-hint">
							<a href={preset.source_url} target="_blank" rel="noreferrer">TRaSH guide</a> ·
							Snapshot {preset.version}
						</p>{/if}
				</div>
				<div class="connection-grid">
					<label
						>Dual audio<select bind:value={config.dual_audio} aria-label="Dual audio"
							><option value="optional">Optional · guide default</option><option value="within-tier"
								>Prefer within release tier</option
							><option value="above-tier">Prefer above release tier</option><option value="required"
								>Require dual-audio label</option
							></select
						><span class="connection-hint"
							>Uses release-title labels; embedded audio tracks are not inspected. Release-tier
							preferences apply within the same quality group.</span
						></label
					>
				</div>
				<details class="advanced-scoring">
					<summary>Advanced scoring</summary>
					<div class="connection-grid">
						<label
							>Minimum format score<input
								type="number"
								step="1"
								bind:value={config.minimum_format_score}
								required
							/></label
						>
						<label
							>Upgrade until quality<select bind:value={config.upgrade_until_quality}
								>{#each groups.flatMap((group) => group
										.split(',')
										.map((quality) => quality.trim())
										.filter(Boolean)) as quality}<option value={quality}>{quality}</option
									>{/each}</select
							></label
						>
						<label
							>Upgrade until score<input
								type="number"
								step="1"
								bind:value={config.upgrade_until_format_score}
								required
							/></label
						>
					</div>
					<p class="connection-hint">
						Quality groups, best first. Comma-separated qualities in one group are equal; format
						scores break the tie.
					</p>
					{#each groups as group, index}
						<div class="quality-group">
							<label
								>Quality group {index + 1}<input
									type="text"
									bind:value={groups[index]}
									required
								/></label
							>
							<div class="connection-actions">
								<button
									type="button"
									class="connection-button"
									disabled={index === 0}
									on:click={() => moveGroup(index, -1)}
									aria-label={`Move quality group ${index + 1} up`}>↑</button
								><button
									type="button"
									class="connection-button"
									disabled={index === groups.length - 1}
									on:click={() => moveGroup(index, 1)}
									aria-label={`Move quality group ${index + 1} down`}>↓</button
								><button
									type="button"
									class="connection-button"
									disabled={groups.length === 1}
									on:click={() => (groups = groups.filter((_, i) => i !== index))}
									aria-label={`Remove quality group ${index + 1}`}>Remove</button
								>
							</div>
						</div>
					{/each}
					<button
						type="button"
						class="connection-button"
						on:click={() => (groups = [...groups, ''])}>Add quality group</button
					>
					<details class="format-overrides">
						<summary>Custom format scores</summary>
						<p class="connection-hint">
							Blank uses the guide score. Overrides are saved with this profile.
						</p>
						<label
							>Find custom format<input
								type="search"
								bind:value={formatFilter}
								placeholder="Search format name"
							/></label
						>
						{#each formats as format}<div class="format-row">
								<label
									>{format.name}<input
										type="number"
										step="1"
										value={format.name === 'Anime Dual Audio' && config.dual_audio !== 'optional'
											? { 'within-tier': 10, 'above-tier': 101, required: 2000 }[config.dual_audio]
											: (config.format_scores?.[format.id] ?? '')}
										disabled={format.name === 'Anime Dual Audio' &&
											config.dual_audio !== 'optional'}
										placeholder={String(format.score)}
										on:input={(event) => setFormatScore(format.id, event)}
										aria-label={`Score for ${format.name}`}
									/></label
								><span class="connection-hint">Guide: {format.score}</span><button
									type="button"
									class="connection-button"
									disabled={config.format_scores?.[format.id] === undefined ||
										(format.name === 'Anime Dual Audio' && config.dual_audio !== 'optional')}
									on:click={() => resetFormatScore(format.id)}
									aria-label={`Reset ${format.name} score`}>Reset</button
								>
							</div>{/each}
						{#if !formats.length}<p class="connection-hint">No matching formats.</p>{/if}
					</details>
					<div class="connection-grid">
						<label
							>Reject patterns (JSON array)<input
								type="text"
								bind:value={profile.reject_patterns}
								required
							/></label
						>
					</div>
				</details>
			{:else}
				<div class="connection-grid">
					<label
						>Profile language<select bind:value={profile.language}
							><option value="en">English</option><option value="ja">Japanese</option><option
								value="ja-en">Japanese with English subtitles</option
							><option value="any">Any language</option></select
						></label
					><label
						>Qualities (JSON array, best first)<input
							type="text"
							bind:value={profile.qualities}
							required
						/></label
					><label
						>Tag bonuses (JSON object)<input
							type="text"
							bind:value={profile.tags}
							required
						/></label
					><label
						>Reject patterns (JSON array)<input
							type="text"
							bind:value={profile.reject_patterns}
							required
						/></label
					>
				</div>
			{/if}
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
	{#if presetError}<p class="connection-error" role="alert">{presetError}</p>{/if}
	{#if error}<p class="connection-error" role="alert">{error}</p>{/if}
</section>

<style>
	.profile-description {
		min-width: 0;
		overflow-wrap: anywhere;
	}
	summary {
		cursor: pointer;
		font-weight: 600;
	}
	.preset-summary {
		padding: 0.8rem 1rem;
		background: var(--bg-input);
		border-radius: var(--radius-sm);
		font-size: 0.85rem;
		line-height: 1.6;
	}
	.preset-summary p {
		margin: 0;
	}
	.advanced-scoring,
	.format-overrides {
		margin: 1rem 0;
		padding: 1rem 0;
		border-top: 1px solid var(--glass-border);
	}
	.quality-group,
	.format-row {
		display: flex;
		gap: 0.65rem;
		align-items: end;
		margin: 0.75rem 0;
	}
	.quality-group label,
	.format-row label {
		flex: 1;
		min-width: 0;
	}
	.format-row > .connection-hint {
		padding-bottom: 0.65rem;
		white-space: nowrap;
	}
	.format-row {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 7rem auto;
	}
	@media (max-width: 600px) {
		.quality-group,
		.format-row {
			flex-wrap: wrap;
		}
		.quality-group label,
		.format-row label {
			flex-basis: 100%;
		}
		.format-row {
			grid-template-columns: 1fr auto;
		}
		.format-row label {
			grid-column: 1 / -1;
		}
	}
</style>
