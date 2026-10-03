package server

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"mediaforge/internal/ai"
	"mediaforge/internal/config"
	"mediaforge/internal/database"
	"mediaforge/internal/grabber"
	"mediaforge/internal/indexer"
	"mediaforge/internal/metadata"
	"mediaforge/internal/postprocess"
	"mediaforge/internal/qbt"
	"mediaforge/internal/rutracker"
	"mediaforge/internal/scanner"
)

//go:embed static/*
var staticFS embed.FS

// Server is the HTTP server for Zarr.
type Server struct {
	metadataUpdater func(*metadata.TMDBClient)
	runtimeMu       sync.RWMutex
	db              *database.DB
	cfg             *config.Config
	grabber         *grabber.Grabber
	newznab         *indexer.NewznabClient
	tmdb            *metadata.TMDBClient
	anilist         *metadata.AniListClient
	mapping         *metadata.Mapping
	processor       *postprocess.Processor
	scanner         *scanner.Scanner
	proxyClient     *http.Client
	directClient    *http.Client
	imgCache        *imageCache
	mux             *http.ServeMux

	// Music clients
	musicbrainz *metadata.MusicBrainzClient
	coverart    *metadata.CoverArtClient
	lastfm      *metadata.LastFMClient

	// Torrent clients
	qbt       *qbt.Client
	rutracker *rutracker.Client

	// AI assistant
	aiOpenRouter *ai.OpenRouterClient
	aiJikan      *ai.JikanClient
	aiBrave      *ai.BraveClient
	aiSession    *ai.SessionManager
}

func New(
	db *database.DB,
	cfg *config.Config,
	g *grabber.Grabber,
	n *indexer.NewznabClient,
	t *metadata.TMDBClient,
	a *metadata.AniListClient,
	m *metadata.Mapping,
	p *postprocess.Processor,
	sc *scanner.Scanner,
	proxyClient *http.Client,
	directClient *http.Client,
	mb *metadata.MusicBrainzClient,
	ca *metadata.CoverArtClient,
	lfm *metadata.LastFMClient,
	qbtClient *qbt.Client,
	rtClient *rutracker.Client,
) *Server {
	s := &Server{
		db:           db,
		cfg:          cfg,
		grabber:      g,
		newznab:      n,
		tmdb:         t,
		anilist:      a,
		mapping:      m,
		processor:    p,
		scanner:      sc,
		proxyClient:  proxyClient,
		directClient: directClient,
		imgCache:     newImageCache(cfg.ConfigDir),
		mux:          http.NewServeMux(),
		musicbrainz:  mb,
		coverart:     ca,
		lastfm:       lfm,
		qbt:          qbtClient,
		rutracker:    rtClient,
	}

	// Initialize AI clients if configured
	s.initAI()

	s.registerRoutes()
	return s
}

// initAI initializes AI assistant clients from settings.
func (s *Server) initAI() {
	s.aiBrave = nil
	orKey, _ := s.db.GetSetting("openrouter_api_key")
	s.aiOpenRouter = nil
	if orKey != "" {
		s.aiOpenRouter = ai.NewOpenRouterClient(s.proxyClient, orKey)
	}
	if s.aiSession == nil {
		s.aiSession = ai.NewSessionManager(s.db.DB, s.aiOpenRouter)
	} else {
		s.aiSession = s.aiSession.WithClient(s.aiOpenRouter)
	}

	// Jikan always available (no key needed)
	// API calls use directClient; CDN image fetches use proxyClient
	s.aiJikan = ai.NewJikanClient(s.directClient, s.proxyClient, s.cfg.ConfigDir)

	braveKey, _ := s.db.GetSetting("brave_api_key")
	if braveKey != "" {
		s.aiBrave = ai.NewBraveClient(s.proxyClient, braveKey)
		slog.Info("Brave Search initialized")
	}
}

// Handler returns the HTTP handler.
func (s *Server) SetMetadataUpdater(update func(*metadata.TMDBClient)) { s.metadataUpdater = update }

func (s *Server) Handler() http.Handler {
	return corsMiddleware(logMiddleware(s.mux))
}

// Take one immutable configuration/service snapshot per request. Slow streams and
// upstream calls never hold the settings lock or stall unrelated UI requests.
func (s *Server) runtimeHandler(handler func(*Server, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/settings" {
			s.runtimeMu.Lock()
			defer s.runtimeMu.Unlock()
			handler(s, w, r)
			return
		}
		s.runtimeMu.RLock()
		snapshot := &Server{db: s.db, cfg: &config.Config{Values: s.cfg.Snapshot()}, grabber: s.grabber, newznab: s.newznab, tmdb: s.tmdb, anilist: s.anilist, mapping: s.mapping, processor: s.processor, scanner: s.scanner, proxyClient: s.proxyClient, directClient: s.directClient, imgCache: s.imgCache, musicbrainz: s.musicbrainz, coverart: s.coverart, lastfm: s.lastfm, qbt: s.qbt, rutracker: s.rutracker, aiOpenRouter: s.aiOpenRouter, aiJikan: s.aiJikan, aiBrave: s.aiBrave, aiSession: s.aiSession}
		s.runtimeMu.RUnlock()
		handler(snapshot, w, r)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A random website must not be able to read local keys or change download
		// clients. Svelte's development server proxies /api on the same origin.
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
				writeError(w, http.StatusForbidden, "cross-origin requests are not allowed")
				return
			}
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			writeError(w, http.StatusForbidden, "cross-site requests are not allowed")
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("http request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, 404, "API route not found")
		return
	}
	// Try to serve static file
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		http.Error(w, "Internal error", 500)
		return
	}

	// Try the requested path
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}

	f, err := subFS.Open(path[1:]) // strip leading /
	if err != nil {
		// SPA fallback: serve index.html for all routes
		f, err = subFS.Open("index.html")
		if err != nil {
			http.Error(w, "Not found", 404)
			return
		}
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "Internal error", 500)
		return
	}

	if stat.IsDir() {
		// Try index.html in the directory
		f2, err := subFS.Open(path[1:] + "/index.html")
		if err != nil {
			f2, _ = subFS.Open("index.html")
		}
		if f2 != nil {
			defer f2.Close()
			stat, _ = f2.Stat()
			http.ServeContent(w, r, stat.Name(), stat.ModTime(), f2.(readSeeker))
			return
		}
	}

	if rs, ok := f.(readSeeker); ok {
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), rs)
	}
}

type readSeeker interface {
	Read(p []byte) (n int, err error)
	Seek(offset int64, whence int) (int64, error)
}
