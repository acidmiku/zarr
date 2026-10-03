export function scoringConfig(value) {
	try {
		const config = typeof value === 'string' ? JSON.parse(value) : value;
		return config && typeof config === 'object' && !Array.isArray(config) ? config : {};
	} catch {
		return {};
	}
}

export function preferredVideoProfile(item, profiles) {
	const video = profiles.filter((profile) => profile.profile_type !== 'music');
	const anime = item?.is_anime || item?.anime;
	const preset = item?.type === 'movie' ? 'radarr-anime' : 'sonarr-anime';
	const exact =
		anime && video.find((profile) => scoringConfig(profile.scoring_config).preset === preset);
	const keyword = anime ? 'anime' : item?.type === 'movie' ? 'movie' : 'series';
	const fallback = video.find(
		(profile) =>
			!scoringConfig(profile.scoring_config).preset && profile.name.toLowerCase().includes(keyword)
	);
	return exact?.id || fallback?.id || video[0]?.id || 0;
}
