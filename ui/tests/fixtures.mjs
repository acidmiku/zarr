import fs from 'node:fs';
import path from 'node:path';

const art = (name) => `https://art.example.test/${name}.jpg`;
export const media = [
	{
		id: 1,
		tmdb_id: 1001,
		title: 'Serial Experiments Lain',
		type: 'series',
		anime: true,
		year: 1998,
		status: 'available',
		poster_url: art('lain'),
		backdrop_url: art('lain'),
		overview:
			'A surreal exploration of identity, technology, and reality in a connected world. Follow the signal. Question everything.',
		rating: 8.6,
		quality_profile_id: 3
	},
	{
		id: 2,
		tmdb_id: 1002,
		title: 'Blade Runner 2049',
		type: 'movie',
		anime: false,
		year: 2017,
		status: 'available',
		poster_url: art('blade'),
		overview: 'A young blade runner uncovers a secret that could change everything.',
		rating: 8.0,
		quality_profile_id: 1
	},
	{
		id: 3,
		tmdb_id: 1003,
		title: 'Frieren: Beyond Journey’s End',
		type: 'series',
		anime: true,
		year: 2023,
		status: 'wanted',
		poster_url: art('frieren'),
		rating: 9.1,
		quality_profile_id: 3
	},
	{
		id: 4,
		tmdb_id: 1004,
		title: 'Dune: Part Two',
		type: 'movie',
		anime: false,
		year: 2024,
		status: 'downloading',
		poster_url: art('dune'),
		rating: 8.4,
		quality_profile_id: 1
	},
	{
		id: 5,
		tmdb_id: 1005,
		title: 'Arrival',
		type: 'movie',
		anime: false,
		year: 2016,
		status: 'wanted',
		poster_url: art('arrival'),
		rating: 8.0,
		quality_profile_id: 1
	},
	{
		id: 6,
		tmdb_id: 1006,
		title: 'Severance',
		type: 'series',
		anime: false,
		year: 2022,
		status: 'available',
		poster_url: '',
		rating: 8.7,
		quality_profile_id: 2
	}
];
export const albums = [
	{
		id: 11,
		title: 'Mezzanine',
		artist_name: 'Massive Attack',
		year: 1998,
		image_url: art('mezzanine'),
		status: 'available',
		track_count: 11
	},
	{
		id: 12,
		title: 'Resist',
		artist_name: 'Kosheen',
		year: 2001,
		image_url: art('resist'),
		status: 'available',
		track_count: 12
	},
	{
		id: 13,
		title: 'Immersion',
		artist_name: 'Pendulum',
		year: 2010,
		image_url: art('immersion'),
		status: 'wanted',
		track_count: 15
	}
];
export const initialDownloads = [
	{
		id: 31,
		media_item_id: 4,
		media_title: 'Dune: Part Two',
		nzb_title: 'Dune.Part.Two.1080p.BluRay',
		status: 'downloading',
		download_type: 'nzb',
		percentage: '78',
		speed: '8.1 MB',
		time_left: '02:15',
		quality: '1080p'
	},
	{
		id: 32,
		media_item_id: 3,
		media_title: 'Frieren · S01E12',
		nzb_title: 'Frieren.S01E12.1080p',
		status: 'downloading',
		download_type: 'torrent',
		percentage: '0',
		speed: '',
		time_left: '',
		quality: '1080p'
	},
	{
		id: 33,
		media_item_id: 5,
		media_title: 'Arrival',
		nzb_title: 'Arrival.1080p',
		status: 'failed',
		download_type: 'nzb',
		percentage: '',
		quality: '1080p'
	}
];
export const profiles = [
	{
		id: 1,
		name: 'HD Movies',
		profile_type: 'video',
		qualities: '["bluray-1080p","web-1080p"]',
		tags: '{}',
		reject_patterns: '[]',
		language: 'en'
	},
	{
		id: 2,
		name: 'HD Series',
		profile_type: 'video',
		qualities: '[]',
		tags: '{}',
		reject_patterns: '[]',
		language: 'en'
	},
	{
		id: 3,
		name: 'Anime',
		profile_type: 'video',
		qualities: '[]',
		tags: '{}',
		reject_patterns: '[]',
		language: 'ja'
	},
	{
		id: 4,
		name: 'Lossless Music',
		profile_type: 'music',
		qualities: '["flac"]',
		tags: '{}',
		reject_patterns: '[]',
		language: 'any'
	}
];

export async function mockApi(page, options = {}) {
	const requests = [];
	const state = {
		downloads: structuredClone(initialDownloads),
		library: options.empty ? [] : structuredClone(media)
	};
	await page.route('**/api/**', async (route) => {
		const request = route.request(),
			url = new URL(request.url()),
			p = url.pathname;
		requests.push({
			method: request.method(),
			path: p,
			search: url.search,
			body: request.postDataJSON()
		});
		const json = (data) => route.fulfill({ json: data });
		if (p === '/api/image' || p.startsWith('/api/music/cover')) {
			const name = new URL(url.searchParams.get('url') || art('mezzanine')).pathname
				.split('/')
				.pop();
			const file = process.env.ZARR_QA_ART_DIR && path.join(process.env.ZARR_QA_ART_DIR, name);
			if (file && fs.existsSync(file))
				return route.fulfill({ contentType: 'image/jpeg', body: fs.readFileSync(file) });
			const title = name.replace('.jpg', '').toUpperCase();
			const hue = [...title].reduce((n, c) => n + c.charCodeAt(0), 0) % 360;
			return route.fulfill({
				contentType: 'image/svg+xml',
				body: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 600 800"><defs><linearGradient id="b" x2="1" y2="1"><stop stop-color="hsl(${hue} 30% 8%)"/><stop offset="1" stop-color="hsl(${hue} 40% 35%)"/></linearGradient></defs><rect width="600" height="800" fill="url(#b)"/><circle cx="420" cy="300" r="180" fill="none" stroke="hsl(${hue} 55% 60%)" stroke-width="2"/><path d="M0 620L600 190M0 680L600 250" stroke="white" opacity=".2"/><text x="35" y="710" fill="white" font-family="sans-serif" font-size="40">${title}</text></svg>`
			});
		}
		if (p === '/api/system/status')
			return options.bootError
				? route.fulfill({ status: 503, json: { error: 'Server offline' } })
				: json({
						setup_complete: options.setup !== false,
						database: 'ok',
						disk: { total_gb: 4000, free_gb: 1800 },
						library: { movies: 3, series: 1, anime: 2 }
					});
		if (p === '/api/profiles') return json(profiles);
		if (p === '/api/library' && request.method() === 'POST') return json({ id: 100 });
		if (p === '/api/library') {
			if (options.libraryError)
				return route.fulfill({ status: 500, json: { error: 'Library offline' } });
			const type = url.searchParams.get('type'),
				status = url.searchParams.get('status');
			const items = state.library.filter(
				(m) =>
					(!type ||
						type === 'all' ||
						(type === 'anime'
							? m.anime
							: type === 'series'
								? m.type === type && !m.anime
								: m.type === type)) &&
					(!status || status === 'all' || status === m.status)
			);
			return json({ items, total: items.length, page: 1 });
		}
		if (/^\/api\/library\/\d+$/.test(p))
			return json(state.library.find((m) => m.id === Number(p.split('/').pop())) || media[0]);
		if (p.endsWith('/episodes'))
			return json([
				{
					id: 1,
					number: 1,
					title: 'Season 1',
					episodes: [
						{ id: 1, number: 1, title: 'Weird', status: 'available' },
						{ id: 2, number: 2, title: 'Girls', status: 'wanted' }
					]
				}
			]);
		if (p.startsWith('/api/metadata/'))
			return json({ seasons: [{ season_number: 1, name: 'Season 1', episode_count: 13 }] });
		if (p === '/api/downloads')
			return options.queueError
				? route.fulfill({ status: 503, json: { error: 'Downloader offline' } })
				: json(options.empty ? [] : state.downloads);
		if (/\/downloads\/\d+\/retry/.test(p)) {
			state.downloads.find((d) => d.id === Number(p.split('/')[3])).status = 'queued';
			return json({ status: 'ok' });
		}
		if (/\/downloads\/\d+$/.test(p) && request.method() === 'DELETE') {
			state.downloads = state.downloads.filter((d) => d.id !== Number(p.split('/').pop()));
			return json({ status: 'ok' });
		}
		if (p === '/api/downloads/failed') {
			state.downloads = state.downloads.filter((d) => d.status !== 'failed');
			return json({ cleared: 1 });
		}
		if (p === '/api/activity')
			return json({
				items: [
					{
						id: 1,
						action: 'added',
						media_title: 'Serial Experiments Lain',
						details: 'Added to library',
						created_at: '2026-10-03T17:00:00Z'
					}
				],
				total: 1
			});
		if (p === '/api/trending' || p === '/api/search')
			return json(
				media
					.map((m) => ({ ...m, in_library: false }))
					.filter((m) =>
						url.searchParams.get('type') === 'anime'
							? m.anime
							: m.type === (url.searchParams.get('type') || 'movie')
					)
			);
		if (p === '/api/music/library') return json(options.empty ? [] : albums);
		if (p === '/api/music/library/artists') return json([]);
		if (/^\/api\/music\/library\/\d+$/.test(p))
			return json({
				album: albums.find((a) => a.id === Number(p.split('/').pop())) || albums[0],
				tracks: []
			});
		if (p === '/api/music/trending' || p === '/api/music/search')
			return json(albums.map((a) => ({ ...a, artist: a.artist_name, cover_url: a.image_url })));
		if (p === '/api/settings')
			return json({
				openrouter_configured: true,
				tmdb_configured: true,
				sabnzbd_url: 'http://localhost:8080',
				media_root: '/media',
				ai_personality_preset: 'default'
			});
		if (p === '/api/ai/sessions') return json(request.method() === 'POST' ? { id: 1 } : []);
		if (p === '/api/ai/models') return json([]);
		if (p === '/api/ratings') return json([]);
		if (p.startsWith('/api/ratings/')) return json(null);
		if (p === '/api/indexers' || p === '/api/releases') return json([]);
		return json({ status: 'ok' });
	});
	return { requests, state };
}
