//go:build ignore
package zimfs

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	// "github.com/cookiengineer/zimdex/internal/archive/filters"
)

type Queue struct {
	Host          string       `json:"host"`
	StartURL      *url.URL     `json:"-"`
	RespectRobots bool         `json:"respect_robots"`
	CreatedAt     string       `json:"created_at"`
	UpdatedAt     string       `json:"updated_at"`
	Status        string       `json:"status"`
	Info          QueueInfo    `json:"stats"`

	Entries       []QueueEntry `json:"entries"`

	filePath string
	dataDir  string
	mu       sync.Mutex
	urlIndex map[string]int
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

// TODO: zimpath should be based on ZimURL
// TODO: After Refactor it should use only WebURL and ZimURL, ZimURL is computed by Scraper via filters.MediaWiki or other filters

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
