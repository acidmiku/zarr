import { test, expect } from '@playwright/test';
import { mockApi, albums } from './fixtures.mjs';

test('music artist links filter albums and survive reload', async ({ page }) => {
	await mockApi(page);
	await page.route('**/api/music/library?**', (route) =>
		route.fulfill({
			json: albums.map((album, index) => ({
				...album,
				artist_mbid: index === 0 ? 'massive-attack' : 'other-artist'
			}))
		})
	);
	await page.route('**/api/music/library/artists', (route) =>
		route.fulfill({
			json: [
				{
					id: 1,
					mbid: 'massive-attack',
					name: 'Massive Attack',
					album_count: 1,
					available_count: 1
				}
			]
		})
	);
	await page.goto('/music?tab=library');
	await page.getByRole('button', { name: 'Artists', exact: true }).click();
	await page.getByRole('link', { name: /Massive Attack/ }).click();
	await expect(page).toHaveURL(/tab=library&artist=massive-attack/);
	await expect(page.locator('.album-card')).toHaveCount(1);
	await expect(page.locator('.album-card')).toContainText('Mezzanine');
	await page.reload();
	await expect(page.locator('.album-card')).toHaveCount(1);
});

test('music cards open with a keyboard and Escape closes details', async ({ page }) => {
	await mockApi(page);
	await page.goto('/music');
	const card = page.getByRole('button', { name: 'View Mezzanine', exact: true });
	await card.focus();
	await page.keyboard.press('Enter');
	await expect(page.locator('.modal-overlay')).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(page.locator('.modal-overlay')).not.toBeVisible();
});

test('import uses a matching profile and cannot change media type during a scan', async ({
	page
}) => {
	await mockApi(page);
	let release;
	const gate = new Promise((resolve) => {
		release = resolve;
	});
	await page.route('**/api/import/scan', async (route) => {
		await gate;
		await route.fulfill({ json: [] });
	});
	await page.goto('/import');
	await page.locator('.type-buttons').getByRole('button', { name: 'Music', exact: false }).click();
	await expect(page.getByLabel('Quality profile')).toHaveValue('4');
	await expect(page.getByLabel('Quality profile').locator('option')).toHaveCount(1);
	await page.getByLabel('Source directory').fill('/data/import/music');
	await page.getByRole('button', { name: 'Scan Directory' }).click();
	await expect(
		page.locator('.type-buttons').getByRole('button', { name: 'Movies', exact: false })
	).toBeDisabled();
	await expect(page.getByLabel('Source directory')).toBeDisabled();
	release();
	await expect(page.getByRole('button', { name: 'Scan Directory' })).toBeEnabled();
});
