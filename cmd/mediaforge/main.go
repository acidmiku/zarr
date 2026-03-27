package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"mediaforge/internal/config"
	"mediaforge/internal/database"
	"mediaforge/internal/grabber"
	"mediaforge/internal/httpclient"
	"mediaforge/internal/indexer"
	"mediaforge/internal/metadata"
	"mediaforge/internal/postprocess"
	"mediaforge/internal/qbt"
	"mediaforge/internal/rutracker"
	"mediaforge/internal/scanner"
	"mediaforge/internal/scheduler"
	"mediaforge/internal/server"
)

func main() {
	// Configure logging
	logLevel := os.Getenv("MEDIAFORGE_LOG_LEVEL")
	var level slog.Level
	switch strings.ToLower(logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	slog.Info("Zarr starting", "version", "0.1.0")

	// Open database
	configDir := os.Getenv("MEDIAFORGE_CONFIG_DIR")
	if configDir == "" {
		configDir = "/config"
	}

	db, err := database.Open(configDir)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Load configuration
	cfg := config.Load(db)

	// Create HTTP clients
	clients, err := httpclient.New(cfg.Proxy)
	if err != nil {
		slog.Warn("proxy configuration failed, running without proxy", "error", err)
		clients, _ = httpclient.New("")
	}

	// Initialize metadata clients
	var tmdbClient *metadata.TMDBClient
	if cfg.TMDBApiKey != "" {
		tmdbClient = metadata.NewTMDBClient(clients.Proxy, cfg.TMDBApiKey)
		slog.Info("TMDB client initialized")
	} else {
		slog.Warn("TMDB API key not configured — discovery features will be limited")
	}

	anilistClient := metadata.NewAniListClient(clients.Proxy)
	slog.Info("AniList client initialized")

	// Initialize anime ID mapping
	mapping := metadata.NewMapping(clients.Proxy)
	go func() {
		if err := mapping.Refresh(); err != nil {
			slog.Warn("initial anime mapping refresh failed", "error", err)
		}
	}()

	// Initialize music clients
	musicbrainzClient := metadata.NewMusicBrainzClient(clients.Proxy)
	coverartClient := metadata.NewCoverArtClient(clients.Proxy, configDir)
	slog.Info("MusicBrainz and CoverArt clients initialized")

	var lastfmClient *metadata.LastFMClient
	if cfg.LastFMApiKey != "" {
		lastfmClient = metadata.NewLastFMClient(clients.Proxy, cfg.LastFMApiKey, db, configDir)
		slog.Info("Last.fm client initialized")
	} else {
		slog.Warn("Last.fm API key not configured — music trending features will be limited")
	}

	// Initialize indexer client
	newznabClient := indexer.NewNewznabClient(clients.Proxy)

	// Initialize grabber (SABnzbd integration)
	grabberSvc := grabber.New(clients.Proxy, clients.Direct, cfg.SABnzbdURL, cfg.SABnzbdKey)

	// Wait for SABnzbd with backoff
	go func() {
		for i := 0; i < 30; i++ {
			if err := grabberSvc.TestConnection(); err == nil {
				slog.Info("SABnzbd connected")
				return
			}
			delay := time.Duration(i+1) * 2 * time.Second
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
			slog.Debug("waiting for SABnzbd", "attempt", i+1, "retry_in", delay)
			time.Sleep(delay)
		}
		slog.Warn("SABnzbd not reachable after retries — downloads will fail until connected")
	}()

	// Initialize qBittorrent client (uses directClient like SABnzbd — local Docker network)
	qbtClient := qbt.New(clients.Direct, cfg.QBTURL, cfg.QBTUsername, cfg.QBTPassword)
	if cfg.QBTEnabled {
		go func() {
			if err := qbtClient.TestConnection(); err == nil {
				slog.Info("qBittorrent connected")
			} else {
				slog.Warn("qBittorrent not reachable — torrent downloads will fail until connected", "error", err)
			}
		}()
	}

	// Initialize Rutracker client (uses proxyClient — external internet)
	// Credentials are per-indexer, loaded from DB at search time
	rutrackerClient := rutracker.New(clients.Proxy, "https://rutracker.org")

	// Initialize post-processor
	coverCacheDir := filepath.Join(configDir, "cache", "covers")
	processor := postprocess.New(db, cfg.MediaRoot, coverCacheDir)

	// Initialize scanner
	scannerSvc := scanner.New(db, cfg.MediaRoot)

	// Initialize scheduler
	sched := scheduler.New(db, cfg, grabberSvc, newznabClient, tmdbClient, anilistClient, mapping, processor, musicbrainzClient, qbtClient, rutrackerClient)
	sched.Start()
	defer sched.Stop()

	// Initialize HTTP server
	srv := server.New(db, cfg, grabberSvc, newznabClient, tmdbClient, anilistClient, mapping, processor, scannerSvc, clients.Proxy, clients.Direct, musicbrainzClient, coverartClient, lastfmClient, qbtClient, rutrackerClient)

	httpServer := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: srv.Handler(),
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("HTTP server listening", "port", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	slog.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)

	slog.Info("Zarr stopped")
}
