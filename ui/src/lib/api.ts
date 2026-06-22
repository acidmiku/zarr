const BASE = '/api';

async function request(method: string, path: string, body?: any) {
	const opts: RequestInit = {
		method,
		headers: { 'Content-Type': 'application/json' }
	};
	if (body) opts.body = JSON.stringify(body);

	const res = await fetch(`${BASE}${path}`, opts);
	if (!res.ok) {
		const err = await res.json().catch(() => ({ error: res.statusText }));
		throw new Error(err.error || res.statusText);
	}
	return res.json();
}

export const api = {
	// Discovery
	search: (q: string, type: string) => request('GET', `/search?q=${encodeURIComponent(q)}&type=${type}`),
	trending: (type: string, page = 1) => request('GET', `/trending?type=${type}&page=${page}`),
	metadata: (tmdbId: number, type: string) => request('GET', `/metadata/${tmdbId}?type=${type}`),
	anilistMetadata: (id: number) => request('GET', `/metadata/anilist/${id}`),

	// Library
	addToLibrary: (data: any) => request('POST', '/library', data),
	getLibrary: (params: Record<string, string>) => {
		const qs = new URLSearchParams(params).toString();
		return request('GET', `/library?${qs}`);
	},
	getLibraryItem: (id: number) => request('GET', `/library/${id}`),
	updateLibraryItem: (id: number, data: any) => request('PUT', `/library/${id}`, data),
	deleteLibraryItem: (id: number, deleteFiles = false) =>
		request('DELETE', `/library/${id}?delete_files=${deleteFiles}`),

	// Episodes
	getEpisodes: (id: number) => request('GET', `/library/${id}/episodes`),
	searchEpisode: (mediaId: number, epId: number) =>
		request('POST', `/library/${mediaId}/episodes/${epId}/search`),
	searchAll: (id: number) => request('POST', `/library/${id}/search`),
	deleteEpisodeFile: (mediaId: number, epId: number) =>
		request('DELETE', `/library/${mediaId}/episodes/${epId}/file`),
	resetEpisode: (mediaId: number, epId: number) =>
		request('POST', `/library/${mediaId}/episodes/${epId}/reset`),
	addSeasons: (mediaId: number, seasons: number[]) =>
		request('POST', `/library/${mediaId}/seasons`, { seasons }),

	// Downloads
	getDownloads: () => request('GET', '/downloads'),
	retryDownload: (id: number) => request('POST', `/downloads/${id}/retry`),
	cancelDownload: (id: number) => request('DELETE', `/downloads/${id}`),
	clearFailedDownloads: () => request('DELETE', '/downloads/failed'),

	// Activity
	getActivity: (page = 1, limit = 50) => request('GET', `/activity?page=${page}&limit=${limit}`),

	// Releases
	searchReleases: (mediaItemId: number, episodeId?: number) => {
		let url = `/releases?media_item_id=${mediaItemId}`;
		if (episodeId) url += `&episode_id=${episodeId}`;
		return request('GET', url);
	},
	grabRelease: (data: any) => request('POST', '/releases/grab', data),

	// Profiles
	getProfiles: () => request('GET', '/profiles'),
	createProfile: (data: any) => request('POST', '/profiles', data),
	updateProfile: (id: number, data: any) => request('PUT', `/profiles/${id}`, data),
	deleteProfile: (id: number) => request('DELETE', `/profiles/${id}`),

	// Indexers
	getIndexers: () => request('GET', '/indexers'),
	createIndexer: (data: any) => request('POST', '/indexers', data),
	updateIndexer: (id: number, data: any) => request('PUT', `/indexers/${id}`, data),
	deleteIndexer: (id: number) => request('DELETE', `/indexers/${id}`),
	testIndexer: (id: number) => request('POST', `/indexers/${id}/test`),

	// Ratings
	listRatings: (type = 'all', sort = 'rating', order = 'desc') =>
		request('GET', `/ratings?type=${type}&sort=${sort}&order=${order}`),
	getRating: (tmdbId: number, type: string) =>
		request('GET', `/ratings/${tmdbId}?type=${type}`),
	upsertRating: (data: { tmdb_id: number; media_type: string; rating: number; comment: string; title?: string; poster_url?: string; year?: number; anime?: boolean }) =>
		request('PUT', '/ratings', data),
	deleteRating: (tmdbId: number, type: string) =>
		request('DELETE', `/ratings/${tmdbId}?type=${type}`),

	// Music
	musicSearch: (q: string, type = 'album') => request('GET', `/music/search?q=${encodeURIComponent(q)}&type=${type}`),
	musicTrending: (page = 1) => request('GET', `/music/trending?page=${page}`),
	musicArtist: (mbid: string) => request('GET', `/music/artist/${mbid}`),
	musicAlbum: (rgid: string) => request('GET', `/music/album/${rgid}`),
	addMusicToLibrary: (data: any) => request('POST', '/music/library', data),
	getMusicLibrary: (status = 'all') => request('GET', `/music/library?status=${status}`),
	getMusicArtists: () => request('GET', '/music/library/artists'),
	getMusicLibraryItem: (id: number) => request('GET', `/music/library/${id}`),
	deleteMusicLibraryItem: (id: number) => request('DELETE', `/music/library/${id}`),
	searchMusicAlbum: (id: number) => request('POST', `/music/library/${id}/search`),
	rateMusicAlbum: (id: number, data: { rating: number; comment: string }) =>
		request('PUT', `/music/library/${id}/rate`, data),
	getMusicReleases: (id: number) => request('GET', `/music/library/${id}/releases`),
	grabMusicRelease: (data: { release_url: string; album_id: number }) =>
		request('POST', '/music/releases/grab', data),
	musicCoverUrl: (rgid: string) => `${BASE}/music/cover?rgid=${rgid}`,

	// Import
	importScan: (path: string, type: string) => request('POST', '/import/scan', { path, type }),
	importExecute: (items: any[]) => request('POST', '/import/execute', { items }),

	// Settings
	getSettings: () => request('GET', '/settings'),
	updateSettings: (data: any) => request('PUT', '/settings', data),
	testQBittorrent: (data?: { url?: string; username?: string; password?: string }) =>
		request('POST', '/settings/test-qbittorrent', data || {}),

	// System
	getStatus: () => request('GET', '/system/status'),
	triggerScan: () => request('POST', '/system/scan'),
	getLogs: (lines = 100) => request('GET', `/system/logs?lines=${lines}`),

	// Image proxy
	imageUrl: (url: string) => `${BASE}/image?url=${encodeURIComponent(url)}`,
	jikanImageUrl: (malId: number) => `${BASE}/image/jikan/${malId}`,

	// AI Assistant
	aiSessions: () => request('GET', '/ai/sessions'),
	aiCreateSession: () => request('POST', '/ai/sessions'),
	aiGetSession: (id: number) => request('GET', `/ai/sessions/${id}`),
	aiDeleteSession: (id: number) => request('DELETE', `/ai/sessions/${id}`),
	aiModels: () => request('GET', '/ai/models'),
	testOpenRouter: (key?: string) => request('POST', '/settings/test-openrouter', key ? { key } : {}),

	// AI streaming (returns raw Response for SSE parsing)
	aiChat: (sessionId: number, content: string) =>
		fetch(`${BASE}/ai/sessions/${sessionId}/messages`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ content })
		}),
	aiRecommend: (sessionId: number) =>
		fetch(`${BASE}/ai/sessions/${sessionId}/recommend`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' }
		})
};
