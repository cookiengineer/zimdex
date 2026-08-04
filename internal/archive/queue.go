package archive

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cookiengineer/zimdex/internal/archive/filters"
)

type EntryType string

const (
	EntryTypePage         EntryType = "page"
	EntryTypeAsset        EntryType = "asset"
	EntryTypeExternalPage EntryType = "external_page"
)

type QueueStatus string

const (
	StatusPending     QueueStatus = "pending"
	StatusDownloading QueueStatus = "downloading"
	StatusDownloaded  QueueStatus = "downloaded"
	StatusFailed      QueueStatus = "failed"
	StatusSkipped     QueueStatus = "skipped"
)

type QueueEntry struct {
	URL          *url.URL    `json:"-"`
	DownloadURL  *url.URL    `json:"-"`
	Path         string      `json:"path"`
	ZimPath      string      `json:"zim_path"`
	EntryType    EntryType   `json:"entry_type"`
	MimeType     string      `json:"mime_type,omitempty"`
	Status       QueueStatus `json:"status"`
	Size         int64       `json:"size,omitempty"`
	Referrer     *url.URL    `json:"-"`
	DownloadedAt string      `json:"downloaded_at,omitempty"`
	Error        string      `json:"error,omitempty"`
	RetryCount   int         `json:"retry_count"`
}

func (e QueueEntry) MarshalJSON() ([]byte, error) {
	type Alias QueueEntry
	return json.Marshal(&struct {
		URL         string `json:"url"`
		DownloadURL string `json:"download_url,omitempty"`
		Referrer    string `json:"referrer,omitempty"`
		*Alias
	}{
		URL:         urlStr(e.URL),
		DownloadURL: urlStr(e.DownloadURL),
		Referrer:    urlStr(e.Referrer),
		Alias:       (*Alias)(&e),
	})
}

func (e *QueueEntry) UnmarshalJSON(data []byte) error {
	type Alias QueueEntry
	aux := &struct {
		URL         string `json:"url"`
		DownloadURL string `json:"download_url,omitempty"`
		Referrer    string `json:"referrer,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(e),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	e.URL = parseURLStr(aux.URL)
	e.DownloadURL = parseURLStr(aux.DownloadURL)
	e.Referrer = parseURLStr(aux.Referrer)
	return nil
}

type QueueStats struct {
	Pending     int `json:"pending"`
	Downloading int `json:"downloading"`
	Downloaded  int `json:"downloaded"`
	Failed      int `json:"failed"`
	Skipped     int `json:"skipped"`
}

type Queue struct {
	Host          string       `json:"host"`
	StartURL      *url.URL     `json:"-"`
	RespectRobots bool         `json:"respect_robots"`
	CreatedAt     string       `json:"created_at"`
	UpdatedAt     string       `json:"updated_at"`
	Status        string       `json:"status"`
	PageLimit     int          `json:"page_limit"`
	Stats         QueueStats   `json:"stats"`
	Entries       []QueueEntry `json:"entries"`

	filePath string
	dataDir  string
	mu       sync.Mutex
	urlIndex map[string]int
}

func (q *Queue) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Host          string       `json:"host"`
		StartURL      string       `json:"start_url"`
		RespectRobots bool         `json:"respect_robots"`
		CreatedAt     string       `json:"created_at"`
		UpdatedAt     string       `json:"updated_at"`
		Status        string       `json:"status"`
		PageLimit     int          `json:"page_limit"`
		Stats         QueueStats   `json:"stats"`
		Entries       []QueueEntry `json:"entries"`
	}{
		Host:          q.Host,
		StartURL:      urlStr(q.StartURL),
		RespectRobots: q.RespectRobots,
		CreatedAt:     q.CreatedAt,
		UpdatedAt:     q.UpdatedAt,
		Status:        q.Status,
		PageLimit:     q.PageLimit,
		Stats:         q.Stats,
		Entries:       q.Entries,
	})
}

func (q *Queue) UnmarshalJSON(data []byte) error {
	type Alias Queue
	aux := &struct {
		StartURL string `json:"start_url"`
		*Alias
	}{
		Alias: (*Alias)(q),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	q.StartURL = parseURLStr(aux.StartURL)
	return nil
}

func urlStr(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.String()
}

func parseURLStr(s string) *url.URL {
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil
	}
	return u
}

func NewQueue(filePath string, host string, startURL *url.URL, respectRobots bool, pageLimit int) *Queue {
	q := &Queue{
		Host:          host,
		StartURL:      startURL,
		RespectRobots: respectRobots,
		PageLimit:     pageLimit,
		filePath:      filePath,
		dataDir:       filepath.Dir(filePath),
		urlIndex:      make(map[string]int),
	}

	if _, err := os.Stat(filePath); err == nil {
		q.Load()
	} else {
		q.Status = "idle"
		q.Save()
	}

	return q
}

func (q *Queue) Load() {
	q.mu.Lock()
	defer q.mu.Unlock()

	data, err := os.ReadFile(q.filePath)
	if err != nil {
		return
	}

	var loaded Queue
	if err := json.Unmarshal(data, &loaded); err != nil {
		return
	}

	q.Host = loaded.Host
	q.StartURL = loaded.StartURL
	q.RespectRobots = loaded.RespectRobots
	q.CreatedAt = loaded.CreatedAt
	q.UpdatedAt = loaded.UpdatedAt
	q.Status = loaded.Status
	q.PageLimit = loaded.PageLimit
	q.Entries = loaded.Entries

	q.urlIndex = make(map[string]int)
	for i, e := range q.Entries {
		q.urlIndex[canonicalStr(e.URL)] = i
	}
	q.RecalcStats()
}

func (q *Queue) Save() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.RecalcStatsLRU()
	data, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return
	}

	dir := filepath.Dir(q.filePath)
	os.MkdirAll(dir, 0755)

	tmpPath := q.filePath + ".tmp"
	os.WriteFile(tmpPath, data, 0644)
	os.Rename(tmpPath, q.filePath)
}

func (q *Queue) Add(entry QueueEntry) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	canon := canonicalStr(entry.URL)
	if _, exists := q.urlIndex[canon]; exists {
		return false
	}

	if entry.Path != "" {
		diskPath := filepath.Join(q.dataDir, entry.Path)
		if info, err := os.Stat(diskPath); err == nil && info.Size() > 0 {
			entry.Status = StatusDownloaded
			entry.Size = info.Size()
			q.urlIndex[canon] = len(q.Entries)
			q.Entries = append(q.Entries, entry)
			q.adjustStats(StatusDownloaded, 1)
			return true
		}
	}

	q.urlIndex[canon] = len(q.Entries)
	q.Entries = append(q.Entries, entry)
	q.adjustStats(entry.Status, 1)
	return true
}

func (q *Queue) AddIfNew(rawURL *url.URL, localPath, zimPath string, entryType EntryType, referrer *url.URL) bool {
	canon := canonicalStr(rawURL)

	q.mu.Lock()
	if _, exists := q.urlIndex[canon]; exists {
		q.mu.Unlock()
		return false
	}
	q.mu.Unlock()

	return q.Add(QueueEntry{
		URL:       CanonicalizeURL(rawURL),
		Path:      localPath,
		ZimPath:   zimPath,
		EntryType: entryType,
		Status:    StatusPending,
		Referrer:  referrer,
	})
}

func (q *Queue) HasURL(u *url.URL) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	_, exists := q.urlIndex[canonicalStr(u)]
	return exists
}

func (q *Queue) GetByStatus(status QueueStatus) []*QueueEntry {
	q.mu.Lock()
	defer q.mu.Unlock()

	var result []*QueueEntry
	for i := range q.Entries {
		if q.Entries[i].Status == status {
			result = append(result, &q.Entries[i])
		}
	}
	return result
}

func (q *Queue) PopPending(entryType EntryType) *QueueEntry {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i := range q.Entries {
		e := &q.Entries[i]
		if e.Status == StatusPending && e.EntryType == entryType {
			e.Status = StatusDownloading
			q.Stats.Pending--
			q.Stats.Downloading++
			return e
		}
	}
	return nil
}

func (q *Queue) UpdateEntry(u *url.URL, updater func(*QueueEntry)) {
	q.mu.Lock()
	defer q.mu.Unlock()

	canon := canonicalStr(u)
	idx, ok := q.urlIndex[canon]
	if !ok {
		return
	}

	before := q.Entries[idx].Status
	updater(&q.Entries[idx])
	after := q.Entries[idx].Status

	if before != after {
		q.adjustStats(before, -1)
		q.adjustStats(after, 1)
	}

	if after != StatusPending {
		delete(q.urlIndex, canon)
	}
}

func (q *Queue) adjustStats(status QueueStatus, delta int) {
	switch status {
	case StatusPending:
		q.Stats.Pending += delta
	case StatusDownloading:
		q.Stats.Downloading += delta
	case StatusDownloaded:
		q.Stats.Downloaded += delta
	case StatusFailed:
		q.Stats.Failed += delta
	case StatusSkipped:
		q.Stats.Skipped += delta
	}
}

func (q *Queue) RecalcStats() {
	q.Stats = QueueStats{}
	for _, e := range q.Entries {
		q.adjustStats(e.Status, 1)
	}
}

func (q *Queue) RecalcStatsLRU() {
	q.Stats = QueueStats{}
	for _, e := range q.Entries {
		q.adjustStats(e.Status, 1)
	}
}

func (q *Queue) PendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.Stats.Pending
}

func (q *Queue) DownloadedCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.Stats.Downloaded
}

func (q *Queue) DownloadingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.Stats.Downloading
}

func (q *Queue) FailedCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.Stats.Failed
}

func (q *Queue) SkippedCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.Stats.Skipped
}

func (q *Queue) FailedEntries() []*QueueEntry {
	return q.GetByStatus(StatusFailed)
}

func (q *Queue) ResetStale() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	count := 0
	for i := range q.Entries {
		e := &q.Entries[i]
		if e.Status == StatusFailed || e.Status == StatusDownloading {
			e.Status = StatusPending
			e.RetryCount = 0
			e.Error = ""
			count++
		}
	}

	q.RecalcStatsLRU()
	return count
}

func canonicalStr(u *url.URL) string {
	if u == nil {
		return ""
	}
	return CanonicalizeURL(u).String()
}

func filterTrackingParams(u *url.URL) {
	filters.FilterTrackingParams(u)
}

func CanonicalizeURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}

	clone := *u
	clone.Fragment = ""
	clone.RawFragment = ""

	filterTrackingParams(&clone)

	clone.Scheme = strings.ToLower(clone.Scheme)
	clone.Host = strings.ToLower(clone.Host)

	if clone.Scheme == "https" && strings.HasSuffix(clone.Host, ":443") {
		clone.Host = strings.TrimSuffix(clone.Host, ":443")
	} else if clone.Scheme == "http" && strings.HasSuffix(clone.Host, ":80") {
		clone.Host = strings.TrimSuffix(clone.Host, ":80")
	}

	if clone.Path == "" {
		clone.Path = "/"
	} else if len(clone.Path) > 1 && strings.HasSuffix(clone.Path, "/") {
		clone.Path = strings.TrimSuffix(clone.Path, "/")
	}

	return &clone
}

func URLToPath(u *url.URL) (hostname, localPath, zimPath string) {
	if u == nil {
		return
	}

	hostname = u.Hostname()
	path := u.Path
	if path == "" || path == "/" {
		path = "/index.html"
	}
	if u.RawQuery != "" {
		path = path + "?" + u.RawQuery
	}

	safePath := strings.ReplaceAll(strings.ReplaceAll(path, "?", "%3F"), "&", "%26")
	localPath = filepath.Join(hostname, filepath.FromSlash(strings.TrimPrefix(safePath, "/")))

	zimPath = fmt.Sprintf("%s%s", hostname, path)

	return
}
