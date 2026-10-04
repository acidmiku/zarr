import { api } from '$lib/api';

export function releaseGroupId(album) {
	if (album?.provider === 'lastfm') return '';
	return album?.release_group_id || album?.mbid || (typeof album?.id === 'string' ? album.id : '');
}

export function musicArtwork(album) {
	const id = releaseGroupId(album);
	return id ? api.musicCoverUrl(id) : album?.cover_url || album?.image_url || '';
}

export function acquisitionState(album) {
	if (album?.acquisition?.status) return album.acquisition;
	const status = ['available', 'downloading', 'searching', 'queued', 'failed'].includes(
		album?.status
	)
		? album.status
		: 'saved';
	return { status, message: '', retryable: ['failed'].includes(status) };
}

export const acquisitionLabels = {
	saved: 'Saved',
	queued: 'Queued',
	searching: 'Searching sources',
	no_results: 'No matching release',
	blocked: 'Source unavailable',
	downloading: 'Downloading',
	importing: 'Importing',
	failed: 'Acquisition failed',
	available: 'Available',
	cancelled: 'Cancelled'
};

export const activeAcquisition = (album) =>
	['queued', 'searching', 'downloading', 'importing'].includes(acquisitionState(album).status);
