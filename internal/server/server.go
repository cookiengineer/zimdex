package server

import (
	"context"
	_ "embed"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/cookiengineer/gozim/archive/zim"
	"github.com/cookiengineer/zimdex/internal/archive"
	"github.com/cookiengineer/zimdex/internal/search"
	"github.com/cookiengineer/zimdex/internal/zimfs"
)

//go:embed templates/index.html
var indexHTML string

//go:embed templates/archive.html
var archiveHTML string

func serveTemplate(w http.ResponseWriter, name string) {
	var data string
	if name == "index.html" {
		data = indexHTML
	} else if name == "archive.html" {
		data = archiveHTML
	} else {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(data))
}

type Server struct {
	manager   *zimfs.Manager
	searcher  *search.Searcher
	scrapers  map[string]*archive.Scraper
	scraperMu sync.RWMutex
	mux       *http.ServeMux
	port      int
	httpServer *http.Server
}

func NewServer(manager *zimfs.Manager, port int) *Server {
	archives := make(map[string]*zim.Archive)
	for _, info := range manager.List() {
		archive, ok := manager.Get(info.Filename)
		if ok {
			archives[info.Filename] = archive
		}
	}

	s := &Server{
		manager:  manager,
		searcher: search.NewSearcher(archives),
		scrapers: make(map[string]*archive.Scraper),
		mux:      http.NewServeMux(),
		port:     port,
	}

	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	handlers := &Handlers{
		Manager:  s.manager,
		Searcher: s.searcher,
		Scrapers: s.scrapers,
		ScraperMu: &s.scraperMu,
		DataDir:  s.manager.Dir,
	}

	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/index.html", http.StatusSeeOther)
	})

	s.mux.HandleFunc("GET /index.html", func(w http.ResponseWriter, r *http.Request) {
		serveTemplate(w, "index.html")
	})

	s.mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	s.mux.HandleFunc("GET /archive.html", func(w http.ResponseWriter, r *http.Request) {
		serveTemplate(w, "archive.html")
	})

	s.mux.HandleFunc("GET /api/search", handlers.handleSearch)
	s.mux.HandleFunc("GET /api/suggest", handlers.handleSuggest)
	s.mux.HandleFunc("GET /api/archives", handlers.handleArchives)
	s.mux.HandleFunc("GET /api/filters", handlers.handleFilters)

	s.mux.HandleFunc("POST /api/archive/start", handlers.handleArchiveStart)
	s.mux.HandleFunc("GET /api/archive/{hostname}/status", handlers.handleArchiveStatus)
	s.mux.HandleFunc("GET /api/archive/{hostname}/queue", handlers.handleArchiveQueue)
	s.mux.HandleFunc("POST /api/archive/{hostname}/pause", handlers.handleArchivePause)
	s.mux.HandleFunc("POST /api/archive/{hostname}/continue", handlers.handleArchiveContinue)
	s.mux.HandleFunc("POST /api/archive/{hostname}/stop", handlers.handleArchiveStop)
	s.mux.HandleFunc("POST /api/archive/{hostname}/build", handlers.handleArchiveBuild)
	s.mux.HandleFunc("POST /api/archive/{hostname}/retry", handlers.handleArchiveRetry)

	s.mux.HandleFunc("GET /{zimfile}/{remainder...}", handlers.handleRender)
}

func (s *Server) Start() error {
	addr := ":" + strconv.Itoa(s.port)

	handler := ChainMiddleware(
		s.mux,
		RecoveryMiddleware,
		LoggingMiddleware,
		CSPMiddleware,
	)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	log.Printf("ZIMdex listening on http://localhost%s", addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
