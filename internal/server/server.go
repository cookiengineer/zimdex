package server

import "github.com/cookiengineer/zimdex/internal/archive"
import "github.com/cookiengineer/zimdex/internal/server/middlewares"
import routes_api "github.com/cookiengineer/zimdex/internal/server/routes/api"
import "github.com/cookiengineer/zimdex/io/zimfs"
import "context"
import _ "embed"
import "log"
import "net/http"
import "strconv"
import "sync"

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
	scrapers  map[string]*archive.Scraper
	scraperMu sync.RWMutex
	mux       *http.ServeMux
	port      int
	httpServer *http.Server
}

func NewServer(folder string, port int) *Server {

	manager  := zimfs.NewManager(folder)

	s := &Server{
		manager:  manager,
		scrapers: make(map[string]*archive.Scraper),
		mux:      http.NewServeMux(),
		port:     port,
	}

	s.registerRoutes()
	return s
}

func (server *Server) registerRoutes() {

	handlers := &Handlers{
		Manager:   server.manager,
		Scrapers:  server.scrapers,
		ScraperMu: &server.scraperMu,
		DataDir:   server.manager.Folder,
	}

	server.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/index.html", http.StatusSeeOther)
	})

	server.mux.HandleFunc("GET /index.html", func(w http.ResponseWriter, r *http.Request) {
		serveTemplate(w, "index.html")
	})

	server.mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	server.mux.HandleFunc("GET /archive.html", func(w http.ResponseWriter, r *http.Request) {
		serveTemplate(w, "archive.html")
	})






	server.mux.HandleFunc("GET /api/search", func(response http.ResponseWriter, request *http.Request) {
		routes_api.Search(server.manager, response, request)
	})

	server.mux.HandleFunc("GET /api/archives", func(response http.ResponseWriter, request *http.Request) {
		routes_api.Archives(server.manager, response, request)
	})

	server.mux.HandleFunc("GET /api/filters",  func(response http.ResponseWriter, request *http.Request) {
		routes_api.Filters(response, request)
	})






	// TODO: routes_archive
	server.mux.HandleFunc("POST /api/archive/detect", handlers.handleArchiveDetect)
	server.mux.HandleFunc("POST /api/archive/start", handlers.handleArchiveStart)
	server.mux.HandleFunc("GET /api/archive/{hostname}/status", handlers.handleArchiveStatus)
	server.mux.HandleFunc("GET /api/archive/{hostname}/queue", handlers.handleArchiveQueue)
	server.mux.HandleFunc("POST /api/archive/{hostname}/pause", handlers.handleArchivePause)
	server.mux.HandleFunc("POST /api/archive/{hostname}/continue", handlers.handleArchiveContinue)
	server.mux.HandleFunc("POST /api/archive/{hostname}/stop", handlers.handleArchiveStop)
	server.mux.HandleFunc("POST /api/archive/{hostname}/build", handlers.handleArchiveBuild)
	server.mux.HandleFunc("POST /api/archive/{hostname}/retry", handlers.handleArchiveRetry)

	server.mux.HandleFunc("GET /{zimfile}/{remainder...}", handlers.handleRender)

}

func (s *Server) Start() error {
	addr := ":" + strconv.Itoa(s.port)

	handler := middlewares.Join(
		s.mux,
		middlewares.Recover,
		middlewares.Log,
		middlewares.ContentSecurityPolicy,
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
