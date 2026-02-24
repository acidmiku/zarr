package server

func (s *Server) registerRoutes() {
	// Discovery / Search
	s.mux.HandleFunc("GET /api/search", s.handleSearch)
	s.mux.HandleFunc("GET /api/trending", s.handleTrending)
	s.mux.HandleFunc("GET /api/metadata/{tmdbID}", s.handleMetadata)
	s.mux.HandleFunc("GET /api/metadata/anilist/{anilistID}", s.handleAnilistMetadata)

	// Library
	s.mux.HandleFunc("POST /api/library", s.handleAddToLibrary)
	s.mux.HandleFunc("GET /api/library", s.handleListLibrary)
	s.mux.HandleFunc("GET /api/library/{id}", s.handleGetLibraryItem)
	s.mux.HandleFunc("PUT /api/library/{id}", s.handleUpdateLibraryItem)
	s.mux.HandleFunc("DELETE /api/library/{id}", s.handleDeleteLibraryItem)

	// Episodes / Seasons
	s.mux.HandleFunc("GET /api/library/{id}/episodes", s.handleGetEpisodes)
	s.mux.HandleFunc("POST /api/library/{id}/episodes/{episodeID}/search", s.handleSearchEpisode)
	s.mux.HandleFunc("POST /api/library/{id}/search", s.handleSearchAll)
	s.mux.HandleFunc("DELETE /api/library/{id}/episodes/{episodeID}/file", s.handleDeleteEpisodeFile)
	s.mux.HandleFunc("POST /api/library/{id}/episodes/{episodeID}/reset", s.handleResetEpisode)
	s.mux.HandleFunc("POST /api/library/{id}/seasons", s.handleAddSeasons)

	// Downloads / Activity
	s.mux.HandleFunc("GET /api/downloads", s.handleGetDownloads)
	s.mux.HandleFunc("GET /api/activity", s.handleGetActivity)
	s.mux.HandleFunc("POST /api/downloads/{id}/retry", s.handleRetryDownload)
	s.mux.HandleFunc("DELETE /api/downloads/{id}", s.handleCancelDownload)

	// Releases (manual search)
	s.mux.HandleFunc("GET /api/releases", s.handleSearchReleases)
	s.mux.HandleFunc("POST /api/releases/grab", s.handleGrabRelease)

	// Quality Profiles
	s.mux.HandleFunc("GET /api/profiles", s.handleListProfiles)
	s.mux.HandleFunc("POST /api/profiles", s.handleCreateProfile)
	s.mux.HandleFunc("PUT /api/profiles/{id}", s.handleUpdateProfile)
	s.mux.HandleFunc("DELETE /api/profiles/{id}", s.handleDeleteProfile)

	// Indexers
	s.mux.HandleFunc("GET /api/indexers", s.handleListIndexers)
	s.mux.HandleFunc("POST /api/indexers", s.handleCreateIndexer)
	s.mux.HandleFunc("PUT /api/indexers/{id}", s.handleUpdateIndexer)
	s.mux.HandleFunc("DELETE /api/indexers/{id}", s.handleDeleteIndexer)
	s.mux.HandleFunc("POST /api/indexers/{id}/test", s.handleTestIndexer)

	// Ratings
	s.mux.HandleFunc("GET /api/ratings", s.handleListRatings)
	s.mux.HandleFunc("PUT /api/ratings", s.handleUpsertRating)
	s.mux.HandleFunc("GET /api/ratings/{tmdbID}", s.handleGetRating)
	s.mux.HandleFunc("DELETE /api/ratings/{tmdbID}", s.handleDeleteRating)

	// Settings
	s.mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	s.mux.HandleFunc("PUT /api/settings", s.handleUpdateSettings)
	s.mux.HandleFunc("POST /api/settings/test-openrouter", s.handleTestOpenRouter)

	// System
	s.mux.HandleFunc("GET /api/system/status", s.handleSystemStatus)
	s.mux.HandleFunc("POST /api/system/scan", s.handleTriggerScan)
	s.mux.HandleFunc("GET /api/system/logs", s.handleGetLogs)

	// Image proxy
	s.mux.HandleFunc("GET /api/image", s.handleImageProxy)
	s.mux.HandleFunc("GET /api/image/jikan/{malID}", s.handleJikanImage)

	// AI Assistant
	s.mux.HandleFunc("GET /api/ai/sessions", s.handleListAISessions)
	s.mux.HandleFunc("POST /api/ai/sessions", s.handleCreateAISession)
	s.mux.HandleFunc("GET /api/ai/sessions/{id}", s.handleGetAISession)
	s.mux.HandleFunc("DELETE /api/ai/sessions/{id}", s.handleDeleteAISession)
	s.mux.HandleFunc("POST /api/ai/sessions/{id}/messages", s.handleAIChat)
	s.mux.HandleFunc("POST /api/ai/sessions/{id}/recommend", s.handleAIRecommend)
	s.mux.HandleFunc("GET /api/ai/models", s.handleAIModels)

	// SPA fallback — serve frontend for all other routes
	s.mux.HandleFunc("/", s.serveSPA)
}
