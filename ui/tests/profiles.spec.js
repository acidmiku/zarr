import { test, expect } from '@playwright/test';
import { mockApi, profiles as legacyProfiles } from './fixtures.mjs';

const presets = ['sonarr-anime', 'radarr-anime'].map((id) => ({
	id,
	name: id === 'sonarr-anime' ? 'Anime Series · TRaSH' : 'Anime Movies · TRaSH',
	media_type: id === 'sonarr-anime' ? 'series' : 'movie',
	version: '2026-10-04-test',
	source_url: 'https://trash-guides.info/Sonarr/sonarr-setup-quality-profiles-anime/',
	qualities: ['remux-1080p', 'bluray-1080p', 'hdtv-1080p', 'web-1080p'],
	config: {
		preset: id,
		quality_groups: [
			['remux-1080p', 'bluray-1080p'],
			['hdtv-1080p', 'web-1080p']
		],
		minimum_format_score: 100,
		upgrade_until_quality: 'bluray-1080p',
		upgrade_until_format_score: 10000,
		dual_audio: 'optional',
		format_scores: {}
	},
	formats: [
		{ id: 'dual-audio-id', name: 'Anime Dual Audio', score: 0 },
		{ id: 'tier-id', name: 'Anime BD Tier 01', score: 1400 },
		{ id: 'bad-id', name: 'Anime LQ', score: -10000 }
	]
}));
const animeProfiles = presets.map((preset, index) => ({
	id: index + 5,
	name: preset.name,
	profile_type: 'video',
	language: 'any',
	qualities: preset.qualities,
	tags: {},
	reject_patterns: [],
	upgrade_allowed: true,
	scoring_config: preset.config
}));

async function profileApi(page, values = [...legacyProfiles, ...animeProfiles]) {
	const fixture = await mockApi(page);
	const saves = [];
	await page.route('**/api/profiles', async (route) => {
		if (route.request().method() === 'POST') {
			saves.push(route.request().postDataJSON());
			await route.fulfill({ json: { id: 8 } });
		} else await route.fulfill({ json: values });
	});
	await page.route('**/api/profiles/presets', (route) => route.fulfill({ json: presets }));
	await page.route(/\/api\/profiles\/\d+$/, async (route) => {
		saves.push(route.request().postDataJSON());
		await route.fulfill({ json: { status: 'updated' } });
	});
	return { ...fixture, saves };
}

test('anime preset creation uses guide defaults with optional advanced overrides', async ({
	page
}) => {
	const { saves } = await profileApi(page);
	await page.goto('/settings');
	await page.getByText('Create quality profile', { exact: true }).click();
	await page.getByLabel('Scoring preset').selectOption('radarr-anime');
	await expect(page.getByLabel('Profile name', { exact: true })).toHaveValue(
		'Anime Movies · TRaSH (custom)'
	);
	await page.getByLabel('Profile name', { exact: true }).fill('My anime films');
	await page.getByLabel('Dual audio', { exact: true }).selectOption('above-tier');
	await expect(page.getByLabel('Qualities (JSON array, best first)')).toHaveCount(0);
	await page.getByText('Advanced scoring', { exact: true }).click();
	await expect(page.getByLabel('Minimum format score')).toHaveValue('100');
	await page.getByLabel('Minimum format score').fill('250');
	await page.getByLabel('Quality group 2', { exact: true }).fill('web-1080p');
	await page.getByText('Custom format scores', { exact: true }).click();
	await page.getByLabel('Find custom format').fill('LQ');
	await page.getByLabel('Score for Anime LQ').fill('-20000');
	await page.getByRole('button', { name: 'Create profile', exact: true }).click();
	await expect.poll(() => saves.length).toBe(1);
	expect(saves[0]).toMatchObject({
		name: 'My anime films',
		profile_type: 'video',
		language: 'any',
		qualities: ['remux-1080p', 'bluray-1080p', 'web-1080p'],
		scoring_config: {
			preset: 'radarr-anime',
			minimum_format_score: 250,
			dual_audio: 'above-tier',
			upgrade_until_format_score: 10000,
			quality_groups: [['remux-1080p', 'bluray-1080p'], ['web-1080p']],
			format_scores: { 'bad-id': -20000 }
		}
	});
});

test('changing a dual-audio requirement preserves custom thresholds and format overrides', async ({
	page
}) => {
	const custom = {
		...animeProfiles[0],
		scoring_config: {
			...presets[0].config,
			dual_audio: 'required',
			minimum_format_score: 3500,
			format_scores: { 'tier-id': 1600, 'dual-audio-id': 250 }
		}
	};
	const { saves } = await profileApi(page, [custom]);
	await page.goto('/settings');
	await page.getByRole('button', { name: 'Edit profile Anime Series · TRaSH' }).click();
	await expect(page.getByLabel('Dual audio', { exact: true })).toHaveValue('required');
	await page.getByText('Advanced scoring', { exact: true }).click();
	await page.getByText('Custom format scores', { exact: true }).click();
	await expect(page.getByLabel('Profile language')).toHaveCount(0);
	await expect(page.getByLabel('Score for Anime Dual Audio')).toHaveValue('2000');
	await expect(page.getByLabel('Score for Anime Dual Audio')).toBeDisabled();
	await page.getByLabel('Dual audio', { exact: true }).selectOption('optional');
	await expect(page.getByLabel('Score for Anime Dual Audio')).toHaveValue('250');
	await expect(page.getByLabel('Score for Anime Dual Audio')).toBeEnabled();
	await page.getByRole('button', { name: 'Save profile', exact: true }).click();
	await expect.poll(() => saves.length).toBe(1);
	expect(saves[0].scoring_config).toMatchObject({
		dual_audio: 'optional',
		minimum_format_score: 3500,
		format_scores: { 'tier-id': 1600, 'dual-audio-id': 250 }
	});
});

test('legacy profiles remain editable and music never retains an anime preset', async ({
	page
}) => {
	const { saves } = await profileApi(page);
	await page.goto('/settings');
	await page.getByRole('button', { name: 'Edit profile HD Movies' }).click();
	await expect(page.getByLabel('Scoring preset')).toHaveValue('');
	await page.getByLabel('Profile name', { exact: true }).fill('Custom movies');
	await page.getByRole('button', { name: 'Save profile', exact: true }).click();
	await expect.poll(() => saves.length).toBe(1);
	expect(saves[0].scoring_config).toEqual({});
	await page.getByRole('button', { name: 'Edit profile Anime Series · TRaSH' }).click();
	await page.getByLabel('Profile media type').selectOption('music');
	await expect(page.getByLabel('Scoring preset')).toHaveCount(0);
	await expect(page.getByLabel('Qualities (JSON array, best first)')).toHaveValue(
		'["flac","mp3-320"]'
	);
	await page.getByRole('button', { name: 'Save profile', exact: true }).click();
	await expect.poll(() => saves.length).toBe(2);
	expect(saves[1]).toMatchObject({
		profile_type: 'music',
		scoring_config: {},
		qualities: ['flac', 'mp3-320']
	});
});

test('anime detail selects the matching movie or series preset and excludes music', async ({
	page
}) => {
	const { requests } = await profileApi(page, [
		{ ...animeProfiles[1], id: 20, name: 'Anime Music', profile_type: 'music' },
		...legacyProfiles,
		...animeProfiles
	]);
	await page.route('**/api/search?**', (route) =>
		route.fulfill({
			json: [
				{ tmdb_id: 100, title: 'Anime movie', type: 'movie', is_anime: true },
				{ tmdb_id: 101, title: 'Anime series', type: 'series', is_anime: true }
			]
		})
	);
	await page.route('**/api/metadata/**', (route) => route.fulfill({ json: { seasons: [] } }));
	await page.goto('/?type=anime&q=anime');
	for (const [title, id] of [
		['Anime movie', '6'],
		['Anime series', '5']
	]) {
		await page.getByRole('button', { name: `View ${title}`, exact: true }).click();
		await expect(page.locator('.add-row select')).toHaveValue(id);
		await expect(page.locator('.add-row select option')).not.toContainText(['Anime Music']);
		await page.getByRole('button', { name: 'Add to Library', exact: true }).click();
		await expect
			.poll(
				() =>
					requests
						.filter((request) => request.path === '/api/library' && request.method === 'POST')
						.at(-1)?.body.quality_profile_id
			)
			.toBe(Number(id));
	}
});

test('release explanations preserve quality-first API order and show matched penalties', async ({
	page
}) => {
	await mockApi(page);
	await page.route('**/api/releases?**', (route) =>
		route.fulfill({
			json: [
				{
					title: 'Higher quality',
					quality: 'bluray-1080p',
					acceptable: true,
					score: 100,
					quality_rank: 4,
					format_score: 100,
					scoring_preset: 'sonarr-anime',
					scoring_version: '2026-10-04-test',
					matched_formats: [
						{ id: 'tier-id', name: 'Anime BD Tier 01', score: 1400 },
						{ id: 'bad-id', name: 'Penalty', score: -1300 }
					]
				},
				{
					title: 'Lower quality',
					quality: 'web-1080p',
					acceptable: true,
					score: 9000,
					quality_rank: 3,
					format_score: 9000,
					scoring_preset: 'sonarr-anime',
					matched_formats: []
				},
				{
					title: 'Rejected release',
					acceptable: false,
					score: -10000,
					quality_rank: 4,
					format_score: -10000,
					scoring_preset: 'sonarr-anime',
					reject_reason: 'Below minimum format score',
					matched_formats: [{ id: 'bad-id', name: 'Anime LQ', score: -10000 }]
				}
			]
		})
	);
	await page.goto('/library/1?tab=releases');
	await expect(page.locator('.rel-title')).toHaveText([
		'Higher quality',
		'Lower quality',
		'Rejected release'
	]);
	await page.getByLabel('Explain scoring for Higher quality').click();
	await expect(
		page
			.locator('.release')
			.filter({ hasText: 'Higher quality' })
			.getByText('quality rank 4', { exact: false })
	).toBeVisible();
	await expect(page.getByText('Anime BD Tier 01', { exact: true })).toBeVisible();
	await expect(page.getByText('-1300', { exact: true })).toBeVisible();
	await expect(page.getByText('TRaSH anime series · 2026-10-04-test')).toBeVisible();
	await expect(
		page
			.locator('.release')
			.filter({ hasText: 'Rejected release' })
			.getByRole('button', { name: 'Grab' })
	).toHaveCount(0);
	await page.setViewportSize({ width: 390, height: 840 });
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1)
	).toBeFalsy();
});

test('preset editor and advanced controls fit narrow screens', async ({ page }) => {
	await profileApi(page);
	await page.goto('/settings');
	await page.getByRole('button', { name: 'Edit profile Anime Movies · TRaSH' }).click();
	await page.getByText('Advanced scoring', { exact: true }).click();
	await page.getByText('Custom format scores', { exact: true }).click();
	for (const width of [1440, 768, 390, 320]) {
		await page.setViewportSize({ width, height: 1000 });
		expect(
			await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1),
			`${width}px overflow`
		).toBeFalsy();
	}
	await page.setViewportSize({ width: 1440, height: 1000 });
	await page
		.locator('.quality-profiles')
		.screenshot({ path: 'test-results/anime-profile-settings.png' });
});
