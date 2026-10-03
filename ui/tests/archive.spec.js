import { test, expect } from '@playwright/test';
import { mockApi } from './fixtures.mjs';

test('collection combines real API media, music and queue state', async ({ page }) => {
	await mockApi(page);
	await page.goto('/library');
	await expect(
		page.getByRole('heading', { name: 'Serial Experiments Lain', exact: true })
	).toBeVisible();
	await expect(page.getByText('Massive Attack', { exact: true })).toBeVisible();
	await expect(page.getByText('1 failed download', { exact: true })).toBeVisible();
	await expect(
		page.getByRole('progressbar', { name: 'Frieren · S01E12 progress' })
	).toHaveAttribute('value', '0');
	await expect(page.locator('.collection-toolbar')).toContainText('6 titles');
	await page.screenshot({
		path: 'test-results/neon-archive-desktop.png',
		fullPage: true,
		animations: 'disabled'
	});
});

test('category and status filters survive direct links and navigation', async ({ page }) => {
	const { requests } = await mockApi(page);
	await page.goto('/library');
	await page
		.getByRole('navigation', { name: 'Media categories' })
		.getByRole('link', { name: /03 Anime/ })
		.click();
	await expect(page).toHaveURL(/type=anime/);
	await expect(page.locator('.collection-grid .poster-card')).toHaveCount(2);
	await page.getByLabel('Filter by status').selectOption('wanted');
	await expect(page.locator('.collection-grid .poster-card')).toHaveCount(1);
	expect(
		requests.some(
			(r) =>
				r.path === '/api/library' &&
				r.search.includes('type=anime') &&
				r.search.includes('status=wanted')
		)
	).toBeTruthy();
	await page.reload();
	await expect(page.getByLabel('Filter by status')).toHaveValue('wanted');
});

test('header search routes video and music to the right API', async ({ page }) => {
	const { requests } = await mockApi(page);
	await page.goto('/library');
	await page.getByLabel('Search titles').fill('Arrival');
	await page.getByRole('button', { name: 'Search', exact: true }).click();
	await expect(page).toHaveURL(/q=Arrival/);
	await expect(page.getByRole('heading', { name: /results/ })).toBeVisible();
	expect(
		requests.some((r) => r.path === '/api/search' && r.search.includes('q=Arrival'))
	).toBeTruthy();
	await page.getByLabel('Search media type').selectOption('music');
	await page.getByLabel('Search titles').fill('Kosheen');
	await page.getByRole('button', { name: 'Search', exact: true }).click();
	await expect(page).toHaveURL(/music\?q=Kosheen/);
	await expect
		.poll(() =>
			requests.some((r) => r.path === '/api/music/search' && r.search.includes('q=Kosheen'))
		)
		.toBeTruthy();
});

test('discovery supports keyboard details, Escape and adding with a profile', async ({ page }) => {
	const { requests } = await mockApi(page);
	await page.goto('/?q=Arrival&type=movie');
	const card = page.getByRole('button', { name: 'View Arrival', exact: true });
	await card.focus();
	await page.keyboard.press('Enter');
	await expect(page.locator('dialog')).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(page.locator('dialog')).not.toBeVisible();
	await card.click();
	await page.locator('dialog').getByRole('button', { name: 'Add to Library', exact: true }).click();
	await expect(page.locator('dialog')).not.toBeVisible();
	expect(
		requests.find((r) => r.path === '/api/library' && r.method === 'POST')?.body
	).toMatchObject({ tmdb_id: 1005, type: 'movie', quality_profile_id: 1 });
});

test('queue retry and cancel refresh the shared dashboard state', async ({ page }) => {
	const { requests } = await mockApi(page);
	await page.goto('/library');
	await page.getByRole('button', { name: 'Retry Arrival', exact: true }).click();
	await expect(page.getByText('1 failed download', { exact: true })).not.toBeVisible();
	await page.getByRole('button', { name: 'Cancel Arrival', exact: true }).click();
	await expect(page.getByRole('button', { name: 'Cancel Arrival', exact: true })).not.toBeVisible();
	expect(
		requests.some((r) => r.method === 'POST' && r.path === '/api/downloads/33/retry')
	).toBeTruthy();
	expect(
		requests.some((r) => r.method === 'DELETE' && r.path === '/api/downloads/33')
	).toBeTruthy();
});

test('assistant shortcut prefills but never automatically sends a prompt', async ({ page }) => {
	const { requests } = await mockApi(page);
	await page.goto('/library');
	await page.getByLabel('Ask your assistant').fill('Dark science fiction');
	await page.getByRole('button', { name: 'Ask assistant', exact: true }).click();
	await expect(page).toHaveURL(/assistant\?prompt=Dark/);
	await expect(page.locator('textarea')).toHaveValue('Dark science fiction');
	expect(requests.some((r) => r.method === 'POST' && r.path.startsWith('/api/ai/'))).toBeFalsy();
});

test('empty archive and service failures have explicit useful states', async ({ page }) => {
	await mockApi(page, { empty: true });
	await page.goto('/library');
	await expect(page.getByRole('heading', { name: 'Make room for your obsessions.' })).toBeVisible();
	await page.unroute('**/api/**');
	await mockApi(page, { libraryError: true, queueError: true });
	await page.reload();
	await expect(page.getByRole('heading', { name: 'Archive unavailable' })).toBeVisible();
	await expect(page.getByText(/Downloads unavailable/)).toBeVisible();
	await expect(page.getByText('All quiet. Your next discovery starts it.')).not.toBeVisible();
});

test('backend failure does not masquerade as first-run setup', async ({ page }) => {
	await mockApi(page, { bootError: true });
	await page.goto('/library');
	await expect(page.getByRole('heading', { name: 'Connection interrupted' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Try again' })).toBeVisible();
});

test('new default and saved alternative theme both render', async ({ page }) => {
	await mockApi(page);
	await page.goto('/library');
	await expect(page.locator('.masthead-brand b')).toHaveCSS('color', 'rgb(255, 59, 139)');
	await page.getByLabel('Choose theme').click();
	await page.getByRole('button', { name: 'Light', exact: true }).click();
	await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
	await expect(page.locator('body')).toHaveCSS('background-color', 'rgb(242, 244, 248)');
	await page.reload();
	await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
});

test('mobile exposes all sections and stays within the viewport', async ({ page }) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await mockApi(page);
	const errors = [];
	page.on('pageerror', (e) => errors.push(e.message));
	for (const route of [
		'/library',
		'/',
		'/music?tab=library',
		'/activity',
		'/assistant',
		'/ratings',
		'/import',
		'/settings',
		'/library/1',
		'/music/11'
	]) {
		await page.goto(route);
		await expect(page.locator('#main-content')).toBeVisible();
		await page.waitForTimeout(150);
		const overflow = await page.evaluate(
			() => document.documentElement.scrollWidth > window.innerWidth + 1
		);
		expect(overflow, `horizontal overflow on ${route}`).toBeFalsy();
	}
	await page.goto('/library');
	await page.getByLabel('Open navigation').click();
	const nav = page.getByRole('navigation', { name: 'Mobile navigation' });
	await expect(nav.getByRole('link')).toHaveCount(8);
	await nav.getByRole('link', { name: 'Activity', exact: false }).click();
	await expect(page).toHaveURL(/activity/);
	await expect(nav).not.toBeVisible();
	expect(errors).toEqual([]);
	await page.goto('/library');
	await expect(
		page.getByRole('heading', { name: 'Serial Experiments Lain', exact: true })
	).toBeVisible();
	await page.screenshot({
		path: 'test-results/neon-archive-mobile.png',
		fullPage: true,
		animations: 'disabled'
	});
});

test('narrow music controls wrap without horizontal scrolling', async ({ page }) => {
	await page.setViewportSize({ width: 320, height: 760 });
	await mockApi(page);
	await page.goto('/music?tab=library');
	await expect(page.getByRole('button', { name: 'Downloading', exact: true })).toBeVisible();
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1)
	).toBeFalsy();
});

test('late discovery responses cannot overwrite a newer category', async ({ page }) => {
	await mockApi(page);
	let releaseOld;
	const gate = new Promise((resolve) => (releaseOld = resolve));
	let requested;
	const started = new Promise((resolve) => (requested = resolve));
	await page.route('**/api/trending?**', async (route) => {
		if (new URL(route.request().url()).searchParams.get('type') === 'movie') {
			requested();
			await gate;
			return route.fulfill({
				json: [{ id: 999, tmdb_id: 999, title: 'Stale movie response', type: 'movie' }]
			});
		}
		return route.fallback();
	});
	await page.goto('/');
	await started;
	await page
		.getByRole('navigation', { name: 'Media categories' })
		.getByRole('link', { name: /03 Anime/ })
		.click();
	await expect(
		page.getByRole('heading', { name: 'Serial Experiments Lain', exact: true })
	).toBeVisible();
	releaseOld();
	await page.waitForTimeout(150);
	await expect(
		page.getByRole('heading', { name: 'Stale movie response', exact: true })
	).not.toBeVisible();
	await expect(
		page.getByRole('heading', { name: 'Serial Experiments Lain', exact: true })
	).toBeVisible();
});
