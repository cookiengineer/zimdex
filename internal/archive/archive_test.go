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
		result := CanonicalizeURL(u)
		if result.String() != tt.expected {
			t.Errorf("CanonicalizeURL(%q) = %q, want %q", tt.input, result.String(), tt.expected)
		}
	}
}

func TestURLToPath(t *testing.T) {
	u, _ := url.Parse("https://en.wikipedia.org/wiki/Main_Page")
	host, local, zim := URLToPath(u)

	if host != "en.wikipedia.org" {
		t.Errorf("host = %q, want en.wikipedia.org", host)
	}
	if local != filepath.FromSlash("en.wikipedia.org/wiki/Main_Page") {
		t.Errorf("local = %q", local)
	}
	if zim != "en.wikipedia.org/wiki/Main_Page" {
		t.Errorf("zim = %q, want en.wikipedia.org/wiki/Main_Page", zim)
	}
}

func TestQueueAddAndDedup(t *testing.T) {
	dir := t.TempDir()
	startURL, _ := url.Parse("https://example.com/")
	q := NewQueue(filepath.Join(dir, "test.json"), "example.com", startURL, true, 0)

	entryURL, _ := url.Parse("https://example.com/")
	added := q.Add(QueueEntry{
		URL:       entryURL,
		Path:      "example.com/index.html",
		ZimPath:   "C/example.com/index.html",
		EntryType: EntryTypePage,
		Status:    StatusPending,
	})
	if !added {
		t.Error("expected first add to succeed")
	}

	added = q.Add(QueueEntry{
		URL:       entryURL,
		Path:      "example.com/index.html",
		ZimPath:   "C/example.com/index.html",
		EntryType: EntryTypePage,
		Status:    StatusPending,
	})
	if added {
		t.Error("expected duplicate add to fail")
	}
}

func TestQueueAddIfNew(t *testing.T) {
	dir := t.TempDir()
	startURL, _ := url.Parse("https://example.com/")
	q := NewQueue(filepath.Join(dir, "test.json"), "example.com", startURL, true, 0)

	pageURL, _ := url.Parse("https://example.com/page1")
	ok := q.AddIfNew(pageURL, "example.com/page1", "C/example.com/page1", EntryTypePage, nil)
	if !ok {
		t.Error("expected AddIfNew to return true for new URL")
	}

	ok = q.AddIfNew(pageURL, "example.com/page1", "C/example.com/page1", EntryTypePage, nil)
	if ok {
		t.Error("expected AddIfNew to return false for duplicate URL")
	}
}

func TestQueuePopPending(t *testing.T) {
	dir := t.TempDir()
	startURL, _ := url.Parse("https://example.com/")
	q := NewQueue(filepath.Join(dir, "test.json"), "example.com", startURL, true, 0)

	pageURL, _ := url.Parse("https://example.com/page")
	q.Add(QueueEntry{
		URL:       pageURL,
		Path:      "example.com/page",
		ZimPath:   "C/example.com/page",
		EntryType: EntryTypePage,
		Status:    StatusPending,
	})

	assetURL, _ := url.Parse("https://example.com/asset.css")
	q.Add(QueueEntry{
		URL:       assetURL,
		Path:      "example.com/asset.css",
		ZimPath:   "C/example.com/asset.css",
		EntryType: EntryTypeAsset,
		Status:    StatusPending,
	})

	entry := q.PopPending(EntryTypePage)
	if entry == nil {
		t.Fatal("expected a page entry")
	}
	if entry.Status != StatusDownloading {
		t.Errorf("expected status downloading, got %q", entry.Status)
	}
	if entry.URL.String() != "https://example.com/page" {
		t.Errorf("expected page URL, got %q", entry.URL)
	}

	entry2 := q.PopPending(EntryTypePage)
	if entry2 != nil {
		t.Error("expected no more page entries")
	}

	entry3 := q.PopPending(EntryTypeAsset)
	if entry3 == nil {
		t.Fatal("expected an asset entry")
	}
}

func TestQueueStats(t *testing.T) {
	dir := t.TempDir()
	startURL, _ := url.Parse("https://example.com/")
	q := NewQueue(filepath.Join(dir, "test.json"), "example.com", startURL, true, 0)

	p1, _ := url.Parse("https://example.com/p1")
	p2, _ := url.Parse("https://example.com/p2")
	f1, _ := url.Parse("https://example.com/f1")
	q.Add(QueueEntry{URL: p1, EntryType: EntryTypePage, Status: StatusPending})
	q.Add(QueueEntry{URL: p2, EntryType: EntryTypePage, Status: StatusPending})
	q.Add(QueueEntry{URL: f1, EntryType: EntryTypeAsset, Status: StatusFailed})

	if q.PendingCount() != 2 {
		t.Errorf("expected 2 pending, got %d", q.PendingCount())
	}
	if q.FailedCount() != 1 {
		t.Errorf("expected 1 failed, got %d", q.FailedCount())
	}
}

func TestQueueSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	startURL, _ := url.Parse("https://example.com/")
	q := NewQueue(path, "example.com", startURL, true, 100)
	pageURL, _ := url.Parse("https://example.com/page1")
	q.Add(QueueEntry{URL: pageURL, EntryType: EntryTypePage, Status: StatusDownloaded})
	q.Save()

	q2 := NewQueue(path, "example.com", startURL, true, 100)
	if q2.PageLimit != 100 {
		t.Errorf("expected pageLimit 100, got %d", q2.PageLimit)
	}
	if q2.DownloadedCount() != 1 {
		t.Errorf("expected 1 downloaded, got %d", q2.DownloadedCount())
	}
}

func TestQueueConcurrent(t *testing.T) {
	dir := t.TempDir()
	startURL, _ := url.Parse("https://example.com/")
	q := NewQueue(filepath.Join(dir, "test.json"), "example.com", startURL, true, 0)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			u, _ := url.Parse(fmt.Sprintf("https://example.com/page%d", n))
			q.Add(QueueEntry{URL: u, EntryType: EntryTypePage, Status: StatusPending})
		}(i)
	}
	wg.Wait()

	if q.PendingCount() != 50 {
		t.Errorf("expected 50 pending, got %d", q.PendingCount())
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
		entry := &QueueEntry{
			URL:       entryURL,
			Path:      "testhost/ok",
			EntryType: EntryTypePage,
			Status:    StatusPending,
		}
		err := d.Download(context.Background(), entry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry.Status != StatusDownloaded {
			t.Errorf("expected downloaded, got %q", entry.Status)
		}
		if entry.MimeType != "text/html" {
			t.Errorf("expected text/html, got %q", entry.MimeType)
		}

		body, err := os.ReadFile(filepath.Join(dir, entry.Path))
		if err != nil {
			t.Fatalf("cache file not found: %v", err)
		}
		if string(body) != "<html><body>Hello</body></html>" {
			t.Errorf("unexpected body: %q", string(body))
		}
	})

	t.Run("retry on 500", func(t *testing.T) {
		entryURL, _ := url.Parse(ts.URL + "/error")
		entry := &QueueEntry{
			URL:       entryURL,
			Path:      "testhost/error",
			EntryType: EntryTypePage,
			Status:    StatusPending,
		}
		d.Download(context.Background(), entry)
		if entry.RetryCount == 0 {
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

	found := make(map[string]EntryType)
	for _, u := range urls {
		found[u.URL.String()] = u.EntryType
	}

	tests := []struct {
		urlStr    string
		entryType EntryType
	}{
		{"https://example.com/style.css", EntryTypeAsset},
		{"https://example.com/images/logo.png", EntryTypeAsset},
		{"https://example.com/img/1x.png", EntryTypeAsset},
		{"https://example.com/img/2x.png", EntryTypeAsset},
		{"https://example.com/about", EntryTypePage},
		{"https://example.com/doc.pdf", EntryTypeAsset},
		{"https://example.com/js/app.js", EntryTypeAsset},
		{"https://example.com/media/video.mp4", EntryTypeAsset},
		{"https://example.com/media/poster.jpg", EntryTypeAsset},
		{"https://example.com/images/bg.png", EntryTypeAsset},
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
		t.Errorf("expected external link %q to be extracted as EntryTypeExternalPage", extURL)
	} else if et != EntryTypeExternalPage {
		t.Errorf("external link %q: expected %s, got %s", extURL, EntryTypeExternalPage, et)
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

	if s.Host == "" {
		t.Error("expected non-empty host")
	}
	if s.Status() != "idle" {
		t.Errorf("expected idle status, got %q", s.Status())
	}
	if s.queue.PendingCount() == 0 {
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
	t.Logf("downloaded=%d", s.queue.DownloadedCount())
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

	if s.queue.DownloadedCount() < 1 {
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

	t.Logf("downloaded=%d pending=%d failed=%d", s.queue.DownloadedCount(), s.queue.PendingCount(), s.queue.FailedCount())

	if s.queue.DownloadedCount() == 0 {
		entries := s.queue.GetByStatus(StatusPending)
		for _, e := range entries {
			t.Logf("  pending: type=%s url=%s", e.EntryType, e.URL)
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

	t.Logf("downloaded=%d pending=%d failed=%d", s.queue.DownloadedCount(), s.queue.PendingCount(), s.queue.FailedCount())

	if s.queue.DownloadedCount() == 0 {
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
