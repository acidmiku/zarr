<script>
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import {
		Compass,
		Library,
		Disc3,
		Star,
		FolderInput,
		Download,
		Sparkles,
		Settings,
		Search,
		Menu,
		X,
		Palette,
		ArrowRight
	} from 'lucide-svelte';
	import { api } from '$lib/api';
	import {
		setupComplete,
		notifications,
		theme,
		setTheme,
		THEMES,
		downloads
	} from '$lib/stores/app';
	import { pollDownloads, isActiveDownload } from '$lib/stores/queue';
	import SetupWizard from '$lib/components/SetupWizard.svelte';
	import '@fontsource/barlow-condensed/400.css';
	import '@fontsource/barlow-condensed/700.css';
	import '@fontsource/barlow-condensed/800.css';
	import '@fontsource/inter/400.css';
	import '@fontsource/inter/500.css';
	import '@fontsource/inter/600.css';
	import '@fontsource/inter/700.css';
	import '../app.css';
	import '../archive.css';

	let showSetup = false;
	let loaded = false;
	let bootError = '';
	let mobileOpen = false;
	let themesOpen = false;
	let searchInput;
	let query = '';
	let searchType = 'movie';
	let stopPolling = () => {};
	let disposed = false;
	const navItems = [
		{ path: '/', label: 'Discover', icon: Compass },
		{ path: '/library', label: 'Collection', icon: Library },
		{ path: '/music', label: 'Music', icon: Disc3 },
		{ path: '/assistant', label: 'Assistant', icon: Sparkles },
		{ path: '/activity', label: 'Activity', icon: Download },
		{ path: '/ratings', label: 'Ratings', icon: Star },
		{ path: '/import', label: 'Import', icon: FolderInput },
		{ path: '/settings', label: 'Settings', icon: Settings }
	];
	$: activePath = navItems.find((item) =>
		item.path === '/' ? $page.url.pathname === '/' : $page.url.pathname.startsWith(item.path)
	)?.path;
	$: activityCount = $downloads.filter(isActiveDownload).length;
	$: {
		$page.url;
		mobileOpen = false;
		themesOpen = false;
	}

	onMount(() => {
		setTheme($theme);
		boot();
		return () => {
			disposed = true;
			stopPolling();
		};
	});
	async function boot() {
		loaded = false;
		bootError = '';
		try {
			const status = await api.getStatus();
			if (disposed) return;
			setupComplete.set(status.setup_complete);
			showSetup = !status.setup_complete;
			if (status.setup_complete) {
				stopPolling();
				stopPolling = pollDownloads();
			}
		} catch (error) {
			bootError = error.message || 'Cannot connect to Zarr.';
		}
		loaded = true;
	}
	function onSetupComplete() {
		showSetup = false;
		setupComplete.set(true);
		stopPolling();
		stopPolling = pollDownloads();
	}
	function search() {
		if (!query.trim()) {
			searchInput?.focus();
			return;
		}
		const params = new URLSearchParams({
			q: query.trim(),
			type: searchType === 'music' ? 'album' : searchType
		});
		goto(`${searchType === 'music' ? '/music' : '/'}?${params}`);
	}
	function onKeydown(event) {
		if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
			event.preventDefault();
			searchInput?.focus();
		}
		if (event.key === 'Escape') {
			mobileOpen = false;
			themesOpen = false;
		}
	}
</script>

<svelte:window on:keydown={onKeydown} />

{#if !loaded}
	<div class="archive-boot" role="status">
		<span class="wordmark">zarr</span><span class="eyebrow">Opening your archive</span>
	</div>
{:else if bootError}
	<div class="archive-boot">
		<span class="wordmark">zarr</span>
		<h1>Connection interrupted</h1>
		<p>{bootError}</p>
		<button class="archive-button" on:click={boot}>Try again <ArrowRight size={16} /></button>
	</div>
{:else if showSetup}
	<SetupWizard on:complete={onSetupComplete} />
{:else}
	<a class="skip-link" href="#main-content">Skip to content</a>
	<aside class="archive-rail">
		<a href="/library" class="rail-logo" aria-label="Zarr collection">zarr</a>
		<nav aria-label="Main navigation">
			{#each navItems as item}
				<a
					href={item.path}
					class:active={activePath === item.path}
					aria-current={activePath === item.path ? 'page' : undefined}
					aria-label={item.label}
					title={item.label}
				>
					<svelte:component this={item.icon} size={22} strokeWidth={1.6} />
					{#if item.path === '/activity' && activityCount}<span class="rail-count"
							>{activityCount}</span
						>{/if}
				</a>
			{/each}
		</nav>
		<div class="rail-bottom">
			<span class="rail-caption">YOUR MEDIA.<br />YOUR RULES.</span><span class="rail-dot"></span>
		</div>
	</aside>
	<div class="archive-shell">
		<header class="archive-masthead">
			<a class="masthead-brand" href="/library" aria-label="Zarr Archive"
				><span>ZARR<span class="brand-slash">/</span><b>ARCHIVE</b></span><small
					>DISCOVER<br />DOWNLOAD<br />ORGANIZE</small
				></a
			>
			<div class="masthead-tools">
				<nav class="top-nav" aria-label="Quick navigation">
					{#each navItems.slice(0, 5) as item}<a
							href={item.path}
							class:active={activePath === item.path}
							aria-current={activePath === item.path ? 'page' : undefined}
							>{item.label}{#if item.path === '/activity' && activityCount}<span
									>{activityCount}</span
								>{/if}</a
						>{/each}
				</nav>
				<form
					class="global-search"
					on:submit|preventDefault={search}
					role="search"
					aria-label="Search media"
				>
					<Search size={17} /><input
						bind:this={searchInput}
						bind:value={query}
						aria-label="Search titles"
						placeholder="Search the archive…"
					/>
					<select bind:value={searchType} aria-label="Search media type"
						><option value="movie">Movies</option><option value="series">Series</option><option
							value="anime">Anime</option
						><option value="music">Music</option></select
					>
					<button aria-label="Search" title="Search (Ctrl+K to focus)"
						><ArrowRight size={17} /></button
					>
				</form>
			</div>
			<div class="shell-actions">
				<div class="theme-control">
					<button
						class="icon-button"
						aria-label="Choose theme"
						aria-expanded={themesOpen}
						on:click={() => (themesOpen = !themesOpen)}><Palette size={19} /></button
					>
					{#if themesOpen}<div class="theme-menu">
							<span class="eyebrow">Color signal</span>{#each THEMES as t}<button
									class:chosen={$theme === t.id}
									aria-pressed={$theme === t.id}
									on:click={() => {
										setTheme(t.id);
										themesOpen = false;
									}}><span style="background:{t.swatch}"></span>{t.label}</button
								>{/each}
						</div>{/if}
				</div>
				<button
					class="icon-button mobile-toggle"
					aria-label={mobileOpen ? 'Close navigation' : 'Open navigation'}
					aria-expanded={mobileOpen}
					aria-controls="mobile-navigation"
					on:click={() => (mobileOpen = !mobileOpen)}
					>{#if mobileOpen}<X size={22} />{:else}<Menu size={22} />{/if}</button
				>
			</div>
		</header>
		{#if mobileOpen}<nav id="mobile-navigation" class="mobile-menu" aria-label="Mobile navigation">
				{#each navItems as item}<a href={item.path} class:active={activePath === item.path}
						><svelte:component
							this={item.icon}
							size={19}
						/>{item.label}{#if item.path === '/activity' && activityCount}<span
								>{activityCount}</span
							>{/if}</a
					>{/each}
			</nav>{/if}
		<main class="content" id="main-content" tabindex="-1"><slot /></main>
		<footer class="archive-footer">
			<span>ZARR <b>/</b> SELF-HOSTED MEDIA ARCHIVE</span><span>COLLECT WHAT MOVES YOU.</span>
		</footer>
	</div>
{/if}
<div class="archive-notifications" aria-live="polite" aria-atomic="false">
	{#each $notifications as notif (notif.id)}<div
			class="archive-notif"
			class:error={notif.type === 'error'}
		>
			{notif.message}
		</div>{/each}
</div>
