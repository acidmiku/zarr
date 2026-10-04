import { test, expect } from '@playwright/test';
import { mockApi } from './fixtures.mjs';

const rgid = '11111111-1111-4111-8111-111111111111';
const releaseID = '22222222-2222-4222-8222-222222222222';
const alternateID = '33333333-3333-4333-8333-333333333333';
const record = {
	id: rgid,
	release_group_id: rgid,
	title: 'Night Signals',
	artist: 'Signal Club',
	artist_mbid: 'artist-1',
	year: 2025,
	type: 'Album',
	reason: 'Because you saved atmospheric electronic music.',
	in_library: false
};
const saved = {
	...record,
	id: 71,
	artist_name: record.artist,
	album_type: 'Album',
	monitored: false,
	favorite: false,
	quality_profile_id: 4,
	status: 'wanted',
	track_count: 2,
	acquisition: { status: 'saved', message: 'Saved without requesting a download.', retryable: true }
};

async function musicApi(page, initialAlbum = null) {
	await mockApi(page);
	let album = initialAlbum ? structuredClone(initialAlbum) : null;
	const calls = [];
	const editions = [
		{
			id: releaseID,
			title: 'Standard edition',
			date: '2025',
			country: 'GB',
			media: [{ 'track-count': 2 }]
		},
		{
			id: alternateID,
			title: 'Expanded edition',
			date: '2025',
			country: 'US',
			media: [{ 'track-count': 3 }]
		}
	];
	await page.route('**/api/music/discover', (route) =>
		route.fulfill({
			json: {
				recommendations: [record],
				recently_saved: album
					? [{ ...record, library_id: album.id, in_library: true, acquisition: album.acquisition }]
					: [],
				similar_artists: []
			}
		})
	);
	await page.route('**/api/music/library?**', (route) =>
		route.fulfill({ json: album ? [album] : [] })
	);
	await page.route('**/api/music/library/71', (route) =>
		route.fulfill({ json: { album, tracks: [] } })
	);
	await page.route('**/api/music/album/**', (route) => {
		const selected = new URL(route.request().url()).searchParams.get('release_id') || releaseID;
		calls.push({ method: 'GET', path: '/album', selected });
		return route.fulfill({
			json: {
				release_group: { id: rgid },
				release: {
					id: selected,
					media: [
						{
							position: 1,
							tracks: [
								{
									position: 1,
									title: selected === alternateID ? 'Bonus Signal' : 'First Signal',
									length: 180000
								}
							]
						}
					]
				},
				editions
			}
		});
	});
	await page.route('**/api/music/library', async (route) => {
		const body = route.request().postDataJSON();
		calls.push({ method: 'POST', path: '/save', body });
		album = {
			...structuredClone(saved),
			monitored: body.monitored,
			acquisition: { status: body.download_now ? 'queued' : 'saved', retryable: true }
		};
		await route.fulfill({ json: { id: 71 } });
	});
	for (const operation of ['download', 'monitor', 'favorite']) {
		await page.route(`**/api/music/library/71/${operation}`, async (route) => {
			const body = route.request().postDataJSON();
			calls.push({ method: route.request().method(), path: `/${operation}`, body });
			if (operation === 'download')
				album.acquisition = { status: 'queued', message: 'Search requested.', retryable: false };
			if (operation === 'monitor') album.monitored = body.monitored;
			if (operation === 'favorite') album.favorite = body.favorite;
			await route.fulfill({
				json: {
					acquisition: album.acquisition,
					monitored: album.monitored,
					favorite: album.favorite
				}
			});
		});
	}
	return {
		calls,
		getAlbum: () => album,
		setAlbum: (value) => {
			album = value;
		}
	};
}

test('saving keeps acquisition separate from downloading and preserves the selected edition', async ({
	page
}) => {
	const { calls } = await musicApi(page);
	await page.goto('/music');
	await page.getByRole('button', { name: 'View Night Signals', exact: true }).click();
	await expect(page.getByLabel('Monitor album', { exact: true })).not.toBeChecked();
	await page.getByLabel('Edition', { exact: true }).selectOption(alternateID);
	await expect(page.getByText('Bonus Signal', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: 'Save to library', exact: true }).click();
	await expect(page.locator('.music-acquisition')).toContainText('Saved');
	const save = calls.find((call) => call.path === '/save');
	expect(save.body).toMatchObject({
		release_group_id: rgid,
		release_id: alternateID,
		monitored: false,
		download_now: false
	});
	expect(calls.filter((call) => call.path === '/download')).toHaveLength(0);
	await page.getByRole('button', { name: 'Download now', exact: true }).click();
	await expect(page.locator('.music-acquisition')).toContainText('Queued');
	expect(calls.filter((call) => call.path === '/download')).toHaveLength(1);
});

test('download now requests acquisition explicitly without enabling automatic monitoring', async ({
	page
}) => {
	const { calls } = await musicApi(page);
	await page.goto('/music');
	await page.getByRole('button', { name: 'View Night Signals', exact: true }).click();
	await page.getByRole('button', { name: 'Download now', exact: true }).click();
	await expect(page.locator('.music-acquisition')).toContainText('Queued');
	expect(calls.find((call) => call.path === '/save').body).toMatchObject({
		download_now: true,
		monitored: false
	});
});

test('acquisition outcomes persist on library cards and details with distinct explanations', async ({
	page
}) => {
	await mockApi(page);
	const albums = [
		{
			...saved,
			id: 71,
			title: 'No match',
			acquisition: {
				status: 'no_results',
				message: 'Searched both music indexers; nothing matched the profile.',
				retryable: true
			}
		},
		{
			...saved,
			id: 72,
			title: 'Offline source',
			acquisition: {
				status: 'blocked',
				message: 'The configured music indexer is unavailable.',
				retryable: true
			}
		},
		{
			...saved,
			id: 73,
			title: 'Import failure',
			acquisition: {
				status: 'failed',
				message: 'Import failed: download folder is missing.',
				retryable: true,
				download_id: 93
			}
		}
	];
	await page.route('**/api/music/library?**', (route) => route.fulfill({ json: albums }));
	await page.route(/\/api\/music\/library\/\d+$/, (route) =>
		route.fulfill({
			json: {
				album: albums.find((album) => album.id === Number(route.request().url().split('/').pop())),
				tracks: []
			}
		})
	);
	await page.goto('/music?tab=library');
	await expect(page.locator('[data-acquisition=no_results]')).toContainText('No matching release');
	await expect(page.locator('[data-acquisition=blocked]')).toContainText('Source unavailable');
	await expect(page.locator('[data-acquisition=failed]')).toContainText('Acquisition failed');
	await page.reload();
	await page.getByRole('link', { name: 'View Offline source' }).click();
	await expect(page.getByText('The configured music indexer is unavailable.')).toBeVisible();
	await expect(
		page.getByRole('link', { name: 'Review music sources & download clients' })
	).toBeVisible();
	await expect(page.getByRole('button', { name: 'Retry acquisition' })).toBeEnabled();
});

test('retrying a failed import uses the retained download and refreshes its state', async ({
	page
}) => {
	const fixture = await musicApi(page, {
		...saved,
		acquisition: {
			status: 'failed',
			message: 'Import failed: media folder was unavailable.',
			retryable: true,
			download_id: 93
		}
	});
	let retried = 0;
	await page.route('**/api/downloads/93/retry', (route) => {
		retried++;
		fixture.setAlbum({
			...saved,
			acquisition: {
				status: 'importing',
				message: 'Retrying the retained files.',
				retryable: false,
				download_id: 93
			}
		});
		return route.fulfill({ json: { status: 'ok' } });
	});
	await page.goto('/music/71');
	await expect(page.getByText('Import failed: media folder was unavailable.')).toBeVisible();
	await page.getByRole('button', { name: 'Retry acquisition' }).click();
	await expect(page.locator('.music-acquisition')).toContainText('Importing');
	expect(retried).toBe(1);
	expect(fixture.calls.filter((call) => call.path === '/download')).toHaveLength(0);
	await page.reload();
	await expect(page.locator('.music-acquisition')).toContainText('Importing');
});

test('monitoring and favorites are explicit and survive reload', async ({ page }) => {
	const { calls } = await musicApi(page, saved);
	await page.goto('/music/71');
	await page.getByLabel('Monitor album', { exact: true }).check();
	await expect.poll(() => calls.some((call) => call.path === '/monitor')).toBe(true);
	await page.getByRole('button', { name: '♡ Favorite', exact: true }).click();
	await expect(page.getByRole('button', { name: '♥ Favorited', exact: true })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	await page.reload();
	await expect(page.getByLabel('Monitor album', { exact: true })).toBeChecked();
	await expect(page.getByRole('button', { name: '♥ Favorited', exact: true })).toBeVisible();
	expect(calls.find((call) => call.path === '/monitor')).toMatchObject({
		method: 'PATCH',
		body: { monitored: true }
	});
	expect(calls.filter((call) => call.path === '/download')).toHaveLength(0);
});

test('artist discography retains EPs and singles and filters them locally', async ({ page }) => {
	await musicApi(page);
	await page.route('**/api/music/search?**', (route) =>
		route.fulfill({ json: [{ id: 'artist-1', name: 'Signal Club' }] })
	);
	await page.route('**/api/music/artist/artist-1', (route) =>
		route.fulfill({
			json: {
				albums: [
					{ id: rgid, title: 'Studio Album', 'primary-type': 'Album' },
					{ id: releaseID, title: 'Night EP', 'primary-type': 'EP' },
					{ id: alternateID, title: 'One Signal', 'primary-type': 'Single' }
				]
			}
		})
	);
	await page.goto('/music?type=artist&q=Signal');
	await page.getByRole('button', { name: /Signal Club/ }).click();
	await expect(page.locator('.album-card')).toHaveCount(3);
	await page.getByLabel('Discography release type').selectOption('EP');
	await expect(page.locator('.album-card')).toHaveCount(1);
	await expect(page.locator('.album-card')).toContainText('Night EP');
	await page.getByLabel('Discography release type').selectOption('Single');
	await expect(page.locator('.album-card')).toContainText('One Signal');
});

test('offline discovery leaves search usable and broken covers retain a fallback', async ({
	page
}) => {
	await musicApi(page);
	await page.route('**/api/music/discover', (route) =>
		route.fulfill({ status: 503, json: { error: 'Discovery provider offline' } })
	);
	await page.route('**/api/music/trending?**', (route) => route.fulfill({ json: [] }));
	await page.route('**/api/music/search?**', (route) => route.fulfill({ json: [record] }));
	await page.route('**/api/music/cover?**', (route) => route.fulfill({ status: 404, body: '' }));
	await page.goto('/music');
	await expect(page.getByText(/Discovery provider offline/)).toBeVisible();
	await page.goto('/music?q=Night');
	await expect(page.getByRole('button', { name: 'View Night Signals' })).toBeVisible();
	await expect(page.locator('.album-card .artwork-placeholder')).toBeVisible();
	await expect(page.locator('.album-card img')).toHaveCount(0);
});

test('discovery shelves show reasons and stay usable on narrow screens', async ({ page }) => {
	await musicApi(page);
	await page.route('**/api/music/discover', (route) =>
		route.fulfill({
			json: {
				recommendations: [record],
				recently_saved: [
					{
						...record,
						title: 'Saved Signals',
						library_id: 71,
						in_library: true,
						acquisition: { status: 'saved' }
					}
				],
				similar_artists: [
					{
						id: 'artist-2',
						artist_mbid: 'artist-2',
						name: 'Next Station',
						reason: 'Similar to Signal Club.'
					}
				],
				played_not_owned: [
					{ ...record, title: 'Past Signals', reason: 'Played 24 times on Last.fm.' }
				]
			}
		})
	);
	await page.goto('/music');
	await expect(page.getByRole('heading', { name: 'Recommended for you' })).toBeVisible();
	await expect(page.getByRole('heading', { name: 'Recently saved' })).toBeVisible();
	await expect(page.getByRole('heading', { name: 'From similar artists' })).toBeVisible();
	await expect(page.getByRole('heading', { name: 'Played but not owned' })).toBeVisible();
	await expect(page.getByText('Similar to Signal Club.')).toBeVisible();
	await expect(page.locator('.album-card img').first()).toHaveAttribute(
		'src',
		`/api/music/cover?rgid=${rgid}`
	);
	await page.screenshot({ path: 'test-results/music-discovery.png', fullPage: true });
	await page.setViewportSize({ width: 320, height: 800 });
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1)
	).toBeFalsy();
	await page.getByRole('button', { name: 'View Night Signals', exact: true }).click();
	await expect(page.getByRole('button', { name: 'Save to library' })).toBeVisible();
	await expect(page.locator('.modal-overlay')).toHaveCSS('position', 'fixed');
	await expect(page.getByRole('button', { name: 'Close album details' })).toBeFocused();
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1)
	).toBeFalsy();
	await page.screenshot({ path: 'test-results/music-detail-mobile.png' });
	await page.getByLabel('Edition', { exact: true }).focus();
	await page.keyboard.press('Escape');
	await expect(page.locator('.modal-overlay')).toHaveCount(0);
});

test('cached music search stays visible when the remote catalog fails', async ({ page }) => {
	await musicApi(page);
	let finishRemote;
	const remote = new Promise((resolve) => {
		finishRemote = resolve;
	});
	await page.route('**/api/music/search?**', async (route) => {
		if (new URL(route.request().url()).searchParams.get('local') === 'true')
			return route.fulfill({ json: [record] });
		await remote;
		return route.fulfill({ status: 502, json: { error: 'Catalog offline' } });
	});
	await page.goto('/music?q=Night');
	await expect(page.getByRole('button', { name: 'View Night Signals' })).toBeVisible();
	await expect(page.getByText('Showing cached results. Refreshing the catalog…')).toBeVisible();
	finishRemote();
	await expect(
		page.getByText(/Showing cached results. The catalog could not refresh/)
	).toBeVisible();
	await expect(page.getByRole('button', { name: 'View Night Signals' })).toBeVisible();
});

test('Last.fm suggestions resolve canonical identity before album details and saving', async ({
	page
}) => {
	const { calls } = await musicApi(page);
	await page.route('**/api/music/discover', (route) =>
		route.fulfill({
			json: {
				recommendations: [
					{
						...record,
						provider: 'lastfm',
						id: '',
						release_group_id: '',
						mbid: 'a-release-not-a-release-group',
						image_url: 'https://art.example.test/lastfm.jpg'
					}
				]
			}
		})
	);
	let resolved = false;
	await page.route('**/api/music/resolve?**', (route) => {
		const params = new URL(route.request().url()).searchParams;
		expect(params.get('artist')).toBe(record.artist);
		expect(params.get('title')).toBe(record.title);
		resolved = true;
		return route.fulfill({ json: { ...record, provider: 'musicbrainz' } });
	});
	await page.goto('/music');
	await expect(page.locator('.album-card img')).not.toHaveAttribute('src', /rgid=a-release/);
	await page.getByRole('button', { name: 'View Night Signals' }).click();
	await expect(page.getByRole('button', { name: 'Save to library' })).toBeEnabled();
	expect(resolved).toBeTruthy();
	await page.getByRole('button', { name: 'Save to library' }).click();
	await expect(page.locator('.music-acquisition')).toBeVisible();
	expect(calls.find((call) => call.path === '/save').body.release_group_id).toBe(rgid);
});

test('a failed selected edition cannot save the previous edition and can be retried', async ({
	page
}) => {
	const { calls } = await musicApi(page);
	let failEdition = true;
	await page.route(`**/api/music/album/${rgid}?release_id=${alternateID}`, (route) =>
		failEdition
			? route.fulfill({ status: 502, json: { error: 'Edition unavailable' } })
			: route.fallback()
	);
	await page.goto('/music');
	await page.getByRole('button', { name: 'View Night Signals' }).click();
	await page.getByLabel('Edition', { exact: true }).selectOption(alternateID);
	await expect(page.getByText(/Load a complete edition before saving/)).toBeVisible();
	await expect(page.getByRole('button', { name: 'Save to library' })).toBeDisabled();
	await expect(page.getByRole('button', { name: 'Download now' })).toBeDisabled();
	expect(calls.filter((call) => call.path === '/save')).toHaveLength(0);
	failEdition = false;
	await page.getByRole('button', { name: 'Retry album details' }).click();
	await expect(page.getByText('Bonus Signal', { exact: true })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Save to library' })).toBeEnabled();
});

test('background discovery enrichment refreshes shelves without hiding existing records', async ({
	page
}) => {
	await musicApi(page);
	let requests = 0;
	await page.route('**/api/music/discover', (route) => {
		requests++;
		return route.fulfill({
			json: {
				recommendations: [record],
				similar_artists:
					requests > 1
						? [{ id: 'artist-2', artist_mbid: 'artist-2', name: 'New recommendation' }]
						: [],
				enriching: requests === 1
			}
		});
	});
	await page.goto('/music');
	await expect(page.getByRole('button', { name: 'View Night Signals' })).toBeVisible();
	await expect(page.getByText('Refining recommendations from your collection…')).toBeVisible();
	await expect(page.getByRole('button', { name: 'View artist New recommendation' })).toBeVisible({
		timeout: 7000
	});
	await expect(page.getByRole('button', { name: 'View Night Signals' })).toBeVisible();
	expect(requests).toBe(2);
});

test('similar artists open their discography without treating an artist ID as an album', async ({
	page
}) => {
	await musicApi(page);
	await page.route('**/api/music/discover', (route) =>
		route.fulfill({
			json: {
				similar_artists: [
					{
						id: 'artist-2',
						artist_mbid: 'artist-2',
						name: 'Next Station',
						reason: 'Similar to Signal Club.'
					}
				]
			}
		})
	);
	await page.route('**/api/music/artist/artist-2', (route) =>
		route.fulfill({ json: { albums: [{ ...record, id: rgid, 'primary-type': 'Album' }] } })
	);
	const wrongRequests = [];
	page.on('request', (request) => {
		if (request.url().includes('/album/artist-2') || request.url().includes('rgid=artist-2'))
			wrongRequests.push(request.url());
	});
	await page.goto('/music');
	await page.getByRole('button', { name: 'View artist Next Station' }).click();
	await expect(page.getByRole('heading', { name: 'Next Station', exact: true })).toBeVisible();
	await expect(page.getByRole('button', { name: 'View Night Signals' })).toBeVisible();
	expect(wrongRequests).toHaveLength(0);
});

test('empty discovery without Last.fm stays an empty collection instead of an offline error', async ({
	page
}) => {
	await musicApi(page);
	await page.route('**/api/music/discover', (route) =>
		route.fulfill({
			json: {
				configured: { lastfm: false },
				recommendations: [],
				recently_saved: [],
				similar_artists: [],
				played_not_owned: []
			}
		})
	);
	let trendingCalls = 0;
	await page.route('**/api/music/trending?**', (route) => {
		trendingCalls++;
		return route.fulfill({ status: 503, json: { error: 'Last.fm not configured' } });
	});
	await page.goto('/music');
	await expect(page.getByRole('heading', { name: 'Start with an album you love' })).toBeVisible();
	await expect(page.getByText(/Discovery could not fully refresh/)).toHaveCount(0);
	expect(trendingCalls).toBe(0);
});
