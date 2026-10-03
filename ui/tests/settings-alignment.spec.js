import { test, expect } from '@playwright/test';
import { mockApi } from './fixtures.mjs';

for (const configured of [false, true]) {
	test(`settings controls align with ${configured ? 'saved' : 'empty'} credentials at responsive widths`, async ({
		page
	}) => {
		await mockApi(page);
		await page.route('**/api/settings', (route) =>
			route.fulfill({
				json: {
					media_root: '/data/media',
					qbittorrent_enabled: true,
					tmdb_configured: configured,
					openrouter_configured: configured,
					sabnzbd_configured: configured,
					qbittorrent_configured: configured,
					lastfm_configured: configured,
					brave_configured: configured
				}
			})
		);
		await page.route('**/api/usenet/servers', (route) => route.fulfill({ json: [] }));
		await page.goto('/settings');
		await expect(page.getByLabel('qBittorrent URL')).toBeVisible();
		await page.locator('summary').filter({ hasText: 'Music, web search & network' }).click();
		await page.locator('summary').filter({ hasText: 'Usenet providers & backbones' }).click();
		await page.getByText('Create quality profile', { exact: true }).click();
		for (const width of [1440, 1024, 768, 390, 320]) {
			await page.setViewportSize({ width, height: 1000 });
			const grids = await page.locator('.connection-grid').evaluateAll((elements) =>
				elements.map((grid) => {
					const fields = [...grid.querySelectorAll(':scope > label')]
						.map((label) => {
							const control = label.querySelector('input, select, textarea');
							const box = control?.getBoundingClientRect();
							return box && box.height > 0
								? {
										name: label.textContent.trim(),
										labelTop: label.getBoundingClientRect().top,
										top: box.top,
										height: box.height
									}
								: null;
						})
						.filter(Boolean);
					return fields;
				})
			);
			for (const fields of grids) {
				for (const field of fields) {
					const peers = fields.filter((other) => Math.abs(other.labelTop - field.labelTop) < 1);
					const tops = peers.map((peer) => peer.top);
					const heights = peers.map((peer) => peer.height);
					expect(
						Math.max(...tops) - Math.min(...tops),
						`${width}px: ${peers.map((peer) => peer.name).join(' / ')}`
					).toBeLessThanOrEqual(1);
					expect(
						Math.max(...heights) - Math.min(...heights),
						`${width}px: control heights`
					).toBeLessThanOrEqual(1);
				}
			}
			expect(
				await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1),
				`${width}px horizontal overflow`
			).toBeFalsy();
		}
		await page.setViewportSize({ width: 1440, height: 1000 });
		await page.screenshot({
			path: `test-results/settings-aligned-${configured ? 'saved' : 'empty'}.png`,
			fullPage: true
		});
	});
}
