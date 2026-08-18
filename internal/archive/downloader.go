package archive

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cookiengineer/zimdex/internal/zimfs"
)

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:133.0) Gecko/20100101 Firefox/133.0",
	"Mozilla/5.0 (X11; Linux i686; rv:133.0) Gecko/20100101 Firefox/133.0",
}

var acceptForType = map[string]string{
	"text/html":              "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
	"text/css":               "text/css,*/*;q=0.1",
	"application/javascript": "*/*",
	"image/":                 "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8",
}

func randomUA() string {
	return userAgents[rand.Intn(len(userAgents))]
}

func setRequestHeaders(req *http.Request, entry *zimfs.QueueEntry) {
	ua := randomUA()
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("DNT", "1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	mime := entry.MimeType
	if mime == "" {
		mime = "text/html"
	}

	accept := "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"
	for prefix, val := range acceptForType {
		if strings.HasPrefix(mime, prefix) {
			accept = val
			break
		}
	}
	req.Header.Set("Accept", accept)

	if strings.Contains(ua, "Chrome") {
		req.Header.Set("Sec-CH-UA", `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`)
		req.Header.Set("Sec-CH-UA-Mobile", "?0")
		req.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	}
}

type ThrottleState struct {
	Host            string
	LastRequest     time.Time
	MinDelay        time.Duration
	CurrentDelay    time.Duration
	MaxDelay        time.Duration
	ErrorTimestamps []time.Time
	WindowDuration  time.Duration
	ErrorThreshold  int
}

func NewThrottleState(host string) *ThrottleState {
	return &ThrottleState{
		Host:           host,
		MinDelay:       100 * time.Millisecond,
		CurrentDelay:   100 * time.Millisecond,
		MaxDelay:       30 * time.Second,
		WindowDuration: 15 * time.Minute,
		ErrorThreshold: 10,
	}
}

func (t *ThrottleState) SetCrawlDelay(delay time.Duration) {
	if delay > t.MinDelay {
		t.MinDelay = delay
	}
	if t.CurrentDelay < delay {
		t.CurrentDelay = delay
	}
}

func (t *ThrottleState) Wait() {
	elapsed := time.Since(t.LastRequest)
	if elapsed < t.CurrentDelay {
		time.Sleep(t.CurrentDelay - elapsed)
	}
}

func (t *ThrottleState) RecordSuccess() {
	t.LastRequest = time.Now()
	if t.CurrentDelay > t.MinDelay {
		t.CurrentDelay = time.Duration(float64(t.CurrentDelay) * 0.9)
		if t.CurrentDelay < t.MinDelay {
			t.CurrentDelay = t.MinDelay
		}
	}
	if t.CurrentDelay <= 0 {
		t.CurrentDelay = t.MinDelay
	}
}

func (t *ThrottleState) RecordError() {
	now := time.Now()
	t.ErrorTimestamps = append(t.ErrorTimestamps, now)
	t.pruneWindow()

	if len(t.ErrorTimestamps) > t.ErrorThreshold {
		if t.CurrentDelay <= 0 {
			t.CurrentDelay = t.MinDelay
			if t.CurrentDelay <= 0 {
				t.CurrentDelay = 1 * time.Second
			}
		}
		t.CurrentDelay = t.CurrentDelay * 2
		if t.CurrentDelay > t.MaxDelay {
			t.CurrentDelay = t.MaxDelay
		}
	}
	t.LastRequest = now
}

func (t *ThrottleState) RecordRetryAfter(seconds int) {
	delay := time.Duration(seconds) * time.Second
	if delay > t.CurrentDelay {
		t.CurrentDelay = delay
	}
	if t.CurrentDelay > t.MaxDelay {
		t.CurrentDelay = t.MaxDelay
	}
}

func (t *ThrottleState) pruneWindow() {
	cutoff := time.Now().Add(-t.WindowDuration)
	kept := t.ErrorTimestamps[:0]
	for _, ts := range t.ErrorTimestamps {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	t.ErrorTimestamps = kept
}

func (t *ThrottleState) ErrorsInWindow() int {
	t.pruneWindow()
	return len(t.ErrorTimestamps)
}

var ErrRetry = fmt.Errorf("retry")
var ErrRedirect = fmt.Errorf("redirect")

type Downloader struct {
	client     *http.Client
	dataDir    string
	throttles  map[string]*ThrottleState
	mu         sync.Mutex
	maxRetries int
	retryDelay time.Duration
	timeout    time.Duration
}

func NewDownloader(dataDir string, insecureSkipVerify bool) *Downloader {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecureSkipVerify,
		},
	}

	return &Downloader{
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		dataDir:    dataDir,
		throttles:  make(map[string]*ThrottleState),
		maxRetries: 3,
		retryDelay: 30 * time.Second,
		timeout:    30 * time.Second,
	}
}

func (d *Downloader) getThrottle(host string) *ThrottleState {
	d.mu.Lock()
	defer d.mu.Unlock()

	t, ok := d.throttles[host]
	if !ok {
		t = NewThrottleState(host)
		d.throttles[host] = t
	}
	return t
}

func (d *Downloader) Download(ctx context.Context, entry *zimfs.QueueEntry) error {
	downloadURL := entry.WebURL
	if downloadURL == nil {
		entry.Status = zimfs.QueueEntryStatusFailed
		return fmt.Errorf("no URL to download")
	}

	downloadURLStr := downloadURL.String()
	host := downloadURL.Hostname()
	throttle := d.getThrottle(host)
	throttle.Wait()

	reqCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", downloadURLStr, nil)
	if err != nil {
		entry.Status = zimfs.QueueEntryStatusFailed
		throttle.RecordError()
		return err
	}

	setRequestHeaders(req, entry)

	resp, err := d.client.Do(req)
	if err != nil {
		if reqCtx.Err() == context.DeadlineExceeded {
			throttle.RecordError()
			return d.handleRetry(entry, "timeout")
		}
		throttle.RecordError()
		return d.handleRetry(entry, err.Error())
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		body, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024*1024))
		if err != nil {
			throttle.RecordError()
			return d.handleRetry(entry, "read error: "+err.Error())
		}

		if err := d.SaveToCache(entry, body); err != nil {
			entry.Status = zimfs.QueueEntryStatusFailed
			throttle.RecordError()
			return err
		}

		entry.MimeType = DetectMimeType(resp.Header.Get("Content-Type"), downloadURLStr)
		entry.Size = int64(len(body))
		entry.Status = zimfs.QueueEntryStatusDownloaded
		entry.StatusCode = resp.StatusCode
		throttle.RecordSuccess()
		return nil

	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		location := resp.Header.Get("Location")
		if location == "" {
			entry.Status = zimfs.QueueEntryStatusFailed
			entry.StatusCode = resp.StatusCode
			return fmt.Errorf("redirect with no Location header")
		}

		resolved, _ := resolveURL(downloadURL, location)
		if resolved != nil && resolved.String() != downloadURLStr {
			entry.Status = zimfs.QueueEntryStatusSkipped
			entry.StatusCode = resp.StatusCode
			return fmt.Errorf("%w: %s", ErrRedirect, resolved.String())
		}
		return d.handleRetry(entry, fmt.Sprintf("HTTP %d", resp.StatusCode))

	case resp.StatusCode == 404:
		if entry.Retries >= 1 {
			entry.Status = zimfs.QueueEntryStatusFailed
			entry.StatusCode = resp.StatusCode
			return fmt.Errorf("404 Not Found")
		}
		return d.handleRetry(entry, "404 Not Found")

	case resp.StatusCode == 429:
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			if sec, err := parseInt(retryAfter); err == nil {
				throttle.RecordRetryAfter(sec)
			}
		}
		throttle.RecordError()
		return d.handleRetry(entry, fmt.Sprintf("HTTP %d", resp.StatusCode))

	case resp.StatusCode >= 500:
		throttle.RecordError()
		return d.handleRetry(entry, fmt.Sprintf("HTTP %d", resp.StatusCode))

	default:
		throttle.RecordError()
		entry.Status = zimfs.QueueEntryStatusFailed
		entry.StatusCode = resp.StatusCode
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
}

func (d *Downloader) handleRetry(entry *zimfs.QueueEntry, errMsg string) error {
	entry.Retries++
	if entry.Retries > d.maxRetries {
		entry.Status = zimfs.QueueEntryStatusFailed
		return fmt.Errorf("%s", errMsg)
	}
	return fmt.Errorf("%w: %s", ErrRetry, errMsg)
}

func (d *Downloader) SaveToCache(entry *zimfs.QueueEntry, body []byte) error {
	fullPath := filepath.Join(d.dataDir, entry.Path())
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(fullPath, body, 0644)
}

func DetectMimeType(contentTypeHeader, urlPath string) string {
	if contentTypeHeader != "" && contentTypeHeader != "application/octet-stream" {
		if idx := strings.Index(contentTypeHeader, ";"); idx != -1 {
			return strings.TrimSpace(contentTypeHeader[:idx])
		}
		return strings.TrimSpace(contentTypeHeader)
	}

	ext := strings.ToLower(filepath.Ext(urlPath))
	if mime, ok := extMimeMap[ext]; ok {
		return mime
	}

	return "application/octet-stream"
}

var extMimeMap = map[string]string{
	".html": "text/html", ".htm": "text/html",
	".css":  "text/css",
	".js":   "application/javascript", ".mjs": "application/javascript",
	".json": "application/json",
	".xml":  "application/xml",
	".txt":  "text/plain",
	".png":  "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif":  "image/gif", ".webp": "image/webp", ".svg": "image/svg+xml",
	".ico":  "image/x-icon", ".bmp": "image/bmp",
	".woff": "font/woff", ".woff2": "font/woff2",
	".ttf":  "font/ttf", ".eot": "application/vnd.ms-fontobject",
	".mp4":  "video/mp4", ".webm": "video/webm",
	".mp3":  "audio/mpeg", ".ogg": "audio/ogg", ".wav": "audio/wav",
	".pdf":  "application/pdf",
	".zip":  "application/zip", ".tar": "application/x-tar",
	".gz":   "application/gzip", ".bz2": "application/x-bzip2",
}

func parseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	return u, nil
}

func resolveURL(base *url.URL, ref string) (*url.URL, error) {
	refURL, err := url.Parse(ref)
	if err != nil {
		return nil, err
	}
	return base.ResolveReference(refURL), nil
}

func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number: %s", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
