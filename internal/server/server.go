package server

import "github.com/cookiengineer/zimdex/internal/archive"
import "github.com/cookiengineer/zimdex/internal/server/middlewares"
import routes_api "github.com/cookiengineer/zimdex/internal/server/routes/api"
import routes_zim "github.com/cookiengineer/zimdex/internal/server/routes/zim"
import "github.com/cookiengineer/zimdex/io/zimfs"
import "context"
import "fmt"
import "log"
import "net/http"
import "path/filepath"
import "runtime"
import "strconv"
import "strings"
import "sync"

var publicDir = func() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "public"
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "public")
}()

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

	file_server := http.FileServer(http.Dir(publicDir))

	server.mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {

		if routes_zim.IsPath(request.URL.Path) {

			if strings.HasSuffix(request.URL.Path, "/") {
				http.Redirect(response, request, fmt.Sprintf("%sindex.html", request.URL.Path), http.StatusSeeOther)
			} else {
				routes_zim.Render(server.manager, response, request)
			}

		} else {
			file_server.ServeHTTP(response, request)
		}

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
