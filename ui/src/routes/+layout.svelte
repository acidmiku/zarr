<script>
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/stores';
	import { api } from '$lib/api';
	import { setupComplete, notifications, theme, setTheme, THEMES } from '$lib/stores/app';
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
				countInterval = setInterval(updateActivityCount, 10000);
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
			const downloads = await api.getDownloads();
			if (Array.isArray(downloads)) {
				activityCount = downloads.filter(d =>
					d.status === 'downloading' || d.status === 'queued' || d.status === 'processing'
				).length;
			} else {
				activityCount = 0;
			}
		} catch {
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
		{ path: '/music', label: 'Music', icon: '♫' },
		{ path: '/ratings', label: 'Ratings', icon: '★' },
		{ path: '/activity', label: 'Activity', icon: '↓' },
		{ path: '/assistant', label: 'Assistant', icon: '✦' },
		{ path: '/settings', label: 'Settings', icon: '⚙' }
	];

	function isActive(path) {
		if (path === '/') return currentPath === '/';
		return currentPath.startsWith(path);
	}
</script>

{#if !loaded}
	<div class="boot-screen">
		<div class="boot-icon">⚒</div>
	</div>
{:else if showSetup}
	<SetupWizard on:complete={onSetupComplete} />
{:else}
	<!-- Sidebar -->
	<nav class="sidebar" aria-label="Main navigation">
		<div class="sidebar-inner">
			<a href="/" class="sidebar-logo">
				<span class="logo-icon">⚒</span>
				<span class="logo-text">Zarr</span>
			</a>

			<div class="nav-links">
				{#each navItems as item}
					<a
						href={item.path}
						class="nav-item"
						class:active={isActive(item.path)}
					>
						{#if isActive(item.path)}
							<span class="active-bar"></span>
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
				<div class="theme-swatches">
					{#each THEMES as t}
						<button
							class="swatch"
							class:active={$theme === t.id}
							style="--sw: {t.swatch}"
							title={t.label}
							on:click={() => setTheme(t.id)}
						></button>
					{/each}
				</div>
			</div>
		</div>
	</nav>

	<!-- Mobile bottom nav -->
	<nav class="mobile-nav">
		{#each navItems.slice(0, 5) as item}
			<a href={item.path} class="mobile-item" class:active={isActive(item.path)}>
				<span class="mobile-icon">{item.icon}</span>
				{#if item.path === '/activity' && activityCount > 0}
					<span class="mobile-badge">{activityCount}</span>
				{/if}
			</a>
		{/each}
	</nav>

	<main class="content">
		<slot />
	</main>
{/if}

<!-- Notifications -->
<div class="notifications">
	{#each $notifications as notif (notif.id)}
		<div class="notif notif-{notif.type}">
			<span class="notif-dot"></span>
			{notif.message}
		</div>
	{/each}
</div>

<style>
	/* ===== Boot ===== */
	.boot-screen {
		position: fixed;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--bg-base);
	}
	.boot-icon {
		font-size: 3rem;
		opacity: 0.25;
		animation: pulse 2s ease-in-out infinite;
	}

	/* ===== Sidebar ===== */
	.sidebar {
		position: fixed;
		top: 0;
		left: 0;
		bottom: 0;
		width: var(--sidebar-width);
		z-index: 100;
		transition: width 0.35s cubic-bezier(0.16, 1, 0.3, 1);
		overflow: hidden;
	}
	.sidebar:hover {
		width: var(--sidebar-expanded);
	}

	.sidebar-inner {
		height: 100%;
		display: flex;
		flex-direction: column;
		padding: 0.5rem;
		background: var(--glass-bg);
		backdrop-filter: blur(var(--glass-blur));
		-webkit-backdrop-filter: blur(var(--glass-blur));
		border-right: 1px solid var(--glass-border);
	}

	/* Logo */
	.sidebar-logo {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 1rem 0.85rem;
		margin-bottom: 0.25rem;
		text-decoration: none;
		white-space: nowrap;
		overflow: hidden;
	}
	.logo-icon {
		font-size: 1.5rem;
		flex-shrink: 0;
		width: 36px;
		text-align: center;
		filter: drop-shadow(0 0 12px var(--accent-glow));
	}
	.logo-text {
		font-family: var(--font-display);
		font-size: 1.25rem;
		font-weight: 800;
		letter-spacing: -0.03em;
		background: linear-gradient(135deg, var(--accent), var(--accent-hover));
		-webkit-background-clip: text;
		-webkit-text-fill-color: transparent;
		background-clip: text;
		opacity: 0;
		transform: translateX(-6px);
		transition: opacity 0.25s ease 0.1s, transform 0.3s ease 0.1s;
	}
	.sidebar:hover .logo-text {
		opacity: 1;
		transform: translateX(0);
	}

	/* Nav links */
	.nav-links {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: 0.25rem 0;
	}
	.nav-item {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.65rem 0.85rem;
		border-radius: var(--radius-sm);
		color: var(--text-muted);
		text-decoration: none;
		white-space: nowrap;
		overflow: hidden;
		position: relative;
		transition: color 0.2s, background 0.2s;
	}
	.nav-item:hover {
		color: var(--text-primary);
		background: var(--bg-hover);
	}
	.nav-item.active {
		color: var(--accent);
		background: var(--accent-subtle);
	}

	.active-bar {
		position: absolute;
		left: 0;
		top: 50%;
		transform: translateY(-50%);
		width: 3px;
		height: 20px;
		background: var(--accent);
		border-radius: 0 3px 3px 0;
		box-shadow: 0 0 12px var(--accent-glow), 0 0 4px var(--accent);
	}

	.nav-icon {
		font-size: 1.15rem;
		flex-shrink: 0;
		width: 36px;
		text-align: center;
		transition: transform 0.15s;
	}
	.nav-item:hover .nav-icon {
		transform: scale(1.1);
	}
	.nav-label {
		font-size: 0.85rem;
		font-weight: 500;
		opacity: 0;
		transform: translateX(-6px);
		transition: opacity 0.2s ease 0.06s, transform 0.2s ease 0.06s;
	}
	.sidebar:hover .nav-label {
		opacity: 1;
		transform: translateX(0);
	}
	.nav-badge {
		position: absolute;
		top: 6px;
		right: 10px;
		background: var(--accent);
		color: var(--text-inverse);
		font-size: 0;
		font-weight: 700;
		min-width: 8px;
		height: 8px;
		padding: 0;
		border-radius: 8px;
		display: flex;
		align-items: center;
		justify-content: center;
		box-shadow: 0 0 8px var(--accent-glow);
		transition: all 0.2s ease 0.06s;
	}
	.sidebar:hover .nav-badge {
		position: static;
		margin-left: auto;
		font-size: 0.65rem;
		min-width: 18px;
		height: 18px;
		padding: 0 5px;
	}

	/* Footer — theme swatches */
	.sidebar-footer {
		padding: 0.75rem 0.5rem;
		border-top: 1px solid var(--border);
	}
	.theme-swatches {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		justify-content: center;
	}
	.swatch {
		width: 16px;
		height: 16px;
		border-radius: 50%;
		background: var(--sw);
		border: 2px solid transparent;
		transition: all 0.2s;
		opacity: 0.5;
		flex-shrink: 0;
		cursor: pointer;
	}
	.swatch:hover {
		opacity: 1;
		transform: scale(1.2);
	}
	.swatch.active {
		opacity: 1;
		border-color: var(--text-primary);
		box-shadow: 0 0 10px var(--sw);
	}

	/* ===== Main content ===== */
	.content {
		margin-left: var(--sidebar-width);
		padding: 2rem 2.5rem;
		min-height: 100vh;
		position: relative;
		z-index: 1;
		animation: fadeIn 0.3s ease;
	}

	/* ===== Mobile nav ===== */
	.mobile-nav {
		display: none;
		position: fixed;
		bottom: 0;
		left: 0;
		right: 0;
		height: 58px;
		background: var(--glass-bg);
		backdrop-filter: blur(var(--glass-blur));
		-webkit-backdrop-filter: blur(var(--glass-blur));
		border-top: 1px solid var(--glass-border);
		z-index: 100;
		justify-content: space-around;
		align-items: center;
		padding: 0 0.5rem;
	}
	.mobile-item {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
		padding: 0.5rem 0.75rem;
		color: var(--text-muted);
		text-decoration: none;
		position: relative;
		transition: color 0.15s;
	}
	.mobile-item.active {
		color: var(--accent);
	}
	.mobile-icon {
		font-size: 1.2rem;
	}
	.mobile-badge {
		position: absolute;
		top: 2px;
		right: 4px;
		background: var(--accent);
		color: var(--text-inverse);
		font-size: 0.55rem;
		font-weight: 700;
		min-width: 14px;
		height: 14px;
		padding: 0 3px;
		border-radius: 7px;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	/* ===== Notifications ===== */
	.notifications {
		position: fixed;
		top: 1.25rem;
		right: 1.25rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		z-index: 10001;
		max-width: 360px;
	}
	.notif {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.7rem 1rem;
		border-radius: var(--radius-md);
		backdrop-filter: blur(20px);
		-webkit-backdrop-filter: blur(20px);
		font-size: 0.82rem;
		font-weight: 500;
		animation: slideUp 0.35s cubic-bezier(0.16, 1, 0.3, 1);
		box-shadow: var(--shadow-md);
		border: 1px solid;
	}
	.notif-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: currentColor;
		flex-shrink: 0;
	}
	.notif-success { background: var(--success-bg); border-color: var(--success-border); color: var(--success); }
	.notif-error { background: var(--danger-bg); border-color: var(--danger-border); color: var(--danger); }
	.notif-info { background: var(--info-bg); border-color: var(--info-border); color: var(--info); }

	/* ===== Responsive ===== */
	@media (max-width: 768px) {
		.sidebar { display: none; }
		.mobile-nav { display: flex; }
		.content {
			margin-left: 0;
			padding: 1.25rem;
			padding-bottom: calc(58px + 1.25rem);
		}
		.notifications {
			top: auto;
			bottom: calc(58px + 0.75rem);
			right: 0.75rem;
			left: 0.75rem;
			max-width: none;
		}
	}
</style>
