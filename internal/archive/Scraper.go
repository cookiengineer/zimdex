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
	"github.com/cookiengineer/zimdex/internal/utils"
	"github.com/cookiengineer/zimdex/internal/zimfs"
)

type Scraper struct {
	Options ScraperOptions

	StartURL *url.URL

	queue      *zimfs.Queue
	downloader *Downloader
	robots     *RobotsMatcher

	mutex      sync.RWMutex
	status     string
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

	starturl := options.URL

	if options.PageWorkers <= 0 {
		options.PageWorkers = 2
	}

	if options.AssetWorkers <= 0 {
		options.AssetWorkers = 4
	}

	scraper := &Scraper{
		Options:    options,
		StartURL:   starturl,
		status:     "idle",
		queue:      zimfs.NewQueue(options.Folder, starturl),
		downloader: NewDownloader(options.Folder, options.IgnoreInsecureSSL),
	}

	if scraper.queue.Reset() == true {
		scraper.log("Reset failed/stale entries for retry")
	}

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

	enqueueSeed := func(newURL, downloadURL *url.URL) {
		if newURL == nil {
			return
		}

		dl := downloadURL
		if dl == nil {
			dl = newURL
		}

		scraper.queue.Add(*zimfs.NewQueueEntry(dl, newURL, zimfs.QueueEntryTypePage, starturl))
	}

	// TODO: This should be Downloader.FetchRobots()
	// TODO: This should be Downloader.FetchSitemap()
	if options.RespectRobots == true {
		matcher, sitemaps, delay, err := FetchRobotsTxt(starturl.Hostname(), scraper.downloader.client)
		if err == nil {
			scraper.robots = matcher
			scraper.log("robots.txt loaded, crawl-delay=%v", delay)
			if delay > 0 {
				ts := scraper.downloader.getThrottle(starturl.Hostname())
				ts.SetCrawlDelay(delay)
			}
			if len(sitemaps) > 0 {
				seeds := FetchSitemaps(starturl.Hostname(), sitemaps, scraper.downloader.client)
				for _, seedURL := range seeds {
					enqueueSeed(filterSeed(seedURL))
				}
				scraper.log("sitemap: %d seed URLs from %d sitemaps", len(seeds), len(sitemaps))
			}
		}
	} else {
		seeds := FetchDefaultSitemap(starturl.Hostname(), scraper.downloader.client)
		for _, seedURL := range seeds {
			enqueueSeed(filterSeed(seedURL))
		}
		if len(seeds) > 0 {
			scraper.log("sitemap: %d seed URLs", len(seeds))
		}
	}

	canonStart := utils.CanonicalizeURL(starturl)
	enqueueSeed(filterSeed(canonStart))

	return scraper, nil
}

func (s *Scraper) Start() {
	s.mutex.Lock()
	s.status = "running"
	s.started_at = time.Now()
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.mutex.Unlock()

	s.log("Scraping started: %s", s.StartURL)
	s.queue.Status = zimfs.QueueStatusRunning
	s.queue.Write()

	for i := 0; i < s.Options.PageWorkers; i++ {
		s.wg.Add(1)
		worker := NewScraperWorker(s.ctx, zimfs.QueueEntryTypePage, s.queue, s, &s.wg)
		go worker.Run()
	}

	for i := 0; i < s.Options.AssetWorkers; i++ {
		s.wg.Add(1)
		worker := NewScraperWorker(s.ctx, zimfs.QueueEntryTypeAsset, s.queue, s, &s.wg)
		go worker.Run()
	}
}

func (s *Scraper) Pause() {
	s.mutex.Lock()
	s.status = "paused"
	s.mutex.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()

	s.queue.Status = zimfs.QueueStatusPaused
	s.queue.Write()
	s.log("Scraping paused")
}

func (s *Scraper) Continue() {
	s.queue.Read()
	s.Start()
	s.log("Scraping continued")
}

func (s *Scraper) Status() string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.status
}

func (s *Scraper) completeIfRunning() {
	s.mutex.Lock()
	if s.status == "running" {
		s.status = "complete"
	}
	s.mutex.Unlock()
}

func (s *Scraper) Stop() {
	s.mutex.Lock()
	s.status = "stopped"
	s.mutex.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()

	s.queue.Status = zimfs.QueueStatusPaused
	s.queue.Write()
	s.log("Scraping stopped")
}

func (s *Scraper) Queue() *zimfs.Queue {
	return s.queue
}

func (s *Scraper) BuildZIM() (string, error) {
	return BuildZIM(s.Options.Folder, s.queue, s.Options.Filters)
}

func (s *Scraper) RetryFailed(urls []*url.URL) int {
	failed := s.queue.Query(zimfs.QueueEntryStatusFailed)
	if len(failed) == 0 {
		return 0
	}

	wanted := make(map[string]bool, len(urls))
	for _, u := range urls {
		if u != nil {
			wanted[utils.CanonicalizeURL(u).String()] = true
		}
	}

	count := 0
	for _, entry := range failed {
		if len(wanted) > 0 {
			key := ""
			if entry.WebURL != nil {
				key = utils.CanonicalizeURL(entry.WebURL).String()
			}
			if !wanted[key] {
				continue
			}
		}

		entry.Status = zimfs.QueueEntryStatusPending
		entry.Retries = 0
		entry.StatusCode = 0
		s.queue.Set(entry)
		count++
	}

	return count
}

func (s *Scraper) RecentLogs() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if len(s.log_buffer) == 0 {
		return nil
	}

	n := len(s.log_buffer)
	if n > 10 {
		return s.log_buffer[n-10:]
	}
	return s.log_buffer
}

func (s *Scraper) log(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[%s] %s", s.StartURL.Hostname(), msg)

	s.mutex.Lock()
	s.log_buffer = append(s.log_buffer, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg))
	if len(s.log_buffer) > 100 {
		s.log_buffer = s.log_buffer[len(s.log_buffer)-100:]
	}
	s.mutex.Unlock()
}

type ActivityEntry struct {
	Time     string `json:"time"`
	URL      string `json:"url"`
	Status   string `json:"status"`
	HTTPCode int    `json:"http_code,omitempty"`
}

func (s *Scraper) trackActivity(entry *zimfs.QueueEntry, httpCode int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	shortURL := ""
	if entry.WebURL != nil {
		shortURL = entry.WebURL.String()
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
	case entry.StatusCode != 0 && entry.Status == zimfs.QueueEntryStatusFailed:
		a.Status = fmt.Sprintf("%d %s", entry.StatusCode, entry.Status)
	}

	s.activity = append(s.activity, a)
	if len(s.activity) > 50 {
		s.activity = s.activity[len(s.activity)-50:]
	}
}

func (s *Scraper) RecentActivity() []ActivityEntry {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

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

func (s *Scraper) extractAndEnqueue(entry *zimfs.QueueEntry) {
	cachePath := filepath.Join(s.Options.Folder, entry.Path())
	data, err := os.ReadFile(cachePath)
	if err != nil {
		s.log("Read cache error: %s — %v", entry.Path(), err)
		return
	}

	pageURL := entry.WebURL

	activeFilters := filters.Enabled(s.Options.Filters)

	ext, err := NewExtractor(pageURL, s.StartURL.Hostname(), pageURL)
	if entry.Type == zimfs.QueueEntryTypeExternalPage {
		ext, err = NewExtractorAssetsOnly(pageURL, s.StartURL.Hostname(), pageURL)
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
		if newURL == nil {
			continue
		}

		dl := downloadURL
		if dl == nil {
			dl = newURL
		}

		s.queue.Add(*zimfs.NewQueueEntry(dl, newURL, u.EntryType, pageURL))
	}
}
