package server

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"

	"mediaforge/internal/ai"
	"mediaforge/internal/config"
	"mediaforge/internal/database"
	"mediaforge/internal/grabber"
	"mediaforge/internal/indexer"
	"mediaforge/internal/metadata"
	"mediaforge/internal/postprocess"
	"mediaforge/internal/scanner"
)

//go:embed static/*
var staticFS embed.FS

// Server is the HTTP server for Zarr.
type Server struct {
	db          *database.DB
	cfg         *config.Config
	grabber     *grabber.Grabber
	newznab     *indexer.NewznabClient
	tmdb        *metadata.TMDBClient
	anilist     *metadata.AniListClient
	mapping     *metadata.Mapping
	processor   *postprocess.Processor
	scanner     *scanner.Scanner
	proxyClient *http.Client
	directClient *http.Client
	imgCache    *imageCache
	mux         *http.ServeMux

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
	}

	// Initialize AI clients if configured
	s.initAI()

	s.registerRoutes()
	return s
}

// initAI initializes AI assistant clients from settings.
func (s *Server) initAI() {
	orKey, _ := s.db.GetSetting("openrouter_api_key")
	if orKey != "" {
		s.aiOpenRouter = ai.NewOpenRouterClient(s.proxyClient, orKey)
		s.aiSession = ai.NewSessionManager(s.db.DB, s.aiOpenRouter)
		slog.Info("AI assistant initialized (OpenRouter)")
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
func (s *Server) Handler() http.Handler {
	return corsMiddleware(logMiddleware(s.mux))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
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
