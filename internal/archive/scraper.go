package archive

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cookiengineer/zimdex/internal/archive/filters"
)

type Scraper struct {
	Host              string
	StartURL          string
	DataDir           string
	RespectRobots     bool
	InsecureSkipVerify bool
	PageLimit         int
	PageWorkers       int
	AssetWorkers      int
	FilterNames       []string

	queue      *Queue
	downloader *Downloader
	robots     *RobotsMatcher

	mu         sync.RWMutex
	status     string
	startedAt  time.Time
	updatedAt  time.Time
	logBuf     []string
	activity   []ActivityEntry

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewScraper(dataDir, startURL string, respectRobots bool, insecureSkipVerify bool, pageLimit, pageWorkers, assetWorkers int, filterNames []string) (*Scraper, error) {
	host, err := extractHost(startURL)
	if err != nil {
		return nil, fmt.Errorf("invalid start URL: %w", err)
	}

	if pageWorkers <= 0 {
		pageWorkers = 2
	}
	if assetWorkers <= 0 {
		assetWorkers = 4
	}

	queuePath := filepath.Join(dataDir, host+".json")

	s := &Scraper{
		Host:               host,
		StartURL:           startURL,
		DataDir:            dataDir,
		RespectRobots:      respectRobots,
		InsecureSkipVerify: insecureSkipVerify,
		PageLimit:          pageLimit,
		PageWorkers:        pageWorkers,
		AssetWorkers:       assetWorkers,
		FilterNames:        filterNames,
		status:             "idle",
	}

	s.queue = NewQueue(queuePath, host, startURL, respectRobots, pageLimit)

	if _, err := os.Stat(queuePath); err == nil {
		reset := s.queue.ResetStale()
		if reset > 0 {
			s.log("Reset %d failed/stale entries for retry", reset)
		}
	}

	s.downloader = NewDownloader(dataDir, insecureSkipVerify)

	seedFilters := filters.Enabled(s.FilterNames)
	filterSeed := func(rawURL string) (newURL, downloadURL string) {
		for _, f := range seedFilters {
			if f.Detect(nil, rawURL) {
				filtered := f.FilterURL(rawURL)
				if filtered == "" {
					return "", ""
				}
				rawURL = filtered
			}
		}
		return applyPathRewrite(rawURL, "", seedFilters)
	}

	if respectRobots {
		matcher, sitemaps, delay, err := FetchRobotsTxt(host, s.downloader.client)
		if err == nil {
			s.robots = matcher
			s.log("robots.txt loaded, crawl-delay=%v", delay)
			if delay > 0 {
				ts := s.downloader.getThrottle(host)
				ts.SetCrawlDelay(delay)
			}
			if len(sitemaps) > 0 {
				seeds := FetchSitemaps(host, sitemaps, s.downloader.client)
				for _, seedURL := range seeds {
					newURL, downloadURL := filterSeed(seedURL)
					if newURL != "" {
						_, localPath, zimPath := URLToPath(newURL)
						e := QueueEntry{URL: newURL, Path: localPath, ZimPath: zimPath, EntryType: EntryTypePage, Status: StatusPending, Referrer: startURL}
						if downloadURL != "" && downloadURL != newURL {
							e.DownloadURL = downloadURL
						}
						s.queue.Add(e)
					}
				}
				s.log("sitemap: %d seed URLs from %d sitemaps", len(seeds), len(sitemaps))
			}
		}
	} else {
		seeds := FetchDefaultSitemap(host, s.downloader.client)
		for _, seedURL := range seeds {
			newURL, downloadURL := filterSeed(seedURL)
			if newURL != "" {
				_, localPath, zimPath := URLToPath(newURL)
				e := QueueEntry{URL: newURL, Path: localPath, ZimPath: zimPath, EntryType: EntryTypePage, Status: StatusPending, Referrer: startURL}
				if downloadURL != "" && downloadURL != newURL {
					e.DownloadURL = downloadURL
				}
				s.queue.Add(e)
			}
		}
		if len(seeds) > 0 {
			s.log("sitemap: %d seed URLs", len(seeds))
		}
	}

	canonStart := CanonicalizeURL(startURL)
	newURL, downloadURL := filterSeed(canonStart)
	if newURL != "" {
		_, localPath, zimPath := URLToPath(newURL)
		e := QueueEntry{URL: newURL, Path: localPath, ZimPath: zimPath, EntryType: EntryTypePage, Status: StatusPending}
		if downloadURL != "" && downloadURL != newURL {
			e.DownloadURL = downloadURL
		}
		s.queue.Add(e)
	}

	return s, nil
}

func (s *Scraper) Start() {
	s.mu.Lock()
	s.status = "running"
	s.startedAt = time.Now()
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.mu.Unlock()

	s.log("Scraping started: %s", s.StartURL)
	s.queue.Status = "running"
	s.queue.Save()

	for i := 0; i < s.PageWorkers; i++ {
		s.wg.Add(1)
		go s.worker(s.ctx, EntryTypePage)
	}

	for i := 0; i < s.AssetWorkers; i++ {
		s.wg.Add(1)
		go s.worker(s.ctx, EntryTypeAsset)
	}
}

func (s *Scraper) Pause() {
	s.mu.Lock()
	s.status = "paused"
	s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()

	s.queue.Status = "paused"
	s.queue.Save()
	s.log("Scraping paused")
}

func (s *Scraper) Continue() {
	s.queue.Load()
	s.Start()
	s.log("Scraping continued")
}

func (s *Scraper) Stop() {
	s.mu.Lock()
	s.status = "stopped"
	s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()

	s.queue.Status = "stopped"
	s.queue.Save()
	s.log("Scraping stopped")
}

func (s *Scraper) Status() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *Scraper) Queue() *Queue {
	return s.queue
}

func (s *Scraper) Stats() QueueStats {
	return s.queue.Stats
}

func (s *Scraper) BuildZIM() (string, error) {
	return BuildZIM(s.DataDir, s.queue, s.FilterNames)
}

func (s *Scraper) RetryFailed(urls []string) int {
	if len(urls) == 0 {
		failed := s.queue.FailedEntries()
		for _, e := range failed {
			e.Status = StatusPending
			e.RetryCount = 0
			e.Error = ""
		}
		s.queue.RecalcStats()
		return len(failed)
	}

	count := 0
	for _, rawURL := range urls {
		s.queue.UpdateEntry(rawURL, func(e *QueueEntry) {
			e.Status = StatusPending
			e.RetryCount = 0
			e.Error = ""
		})
		count++
	}
	return count
}

func (s *Scraper) RecentLogs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.logBuf) == 0 {
		return nil
	}

	n := len(s.logBuf)
	if n > 10 {
		return s.logBuf[n-10:]
	}
	return s.logBuf
}

func (s *Scraper) log(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[%s] %s", s.Host, msg)

	s.mu.Lock()
	s.logBuf = append(s.logBuf, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg))
	if len(s.logBuf) > 100 {
		s.logBuf = s.logBuf[len(s.logBuf)-100:]
	}
	s.mu.Unlock()
}

type ActivityEntry struct {
	Time     string `json:"time"`
	URL      string `json:"url"`
	Status   string `json:"status"`
	HTTPCode int    `json:"http_code,omitempty"`
}

func (s *Scraper) trackActivity(entry *QueueEntry, httpCode int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	shortURL := entry.URL
	if len(shortURL) > 80 {
		shortURL = shortURL[:80] + "..."
	}

	a := ActivityEntry{
		Time:   time.Now().Format("15:04:05"),
		URL:    shortURL,
		Status: string(entry.Status),
	}

	switch {
	case httpCode > 0:
		a.HTTPCode = httpCode
		a.Status = fmt.Sprintf("%d %s", httpCode, entry.Status)
	case entry.Error != "":
		a.Status = entry.Error
		if len(a.Status) > 40 {
			a.Status = a.Status[:40] + "..."
		}
	}

	s.activity = append(s.activity, a)
	if len(s.activity) > 50 {
		s.activity = s.activity[len(s.activity)-50:]
	}
}

func (s *Scraper) RecentActivity() []ActivityEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ActivityEntry, len(s.activity))
	copy(result, s.activity)

	n := len(result)
	for i := 0; i < n/2; i++ {
		result[i], result[n-1-i] = result[n-1-i], result[i]
	}

	return result
}

func applyPathRewrite(filteredURL, pageURL string, activeFilters []filters.Filter) (newURL, downloadURL string) {
	return filters.ApplyURLRewriter(filteredURL, activeFilters)
}

func (s *Scraper) worker(ctx context.Context, entryType EntryType) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		s.mu.RLock()
		if s.status != "running" {
			s.mu.RUnlock()
			return
		}
		s.mu.RUnlock()

	entry := s.queue.PopPending(entryType)
	if entry == nil {
		remaining := s.queue.PendingCount()
		active := s.queue.DownloadingCount()
		if remaining == 0 && active == 0 {
			s.mu.Lock()
			if s.status == "running" {
				s.status = "complete"
			}
			s.mu.Unlock()
			s.queue.Status = "complete"
			s.queue.Save()
			s.log("Scraping complete")
			return
		}
		time.Sleep(500 * time.Millisecond)
		continue
	}

		err := s.downloader.Download(ctx, entry)

		s.trackActivity(entry, 0)

		if err != nil {
			if errors.Is(err, ErrRedirect) {
				redirectURL := extractRedirectURL(err)
				if redirectURL != "" {
					_, localPath, zimPath := URLToPath(redirectURL)
					s.queue.AddIfNew(redirectURL, localPath, zimPath, entryType, entry.URL)
				}
			} else if errors.Is(err, ErrRetry) {
				entry.Status = StatusPending
			} else {
				s.log("Failed: %s — %v", entry.URL, err)
			}
		}

		s.queue.RecalcStats()

		if entry.Status == StatusDownloaded && entryType == EntryTypePage {
			if s.PageLimit > 0 && s.queue.DownloadedCount() >= s.PageLimit {
				s.log("Page limit reached (%d)", s.PageLimit)
			} else {
				s.extractAndEnqueue(entry)
			}
		}

		s.queue.Save()
	}
}

func (s *Scraper) extractAndEnqueue(entry *QueueEntry) {
	cachePath := filepath.Join(s.DataDir, entry.Path)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		s.log("Read cache error: %s — %v", entry.Path, err)
		return
	}

	pageURL := entry.URL
	if entry.DownloadURL != "" {
		pageURL = entry.DownloadURL
	}

	activeFilters := filters.Enabled(s.FilterNames)

	ext, err := NewExtractor(pageURL, s.Host, pageURL)
	if err != nil {
		return
	}

	urls := ext.Extract(data)

	for _, u := range urls {
		filtered := filters.ApplyURLFilters(u.URL, data, pageURL, activeFilters)
		if filtered == "" {
			continue
		}

		if s.robots != nil && !s.robots.IsAllowed(filtered) {
			continue
		}

		newURL, downloadURL := applyPathRewrite(filtered, pageURL, activeFilters)
		_, localPath, zimPath := URLToPath(newURL)

		entry := QueueEntry{
			URL:         newURL,
			DownloadURL: downloadURL,
			Path:        localPath,
			ZimPath:     zimPath,
			EntryType:   u.EntryType,
			Status:      StatusPending,
			Referrer:    pageURL,
		}
		if downloadURL != "" && downloadURL != newURL {
			entry.DownloadURL = downloadURL
		} else {
			entry.DownloadURL = ""
		}
		s.queue.Add(entry)
	}
}

func extractHost(rawURL string) (string, error) {
	u, err := parseURL(rawURL)
	if err != nil {
		return "", err
	}
	return u.Hostname(), nil
}

func extractRedirectURL(err error) string {
	msg := err.Error()
	const prefix = "redirect: "
	if idx := strings.Index(msg, prefix); idx >= 0 {
		return msg[idx+len(prefix):]
	}
	return ""
}
