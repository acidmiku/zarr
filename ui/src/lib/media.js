// Anime describes the content, while type determines movie/episode workflows.
export function mediaTypeLabel(item) {
	const format = item?.type === 'movie' ? 'Movie' : 'Series';
	return item?.anime || item?.is_anime ? `Anime ${format.toLowerCase()}` : format;
}
