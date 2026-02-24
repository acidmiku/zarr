<script>
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/stores';
	import { api } from '$lib/api';
	import { setupComplete, notifications, theme, toggleTheme } from '$lib/stores/app';
	import SetupWizard from '$lib/components/SetupWizard.svelte';
	import '../app.css';

	let showSetup = false;
	let loaded = false;
	let activityCount = 0;
	let countInterval;

	onMount(async () => {
		try {
			const status = await api.getStatus();
			setupComplete.set(status.setup_complete);
			showSetup = !status.setup_complete;

			if (status.setup_complete) {
				updateActivityCount();
				countInterval = setInterval(updateActivityCount, 10000); // Update every 10s
			}
		} catch {
			showSetup = true;
		}
		loaded = true;
	});

	onDestroy(() => {
		if (countInterval) clearInterval(countInterval);
	});

	async function updateActivityCount() {
		try {
			// Get active downloads count (API returns array directly)
			const downloads = await api.getDownloads();
			if (Array.isArray(downloads)) {
				activityCount = downloads.filter(d =>
					d.status === 'downloading' || d.status === 'queued' || d.status === 'processing'
				).length;
			} else {
				activityCount = 0;
			}
		} catch (err) {
			// Silently fail
			activityCount = 0;
		}
	}

	function onSetupComplete() {
		showSetup = false;
		setupComplete.set(true);
		updateActivityCount();
		countInterval = setInterval(updateActivityCount, 10000);
	}

	$: currentPath = $page.url.pathname;

	const navItems = [
		{ path: '/', label: 'Discover', icon: '⌕' },
		{ path: '/library', label: 'Library', icon: '▤' },
		{ path: '/ratings', label: 'Ratings', icon: '★' },
		{ path: '/activity', label: 'Activity', icon: '↓' },
		{ path: '/assistant', label: 'Assistant', icon: '✦' },
		{ path: '/settings', label: 'Settings', icon: '⚙' }
	];
</script>

{#if !loaded}
	<div class="loading-screen">
		<div class="spinner"></div>
		<p>Loading Zarr...</p>
	</div>
{:else if showSetup}
	<SetupWizard on:complete={onSetupComplete} />
{:else}
	<div class="app">
		<nav class="sidebar">
			<div class="sidebar-glow"></div>
			<div class="logo">
				<span class="logo-icon">⚒</span>
				<span class="logo-text">Zarr</span>
			</div>
			<div class="nav-links">
				{#each navItems as item}
					<a
						href={item.path}
						class="nav-link"
						class:active={currentPath === item.path || (item.path !== '/' && currentPath.startsWith(item.path))}
					>
						{#if currentPath === item.path || (item.path !== '/' && currentPath.startsWith(item.path))}
							<span class="nav-active-indicator"></span>
						{/if}
						<span class="nav-icon">{item.icon}</span>
						<span class="nav-label">{item.label}</span>
						{#if item.path === '/activity' && activityCount > 0}
							<span class="nav-badge">{activityCount}</span>
						{/if}
					</a>
				{/each}
			</div>
			<div class="sidebar-footer">
				<button class="theme-toggle" on:click={toggleTheme} title="Toggle theme">
					{$theme === 'dark' ? '☀' : '☽'}
				</button>
			</div>
		</nav>
		<main class="content">
			<slot />
		</main>
	</div>
{/if}

<!-- Notifications -->
<div class="notifications">
	{#each $notifications as notif (notif.id)}
		<div class="notif notif-{notif.type}">{notif.message}</div>
	{/each}
</div>

<style>
	.loading-screen {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		height: 100vh;
		gap: 1rem;
		color: var(--text-muted);
	}

	.spinner {
		width: 32px;
		height: 32px;
		border: 3px solid var(--border);
		border-top-color: var(--accent);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin { to { transform: rotate(360deg); } }

	.app {
		display: flex;
		min-height: 100vh;
	}

	/* ---- Sidebar ---- */
	.sidebar {
		width: 220px;
		position: fixed;
		top: 0;
		left: 0;
		bottom: 0;
		z-index: 100;
		display: flex;
		flex-direction: column;
		background: var(--glass-bg);
		backdrop-filter: blur(var(--glass-blur));
		-webkit-backdrop-filter: blur(var(--glass-blur));
		border-right: 1px solid var(--glass-border);
		overflow: hidden;
	}

	.sidebar-glow {
		position: absolute;
		top: -60px;
		left: 30%;
		width: 120px;
		height: 120px;
		background: radial-gradient(circle, var(--accent-glow) 0%, transparent 70%);
		pointer-events: none;
		z-index: 0;
	}

	.logo {
		padding: 1.25rem 1rem;
		display: flex;
		align-items: center;
		gap: 0.6rem;
		border-bottom: 1px solid var(--border);
		position: relative;
		z-index: 1;
	}

	.logo-icon { font-size: 1.5rem; }
	.logo-text {
		font-family: var(--font-display);
		font-size: 1.1rem;
		font-weight: 700;
		background: linear-gradient(135deg, var(--accent), var(--accent-hover));
		-webkit-background-clip: text;
		-webkit-text-fill-color: transparent;
		background-clip: text;
		text-shadow: none;
	}

	.nav-links {
		padding: 0.75rem 0.5rem;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		flex: 1;
		position: relative;
		z-index: 1;
	}

	.nav-link {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.6rem 0.85rem;
		border-radius: var(--radius-sm);
		color: var(--text-secondary);
		transition: all 0.2s ease;
		font-family: var(--font-body);
		font-size: 0.88rem;
		font-weight: 500;
		position: relative;
		overflow: hidden;
	}

	.nav-link:hover {
		color: var(--text-primary);
		background: var(--bg-hover);
	}

	.nav-link.active {
		color: var(--text-primary);
		background: var(--bg-active);
	}

	.nav-link.active .nav-icon {
		color: var(--accent);
	}

	.nav-active-indicator {
		position: absolute;
		left: 0;
		top: 20%;
		bottom: 20%;
		width: 3px;
		border-radius: 0 2px 2px 0;
		background: var(--accent);
		box-shadow: 0 0 12px var(--accent-glow), 0 0 4px var(--accent);
	}

	.nav-icon {
		font-size: 1.1rem;
		width: 1.5rem;
		text-align: center;
		transition: color 0.2s;
	}

	.nav-badge {
		margin-left: auto;
		padding: 0.15rem 0.45rem;
		background: var(--accent);
		color: var(--text-inverse);
		font-size: 0.7rem;
		font-weight: 700;
		border-radius: 10px;
		min-width: 20px;
		text-align: center;
		box-shadow: 0 0 8px var(--accent-glow);
	}

	.sidebar-footer {
		padding: 0.75rem;
		border-top: 1px solid var(--border);
		display: flex;
		justify-content: center;
		position: relative;
		z-index: 1;
	}

	.theme-toggle {
		width: 36px;
		height: 36px;
		border-radius: 50%;
		border: 1px solid var(--border);
		background: var(--bg-hover);
		color: var(--text-secondary);
		font-size: 1.1rem;
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: pointer;
		transition: all 0.2s;
	}
	.theme-toggle:hover {
		color: var(--accent);
		border-color: var(--accent);
		box-shadow: var(--shadow-glow);
	}

	/* ---- Content ---- */
	.content {
		flex: 1;
		margin-left: 220px;
		padding: 1.5rem 2rem;
		min-height: 100vh;
	}

	/* ---- Notifications ---- */
	.notifications {
		position: fixed;
		bottom: 1rem;
		right: 1rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		z-index: 10001;
	}

	.notif {
		padding: 0.75rem 1.25rem;
		border-radius: var(--radius-md);
		font-size: 0.85rem;
		font-family: var(--font-body);
		animation: slideIn 0.25s ease;
		max-width: 360px;
		backdrop-filter: blur(16px);
		-webkit-backdrop-filter: blur(16px);
	}

	.notif-success { background: var(--success-bg); border: 1px solid var(--success-border); color: var(--success); }
	.notif-error { background: var(--danger-bg); border: 1px solid var(--danger-border); color: var(--danger); }
	.notif-info { background: var(--info-bg); border: 1px solid var(--info-border); color: var(--info); }

	@keyframes slideIn {
		from { transform: translateX(100%); opacity: 0; }
		to { transform: translateX(0); opacity: 1; }
	}

	/* ---- Mobile ---- */
	@media (max-width: 768px) {
		.sidebar {
			width: 100%;
			height: auto;
			position: fixed;
			top: auto;
			bottom: 0;
			flex-direction: row;
			border-right: none;
			border-top: 1px solid var(--glass-border);
		}

		.logo, .sidebar-footer, .sidebar-glow { display: none; }

		.nav-links {
			flex-direction: row;
			width: 100%;
			justify-content: space-around;
			padding: 0.5rem;
		}

		.nav-label { display: none; }
		.nav-active-indicator { display: none; }

		.nav-link {
			padding: 0.5rem 0.75rem;
			justify-content: center;
		}

		.content {
			margin-left: 0;
			padding: 1rem;
			padding-bottom: 4rem;
		}
	}
</style>
