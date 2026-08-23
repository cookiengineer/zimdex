package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"

	"github.com/cookiengineer/zimdex/internal/archive"
	"github.com/cookiengineer/zimdex/io/zimfs"
)

var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)
var boldMarkerRegex = regexp.MustCompile(`\*\*(.+?)\*\*`)

func formatSnippet(raw string) string {
	if raw == "" {
		return ""
	}
	cleaned := htmlTagRegex.ReplaceAllString(raw, "")
	cleaned = boldMarkerRegex.ReplaceAllString(cleaned, "<b>$1</b>")
	return cleaned
}

type Handlers struct {
	Manager   *zimfs.Manager
	Scrapers  map[string]*archive.Scraper
	ScraperMu *sync.RWMutex
	DataDir   string
}

func (h *Handlers) handleArchiveDetect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}

	data, _ := io.ReadAll(r.Body)
	json.Unmarshal(data, &body)

	if body.URL == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "url is required",
		})
		return
	}

	parsedURL, err := url.Parse(body.URL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "invalid URL: " + err.Error(),
		})
		return
	}

	names, err := archive.DetectFilters(parsedURL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"url":     body.URL,
		"filters": names,
	})
}

func (h *Handlers) handleArchiveStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL                string   `json:"url"`
		RespectRobots      *bool    `json:"respect_robots"`
		InsecureSkipVerify bool     `json:"insecure_skip_verify"`
		PageLimit          int      `json:"page_limit"`
		PageWorkers        int      `json:"page_workers"`
		AssetWorkers       int      `json:"asset_workers"`
		FilterNames        []string `json:"filter_names"`
	}

	data, _ := io.ReadAll(r.Body)
	json.Unmarshal(data, &body)

	if body.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	respectRobots := true
	if body.RespectRobots != nil {
		respectRobots = *body.RespectRobots
	}

	if body.PageWorkers <= 0 {
		body.PageWorkers = 2
	}
	if body.AssetWorkers <= 0 {
		body.AssetWorkers = 4
	}

	parsedURL, err := url.Parse(body.URL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "invalid URL: " + err.Error(),
		})
		return
	}

	scraper, err := archive.NewScraper(archive.ScraperOptions{
		Folder:            h.DataDir,
		URL:               parsedURL,
		RespectRobots:     respectRobots,
		IgnoreInsecureSSL: body.InsecureSkipVerify,
		PageWorkers:       body.PageWorkers,
		AssetWorkers:      body.AssetWorkers,
		Filters:           body.FilterNames,
		PageLimit:         body.PageLimit,
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	host := scraper.StartURL.Hostname()

	h.ScraperMu.Lock()
	if _, exists := h.Scrapers[host]; exists {
		h.ScraperMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "scraper already running for " + host,
		})
		return
	}
	h.Scrapers[host] = scraper
	h.ScraperMu.Unlock()

	go scraper.Start()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"host":      host,
		"status":    "running",
		"start_url": body.URL,
	})
}

func (h *Handlers) handleArchiveStatus(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("hostname")

	h.ScraperMu.RLock()
	scraper, ok := h.Scrapers[host]
	h.ScraperMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"host":   host,
			"status": "not_found",
		})
		return
	}

	stats := scraper.Queue().Info()
	startURL := ""
	if scraper.StartURL != nil {
		startURL = scraper.StartURL.String()
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"host":      host,
		"status":    scraper.Status(),
		"start_url": startURL,
		"stats": map[string]interface{}{
			"pending":     stats.Pending,
			"downloading": stats.Downloading,
			"downloaded":  stats.Downloaded,
			"failed":      stats.Failed,
			"skipped":     stats.Skipped,
		},
		"recent_logs":     scraper.RecentLogs(),
		"recent_activity": scraper.RecentActivity(),
	})
}

func (h *Handlers) handleArchiveQueue(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("hostname")

	h.ScraperMu.RLock()
	scraper, ok := h.Scrapers[host]
	h.ScraperMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"host":    host,
			"entries": []interface{}{},
		})
		return
	}

	statusFilter := r.URL.Query().Get("status")
	var entries []zimfs.QueueEntry

	if statusFilter != "" {
		entries = scraper.Queue().Query(zimfs.QueueEntryStatus(statusFilter))
	} else {
		entries = scraper.Queue().Query(zimfs.QueueEntryStatusFailed)
	}

	offset, limit := 0, 100
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, _ := strconv.Atoi(v); n >= 0 {
			offset = n
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, _ := strconv.Atoi(v); n > 0 && n <= 500 {
			limit = n
		}
	}

	end := offset + limit
	if end > len(entries) {
		end = len(entries)
	}
	if offset > len(entries) {
		offset = len(entries)
	}

	page := []interface{}{}
	for i := offset; i < end; i++ {
		page = append(page, entries[i])
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"host":    host,
		"total":   len(entries),
		"offset":  offset,
		"limit":   limit,
		"entries": page,
	})
}

func (h *Handlers) handleArchivePause(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("hostname")

	h.ScraperMu.RLock()
	scraper, ok := h.Scrapers[host]
	h.ScraperMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"host": host, "status": "not_found"})
		return
	}

	scraper.Pause()
	json.NewEncoder(w).Encode(map[string]interface{}{"host": host, "status": "paused"})
}

func (h *Handlers) handleArchiveContinue(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("hostname")

	h.ScraperMu.RLock()
	scraper, ok := h.Scrapers[host]
	h.ScraperMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"host": host, "status": "not_found"})
		return
	}

	go scraper.Continue()
	json.NewEncoder(w).Encode(map[string]interface{}{"host": host, "status": "running"})
}

func (h *Handlers) handleArchiveStop(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("hostname")

	h.ScraperMu.RLock()
	scraper, ok := h.Scrapers[host]
	h.ScraperMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"host": host, "status": "not_found"})
		return
	}

	scraper.Stop()

	h.ScraperMu.Lock()
	delete(h.Scrapers, host)
	h.ScraperMu.Unlock()

	json.NewEncoder(w).Encode(map[string]interface{}{"host": host, "status": "stopped"})
}

func (h *Handlers) handleArchiveBuild(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("hostname")

	h.ScraperMu.RLock()
	scraper, ok := h.Scrapers[host]
	h.ScraperMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"host": host, "error": "scraper not found"})
		return
	}

	zimPath, err := scraper.BuildZIM()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"host": host, "error": err.Error()})
		return
	}

	h.Manager.Add(filepath.Base(zimPath))

	json.NewEncoder(w).Encode(map[string]interface{}{
		"host":     host,
		"zim_file": filepath.Base(zimPath),
	})
}

func (h *Handlers) handleArchiveRetry(w http.ResponseWriter, r *http.Request) {
	host := r.PathValue("hostname")

	h.ScraperMu.RLock()
	scraper, ok := h.Scrapers[host]
	h.ScraperMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"host": host, "retried": 0})
		return
	}

	var body struct {
		URLs []string `json:"urls"`
	}
	data, _ := io.ReadAll(r.Body)
	json.Unmarshal(data, &body)

	var parsedURLs []*url.URL
	for _, u := range body.URLs {
		parsed, err := url.Parse(u)
		if err == nil {
			parsedURLs = append(parsedURLs, parsed)
		}
	}

	count := scraper.RetryFailed(parsedURLs)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"host":    host,
		"retried": count,
	})
}
