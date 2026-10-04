// Model-generated titles often include a year. Keep it for matching, but leave
// it out of the metadata search so TMDB can find the title.
export function recommendationQuery(title: string) {
	const match = title.trim().match(/^(.*?)\s+\((\d{4})\)$/);
	return { title: match ? match[1].trim() : title.trim(), year: match ? Number(match[2]) : 0 };
}

function normalizeTitle(title: string) {
	return title
		.normalize('NFKC')
		.toLowerCase()
		.replace(/[^\p{L}\p{N}]+/gu, ' ')
		.trim();
}

export function matchRecommendation(results, recommendation) {
	const query = recommendationQuery(recommendation.title);
	const year = recommendation.year || query.year;
	return (
		results.find(
			(item) =>
				(recommendation.media_type === 'anime' || item.type === recommendation.media_type) &&
				normalizeTitle(item.title) === normalizeTitle(query.title) &&
				(!year || Number(item.year) === Number(year))
		) || null
	);
}
