<script>
	import { api } from '$lib/api';
	import './connections.css';
	let servers = [];
	let loaded = false;
	let busy = false;
	let error = '';
	let message = '';
	let editing = null;
	let form = defaults();
	function defaults() {
		return {
			name: '',
			host: '',
			port: 563,
			username: '',
			password: '',
			ssl: true,
			connections: 20,
			enabled: true,
			priority: 0
		};
	}
	async function load() {
		if (busy) return;
		busy = true;
		error = '';
		try {
			servers = await api.getUsenetServers();
			loaded = true;
		} catch (e) {
			error = e.message;
		} finally {
			busy = false;
		}
	}
	function edit(server) {
		editing = server.id;
		form = { ...defaults(), ...server, password: '' };
		error = '';
		message = '';
	}
	function reset() {
		editing = null;
		form = defaults();
	}
	function preset() {
		reset();
		form = { ...defaults(), name: 'Newsgroup Ninja', host: 'news.newsgroup.ninja' };
	}
	async function save() {
		if (busy) return;
		if (editing === null && !form.password) {
			error = 'Enter your Usenet provider password.';
			return;
		}
		busy = true;
		error = '';
		message = '';
		try {
			const data = {
				name: form.name.trim(),
				host: form.host.trim(),
				port: Number(form.port),
				username: form.username,
				ssl: form.ssl,
				connections: Number(form.connections),
				enabled: form.enabled,
				priority: Number(form.priority)
			};
			if (form.password) data.password = form.password;
			if (editing !== null) await api.updateUsenetServer(editing, data);
			else await api.createUsenetServer(data);
			reset();
			message = 'Usenet server saved.';
			servers = await api.getUsenetServers();
			loaded = true;
		} catch (e) {
			error = e.message;
		} finally {
			busy = false;
		}
	}
	async function action(server, kind) {
		if (busy) return;
		if (kind === 'delete' && !confirm(`Delete ${server.name || server.host}?`)) return;
		busy = true;
		error = '';
		message = '';
		try {
			if (kind === 'test') {
				const result = await api.testUsenetServer(server.id);
				if (!result.success) throw new Error(result.error || 'Connection failed');
				message = `${server.name || server.host}: connection successful.`;
			} else {
				await api.deleteUsenetServer(server.id);
				if (editing === server.id) reset();
				servers = await api.getUsenetServers();
			}
		} catch (e) {
			error = e.message;
		} finally {
			busy = false;
		}
	}
</script>

<details
	class="connection-section"
	on:toggle={(event) => {
		if (event.currentTarget.open && !loaded) load();
	}}
>
	<summary>Usenet providers & backbones</summary>
	<p class="connection-hint">
		Save your SABnzbd connection first, then add the news server supplied by your Usenet
		subscription. These settings are saved directly in SABnzbd.
	</p>
	<button class="connection-button" on:click={load} disabled={busy}
		>{busy ? 'Working…' : 'Refresh servers'}</button
	>
	{#if loaded && !servers.length}<p class="connection-hint">
			No Usenet servers configured yet.
		</p>{/if}
	{#each servers as server}<div class="connection-item">
			<div>
				<strong>{server.name || server.host}</strong>
				<p class="connection-hint">
					{server.host}:{server.port} · {server.ssl ? 'TLS' : 'Unencrypted'} · {server.enabled
						? 'Enabled'
						: 'Disabled'}
				</p>
			</div>
			<div class="connection-actions">
				<button
					class="connection-button"
					disabled={busy}
					on:click={() => edit(server)}
					aria-label={`Edit server ${server.name || server.host}`}>Edit</button
				><button
					class="connection-button"
					disabled={busy}
					on:click={() => action(server, 'test')}
					aria-label={`Test server ${server.name || server.host}`}>Test</button
				><button
					class="connection-button danger"
					disabled={busy}
					on:click={() => action(server, 'delete')}
					aria-label={`Delete server ${server.name || server.host}`}>Delete</button
				>
			</div>
		</div>{/each}
	<form class="connection-editor" on:submit|preventDefault={save}>
		<h3>{editing !== null ? 'Edit news server' : 'Add news server'}</h3>
		{#if editing === null}<button type="button" class="connection-button" on:click={preset}
				>Use Newsgroup Ninja defaults</button
			>{/if}
		<div class="connection-grid">
			<label>Server name<input type="text" bind:value={form.name} required /></label>
			<label
				>News server host<input
					type="text"
					bind:value={form.host}
					placeholder="news.newsgroup.ninja"
					required
				/></label
			>
			<label
				>News server port<input
					type="number"
					bind:value={form.port}
					min="1"
					max="65535"
					step="1"
					required
				/></label
			>
			<label
				>Usenet username<input
					type="text"
					bind:value={form.username}
					autocomplete="username"
					required
				/></label
			>
			<label
				>Usenet password <span class="connection-status"
					>{form.password_configured ? 'Saved' : ''}</span
				><input
					type="password"
					bind:value={form.password}
					autocomplete="new-password"
					placeholder={editing !== null
						? 'Leave blank to keep saved password'
						: 'Provider account password'}
				/></label
			>
			<label
				>Connections<input
					type="number"
					min="1"
					max="100"
					step="1"
					bind:value={form.connections}
					required
				/></label
			>
			<label
				>Server priority<input
					type="number"
					min="0"
					step="1"
					bind:value={form.priority}
					required
				/></label
			>
		</div>
		<div class="connection-actions">
			<label class="connection-check"
				><input type="checkbox" bind:checked={form.ssl} /> Use TLS</label
			><label class="connection-check"
				><input type="checkbox" bind:checked={form.enabled} /> Enable server</label
			>
		</div>
		<div class="connection-actions">
			<button type="submit" class="connection-button primary" disabled={busy}
				>{editing !== null ? 'Save server' : 'Add server'}</button
			>{#if editing !== null}<button
					type="button"
					class="connection-button"
					on:click={reset}
					disabled={busy}>Cancel edit</button
				>{/if}
		</div>
	</form>
	{#if error}<p class="connection-error" role="alert">{error}</p>{/if}
	{#if message}<p class="connection-success" role="status">{message}</p>{/if}
</details>
