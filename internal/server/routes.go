package server

func (s *Server) registerRoutes() {
	// Discovery / Search
	s.mux.HandleFunc("GET /api/search", s.runtimeHandler((*Server).handleSearch))
	s.mux.HandleFunc("GET /api/trending", s.runtimeHandler((*Server).handleTrending))
	s.mux.HandleFunc("GET /api/metadata/{tmdbID}", s.runtimeHandler((*Server).handleMetadata))
	s.mux.HandleFunc("GET /api/metadata/anilist/{anilistID}", s.runtimeHandler((*Server).handleAnilistMetadata))

	// Library
	s.mux.HandleFunc("POST /api/library", s.runtimeHandler((*Server).handleAddToLibrary))
	s.mux.HandleFunc("GET /api/library", s.runtimeHandler((*Server).handleListLibrary))
	s.mux.HandleFunc("GET /api/library/{id}", s.runtimeHandler((*Server).handleGetLibraryItem))
	s.mux.HandleFunc("PUT /api/library/{id}", s.runtimeHandler((*Server).handleUpdateLibraryItem))
	s.mux.HandleFunc("DELETE /api/library/{id}", s.runtimeHandler((*Server).handleDeleteLibraryItem))

	// Episodes / Seasons
	s.mux.HandleFunc("GET /api/library/{id}/episodes", s.runtimeHandler((*Server).handleGetEpisodes))
	s.mux.HandleFunc("POST /api/library/{id}/episodes/{episodeID}/search", s.runtimeHandler((*Server).handleSearchEpisode))
	s.mux.HandleFunc("POST /api/library/{id}/search", s.runtimeHandler((*Server).handleSearchAll))
	s.mux.HandleFunc("DELETE /api/library/{id}/episodes/{episodeID}/file", s.runtimeHandler((*Server).handleDeleteEpisodeFile))
	s.mux.HandleFunc("POST /api/library/{id}/episodes/{episodeID}/reset", s.runtimeHandler((*Server).handleResetEpisode))
	s.mux.HandleFunc("POST /api/library/{id}/seasons", s.runtimeHandler((*Server).handleAddSeasons))

	// Downloads / Activity
	s.mux.HandleFunc("GET /api/downloads", s.runtimeHandler((*Server).handleGetDownloads))
	s.mux.HandleFunc("GET /api/activity", s.runtimeHandler((*Server).handleGetActivity))
	s.mux.HandleFunc("POST /api/downloads/{id}/retry", s.runtimeHandler((*Server).handleRetryDownload))
	s.mux.HandleFunc("DELETE /api/downloads/{id}", s.runtimeHandler((*Server).handleCancelDownload))
	s.mux.HandleFunc("DELETE /api/downloads/failed", s.runtimeHandler((*Server).handleClearFailedDownloads))

	// Releases (manual search)
	s.mux.HandleFunc("GET /api/releases", s.runtimeHandler((*Server).handleSearchReleases))
	s.mux.HandleFunc("POST /api/releases/grab", s.runtimeHandler((*Server).handleGrabRelease))

	// Quality Profiles
	s.mux.HandleFunc("GET /api/profiles", s.runtimeHandler((*Server).handleListProfiles))
	s.mux.HandleFunc("GET /api/profiles/presets", s.runtimeHandler((*Server).handleProfilePresets))
	s.mux.HandleFunc("POST /api/profiles", s.runtimeHandler((*Server).handleCreateProfile))
	s.mux.HandleFunc("PUT /api/profiles/{id}", s.runtimeHandler((*Server).handleUpdateProfile))
	s.mux.HandleFunc("DELETE /api/profiles/{id}", s.runtimeHandler((*Server).handleDeleteProfile))

	// Indexers
	s.mux.HandleFunc("GET /api/indexers", s.runtimeHandler((*Server).handleListIndexers))
	s.mux.HandleFunc("POST /api/indexers", s.runtimeHandler((*Server).handleCreateIndexer))
	s.mux.HandleFunc("PUT /api/indexers/{id}", s.runtimeHandler((*Server).handleUpdateIndexer))
	s.mux.HandleFunc("DELETE /api/indexers/{id}", s.runtimeHandler((*Server).handleDeleteIndexer))
	s.mux.HandleFunc("POST /api/indexers/{id}/test", s.runtimeHandler((*Server).handleTestIndexer))

	// Ratings
	s.mux.HandleFunc("GET /api/ratings", s.runtimeHandler((*Server).handleListRatings))
	s.mux.HandleFunc("PUT /api/ratings", s.runtimeHandler((*Server).handleUpsertRating))
	s.mux.HandleFunc("GET /api/ratings/{tmdbID}", s.runtimeHandler((*Server).handleGetRating))
	s.mux.HandleFunc("DELETE /api/ratings/{tmdbID}", s.runtimeHandler((*Server).handleDeleteRating))

	// Music
	s.mux.HandleFunc("GET /api/music/resolve", s.runtimeHandler((*Server).handleResolveMusicAlbum))
	s.mux.HandleFunc("GET /api/music/search", s.runtimeHandler((*Server).handleMusicSearch))
	s.mux.HandleFunc("GET /api/music/discover", s.runtimeHandler((*Server).handleMusicDiscover))
	s.mux.HandleFunc("GET /api/music/trending", s.runtimeHandler((*Server).handleMusicTrending))
	s.mux.HandleFunc("GET /api/music/artist/{mbid}", s.runtimeHandler((*Server).handleMusicArtist))
	s.mux.HandleFunc("GET /api/music/album/{rgid}", s.runtimeHandler((*Server).handleMusicAlbum))
	s.mux.HandleFunc("POST /api/music/library", s.runtimeHandler((*Server).handleAddMusicToLibrary))
	s.mux.HandleFunc("GET /api/music/library", s.runtimeHandler((*Server).handleListMusicLibrary))
	s.mux.HandleFunc("GET /api/music/library/artists", s.runtimeHandler((*Server).handleListMusicArtists))
	s.mux.HandleFunc("GET /api/music/library/{id}", s.runtimeHandler((*Server).handleGetMusicLibraryItem))
	s.mux.HandleFunc("DELETE /api/music/library/{id}", s.runtimeHandler((*Server).handleDeleteMusicLibraryItem))
	s.mux.HandleFunc("POST /api/music/library/{id}/search", s.runtimeHandler((*Server).handleSearchMusicAlbum))
	s.mux.HandleFunc("POST /api/music/library/{id}/download", s.runtimeHandler((*Server).handleDownloadMusicAlbum))
	s.mux.HandleFunc("PATCH /api/music/library/{id}/monitor", s.runtimeHandler((*Server).handleMonitorMusicAlbum))
	s.mux.HandleFunc("PATCH /api/music/library/{id}/favorite", s.runtimeHandler((*Server).handleFavoriteMusicAlbum))
	s.mux.HandleFunc("PUT /api/music/library/{id}/rate", s.runtimeHandler((*Server).handleRateMusicAlbum))
	s.mux.HandleFunc("GET /api/music/library/{id}/releases", s.runtimeHandler((*Server).handleMusicReleases))
	s.mux.HandleFunc("POST /api/music/releases/grab", s.runtimeHandler((*Server).handleGrabMusicRelease))
	s.mux.HandleFunc("GET /api/music/cover", s.runtimeHandler((*Server).handleMusicCover))

	// Import
	s.mux.HandleFunc("POST /api/import/scan", s.runtimeHandler((*Server).handleImportScan))
	s.mux.HandleFunc("POST /api/import/execute", s.runtimeHandler((*Server).handleImportExecute))

	// Settings
	s.mux.HandleFunc("GET /api/settings", s.runtimeHandler((*Server).handleGetSettings))
	s.mux.HandleFunc("PUT /api/settings", s.runtimeHandler((*Server).handleUpdateSettings))
	s.mux.HandleFunc("POST /api/settings/test-openrouter", s.runtimeHandler((*Server).handleTestOpenRouter))
	s.mux.HandleFunc("POST /api/settings/test-qbittorrent", s.runtimeHandler((*Server).handleTestQBittorrent))
	s.mux.HandleFunc("POST /api/settings/test-tmdb", s.runtimeHandler((*Server).handleTestTMDB))
	s.mux.HandleFunc("POST /api/settings/test-sabnzbd", s.runtimeHandler((*Server).handleTestSABnzbd))
	s.mux.HandleFunc("GET /api/setup/status", s.runtimeHandler((*Server).handleSetupStatus))
	s.mux.HandleFunc("GET /api/usenet/servers", s.runtimeHandler((*Server).handleListUsenetServers))
	s.mux.HandleFunc("POST /api/usenet/servers", s.runtimeHandler((*Server).handleSaveUsenetServer))
	s.mux.HandleFunc("PUT /api/usenet/servers/{id}", s.runtimeHandler((*Server).handleSaveUsenetServer))
	s.mux.HandleFunc("DELETE /api/usenet/servers/{id}", s.runtimeHandler((*Server).handleDeleteUsenetServer))
	s.mux.HandleFunc("POST /api/usenet/servers/{id}/test", s.runtimeHandler((*Server).handleTestUsenetServer))

	// System
	s.mux.HandleFunc("GET /api/system/status", s.runtimeHandler((*Server).handleSystemStatus))
	s.mux.HandleFunc("POST /api/system/scan", s.runtimeHandler((*Server).handleTriggerScan))
	s.mux.HandleFunc("GET /api/system/logs", s.runtimeHandler((*Server).handleGetLogs))

	// Image proxy
	s.mux.HandleFunc("GET /api/image", s.runtimeHandler((*Server).handleImageProxy))
	s.mux.HandleFunc("GET /api/image/jikan/{malID}", s.runtimeHandler((*Server).handleJikanImage))

	// AI Assistant
	s.mux.HandleFunc("GET /api/ai/sessions", s.runtimeHandler((*Server).handleListAISessions))
	s.mux.HandleFunc("POST /api/ai/sessions", s.runtimeHandler((*Server).handleCreateAISession))
	s.mux.HandleFunc("GET /api/ai/sessions/{id}", s.runtimeHandler((*Server).handleGetAISession))
	s.mux.HandleFunc("DELETE /api/ai/sessions/{id}", s.runtimeHandler((*Server).handleDeleteAISession))
	s.mux.HandleFunc("POST /api/ai/sessions/{id}/messages", s.runtimeHandler((*Server).handleAIChat))
	s.mux.HandleFunc("POST /api/ai/sessions/{id}/recommend", s.runtimeHandler((*Server).handleAIRecommend))
	s.mux.HandleFunc("GET /api/ai/models", s.runtimeHandler((*Server).handleAIModels))

	// SPA fallback — serve frontend for all other routes
	s.mux.HandleFunc("/", s.runtimeHandler((*Server).serveSPA))
}
