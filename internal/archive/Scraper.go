package archive

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cookiengineer/zimdex/internal/archive/filters"
)

type Scraper struct {
	Options ScraperOptions

	Host     string
	StartURL *url.URL

	queue      *Queue
	downloader *Downloader
	robots     *RobotsMatcher

	mutex     sync.RWMutex
	status    string
	started_at time.Time
	updated_at time.Time
	log_buffer []string
	activity   []ActivityEntry

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewScraper(options ScraperOptions) (*Scraper, error) {

	if options.URL == nil {
		return nil, fmt.Errorf("URL is required in ScraperOptions")
	}

	hostname := options.URL.Hostname()
	starturl := options.URL

	if options.PageWorkers <= 0 {
		options.PageWorkers = 2
	}

	if options.AssetWorkers <= 0 {
		options.AssetWorkers = 4
	}

	queue_path := filepath.Join(options.Folder, hostname+".json")

	scraper := &Scraper{
		Options:    options,
		Host:       hostname,
		StartURL:   starturl,
		status:     "idle",
		queue:      zimfs.NewQueue(queue_path, hostname, starturl, options.RespectRobots),
		downloader: NewDownloader(options.Folder, options.IgnoreInsecureSSL),
	}


	if _, err_exists := os.Stat(queue_path); err_exists == nil {

		count := scraper.queue.ResetStale()

		if count > 0 {
			scraper.log("Reset %d failed/stale entries for retry", count)
		}

	}


	// TODO: Filters need to be reworked, this rewrites the path for SeedURL!?
	seedFilters := filters.Enabled(options.Filters)
	filterSeed := func(rawURL *url.URL) (newURL, downloadURL *url.URL) {
		for _, f := range seedFilters {
			if f.Detect(nil, rawURL) {
				filtered := f.FilterURL(rawURL)
				if filtered == nil {
					return nil, nil
				}
				rawURL = filtered
			}
		}
		return applyPathRewrite(rawURL, nil, seedFilters)
	}

	// TODO: This should be Downloader.FetchRobots()
	// TODO: This should be Downloader.FetchSitemap()
	if options.RespectRobots == true {
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
					if newURL != nil {
						_, localPath, zimPath := URLToPath(newURL)
						e := QueueEntry{URL: newURL, Path: localPath, ZimPath: zimPath, EntryType: EntryTypePage, Status: StatusPending, Referrer: startURL}
						if downloadURL != nil && downloadURL != newURL {
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
			if newURL != nil {
				_, localPath, zimPath := URLToPath(newURL)
				e := QueueEntry{URL: newURL, Path: localPath, ZimPath: zimPath, EntryType: EntryTypePage, Status: StatusPending, Referrer: startURL}
				if downloadURL != nil && downloadURL != newURL {
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
	if newURL != nil {
		_, localPath, zimPath := URLToPath(newURL)
		e := QueueEntry{URL: newURL, Path: localPath, ZimPath: zimPath, EntryType: EntryTypePage, Status: StatusPending}
		if downloadURL != nil && downloadURL != newURL {
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

	for i := 0; i < s.Options.PageWorkers; i++ {
		s.wg.Add(1)
		worker := NewScraperWorker(s.ctx, EntryTypePage, s.queue, s, &s.wg)
		go worker.Run()
	}

	for i := 0; i < s.Options.AssetWorkers; i++ {
		s.wg.Add(1)
		worker := NewScraperWorker(s.ctx, EntryTypeAsset, s.queue, s, &s.wg)
		go worker.Run()
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

func (s *Scraper) Status() string {
	s.mu.RLock()
	defer r.mu.RUnlock()
	return s.status
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
	return BuildZIM(s.Options.Folder, s.queue, s.Options.Filters)
}

func (s *Scraper) RetryFailed(urls []*url.URL) int {
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
	for _, u := range urls {
		s.queue.UpdateEntry(u, func(e *QueueEntry) {
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

	shortURL := ""
	if entry.URL != nil {
		shortURL = entry.URL.String()
	}
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

func applyPathRewrite(filteredURL *url.URL, pageURL *url.URL, activeFilters []filters.Filter) (newURL, downloadURL *url.URL) {
	return filters.ApplyURLRewriter(filteredURL, activeFilters)
}

func (s *Scraper) extractAndEnqueue(entry *QueueEntry) {
	cachePath := filepath.Join(s.Options.Folder, entry.Path)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		s.log("Read cache error: %s — %v", entry.Path, err)
		return
	}

	pageURL := entry.URL
	if entry.DownloadURL != nil {
		pageURL = entry.DownloadURL
	}

	activeFilters := filters.Enabled(s.Options.Filters)

	ext, err := NewExtractor(pageURL, s.Host, pageURL)
	if entry.EntryType == EntryTypeExternalPage {
		ext, err = NewExtractorAssetsOnly(pageURL, s.Host, pageURL)
	}
	if err != nil {
		return
	}

	urls := ext.Extract(data)

	for _, u := range urls {
		filtered := filters.ApplyURLFilters(u.URL, data, pageURL, activeFilters)
		if filtered == nil {
			continue
		}

		if s.robots != nil && !s.robots.IsAllowed(filtered.Path) {
			continue
		}

		newURL, downloadURL := applyPathRewrite(filtered, pageURL, activeFilters)
		_, localPath, zimPath := URLToPath(newURL)

		newEntry := QueueEntry{
			URL:       newURL,
			Path:      localPath,
			ZimPath:   zimPath,
			EntryType: u.EntryType,
			Status:    StatusPending,
			Referrer:  pageURL,
		}
		if downloadURL != nil && downloadURL != newURL {
			newEntry.DownloadURL = downloadURL
		}
		s.queue.Add(newEntry)
	}
}


