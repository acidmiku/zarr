import { test, expect } from '@playwright/test';
import { mockApi } from './fixtures.mjs';

test('switching indexer and profile types applies matching defaults', async ({ page }) => {
	await mockApi(page);
	await page.goto('/settings');
	await page.getByLabel('Indexer type').selectOption('rutracker');
	await expect(page.getByLabel('Indexer URL')).toHaveValue('https://rutracker.org');
	await expect(page.getByLabel('Tracker username')).toBeVisible();
	await page.getByText('Create quality profile', { exact: true }).click();
	await page.getByLabel('Profile media type').selectOption('music');
	await expect(page.getByLabel('Qualities (JSON array, best first)')).toHaveValue(
		'["flac","mp3-320"]'
	);
});

test('first run finishes without mandatory keys and keeps saved credentials', async ({ page }) => {
	const { requests } = await mockApi(page, { setup: false });
	await page.goto('/library');
	await expect(page.getByRole('heading', { name: 'Less setup. More discovery.' })).toBeVisible();
	await expect(page.getByLabel(/TMDB API key/)).toHaveValue('');
	await page.getByRole('button', { name: 'Start using Zarr' }).click();
	await expect(page.locator('#main-content')).toBeVisible();
	const save = requests.find((r) => r.path === '/api/settings' && r.method === 'PUT');
	expect(save.body).toMatchObject({
		setup_complete: true,
		media_root: '/media',
		qbittorrent_enabled: false,
		openrouter_reasoning_effort: 'high'
	});
	expect(save.body).not.toHaveProperty('tmdb_api_key');
	expect(save.body).not.toHaveProperty('openrouter_api_key');
});

test('settings retain custom model and saved secrets across unrelated saves', async ({ page }) => {
	const { requests } = await mockApi(page);
	await page.route('**/api/ai/models', (route) =>
		route.fulfill({ json: [{ id: 'unrelated/model', name: 'Another model' }] })
	);
	await page.goto('/settings');
	await page.getByLabel('OpenRouter model').fill('moonshotai/kimi-k3');
	await page.getByLabel(/Media folder/).fill('/data/archive');
	await page.getByRole('button', { name: 'Save connections' }).click();
	await expect(page.getByText(/Connections saved/)).toBeVisible();
	const save = requests.find((r) => r.path === '/api/settings' && r.method === 'PUT');
	expect(save.body).toMatchObject({
		media_root: '/data/archive',
		openrouter_model: 'moonshotai/kimi-k3',
		openrouter_reasoning_effort: 'high'
	});
	for (const key of [
		'tmdb_api_key',
		'sabnzbd_api_key',
		'qbittorrent_password',
		'openrouter_api_key'
	])
		expect(save.body).not.toHaveProperty(key);
	await expect(page.getByLabel('OpenRouter model')).toHaveValue('moonshotai/kimi-k3');
});

test('connection test uses unsaved credentials without saving them', async ({ page }) => {
	const { requests } = await mockApi(page);
	await page.route('**/api/settings/test-tmdb', async (route) => {
		expect(route.request().postDataJSON()).toEqual({ api_key: 'test-only-key' });
		await route.fulfill({ json: { success: true } });
	});
	await page.goto('/settings');
	await page.getByLabel(/TMDB API key/).fill('test-only-key');
	await page.getByRole('button', { name: 'Test TMDB', exact: true }).click();
	await expect(page.getByText('Connection successful', { exact: true })).toBeVisible();
	expect(requests.filter((r) => r.path === '/api/settings' && r.method === 'PUT')).toHaveLength(0);
});

test('indexer edit and enable toggle never echo masked credentials', async ({ page }) => {
	await mockApi(page);
	let item = {
		id: 1,
		name: 'Existing indexer',
		url: 'https://example.test/api',
		type: 'newznab',
		api_key: '',
		api_key_configured: true,
		password: '',
		enabled: true,
		priority: 0,
		content_types: ['movie', 'series']
	};
	const updates = [];
	await page.route('**/api/indexers', (route) => route.fulfill({ json: [item] }));
	await page.route('**/api/indexers/1', async (route) => {
		const data = route.request().postDataJSON();
		updates.push(data);
		item = { ...item, ...data };
		await route.fulfill({ json: { status: 'ok' } });
	});
	await page.goto('/settings');
	await page.getByRole('button', { name: 'Edit Existing indexer' }).click();
	await expect(page.getByLabel(/Indexer API key/)).toHaveValue('');
	await page.getByLabel('Indexer name').fill('Renamed indexer');
	await page.getByRole('button', { name: 'Save indexer', exact: true }).click();
	await expect(page.getByText('Indexer updated.', { exact: true })).toBeVisible();
	expect(updates[0]).not.toHaveProperty('api_key');
	expect(updates[0]).not.toHaveProperty('password');
	await page.getByRole('button', { name: 'Disable Renamed indexer' }).click();
	await expect(page.getByRole('button', { name: 'Enable Renamed indexer' })).toBeVisible();
	expect(updates[1]).toEqual({ enabled: false });
});

test('Usenet provider edit keeps password and encodes server IDs', async ({ page }) => {
	await mockApi(page);
	const server = {
		id: 'Primary server',
		name: 'Primary',
		host: 'news.example.test',
		port: 563,
		username: 'account',
		password: '',
		password_configured: true,
		connections: 20,
		ssl: true,
		enabled: true,
		priority: 0
	};
	let payload;
	await page.route('**/api/usenet/servers', (route) => route.fulfill({ json: [server] }));
	await page.route('**/api/usenet/servers/Primary%20server', async (route) => {
		payload = route.request().postDataJSON();
		await route.fulfill({ json: { status: 'ok' } });
	});
	await page.goto('/settings');
	await page.getByText('Usenet providers & backbones', { exact: true }).click();
	await page.getByRole('button', { name: 'Edit server Primary' }).click();
	await page.getByLabel('Connections', { exact: true }).fill('12');
	await page.getByRole('button', { name: 'Save server', exact: true }).click();
	await expect(page.getByText('Usenet server saved.', { exact: true })).toBeVisible();
	expect(payload).toMatchObject({ connections: 12, ssl: true });
	expect(payload).not.toHaveProperty('password');
});

test('quick setup and expanded connection forms fit a narrow mobile viewport', async ({ page }) => {
	await page.setViewportSize({ width: 320, height: 760 });
	await mockApi(page, { setup: false });
	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Less setup. More discovery.' })).toBeVisible();
	await page.locator('summary').filter({ hasText: 'Download clients' }).click();
	await page.getByLabel('Enable qBittorrent', { exact: true }).check();
	await expect(page.getByLabel('qBittorrent URL')).toBeVisible();
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1)
	).toBeFalsy();
	await page.getByRole('button', { name: 'Start using Zarr' }).scrollIntoViewIfNeeded();
	await expect(page.getByRole('button', { name: 'Start using Zarr' })).toBeVisible();
	await page.screenshot({ path: 'test-results/setup-mobile.png', fullPage: true });
});
