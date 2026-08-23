package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cookiengineer/zimdex/internal/server/middlewares"
	routes_zim "github.com/cookiengineer/zimdex/internal/server/routes/zim"
	"github.com/cookiengineer/zimdex/io/zimfs"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()

	return NewServer(t.TempDir(), 0)
}

func newTestServerWithMux(t *testing.T) (*Server, *http.ServeMux) {
	t.Helper()

	srv := NewServer(t.TempDir(), 0)
	return srv, srv.mux
}

func TestRootRedirect(t *testing.T) {
	_, mux := newTestServerWithMux(t)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/index.html" {
		t.Errorf("expected Location /index.html, got %q", loc)
	}
}

func TestIndexHTML(t *testing.T) {
	_, mux := newTestServerWithMux(t)

	req := httptest.NewRequest("GET", "/index.html", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("expected text/html content type, got %q", contentType)
	}
	if len(rec.Body.Bytes()) == 0 {
		t.Error("expected non-empty body")
	}
}

func TestArchiveHTML(t *testing.T) {
	_, mux := newTestServerWithMux(t)

	req := httptest.NewRequest("GET", "/archive.html", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestAPIArchives(t *testing.T) {
	_, mux := newTestServerWithMux(t)

	req := httptest.NewRequest("GET", "/api/archives", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("expected JSON content type, got %q", contentType)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := body["archives"]; !ok {
		t.Error("expected 'archives' key in response")
	}
}

func TestAPISearchEmpty(t *testing.T) {
	_, mux := newTestServerWithMux(t)

	req := httptest.NewRequest("GET", "/api/search", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestAPISearchWithQuery(t *testing.T) {
	_, mux := newTestServerWithMux(t)

	req := httptest.NewRequest("GET", "/api/search?q=test&limit=10", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	// no archives loaded → search index unavailable
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestRenderMissingZIM(t *testing.T) {
	_, mux := newTestServerWithMux(t)

	req := httptest.NewRequest("GET", "/nonexistent.zim/example.com/index.html", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestRenderNoZimSuffix(t *testing.T) {
	_, mux := newTestServerWithMux(t)

	req := httptest.NewRequest("GET", "/notazim/example.com/index.html", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestArchiveAPIRoutes(t *testing.T) {
	_, mux := newTestServerWithMux(t)

	tests := []struct {
		method     string
		path       string
		expectCode int
	}{
		{"GET", "/api/archive/example.org/status", http.StatusOK},
		{"GET", "/api/archive/example.org/queue", http.StatusOK},
		{"POST", "/api/archive/example.org/pause", http.StatusOK},
		{"POST", "/api/archive/example.org/continue", http.StatusOK},
		{"POST", "/api/archive/example.org/stop", http.StatusOK},
		{"POST", "/api/archive/example.org/build", http.StatusOK},
		{"POST", "/api/archive/example.org/retry", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != tt.expectCode {
				t.Errorf("expected %d, got %d", tt.expectCode, rec.Code)
			}
		})
	}
}

func TestCSPHeaders(t *testing.T) {
	srv := newTestServer(t)

	handler := middlewares.Join(
		srv.mux,
		middlewares.Recover,
		middlewares.Log,
		middlewares.ContentSecurityPolicy,
	)

	req := httptest.NewRequest("GET", "/index.html", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("expected CSP header to be set")
	}
	expected := "script-src 'self' 'unsafe-inline'"
	if !strings.Contains(csp, expected) {
		t.Errorf("expected CSP to contain %q, got %q", expected, csp)
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /panic", func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := middlewares.Join(mux, middlewares.Recover)

	req := httptest.NewRequest("GET", "/panic", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 after panic, got %d", rec.Code)
	}
}

func TestRenderEndToEnd(t *testing.T) {
	dir := "../../samples"
	zimName := "devdocs_en_go_2026-07.zim"
	zimPath := dir + "/" + zimName

	if _, err := os.Stat(zimPath); os.IsNotExist(err) {
		t.Skip(zimPath + " not found")
	}

	manager := zimfs.NewManager(dir)
	if err := manager.Scan(); err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	defer manager.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{zimfile}/{zimpath...}", func(w http.ResponseWriter, r *http.Request) {
		routes_zim.Render(manager, w, r)
	})

	handler := middlewares.Join(mux, middlewares.Recover, middlewares.ContentSecurityPolicy)

	tests := []struct {
		name          string
		path          string
		expectCode    int
		expectType    string
		minSize       int
		shouldNotHave []string
		shouldHave    []string
	}{
		{
			name:       "HTML page - scripts stripped",
			path:       "/" + zimName + "/bufio/index",
			expectCode: 200,
			expectType: "text/html",
			minSize:    500,
			shouldNotHave: []string{"<script", "onclick"},
			shouldHave:    []string{"bufio", "Package"},
		},
		{
			name:       "HTML page - builtin/index",
			path:       "/" + zimName + "/builtin/index",
			expectCode: 200,
			expectType: "text/html",
			minSize:    500,
			shouldHave: []string{"builtin", "Package"},
		},
		{
			name:       "CSS served with proper type",
			path:       "/" + zimName + "/application.css",
			expectCode: 200,
			expectType: "text/css",
			minSize:    100,
		},
		{
			name:       "Missing entry returns 404",
			path:       "/" + zimName + "/nonexistent.html",
			expectCode: 404,
			expectType: "application/octet-stream",
		},
		{
			name:       "No .zim suffix returns 400",
			path:       "/nozim/foo/bar",
			expectCode: 400,
			expectType: "application/octet-stream",
		},
		{
			name:       "Missing ZIM file returns 404",
			path:       "/nope.zim/foo/bar",
			expectCode: 404,
			expectType: "application/octet-stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectCode {
				t.Errorf("expected %d, got %d (body: %s)", tt.expectCode, rec.Code, truncate(rec.Body.String(), 200))
			}

			contentType := rec.Header().Get("Content-Type")
			if tt.expectType != "" && !strings.HasPrefix(contentType, tt.expectType) {
				t.Errorf("expected Content-Type prefix %q, got %q", tt.expectType, contentType)
			}

			if tt.minSize > 0 && rec.Body.Len() < tt.minSize {
				t.Errorf("expected body >= %d bytes, got %d", tt.minSize, rec.Body.Len())
			}

			body := rec.Body.String()

			for _, s := range tt.shouldNotHave {
				if strings.Contains(body, s) {
					t.Errorf("body should not contain %q", s)
				}
			}

			for _, s := range tt.shouldHave {
				if !strings.Contains(body, s) {
					t.Errorf("body should contain %q", s)
				}
			}
		})
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
