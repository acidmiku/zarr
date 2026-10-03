import { test, expect } from '@playwright/test';
import { mockApi } from './fixtures.mjs';

const series = {
	tmdb_id: 114410,
	title: 'Chainsaw Man',
	type: 'series',
	is_anime: true,
	year: 2022
};
const movie = {
	tmdb_id: 1218925,
	title: 'Chainsaw Man - The Movie: Reze Arc',
	type: 'movie',
	is_anime: true,
	year: 2025
};

test('anime discovery keeps the movie separate from the series and season picker', async ({
	page
}) => {
	const { requests } = await mockApi(page);
	await page.route('**/api/search?**', (route) => route.fulfill({ json: [series, movie] }));
	await page.route('**/api/metadata/114410?**', (route) =>
		route.fulfill({
			json: {
				seasons: [
					{ season_number: 0, name: 'Specials', episode_count: 1 },
					{ season_number: 1, name: 'Season 1', episode_count: 12 }
				]
			}
		})
	);
	const movieMetadataRequests = [];
	await page.route('**/api/metadata/1218925?**', (route) => {
		movieMetadataRequests.push(route.request().url());
		return route.fulfill({ json: {} });
	});
	await page.goto('/?type=anime&q=Chainsaw%20Man');
	const movieCard = page.getByRole('button', { name: `View ${movie.title}`, exact: true });
	const seriesCard = page.getByRole('button', { name: `View ${series.title}`, exact: true });
	await expect(movieCard).toContainText('ANIME MOVIE');
	await expect(seriesCard).toContainText('ANIME SERIES');
	await movieCard.click();
	await expect(page.locator('.media-type-tag')).toHaveText('Anime movie');
	await expect(page.locator('.add-row select')).toHaveValue('3');
	await page.getByRole('button', { name: 'Add to Library', exact: true }).click();
	await expect
		.poll(() => requests.filter((r) => r.path === '/api/library' && r.method === 'POST').length)
		.toBe(1);
	expect(requests.find((r) => r.path === '/api/library' && r.method === 'POST').body).toEqual({
		tmdb_id: movie.tmdb_id,
		type: 'movie',
		anime: true,
		quality_profile_id: 3
	});
	expect(movieMetadataRequests).toEqual([]);
	await seriesCard.click();
	await expect(page.locator('.media-type-tag')).toHaveText('Anime series');
	await page.getByRole('button', { name: 'Add to Library', exact: true }).click();
	await expect(page.getByText('Select seasons to monitor')).toBeVisible();
	await page.getByRole('button', { name: 'Add 2 seasons', exact: true }).click();
	await expect
		.poll(() => requests.filter((r) => r.path === '/api/library' && r.method === 'POST').length)
		.toBe(2);
	expect(requests.filter((r) => r.path === '/api/library' && r.method === 'POST')[1].body).toEqual({
		tmdb_id: series.tmdb_id,
		type: 'series',
		anime: true,
		quality_profile_id: 3,
		seasons: [0, 1]
	});
});

test('an anime recommendation finds the exact movie after a related series result', async ({
	page
}) => {
	const { requests } = await mockApi(page);
	await page.route('**/api/search?**', (route) => route.fulfill({ json: [series, movie] }));
	await page.route('**/api/ai/sessions/1/messages', (route) =>
		route.fulfill({
			contentType: 'text/event-stream',
			body: `event: recommendations\ndata: ${JSON.stringify({ recommendations: [{ title: movie.title, media_type: 'anime', reason: 'Continue the story.' }] })}\n\nevent: done\ndata: {}\n\n`
		})
	);
	await page.goto('/assistant');
	await page.getByLabel('Message the assistant').fill('Recommend an anime movie');
	await page.getByRole('button', { name: 'Send', exact: true }).click();
	await page.getByRole('button', { name: '+ Add to Library', exact: true }).click();
	await expect(page.locator('.media-type-tag')).toHaveText('Anime movie');
	await page.getByRole('button', { name: 'Add to Library', exact: true }).click();
	await expect
		.poll(() => requests.some((r) => r.path === '/api/library' && r.method === 'POST'))
		.toBe(true);
	expect(requests.find((r) => r.path === '/api/library' && r.method === 'POST').body).toMatchObject(
		{
			tmdb_id: movie.tmdb_id,
			type: 'movie',
			anime: true
		}
	);
	expect(requests.filter((r) => r.path.startsWith('/api/metadata/'))).toEqual([]);
});
