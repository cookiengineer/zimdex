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

	utils_urls "github.com/cookiengineer/zimdex/utils/urls"
	"github.com/cookiengineer/zimdex/io/zimfs"
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
		queue:      zimfs.NewQueue(options.Folder, starturl, options.Filters),
		downloader: NewDownloader(options.Folder, options.IgnoreInsecureSSL),
	}

	if scraper.queue.Reset() == true {
		scraper.log("Reset failed/stale entries for retry")
	}

	enqueueSeed := func(rawURL *url.URL) {
		scraper.queue.Enqueue(rawURL, rawURL, zimfs.QueueEntryTypePage)
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
					enqueueSeed(seedURL)
				}
				scraper.log("sitemap: %d seed URLs from %d sitemaps", len(seeds), len(sitemaps))
			}
		}
	} else {
		seeds := FetchDefaultSitemap(starturl.Hostname(), scraper.downloader.client)
		for _, seedURL := range seeds {
			enqueueSeed(seedURL)
		}
		if len(seeds) > 0 {
			scraper.log("sitemap: %d seed URLs", len(seeds))
		}
	}

	canonStart := utils_urls.Canonicalize(starturl)
	enqueueSeed(canonStart)

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
	builder := zimfs.NewBuilder(s.Options.Folder)
	return builder.Build(s.queue)
}

func (s *Scraper) RetryFailed(urls []*url.URL) int {
	failed := s.queue.Query(zimfs.QueueEntryStatusFailed)
	if len(failed) == 0 {
		return 0
	}

	wanted := make(map[string]bool, len(urls))
	for _, u := range urls {
		if u != nil {
			wanted[utils_urls.Canonicalize(u).String()] = true
		}
	}

	count := 0
	for _, entry := range failed {
		if len(wanted) > 0 {
			key := ""
			if entry.WebURL != nil {
				key = utils_urls.Canonicalize(entry.WebURL).String()
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

func (s *Scraper) extractAndEnqueue(entry *zimfs.QueueEntry) {
	cachePath := filepath.Join(s.Options.Folder, entry.Path())
	data, err := os.ReadFile(cachePath)
	if err != nil {
		s.log("Read cache error: %s — %v", entry.Path(), err)
		return
	}

	pageURL := entry.WebURL

	ext, err := NewExtractor(pageURL, s.StartURL.Hostname(), pageURL)
	if entry.Type == zimfs.QueueEntryTypeExternalPage {
		ext, err = NewExtractorAssetsOnly(pageURL, s.StartURL.Hostname(), pageURL)
	}
	if err != nil {
		return
	}

	urls := ext.Extract(data)

	for _, u := range urls {
		if s.robots != nil && !s.robots.IsAllowed(u.URL.Path) {
			continue
		}

		s.queue.Enqueue(u.URL, pageURL, u.EntryType)
	}
}
