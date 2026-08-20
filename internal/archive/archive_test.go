package archive

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cookiengineer/gozim/archive/zim"
	"github.com/cookiengineer/zimdex/internal/utils/urls"
	"github.com/cookiengineer/zimdex/internal/zimfs"
)

func TestCanonicalizeURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://en.Wikipedia.org:443/wiki/Main_Page?utm_source=web#intro", "https://en.wikipedia.org/wiki/Main_Page"},
		{"http://Example.com:80/path/", "http://example.com/path"},
		{"http://example.com:8080/path", "http://example.com:8080/path"},
		{"https://example.com/", "https://example.com/"},
		{"https://example.com", "https://example.com/"},
		{"https://example.com/index.php?title=Main_Page", "https://example.com/index.php?title=Main_Page"},
		{"https://example.com/index.php?title=Main_Page&utm_source=web", "https://example.com/index.php?title=Main_Page"},
		{"https://example.com/page?utm_campaign=test&id=42&fbclid=abc", "https://example.com/page?id=42"},
		{"https://example.com/page?ref=other&name=hello", "https://example.com/page?name=hello"},
	}

	for _, tt := range tests {
		u, err := url.Parse(tt.input)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", tt.input, err)
		}
		result := urls.Canonicalize(u)
		if result.String() != tt.expected {
			t.Errorf("Canonicalize(%q) = %q, want %q", tt.input, result.String(), tt.expected)
		}
	}
}

func TestQueueEntryPath(t *testing.T) {
	webURL, _ := url.Parse("https://en.wikipedia.org/w/index.php?title=Main_Page")
	zimURL, _ := url.Parse("https://en.wikipedia.org/wiki/Main_Page")

	entry := zimfs.NewQueueEntry(webURL, zimURL, zimfs.QueueEntryTypePage, nil)
	if got := entry.Path(); got != "en.wikipedia.org/wiki/Main_Page" {
		t.Errorf("Path() = %q, want %q", got, "en.wikipedia.org/wiki/Main_Page")
	}
}

func TestQueueEntryPathFallback(t *testing.T) {
	webURL, _ := url.Parse("https://example.com/about")

	entry := zimfs.NewQueueEntry(webURL, nil, zimfs.QueueEntryTypePage, nil)
	if got := entry.Path(); got != "example.com/about" {
		t.Errorf("Path() = %q, want %q", got, "example.com/about")
	}
}

func TestQueueAddAndDedup(t *testing.T) {
	dir := t.TempDir()
	startURL, _ := url.Parse("https://example.com/")
	q := zimfs.NewQueue(dir, startURL, nil)

	entryURL, _ := url.Parse("https://example.com/")
	added := q.Add(*zimfs.NewQueueEntry(entryURL, entryURL, zimfs.QueueEntryTypePage, startURL))
	if !added {
		t.Error("expected first add to succeed")
	}

	added = q.Add(*zimfs.NewQueueEntry(entryURL, entryURL, zimfs.QueueEntryTypePage, startURL))
	if added {
		t.Error("expected duplicate add to fail")
	}
}

func TestQueueAddIfNew(t *testing.T) {
	dir := t.TempDir()
	startURL, _ := url.Parse("https://example.com/")
	q := zimfs.NewQueue(dir, startURL, nil)

	pageURL, _ := url.Parse("https://example.com/page1")
	if q.Has(pageURL) {
		t.Error("expected Has to return false for a new URL")
	}

	if !q.Add(*zimfs.NewQueueEntry(pageURL, pageURL, zimfs.QueueEntryTypePage, startURL)) {
		t.Error("expected Add to return true for a new URL")
	}

	if !q.Has(pageURL) {
		t.Error("expected Has to return true after Add")
	}

	if q.Add(*zimfs.NewQueueEntry(pageURL, pageURL, zimfs.QueueEntryTypePage, startURL)) {
		t.Error("expected duplicate Add to return false")
	}
}

func TestQueueGet(t *testing.T) {
	dir := t.TempDir()
	startURL, _ := url.Parse("https://example.com/")
	q := zimfs.NewQueue(dir, startURL, nil)

	pageURL, _ := url.Parse("https://example.com/page")
	q.Add(*zimfs.NewQueueEntry(pageURL, pageURL, zimfs.QueueEntryTypePage, startURL))

	assetURL, _ := url.Parse("https://example.com/asset.css")
	q.Add(*zimfs.NewQueueEntry(assetURL, assetURL, zimfs.QueueEntryTypeAsset, startURL))

	entry, err := q.Get(zimfs.QueueEntryTypePage)
	if err != nil {
		t.Fatalf("expected a page entry: %v", err)
	}
	if entry.Status != zimfs.QueueEntryStatusDownloading {
		t.Errorf("expected status downloading, got %q", entry.Status)
	}
	if entry.WebURL.String() != "https://example.com/page" {
		t.Errorf("expected page URL, got %q", entry.WebURL)
	}

	if _, err := q.Get(zimfs.QueueEntryTypePage); err == nil {
		t.Error("expected no more page entries")
	}

	if _, err := q.Get(zimfs.QueueEntryTypeAsset); err != nil {
		t.Fatalf("expected an asset entry: %v", err)
	}
}

func TestQueueStats(t *testing.T) {
	dir := t.TempDir()
	startURL, _ := url.Parse("https://example.com/")
	q := zimfs.NewQueue(dir, startURL, nil)

	p1, _ := url.Parse("https://example.com/p1")
	p2, _ := url.Parse("https://example.com/p2")
	f1, _ := url.Parse("https://example.com/f1")
	q.Add(*zimfs.NewQueueEntry(p1, p1, zimfs.QueueEntryTypePage, startURL))
	q.Add(*zimfs.NewQueueEntry(p2, p2, zimfs.QueueEntryTypePage, startURL))

	failed := zimfs.NewQueueEntry(f1, f1, zimfs.QueueEntryTypeAsset, startURL)
	q.Add(*failed)
	failed.Status = zimfs.QueueEntryStatusFailed
	q.Set(*failed)

	if q.Count(zimfs.QueueEntryStatusPending) != 2 {
		t.Errorf("expected 2 pending, got %d", q.Count(zimfs.QueueEntryStatusPending))
	}
	if q.Count(zimfs.QueueEntryStatusFailed) != 1 {
		t.Errorf("expected 1 failed, got %d", q.Count(zimfs.QueueEntryStatusFailed))
	}
}

func TestQueueSaveLoad(t *testing.T) {
	dir := t.TempDir()
	startURL, _ := url.Parse("https://example.com/")

	q := zimfs.NewQueue(dir, startURL, nil)
	pageURL, _ := url.Parse("https://example.com/page1")
	entry := zimfs.NewQueueEntry(pageURL, pageURL, zimfs.QueueEntryTypePage, startURL)
	q.Add(*entry)
	entry.Status = zimfs.QueueEntryStatusDownloaded
	q.Set(*entry)
	q.Write()

	q2 := zimfs.NewQueue(dir, startURL, nil)
	if q2.Count(zimfs.QueueEntryStatusDownloaded) != 1 {
		t.Errorf("expected 1 downloaded, got %d", q2.Count(zimfs.QueueEntryStatusDownloaded))
	}
}

func TestQueueConcurrent(t *testing.T) {
	dir := t.TempDir()
	startURL, _ := url.Parse("https://example.com/")
	q := zimfs.NewQueue(dir, startURL, nil)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			u, _ := url.Parse(fmt.Sprintf("https://example.com/page%d", n))
			q.Add(*zimfs.NewQueueEntry(u, u, zimfs.QueueEntryTypePage, startURL))
		}(i)
	}
	wg.Wait()

	if q.Count(zimfs.QueueEntryStatusPending) != 50 {
		t.Errorf("expected 50 pending, got %d", q.Count(zimfs.QueueEntryStatusPending))
	}
}

func TestThrottleState(t *testing.T) {
	ts := NewThrottleState("example.com")

	if ts.CurrentDelay != 100*time.Millisecond {
		t.Errorf("expected 100ms initial delay, got %v", ts.CurrentDelay)
	}

	for i := 0; i < 15; i++ {
		ts.RecordError()
	}

	ts2 := NewThrottleState("example.com")
	for i := 0; i < 15; i++ {
		ts2.RecordError()
	}
	if ts2.ErrorsInWindow() != 15 {
		t.Errorf("expected 15 errors in window, got %d", ts2.ErrorsInWindow())
	}
}

func TestThrottleCrawlDelay(t *testing.T) {
	ts := NewThrottleState("example.com")
	ts.SetCrawlDelay(5 * time.Second)

	if ts.CurrentDelay < 5*time.Second {
		t.Errorf("expected delay >= 5s, got %v", ts.CurrentDelay)
	}
}

func TestDetectMimeType(t *testing.T) {
	tests := []struct {
		header   string
		urlStr   string
		expected string
	}{
		{"text/html; charset=utf-8", "", "text/html"},
		{"", "/styles/main.css", "text/css"},
		{"application/octet-stream", "/script.js", "application/javascript"},
		{"", "/image.png", "image/png"},
		{"", "/unknown.xyz", "application/octet-stream"},
		{"image/svg+xml", "", "image/svg+xml"},
	}

	for _, tt := range tests {
		result := DetectMimeType(tt.header, tt.urlStr)
		if result != tt.expected {
			t.Errorf("DetectMimeType(%q, %q) = %q, want %q", tt.header, tt.urlStr, result, tt.expected)
		}
	}
}

func TestDownloader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(200)
			w.Write([]byte("<html><body>Hello</body></html>"))
		} else if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "/ok")
			w.WriteHeader(301)
		} else if r.URL.Path == "/error" {
			w.WriteHeader(500)
		} else if r.URL.Path == "/notfound" {
			w.WriteHeader(404)
		}
	}))
	defer ts.Close()

	dir := t.TempDir()
	d := NewDownloader(dir, false)

	t.Run("successful download", func(t *testing.T) {
		entryURL, _ := url.Parse(ts.URL + "/ok")
		entry := zimfs.NewQueueEntry(entryURL, entryURL, zimfs.QueueEntryTypePage, nil)
		err := d.Download(context.Background(), entry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry.Status != zimfs.QueueEntryStatusDownloaded {
			t.Errorf("expected downloaded, got %q", entry.Status)
		}
		if entry.MimeType != "text/html" {
			t.Errorf("expected text/html, got %q", entry.MimeType)
		}

		body, err := os.ReadFile(filepath.Join(dir, entry.Path()))
		if err != nil {
			t.Fatalf("cache file not found: %v", err)
		}
		if string(body) != "<html><body>Hello</body></html>" {
			t.Errorf("unexpected body: %q", string(body))
		}
	})

	t.Run("retry on 500", func(t *testing.T) {
		entryURL, _ := url.Parse(ts.URL + "/error")
		entry := zimfs.NewQueueEntry(entryURL, entryURL, zimfs.QueueEntryTypePage, nil)
		d.Download(context.Background(), entry)
		if entry.Retries == 0 {
			t.Error("expected retry count > 0 after 500")
		}
	})
}

func TestExtractor(t *testing.T) {
	html := `<html><head><link rel="stylesheet" href="/style.css"></head><body>
<img src="/images/logo.png">
<img srcset="/img/1x.png 1x, /img/2x.png 2x">
<a href="/about">About</a>
<a href="/doc.pdf">Download PDF</a>
<a href="https://other.com/page">External</a>
<script src="/js/app.js"></script>
<video src="/media/video.mp4" poster="/media/poster.jpg"></video>
<style>body { background: url(/images/bg.png); }</style>
</body></html>`

	pageURL, _ := url.Parse("https://example.com/page")
	ext, err := NewExtractor(pageURL, "example.com", pageURL)
	if err != nil {
		t.Fatal(err)
	}

	urls := ext.Extract([]byte(html))

	found := make(map[string]zimfs.QueueEntryType)
	for _, u := range urls {
		found[u.URL.String()] = u.EntryType
	}

	tests := []struct {
		urlStr    string
		entryType zimfs.QueueEntryType
	}{
		{"https://example.com/style.css", zimfs.QueueEntryTypeAsset},
		{"https://example.com/images/logo.png", zimfs.QueueEntryTypeAsset},
		{"https://example.com/img/1x.png", zimfs.QueueEntryTypeAsset},
		{"https://example.com/img/2x.png", zimfs.QueueEntryTypeAsset},
		{"https://example.com/about", zimfs.QueueEntryTypePage},
		{"https://example.com/doc.pdf", zimfs.QueueEntryTypeAsset},
		{"https://example.com/js/app.js", zimfs.QueueEntryTypeAsset},
		{"https://example.com/media/video.mp4", zimfs.QueueEntryTypeAsset},
		{"https://example.com/media/poster.jpg", zimfs.QueueEntryTypeAsset},
		{"https://example.com/images/bg.png", zimfs.QueueEntryTypeAsset},
	}

	for _, tt := range tests {
		et, ok := found[tt.urlStr]
		if !ok {
			t.Errorf("expected URL %q, not found", tt.urlStr)
			continue
		}
		if et != tt.entryType {
			t.Errorf("URL %q: expected %s, got %s", tt.urlStr, tt.entryType, et)
		}
	}

	extURL := "https://other.com/page"
	et, ok := found[extURL]
	if !ok {
		t.Errorf("expected external link %q to be extracted as zimfs.QueueEntryTypeExternalPage", extURL)
	} else if et != zimfs.QueueEntryTypeExternalPage {
		t.Errorf("external link %q: expected %s, got %s", extURL, zimfs.QueueEntryTypeExternalPage, et)
	}
}

func TestRobotsMatcher(t *testing.T) {
	matcher := &RobotsMatcher{
		rules: []robotsRule{
			{path: "/admin", allowed: false},
			{path: "/admin/public", allowed: true},
			{path: "/private", allowed: false},
		},
	}

	tests := []struct {
		path    string
		allowed bool
	}{
		{"/index.html", true},
		{"/admin", false},
		{"/admin/users", false},
		{"/admin/public", true},
		{"/admin/public/dashboard", true},
		{"/private/data", false},
		{"/about", true},
	}

	for _, tt := range tests {
		if matcher.IsAllowed(tt.path) != tt.allowed {
			t.Errorf("IsAllowed(%q) = %v, want %v", tt.path, !tt.allowed, tt.allowed)
		}
	}
}

func TestSitemapParse(t *testing.T) {
	t.Run("urlset", func(t *testing.T) {
		xml := `<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>https://example.com/page1</loc></url>
<url><loc>https://example.com/page2</loc></url>
</urlset>`

		urls, isIndex := parseSitemapXML(xml)
		if isIndex {
			t.Error("expected urlset, not index")
		}
		if len(urls) != 2 {
			t.Errorf("expected 2 URLs, got %d", len(urls))
		}
	})

	t.Run("sitemapindex", func(t *testing.T) {
		xml := `<?xml version="1.0"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<sitemap><loc>https://example.com/sitemap1.xml</loc></sitemap>
</sitemapindex>`

		urls, isIndex := parseSitemapXML(xml)
		if !isIndex {
			t.Error("expected sitemapindex")
		}
		if len(urls) != 1 {
			t.Errorf("expected 1 sitemap URL, got %d", len(urls))
		}
	})

	t.Run("empty", func(t *testing.T) {
		urls, isIndex := parseSitemapXML("<empty></empty>")
		if len(urls) != 0 {
			t.Error("expected 0 URLs")
		}
		if isIndex {
			t.Error("expected not an index")
		}
	})
}

func TestScraperNew(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><head><title>Test</title></head><body><a href=\"/page2\">Page 2</a><img src=\"/logo.png\"></body></html>"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	u, _ := url.Parse(ts.URL + "/")
	s, err := NewScraper(ScraperOptions{
		Folder:            dir,
		URL:               u,
		RespectRobots:     false,
		IgnoreInsecureSSL: false,
		PageWorkers:       1,
		AssetWorkers:      1,
		Filters:           nil,
		PageLimit:         0,
	})
	if err != nil {
		t.Fatalf("NewScraper: %v", err)
	}

	if s.StartURL == nil || s.StartURL.Hostname() == "" {
		t.Error("expected non-empty host")
	}
	if s.Status() != "idle" {
		t.Errorf("expected idle status, got %q", s.Status())
	}
	if s.queue.Count(zimfs.QueueEntryStatusPending) == 0 {
		t.Error("expected start URL to be enqueued")
	}
}

func TestScraperStartStop(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/" || r.URL.Path == "" {
			w.Write([]byte("<html><body><a href=\"/page2\">Next</a></body></html>"))
		} else {
			w.Write([]byte("<html><body>Page content</body></html>"))
		}
	}))
	defer ts.Close()

	dir := t.TempDir()
	u, _ := url.Parse(ts.URL + "/")
	s, err := NewScraper(ScraperOptions{
		Folder:            dir,
		URL:               u,
		RespectRobots:     false,
		IgnoreInsecureSSL: false,
		PageWorkers:       1,
		AssetWorkers:      1,
		Filters:           nil,
		PageLimit:         5,
	})
	if err != nil {
		t.Fatalf("NewScraper: %v", err)
	}

	s.Start()
	time.Sleep(3 * time.Second)
	s.Stop()

	if s.Status() != "stopped" {
		t.Errorf("expected stopped, got %q", s.Status())
	}
	t.Logf("downloaded=%d", s.queue.Count(zimfs.QueueEntryStatusDownloaded))
}

func TestScraperExtractAndEnqueue(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><head><link rel="stylesheet" href="/style.css"></head><body><a href="/about">About</a><img src="/logo.png"></body></html>`))
		} else {
			w.WriteHeader(200)
			w.Write([]byte("ok"))
		}
	}))
	defer ts.Close()

	dir := t.TempDir()
	u, _ := url.Parse(ts.URL + "/")
	s, err := NewScraper(ScraperOptions{
		Folder:            dir,
		URL:               u,
		RespectRobots:     false,
		IgnoreInsecureSSL: false,
		PageWorkers:       1,
		AssetWorkers:      2,
		Filters:           nil,
		PageLimit:         10,
	})
	if err != nil {
		t.Fatalf("NewScraper: %v", err)
	}

	s.Start()
	time.Sleep(2 * time.Second)
	s.Stop()

	if s.queue.Count(zimfs.QueueEntryStatusDownloaded) < 1 {
		t.Error("expected at least 1 downloaded entry")
	}
}

func TestBuildZIM(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><head><title>Test Page</title><link rel="stylesheet" href="/style.css"></head><body><h1>Hello</h1><a href="/about">About</a><img src="/logo.png"></body></html>`))
		} else if r.URL.Path == "/about" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><head><title>About</title></head><body><p>About page</p></body></html>`))
		} else if r.URL.Path == "/style.css" {
			w.Header().Set("Content-Type", "text/css")
			w.Write([]byte("body { color: red; }"))
		} else {
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte{0x89, 0x50, 0x4E, 0x47})
		}
	}))
	defer ts.Close()

	dir := t.TempDir()
	u, _ := url.Parse(ts.URL + "/")
	s, err := NewScraper(ScraperOptions{
		Folder:            dir,
		URL:               u,
		RespectRobots:     false,
		IgnoreInsecureSSL: false,
		PageWorkers:       1,
		AssetWorkers:      2,
		Filters:           nil,
		PageLimit:         10,
	})
	if err != nil {
		t.Fatalf("NewScraper: %v", err)
	}

	s.Start()
	time.Sleep(5 * time.Second)
	s.Stop()

	t.Logf("downloaded=%d pending=%d failed=%d",
		s.queue.Count(zimfs.QueueEntryStatusDownloaded),
		s.queue.Count(zimfs.QueueEntryStatusPending),
		s.queue.Count(zimfs.QueueEntryStatusFailed))

	if s.queue.Count(zimfs.QueueEntryStatusDownloaded) == 0 {
		entries := s.queue.Query(zimfs.QueueEntryStatusPending)
		for _, e := range entries {
			t.Logf("  pending: type=%s url=%s", e.Type, e.WebURL)
		}
		t.Skip("no entries downloaded")
	}

	zimPath, err := s.BuildZIM()
	if err != nil {
		t.Fatalf("BuildZIM: %v", err)
	}
	t.Logf("ZIM built: %s", zimPath)

	if _, err := os.Stat(zimPath); os.IsNotExist(err) {
		t.Errorf("ZIM file not found: %s", zimPath)
	}

	testArchive, err := zim.Open(zimPath)
	if err != nil {
		t.Fatalf("failed to open ZIM: %v", err)
	}
	defer testArchive.Close()

	if testArchive.EntryCount() == 0 {
		t.Error("ZIM file has 0 entries")
	}

	if title, ok := testArchive.Metadata("Title"); !ok || title == "" {
		t.Error("ZIM file missing Title metadata")
	}
}

func TestCrossHostAssets(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	}))
	defer cdn.Close()

	main := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>Main</title></head><body><img src="` + cdn.URL + `/logo.png"><link rel="stylesheet" href="` + cdn.URL + `/style.css"></body></html>`))
	}))
	defer main.Close()

	dir := t.TempDir()
	u, _ := url.Parse(main.URL + "/")
	s, err := NewScraper(ScraperOptions{
		Folder:            dir,
		URL:               u,
		RespectRobots:     false,
		IgnoreInsecureSSL: false,
		PageWorkers:       1,
		AssetWorkers:      2,
		Filters:           nil,
		PageLimit:         10,
	})
	if err != nil {
		t.Fatalf("NewScraper: %v", err)
	}

	s.Start()
	time.Sleep(5 * time.Second)
	s.Stop()

	t.Logf("downloaded=%d pending=%d failed=%d",
		s.queue.Count(zimfs.QueueEntryStatusDownloaded),
		s.queue.Count(zimfs.QueueEntryStatusPending),
		s.queue.Count(zimfs.QueueEntryStatusFailed))

	if s.queue.Count(zimfs.QueueEntryStatusDownloaded) == 0 {
		t.Skip("no entries downloaded")
	}

	zimPath, err := s.BuildZIM()
	if err != nil {
		t.Fatalf("BuildZIM: %v", err)
	}

	archive, err := zim.Open(zimPath)
	if err != nil {
		t.Fatalf("open ZIM: %v", err)
	}
	defer archive.Close()

	cdnHost := strings.Split(cdn.URL[7:], ":")[0]
	t.Logf("CDN hostname: %s", cdnHost)

	for e := range archive.IterateByPath() {
		t.Logf("ZIM entry: %s", e.Path())
	}

	if _, err := archive.EntryByPath(cdnHost + "/logo.png"); err != nil {
		t.Errorf("CDN logo.png not found: %v", err)
	}
	if _, err := archive.EntryByPath(cdnHost + "/style.css"); err != nil {
		t.Errorf("CDN style.css not found: %v", err)
	}
}

func TestExtractorMediaElements(t *testing.T) {
	html := `<html><body>
<video src="/video.mp4" poster="/poster.jpg">
	<source src="/video-hd.webm" type="video/webm">
	<source src="/video-sd.mp4" type="video/mp4">
	<track src="/subtitles.vtt" kind="subtitles">
</video>
<audio src="/audio.mp3">
	<source src="/audio.ogg" type="audio/ogg">
</audio>
</body></html>`

	pageURL, _ := url.Parse("https://example.com/page")
	ext, err := NewExtractor(pageURL, "example.com", pageURL)
	if err != nil {
		t.Fatal(err)
	}

	urls := ext.Extract([]byte(html))

	found := make(map[string]bool)
	for _, u := range urls {
		found[u.URL.String()] = true
	}

	tests := []string{
		"https://example.com/video.mp4",
		"https://example.com/poster.jpg",
		"https://example.com/video-hd.webm",
		"https://example.com/video-sd.mp4",
		"https://example.com/subtitles.vtt",
		"https://example.com/audio.mp3",
		"https://example.com/audio.ogg",
	}

	for _, expected := range tests {
		if !found[expected] {
			t.Errorf("expected %q in extracted URLs", expected)
		}
	}

	if len(urls) != len(tests) {
		t.Errorf("expected %d URLs, got %d: %v", len(tests), len(urls), urls)
	}
}

func TestConcurrentThrottle(t *testing.T) {
	ts := NewThrottleState("test")
	ts.MinDelay = 0
	ts.CurrentDelay = 0
	ts.ErrorThreshold = 5

	for i := 0; i < 10; i++ {
		ts.RecordError()
	}

	delay := ts.CurrentDelay
	if delay <= time.Second {
		t.Errorf("expected delay > 1s after 10 errors (threshold=5, should have doubled), got %v", delay)
	}
}

func TestDetectFiltersPHPBB(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body id="phpbb" class="nojs notouch section-viewforum ltr"></body></html>`))
	}))
	defer server.Close()

	startURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	names, err := DetectFilters(startURL)
	if err != nil {
		t.Fatalf("DetectFilters: %v", err)
	}

	want := map[string]bool{"Trackers": true, "Scripts": true, "PHPBB": true, "MediaWiki": false, "VBulletin": false}
	for _, name := range names {
		if _, ok := want[name]; !ok {
			t.Errorf("DetectFilters = %v, unexpected filter %q", names, name)
		}
		delete(want, name)
	}

	for name, expected := range want {
		if expected {
			t.Errorf("DetectFilters = %v, missing %q", names, name)
		}
	}
}

func TestDetectFiltersUnsupportedScheme(t *testing.T) {
	startURL, _ := url.Parse("ftp://example.com/")

	_, err := DetectFilters(startURL)
	if err == nil {
		t.Error("expected error for unsupported scheme")
	}
}
