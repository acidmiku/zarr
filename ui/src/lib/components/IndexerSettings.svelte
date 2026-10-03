<script>
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import './connections.css';
	let indexers = [];
	let loading = true;
	let error = '';
	let message = '';
	let busy = false;
	let editing = null;
	let form = defaults();
	function defaults(type = 'newznab') {
		return {
			name: '',
			url: type === 'rutracker' ? 'https://rutracker.org' : '',
			type,
			api_key: '',
			username: '',
			password: '',
			priority: 0,
			enabled: true,
			content_types: ['movie', 'series', 'anime', 'music']
		};
	}
	onMount(load);
	async function load() {
		loading = true;
		try {
			indexers = await api.getIndexers();
			error = '';
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	}
	function edit(item) {
		editing = item.id;
		let types = item.content_types;
		if (typeof types === 'string') {
			try {
				types = JSON.parse(types);
			} catch {
				types = [];
			}
		}
		form = {
			...defaults(item.type),
			...item,
			api_key: '',
			password: '',
			content_types: Array.isArray(types) ? [...types] : defaults().content_types
		};
		error = '';
		message = '';
	}
	function reset() {
		editing = null;
		form = defaults();
	}
	function preset() {
		editing = null;
		form = { ...defaults(), name: 'NZBGeek', url: 'https://api.nzbgeek.info/api' };
	}
	async function save() {
		if (busy) return;
		error = '';
		message = '';
		if (!form.content_types.length) {
			error = 'Select at least one content type.';
			return;
		}
		if (editing === null && form.type === 'newznab' && !form.api_key) {
			error = 'Enter the indexer API key.';
			return;
		}
		if (form.type === 'rutracker' && (!form.username || (editing === null && !form.password))) {
			error = 'Enter the tracker username and password.';
			return;
		}
		busy = true;
		try {
			const data = {
				name: form.name.trim(),
				url: form.url.trim(),
				type: form.type,
				username: form.username,
				priority: Number(form.priority),
				enabled: form.enabled,
				content_types: form.content_types
			};
			if (form.api_key) data.api_key = form.api_key;
			if (form.password) data.password = form.password;
			if (editing !== null) await api.updateIndexer(editing, data);
			else await api.createIndexer(data);
			message = editing !== null ? 'Indexer updated.' : 'Indexer added.';
			reset();
			await load();
		} catch (e) {
			error = e.message;
		} finally {
			busy = false;
		}
	}
	async function action(item, kind) {
		if (busy) return;
		if (kind === 'delete' && !confirm(`Delete ${item.name}?`)) return;
		busy = true;
		error = '';
		message = '';
		try {
			if (kind === 'test') {
				const result = await api.testIndexer(item.id);
				if (!result.success) throw new Error(result.error || 'Connection failed');
				message = `${item.name}: connection successful.`;
			} else {
				if (kind === 'delete') {
					await api.deleteIndexer(item.id);
					if (editing === item.id) reset();
				} else await api.updateIndexer(item.id, { enabled: !item.enabled });
				await load();
			}
		} catch (e) {
			error = e.message;
		} finally {
			busy = false;
		}
	}
</script>

<section class="connection-section" aria-labelledby="indexers-heading">
	<h2 id="indexers-heading">Indexers</h2>
	<p class="connection-hint">
		Indexers find releases; download clients retrieve them. Add a Newznab Usenet indexer or a
		supported torrent tracker.
	</p>
	{#if loading}<p role="status">Loading indexers…</p>{:else if !indexers.length}<p
			class="connection-hint"
		>
			No indexers yet. You can still browse your library.
		</p>{/if}
	{#each indexers as item}
		<div class="connection-item">
			<div>
				<strong>{item.name}</strong><span class="connection-status"
					>{item.type === 'rutracker' ? 'Torrent' : 'Usenet'} · {item.enabled
						? 'Enabled'
						: 'Disabled'}</span
				>
				<p class="connection-hint">{item.url}</p>
			</div>
			<div class="connection-actions">
				<button
					class="connection-button"
					disabled={busy}
					on:click={() => edit(item)}
					aria-label={`Edit ${item.name}`}>Edit</button
				>
				<button
					class="connection-button"
					disabled={busy}
					on:click={() => action(item, 'toggle')}
					aria-label={`${item.enabled ? 'Disable' : 'Enable'} ${item.name}`}
					>{item.enabled ? 'Disable' : 'Enable'}</button
				>
				<button
					class="connection-button"
					disabled={busy}
					on:click={() => action(item, 'test')}
					aria-label={`Test ${item.name}`}>Test</button
				>
				<button
					class="connection-button danger"
					disabled={busy}
					on:click={() => action(item, 'delete')}
					aria-label={`Delete ${item.name}`}>Delete</button
				>
			</div>
		</div>
	{/each}
	<form class="connection-editor" on:submit|preventDefault={save}>
		<h3>{editing !== null ? 'Edit indexer' : 'Add indexer'}</h3>
		{#if editing === null}<button type="button" class="connection-button" on:click={preset}
				>Use NZBGeek defaults</button
			>{/if}
		<div class="connection-grid">
			<label
				>Indexer type<select
					bind:value={form.type}
					disabled={editing !== null}
					on:change={(event) => {
						form = defaults(event.currentTarget.value);
					}}
					><option value="newznab">Newznab · Usenet</option><option value="rutracker"
						>Rutracker · Torrent</option
					></select
				></label
			>
			<label>Indexer name<input type="text" bind:value={form.name} required /></label>
			<label
				>Indexer URL<input
					type="url"
					bind:value={form.url}
					required
					placeholder="https://api.nzbgeek.info/api"
				/></label
			>
			{#if form.type === 'newznab'}<label
					>Indexer API key <span class="connection-status"
						>{form.api_key_configured ? 'Saved' : ''}</span
					><input
						type="password"
						autocomplete="new-password"
						bind:value={form.api_key}
						placeholder={editing !== null ? 'Leave blank to keep saved key' : 'Enter API key'}
					/></label
				>{:else}<label
					>Tracker username<input
						type="text"
						bind:value={form.username}
						autocomplete="username"
						required
					/></label
				><label
					>Tracker password <span class="connection-status"
						>{form.password_configured ? 'Saved' : ''}</span
					><input
						type="password"
						bind:value={form.password}
						autocomplete="new-password"
						placeholder={editing !== null ? 'Leave blank to keep saved password' : 'Enter password'}
					/></label
				>{/if}
			<label
				>Indexer priority<input
					type="number"
					min="0"
					step="1"
					bind:value={form.priority}
					required
				/></label
			>
		</div>
		<div class="connection-actions">
			{#each ['movie', 'series', 'anime', 'music'] as type}<label class="connection-check"
					><input type="checkbox" bind:group={form.content_types} value={type} />{type}</label
				>{/each}
		</div>
		<label class="connection-check"
			><input type="checkbox" bind:checked={form.enabled} /> Enabled</label
		>
		<div class="connection-actions">
			<button type="submit" class="connection-button primary" disabled={busy}
				>{busy ? 'Working…' : editing !== null ? 'Save indexer' : 'Add indexer'}</button
			>{#if editing !== null}<button
					type="button"
					class="connection-button"
					disabled={busy}
					on:click={reset}>Cancel edit</button
				>{/if}
		</div>
	</form>
	{#if error}<p class="connection-error" role="alert">{error}</p>{/if}
	{#if message}<p class="connection-success" role="status">{message}</p>{/if}
</section>
