# ZIMdex — Development Roadmap

## Overview

This roadmap tracks all implementation tasks for ZIMdex, organized by phase. Each task is independently verifiable. Tasks within a phase are ordered by dependency (earlier tasks must be completed first).

### Status Legend

- `[ ]` — Not started
- `[~]` — In progress
- `[x]` — Complete

---

## Phase 0: Project Cleanup & Foundation

**Goal**: Remove all git-evac references, set up proper module structure, get a bootable binary.

### 0.1 Module Setup

- [x] **0.1.1** — Rewrite `go.mod`
  - Change module path to `github.com/cookiengineer/zimdex`
  - Add `require github.com/cookiengineer/gozim v0.0.0-latest` (use `go get` to resolve)
  - Add `require golang.org/x/net v0.0.0-latest` (use `go get` to resolve)
  - Run `go mod tidy` to generate `go.sum`

- [x] **0.1.2** — Remove all `git-evac/` import paths from existing `.go` files
  - `cmds/zimdex/main.go`: Replaced with new `main.go` at project root
  - `server/Dispatch.go`: Deleted (replaced by `internal/server/`)
  - `server/DispatchRoutes.go`: Deleted (replaced by `internal/server/`)
  - `server/routes/Index.go`: Deleted (replaced by `internal/server/`)

- [x] **0.1.3** — Verify compilation
  - `CGO_ENABLED=0 go build ./...` passes
  - `go vet ./...` passes

### 0.2 main.go Rewrite

- [x] **0.2.1** — Write new `main.go` at project root (replace `cmds/zimdex/main.go`)
  - Parse CLI flags: `--folder` (data directory, default `./data`), `--port` (default `3000`)
  - Initialize `zimfs.Manager` with `--folder` directory
  - Call `manager.Scan()` to load all `.zim` files
  - Initialize `server.Server` with manager
  - Start HTTP server
  - Handle OS signals (SIGINT, SIGTERM) for graceful shutdown

- [x] **0.2.2** — Verify `main.go` compiles and starts
  - `CGO_ENABLED=0 go build -o zimdex .` succeeds (9.3MB static binary)
  - Server starts on specified port, logs loaded archive count
  - `Ctrl+C` triggers graceful shutdown with `Server.Shutdown()` and `Manager.Close()`

### 0.3 ZIM Manager

- [x] **0.3.1** — Implement `internal/zimfs/manager.go`
  - `Manager` struct with `Dir string`, `archives map[string]*zim.Archive`, `sync.RWMutex`
  - `NewManager(dir string) *Manager`
  - `Scan() error` — Uses `filepath.Glob(dir + "/*.zim")` to find ZIM files, opens with `zim.Open()`
  - `Get(filename string) (*zim.Archive, bool)` — Thread-safe lookup
  - `List() []ArchiveInfo` — Iterates archives, reads metadata from each
  - `Reload() error` — Close all, re-scan
  - `Close() error` — Close all archives

- [x] **0.3.2** — Test with a real ZIM file
  - Verified: `Scan()` enumerates `.zim` files in directory
  - Verified: `Get()` returns archive handles
  - Verified: `List()` returns metadata including title, date, language, entry counts
  - Verified: `Close()` releases all file handles

### 0.4 Server Foundation

- [x] **0.4.1** — Implement `internal/server/server.go`
  - `Server` struct holding `*zimfs.Manager`, `*http.ServeMux`, `port int`
  - `NewServer(manager *zimfs.Manager, port int) *Server`
  - `Start() error` — Registers all routes, calls `http.ListenAndServe(addr, mux)`
  - `Shutdown(ctx context.Context) error` — Graceful HTTP shutdown

- [x] **0.4.2** — Implement `internal/server/middleware.go`
  - Logging middleware: logs method, path, status, duration for every request
  - Recovery middleware: catches panics, logs stack trace, returns 500
  - CSP middleware: sets `default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'self'; form-action 'self'`
  - `ChainMiddleware()` function to compose middleware

- [x] **0.4.3** — Register routes
  - `GET /{$}` → redirect 303 to `/index.html`
  - `GET /index.html` → serves embedded `index.html` template via `embed` package
  - `GET /archive.html` → serves embedded `archive.html` template via `embed` package
  - `GET /favicon.ico` → returns 204 No Content
  - `GET /api/archives` → returns JSON with archives list from manager
  - `GET /api/search` → full-text BM25 search with snippet formatting
  - `GET /api/suggest` → title autocomplete across all archives
  - `GET /{zimfile}/{remainder...}` → render view with HTML rewriting
  - All archive CRUD routes registered (placeholder handlers for scraper)

- [x] **0.4.4** — Verify server
  - Server starts on specified port, logs archive count
  - `/` redirects to `/index.html` (303)
  - `/api/archives` returns valid JSON
  - CSP headers present on all responses
  - Unknown paths return appropriate errors (404 for missing ZIM)

---

## Phase 1: Search View

**Goal**: Working search UI with BM25 full-text search and autocomplete across loaded ZIM files.

### 1.1 Search Backend

- [x] **1.1.1** — Implement `internal/search/searcher.go`
  - `Searcher` struct: holds `[]*zim.Archive` and a `*zim.Searcher`
  - `NewSearcher(archives []*zim.Archive) *Searcher` — Calls `zim.NewSearcher(archives...)` to create BM25 index
  - `Search(query string, offset, limit int) (*zim.SearchResultSet, error)` — Delegates to gozim searcher
  - `Suggest(prefix string, limit int) ([]SuggestionResult, error)` — For each archive, calls `zim.NewSuggestionSearcher(a).Suggest(prefix, limit)`, merges results, deduplicates by path, sorts by relevance, returns top N
  - `AddArchive(a *zim.Archive)` — Rebuild searcher with new archive
  - `Close() error`

- [x] **1.1.2** — Define `SuggestionResult` struct
  - `Title`, `Path`, `RenderURL`, `Snippet`, `ZimFile` fields

### 1.2 Search API Handlers

- [x] **1.2.1** — Implement `handleSearch`
  - Parse query params: `q` (required, 400 if missing), `offset` (default 0), `limit` (default 20, max 100)
  - Call `searcher.Search(q, offset, limit)`
  - For each result: compute `render_url` from result path + source ZIM file
  - Return JSON: `{query, offset, limit, estimated, results: [{title, path, zim_file, render_url, score, snippet, word_count}]}`
  - Handle errors: return 500 with error message

- [x] **1.2.2** — Implement `handleSuggest`
  - Parse query params: `q` (required, 400 if missing), `limit` (default 10, max 50)
  - Call `searcher.Suggest(q, limit)`
  - Return JSON: `{query, suggestions: [{title, path, render_url, snippet}]}`

- [x] **1.2.3** — Implement `handleArchives`
  - Call `manager.List()`
  - Return JSON: `{archives: [{filename, title, date, language, source, description, entry_count, article_count, media_count, size_bytes, checksum}]}`

- [x] **1.2.4** — Register routes in `server.go`
  - `GET /api/search` → `handleSearch`
  - `GET /api/suggest` → `handleSuggest`
  - `GET /api/archives` → `handleArchives`

### 1.3 Search UI

- [x] **1.3.1** — Design and implement `web/templates/index.html`
  - Go `html/template` with embedded CSS (or link to `public/design/index.css`)
  - Header: "ZIMdex" title, subtitle with loaded archive count
  - Search bar: `<input type="search">` with autocomplete dropdown
  - Results area: list of result cards, each showing:
    - Title (linked to render URL)
    - Score badge
    - Snippet with highlighted query terms
    - Source ZIM filename + article word count
  - "No results" state: message + links to external search engines (configurable)
  - Loading indicator during API calls
  - Empty state (no query yet): show archive list or recent/popular articles

- [x] **1.3.2** — Implement vanilla JS for search interactivity
  - Debounced input handler (300ms) for autocomplete (`GET /api/suggest`)
  - Form submit / Enter key handler for full search (`GET /api/search`)
  - Render results from JSON response
  - Infinite scroll or "Load more" button using `offset` pagination
  - Keyboard navigation for autocomplete dropdown (arrow keys, Enter, Escape)
  - URL state management: update `?q=query` in browser URL on search

- [x] **1.3.3** — Test search end-to-end
  - Tested manually with devdocs Go ZIM and gobyexample ZIM
  - Search for "Reader", "bufio" return correct results with snippets
  - Results link to correct render URLs (e.g., `/devdocs_en_go_2026-07.zim/bufio/index`)
  - Snippet formatting strips HTML tags, converts `**bold**` to `<b>` tags

---

## Phase 2: Render View

**Goal**: Serve archived content from ZIM files with offline-safe HTML rewriting.

### 2.1 HTML Filters

- [x] **2.1.1** — Define `FilterConfig` and `FilterRule` types in `internal/render/filters.go`
  - `FilterConfig` struct with fields for each rule category:
    - `RemoveElements []string` — tag names to remove entirely
    - `RemoveAttributes []string` — attribute names to strip from all elements
    - `RemoveMetaNames []string` — `<meta name>` values to remove (supports glob: `twitter:*`)
    - `RemoveLinkRels []string` — `<link rel>` values to remove
    - `RemoveComments bool`
    - `RemoveTrackingImages bool`
    - `RemoveEmptyElements bool`
  - `DefaultFilters() FilterConfig` — returns the sensible default ruleset (see implementation plan)
  - `Validate()` — check for conflicts or invalid rules

- [x] **2.1.2** — Implement `Apply(node *html.Node, config FilterConfig)`
  - Walk the node tree recursively (depth-first, post-order so children are processed before parents)
  - For each element (ElementNode):
    - If tag name in `RemoveElements`: replace node with nil (remove from tree)
    - If attribute name in `RemoveAttributes`: delete attribute
    - If `<meta name="...">` matches `RemoveMetaNames` pattern: remove element
    - If `<link rel="...">` matches `RemoveLinkRels` pattern: remove element
    - If `RemoveComments` and node is CommentNode: remove
    - If `RemoveTrackingImages` and `<img width="1" height="1">` with tracking-like src: remove
  - After all removals: if `RemoveEmptyElements` and element has no children and no text content: remove

### 2.2 URL Rewriter

- [x] **2.2.1** — Implement `internal/render/rewriter.go`
  - `Rewriter` struct holding:
    - `ZimFile string` — the ZIM filename (e.g., `en.wikipedia.org-2026-08-02.zim`)
    - `PageHost string` — the hostname of the current page (derived from render URL)
    - `PagePath string` — the path of the current page
    - `PageURL *url.URL` — absolute URL of the current page (for resolving relative URLs)

  - `NewRewriter(zimFile, pageURL string) (*Rewriter, error)`
    - Parse pageURL, extract host and path
    - Store for use in URL resolution

  - `Resolve(rawURL string) string`
    - Parse rawURL with `net/url.Parse()`
    - If scheme is `http`/`https`:
      - Extract hostname, path
      - Return `/<zimFile>/<hostname><path>`
    - If scheme is `//` (protocol-relative):
      - Treat as `https:` + path
      - Return `/<zimFile>/<hostname><path>`
    - If no scheme (relative):
      - Resolve against `PageURL`
      - Return `/<zimFile>/<resolvedHostname><resolvedPath>`
    - If scheme is `data:`, `mailto:`, `tel:`, `javascript:`:
      - For `javascript:`: return `#` (strip)
      - For others: return unchanged
    - If fragment-only (`#anchor`):
      - Return unchanged

  - `Rewrite(node *html.Node)`
    - Walk the node tree
    - For each ElementNode, check attributes that contain URLs:
      - `href`, `src`, `action`, `cite`, `longdesc` → call `Resolve(value)` → update attribute
      - `srcset` → split by `,`, resolve each candidate, reassemble → update attribute
      - `data-src` → same as `src` (lazy loading)
      - `poster` → same as `src`
    - Handle `<style>` elements: scan text content for `url(...)` and `@import`, resolve each, rewrite

### 2.3 CSS Rewriter

- [x] **2.3.1** — Implement `RewriteCSS(cssContent []byte, rewriter *Rewriter) []byte`
  - Find all `url(...)` patterns (handle `url(path)`, `url("path")`, `url('path')`)
  - Find all `@import ...` patterns
  - For each found URL: resolve via rewriter, replace in CSS content
  - Return rewritten CSS bytes

### 2.4 Renderer Pipeline

- [x] **2.4.1** — Implement `internal/render/renderer.go`
  - `Renderer` struct holding `*zim.Archive` and filter config
  - `NewRenderer(archive *zim.Archive, filters FilterConfig) *Renderer`
  - `Render(zimFile string, renderPath string) (data []byte, mimeType string, err error)`
    - Construct ZIM entry path: `C/<hostname><path>` from renderPath
    - Look up entry: `archive.EntryByPath(zimPath)`
    - If entry is redirect: resolve via `entry.RedirectEntry()`
    - Get item: `entry.Item(true)` (follow redirects)
    - Get data: `item.DataAll()`
    - Get MIME: `item.MimeType()`
    - If MIME is `text/html`:
      - Parse HTML: `html.Parse(bytes.NewReader(data))`
      - Apply filters: `filters.Apply(doc, config)`
      - Rewrite URLs: create `Rewriter`, call `Rewriter.Rewrite(doc)`
      - Serialize: `html.Render(&buf, doc)`
      - Return buf.Bytes(), "text/html", nil
    - If MIME starts with `text/css`:
      - Rewrite CSS: `RewriteCSS(data, rewriter)`
      - Return rewritten CSS, "text/css", nil
    - Else:
      - Return raw data, mimeType, nil

### 2.5 Render Handler

- [x] **2.5.1** — Implement `handleRender` in `internal/server/handlers.go`
  - Parse URL: extract `{zimfile}` from Go 1.22+ path pattern `GET /{zimfile}/{remainder...}`
  - Validate `{zimfile}` ends with `.zim` → else 400
  - Combine `{remainder...}` into path string
  - Look up archive in manager → else 404
  - Create renderer, call `Render(zimfile, remainder)`
  - Set `Content-Type` header to returned MIME type
  - Write data to response

- [x] **2.5.2** — Handle edge cases
  - Entry not found → 404 with path variants tried
  - Redirect chain → resolved with max 5 hops limit
  - Large items → capped at 100MB read, partial stream
  - Query string paths → preserved in ZIM path lookup (CSS cached.php?t=...&f=...)
  - Multiple path variants tried: bare path, C/ prefix, www./no-www hostname variants

- [x] **2.5.3** — Register route in `server.go`
  - `GET /{zimfile}/{remainder...}` → `handleRender`
  - Validates `.zim` suffix in handler (not in pattern — Go 1.22+ mux doesn't support `{name}.suffix` syntax)

- [x] **2.5.4** — Test render view end-to-end
  - Tested with real devdocs Go ZIM (212 entries, 1.6MB) — 6 subtests in `TestRenderEndToEnd`
  - HTML pages: scripts stripped, onclick removed, links rewritten to render URLs
  - CSS files: MIME corrected from `application/octet-stream` to `text/css` via extension map
  - Missing entries return 404, missing ZIM files return 404, bad filenames return 400
  - Path variants tried: bare path → C/ prefix → www./no-www hostname → strip first segment

---

## Phase 3: Archive View — Queue & Downloader

**Goal**: JSON-backed download queue, HTTP downloader with adaptive throttling, asset extraction from HTML, robots.txt and sitemap support.

### 3.1 Download Queue

- [x] **3.1.1** — Implement `internal/archive/queue.go`
  - `QueueEntry`, `Queue`, `QueueStats` structs with JSON serialization
  - `NewQueue()` — creates or loads existing queue JSON
  - `Load()` / `Save()` — atomic JSON persistence (write to `.tmp`, rename)
  - `Add()` / `AddIfNew()` — dedup via `CanonicalizeURL`, urlIndex map
  - `GetByStatus()` / `PopPending()` — filtered lookup and status transition
  - `HasURL()` / `UpdateEntry()` — thread-safe with `sync.Mutex`
  - `Stats` counter methods: `PendingCount()`, `DownloadedCount()`, `FailedCount()`, `SkippedCount()`
  - Fixed: `Add()` uses `adjustStats()` based on entry's actual status (not always incrementing `Pending`)

### 3.2 URL Canonicalization

- [x] **3.2.1** — Implement `CanonicalizeURL()` in `queue.go`
  - Parse with `net/url.Parse()`, strip fragment + query string
  - Strip default ports (443 for https, 80 for http)
  - Lowercase scheme and host, normalize trailing `/`

- [x] **3.2.2** — Test canonicalization in `archive_test.go`
  - `https://en.Wikipedia.org:443/wiki/Main_Page?utm_source=web#intro` → `https://en.wikipedia.org/wiki/Main_Page`
  - `http://Example.com:80/path/` → `http://example.com/path`
  - Port 8080 preserved, root path `/` preserved

### 3.3 URL-to-Path Mapping

- [x] **3.3.1** — Implement `URLToPath()` in `queue.go`
  - Returns `hostname`, `localPath` (filesystem cache path), `zimPath` (`C/<hostname><path>`)
  - Preserves query strings in `localPath` and `zimPath`

### 3.4 Adaptive Throttling

- [x] **3.4.1** — Implement `ThrottleState` in `internal/archive/downloader.go`
  - Fields: Host, LastRequest, MinDelay, CurrentDelay, MaxDelay, ErrorTimestamps, WindowDuration, ErrorThreshold
  - `NewThrottleState()` — MinDelay=100ms, MaxDelay=30s, WindowDuration=15min, ErrorThreshold=10
  - `SetCrawlDelay()` — overrides from robots.txt Crawl-Delay
  - `Wait()` — sleeps until delay elapsed since last request
  - `RecordSuccess()` — decays delay by 0.9x, floors at MinDelay
  - `RecordError()` — appends timestamp, prunes window, doubles delay when threshold exceeded
  - Fixed: `RecordError()` handles zero delay (starts at 1s if MinDelay is 0)

### 3.5 HTTP Downloader

- [x] **3.5.1** — Implement `Downloader` in `internal/archive/downloader.go`
  - `http.Client` with 30s timeout, no auto-follow redirects (manual tracking)
  - Per-host `ThrottleState` via `getThrottle()` with `sync.Mutex`
  - `Download()` — handles 2xx (save to cache), 3xx (follow redirect, return ErrRedirect), 404 (retry once), 429 (Retry-After), 5xx (retry up to 3x)
  - `handleRetry()` — increments RetryCount, returns ErrRetry or marks Failed
  - `SaveToCache()` — `os.MkdirAll` + `os.WriteFile` to `dataDir/entry.Path`
  - `DetectMimeType()` — Content-Type header first, extension map fallback
  - Extension map covers: html, css, js, json, xml, images, fonts, video, audio, pdf, archives

### 3.6 Asset Extraction

- [x] **3.6.1** — Implement `internal/archive/extractor.go`
  - `Extractor` struct with `baseURL`, `primaryHost`, `referrer`, `FollowPages bool`
  - `NewExtractor()` — creates extractor with FollowPages=true (extracts both assets and page links)
  - `NewExtractorAssetsOnly()` — creates extractor with FollowPages=false (extracts only assets, skips `<a>` / `<iframe>` page links)
  - `Extract()` — parses HTML with `golang.org/x/net/html`, walks node tree
  - Handles: `<img src/srcset>`, `<link rel=stylesheet>`, `<script src>`, `<source src/srcset>`, `<video/audio src/poster>`, `<track src>`, `<object data>`, `<embed src>`
  - `<a href>` classification: same host → EntryTypePage; different host → EntryTypeExternalPage; download attr OR known file extension → EntryTypeAsset
  - `<iframe src>` classification: same host → EntryTypePage; different host → EntryTypeExternalPage
  - Inline `<style>` CSS: extracts `url()` and `@import` references
  - `ExtractedURL` return type with URL + EntryType classification

### 3.7 Sitemap Support

- [x] **3.7.1** — Implement `internal/archive/sitemap.go`
  - `FetchDefaultSitemap()` — tries `https://{hostname}/sitemap.xml`
  - `FetchSitemaps()` — fetches URLs from `Sitemap:` directives in robots.txt
  - `fetchSitemap()` — recursive fetcher with depth limit (3), URL dedup via `seen` map
  - `parseSitemapXML()` — string-based XML parsing for `<sitemapindex>` and `<urlset>`
  - Returns deduplicated seed URLs

### 3.8 robots.txt Support

- [x] **3.8.1** — Implement robots parsing in `internal/archive/sitemap.go`
  - `FetchRobotsTxt()` — fetches `https://{hostname}/robots.txt`, parses line by line
  - Tracks `User-agent:`, applies `Disallow:` / `Allow:` rules for `*` and `zimdex` agents
  - `RobotsMatcher` with `IsAllowed(path)` — longest-path-prefix match, Allow overrides Disallow
  - Extracts `Crawl-Delay:` and `Sitemap:` directives
  - Returns `*RobotsMatcher`, sitemap URLs, crawl delay

---

## Phase 4: Archive View — Scraper & Builder

**Goal**: Orchestrator that ties together queue, downloader, extractor, sitemap, and robots. Pause/Continue support. ZIM builder.

### 4.1 Scraper Orchestrator

- [x] **4.1.1** — Implement `internal/archive/scraper.go`
  - `Scraper` struct with fields, `NewScraper()` with sitemap/robots integration
  - `Start()` spawns page + asset workers, `Pause()` cancels context and saves queue, `Continue()` reloads and restarts
  - `Stop()` cancels and drains workers
  - `worker()` loop: `PopPending` → `Download` → `RecalcStats` → `extractAndEnqueue` → `Save`
  - Page workers also pop `EntryTypeExternalPage` entries when no primary pages remain
  - External pages use `NewExtractorAssetsOnly` (FollowPages=false) for asset-only extraction, no page link following
  - Fixed: completion check waits for both `PendingCount==0` AND `DownloadingCount==0`
  - Fixed: `RecalcStats` called after each download to keep stats in sync
  - Cross-host asset handling: assets + external pages from other hosts/CDNs enqueued with correct hostname-based paths

### 4.2 ZIM Builder

- [x] **4.2.1** — Implement `internal/archive/builder.go`
  - `BuildZIM(dataDir, queue)` creates ZIM from downloaded cache entries
  - Iterates all `status=downloaded` entries, reads from cache
  - Extracts `<title>` from HTML pages via `golang.org/x/net/html`
  - Fixed: gozim Writer had 4 bugs preventing ZIM creation — all fixed in local gozim
  - Cross-host assets included: CDN entries stored with their hostname-based ZIM paths

### 4.3 Archive API Handlers

- [x] **4.3.1** — Implement archive handlers in `internal/server/handlers.go`
  - All 8 handlers wired to real scraper registry with JSON request/response
  - `handleArchiveStart` — creates scraper, starts async, returns 202 with hostname
  - `handleArchiveStatus` — returns job status, stats (pending/downloading/downloaded/failed/skipped), recent logs
  - `handleArchiveQueue` — returns paginated queue entries with status filter
  - `handleArchiveBuild` — builds ZIM, triggers `manager.Reload()`, reports filename
  - `handleArchiveRetry` — resets specified/all failed entries to pending

### 4.4 Scraper Manager

- [x] **4.4.1** — Implement scraper registry in `internal/server/server.go`
  - `map[string]*archive.Scraper` keyed by hostname
  - `sync.RWMutex` for thread safety
  - Passed to `Handlers` struct alongside Manager and Searcher
  - `DataDir` passed for scraper initialization

### 4.5 Archive UI

- [x] **4.5.1** — Archive UI in `internal/server/templates/archive.html`
  - ZIM files list renders via `/api/archives`
  - New archive form with seed URL, workers, page limit, robots/SSL toggles, filter checkboxes loaded from `/api/filters`
  - Live job cards with progress bar (%), stats (downloaded/failed/pending/active/skipped)
  - Live activity feed — last 50 URL status changes with timestamps, scrolling log
  - Pause / Continue / Stop / Build ZIM buttons per job
  - Auto-polling every 2s via `/api/archive/{hostname}/status`

- [x] **4.5.2** — Implement JS for archive UI
  - `startArchive()` sends `filter_names` array from checked filter checkboxes
  - `pollJobs()` auto-refreshes all active job cards every 2s
  - `pauseJob`/`continueJob`/`stopJob`/`buildJob` call respective API endpoints
  - `loadFilters()` fetches available filters from `/api/filters`

### 4.6 Register Archive Routes

- [x] **4.6.1** — All 8 archive API routes registered and wired
  - `GET/POST` routes all active in `server.go` via Go 1.22+ `http.ServeMux`
  - All handlers return real JSON responses from the scraper registry

---

## Phase 5: Polish & Hardening

**Goal**: Edge cases, error handling, performance, documentation.

### 5.1 Error Handling & Robustness

- [ ] **5.1.1** — Graceful degradation when ZIM files are missing/corrupt
  - ZIM file deleted while open: catch error, remove from manager, log warning
  - Corrupted ZIM file: log error, skip, don't crash
  - Queue JSON corruption: backup previous version, create fresh

- [ ] **5.1.2** — Recovery from crashes during scraping
  - Queue is persisted after every entry update (atomic write)
  - On restart, scraper loads existing queue and resumes
  - Entries with status=downloading at crash time: reset to pending

- [ ] **5.1.3** — Download cache management
  - Estimate cache size, display in archive UI
  - "Clean cache" button: remove cached files for entries already built into ZIM
  - Disk space check before starting a scrape (warn if low)

- [ ] **5.1.4** — Large file handling
  - Stream large files (>10MB) to cache instead of holding in memory
  - Render view: stream large entries with `item.Data(offset, size)` for range requests
  - Support `Range` header for video/audio seeking within ZIM entries

### 5.2 Performance

- [ ] **5.2.1** — ZIM cache tuning
  - gozim Archive uses 64MB LRU cache for decompressed clusters by default. This should be sufficient. Monitor and tune if needed.

- [ ] **5.2.2** — Render view caching
  - Cache rewritten HTML for frequently accessed pages (in-memory LRU, 100 entries)
  - Cache key: `{zimfile}/{zimPath}`
  - Invalidate when ZIM file is reloaded

- [ ] **5.2.3** — Search index pre-warming
  - On startup, optionally pre-build BM25 index for all archives (by calling Search with an empty query, or by accessing the searcher)
  - This avoids first-search latency

- [ ] **5.2.4** — Concurrent download tuning
  - Expose page/asset worker counts in archive UI for runtime tuning
  - Display current throttle delays so user can understand rate limiting

### 5.3 UI Polish

- [ ] **5.3.1** — Responsive design
  - Mobile-friendly layout: single column, larger touch targets
  - Dark mode support (via `prefers-color-scheme` media query or toggle)

- [ ] **5.3.2** — Accessibility
  - Proper ARIA labels on search inputs, buttons
  - Keyboard navigation for search results
  - Focus indicators

- [ ] **5.3.3** — Empty states
  - No ZIM files loaded: show welcome message with instructions
  - No search results: show external search engine links
  - No active scrapes: show "Create your first archive" CTA

### 5.4 Documentation

- [ ] **5.4.1** — Update `README.md`
  - What is ZIMdex
  - How to install (`go install github.com/cookiengineer/zimdex@latest`)
  - How to run (`zimdex --folder=./data --port=3000`)
  - How to scrape a website (walkthrough)
  - How to search
  - Browser integration (Firefox custom search engine setup)

- [ ] **5.4.2** — Document API
  - List all endpoints in README with request/response examples
  - Document error codes

- [ ] **5.4.3** — Add `CONTRIBUTING.md`
  - Development setup
  - Running tests
  - Code conventions

---

## Phase 6: Testing

**Goal**: Comprehensive test coverage.

### 6.1 Unit Tests

- [x] **6.1.1** — Archive queue/URL/downloader tests in `archive_test.go`
  - Queue: create, add, dedup, AddIfNew, PopPending, stats, save/load round-trip, concurrent adds
  - URL: `CanonicalizeURL` (5 cases), `URLToPath` (hostname/local/zimPath mapping)
  - Downloader: successful download via httptest, 500 retry, `SaveToCache`
  - 8 tests passing

- [x] **6.1.2** — Archive extractor/robots/sitemap tests in `archive_test.go`
  - Extractor: img, srcset, link stylesheet, script src, a href (page vs asset vs external), CSS url(), iframe
  - Robots: `RobotsMatcher.IsAllowed()` with Disallow/Allow, longest match wins (7 cases)
  - Sitemap: `<urlset>` parsing, `<sitemapindex>` parsing, empty XML
  - Throttle: error window, crawl delay, delay doubling, zero-delay edge case
  - 6 tests passing

- [x] **6.1.3** — Server integration tests in `server_test.go`
  - Route tests: `/` redirect (303), `/index.html` (200), `/archive.html` (200), `/api/archives` (200)
  - Search API: empty query, query with params, suggest endpoint
  - Render: missing ZIM (404), no .zim suffix (400)
  - Archive API: all 7 routes return 200/202
  - Middleware: CSP headers, panic recovery (500)
  - Render E2E: 6 subtests with real devdocs Go ZIM (HTML filtering, CSS, binary, 404, 400)

- [ ] **6.1.4** — Render filters unit tests (`internal/render/filters_test.go`)
- [ ] **6.1.5** — Render rewriter unit tests (`internal/render/rewriter_test.go`)
- [ ] **6.1.6** — Search searcher unit tests (`internal/search/searcher_test.go`)

### 6.2 Integration Tests

- [ ] **6.2.1** — Search integration test
  - Create a minimal ZIM file with known content using gozim Writer
  - Load it into manager
  - Call searcher.Search() → verify results contain expected entry
  - Call searcher.Suggest() → verify autocomplete works

- [ ] **6.2.2** — Render integration test
  - Create a minimal ZIM file with HTML content referencing an image
  - Load it into manager
  - Call renderer.Render() on HTML entry → verify scripts stripped, img src rewritten
  - Call renderer.Render() on image entry → verify raw bytes returned

- [ ] **6.2.3** — Server integration test
  - Use `httptest.NewServer` with the real server setup
  - Test all API endpoints with real HTTP requests
  - Verify response codes and JSON schemas
  - Test 404 handling for missing entries
  - Test CSP headers are set

- [ ] **6.2.4** — End-to-end scrape + search test
  - Set up httptest server with a mini website (index.html, style.css, image.png, about.html linked)
  - Start scraper on the test server URL
  - Wait for completion
  - Build ZIM
  - Load ZIM into manager
  - Search for content → verify results
  - Render pages → verify HTML is correct, assets are referenced
  - Verify robots.txt is respected (disallow a path, verify it's skipped)

### 6.3 Benchmarks

- [ ] **6.3.1** — Search benchmark
  - Load a real ZIM file (Wikipedia article subset)
  - Benchmark Search() throughput (queries/sec)
  - Benchmark Suggest() latency

- [ ] **6.3.2** — Render benchmark
  - Benchmark HTML parse → filter → rewrite → render pipeline
  - Measure allocation and GC impact

- [ ] **6.3.3** — Download benchmark
  - Benchmark concurrent downloads with throttling
  - Measure throughput at different concurrency levels

---

## Task Summary

| Phase | Tasks | Status |
|---|---|---|
| 0 | 11 | ✅ Complete |
| 1 | 8 | ✅ Complete |
| 2 | 13 | ✅ Complete |
| 3 | 8 | ✅ Complete (19 tests) |
| 4 | 10 | ✅ Complete (all 10 tasks) |
| 5 | 11 | Pending |
| 6 | 15 | 3 complete (archive, server, render E2E), 3 pending (filters, rewriter, search), 9 pending (integration, benchmarks) |
| **Total** | **76** | **58 complete, 0 in progress, 18 pending** |

### Additional fixes beyond roadmap

| Area | Fix |
|---|---|
| gozim BM25 snippet | Removed spurious `snippetLower` rebuild causing slice bounds panic |
| gozim ParseQuery NOT terms | Restructured to detect `-prefix` before normalization |
| gozim BM25 tests | Added 31 tests (`bm25/snippet_test.go` 19 + `bm25/query_test.go` 12) — zero prior coverage |
| gozim Writer | Fixed 4 bugs: `groupItemsIntoClusters` order, `ClusterCount / 8`, `WriteAt` file position, dirent total length, dirent sorting, `resolveRedirects` after sort, checksum padding |
| CSP headers | `script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'` for UI JS + inline styles |
| Render path flexibility | 4 path variants (bare, C/ prefix, www/no-www, strip first segment) for ZIMs with different path schemes |
| MIME correction | Extension-based MIME override when ZIM stores `application/octet-stream` |
| Snippet formatting | Server-side HTML tag stripping and `**bold**` → `<b>` conversion |
| Templates | Embedded via `//go:embed` in server package |
| Favicon | Returns 204 to prevent 400 from wildcard render route |
| ZIM path convention | Removed `C/` prefix from ZIM paths — namespace is a separate dirent field |
| Cross-host assets | Scraper/builder bundle CDN assets from other hostnames into the same ZIM file |
| Media elements | Extractor handles `<video>`, `<audio>`, `<source>`, `<track>` with full attribute coverage |
| Worker completion | Fixed race where workers exited before pending entries finished downloading |
| Scraper deadlock | Fixed `s.log()` called while holding mutex causing self-deadlock |
| Query strings preserved | Canonicalization keeps query strings for MediaWiki-style sites (`?title=Page`) |
| Tracking param filter | Strips 40+ known tracking params (utm_*, fbclid, gclid, etc.) — moved to `internal/archive/filters/` |
| SSL skip | `insecure_skip_verify` flag threaded scraper→downloader→HTTP client TLS config |
| User-Agent faker | Rotates 8 browser agents (Chrome/Edge/Firefox), content-type-aware Accept headers, Sec-CH-UA hints |
| Filter plugin system | `Filter` interface with `Detect`/`FilterURL`/`FilterHTML`, registry, `Enabled(names)` selector |
| MediaWiki filter | Detects via `<meta generator>` / `class="mediawiki"`, skips Talk/User/Special/Template namespaces, skips edit/history/delete actions, rewrites `&oldid=NNN` links to canonical form in HTML |
| HTML patching | `FilterHTML` applied to cached HTML after download — rewritten links stored in cache and ZIM |
| Queue resume | `ResetStale()` resets failed+downloading entries to pending on restart, preserves downloaded |
| Live archive UI | Progress bars, activity feed (50-entry ring buffer via `RecentActivity()`), auto-polling, pause/continue/stop/build buttons |
| `/api/filters` endpoint | Returns available filter plugins for archive UI checkboxes |
| External page extraction | `<a>` / `<iframe>` links to other hostnames now extracted as `EntryTypeExternalPage` instead of discarded. External pages are downloaded, their assets extracted via `NewExtractorAssetsOnly` (FollowPages=false), but their page links are not followed — single-depth only |

---

## Dependency Graph Between Phases

```
Phase 0 (Foundation)
    │
    ├──► Phase 1 (Search View) ──► Phase 3 (Queue & Downloader)
    │                                       │
    ├──► Phase 2 (Render View) ────────────┤
    │                                       │
    └───────────────────────────────────────┤
                                            │
                                    Phase 4 (Scraper & Builder)
                                            │
                                    Phase 5 (Polish & Hardening)
                                            │
                                    Phase 6 (Testing)
```

- Phase 0 is the prerequisite for everything
- Phases 1 and 2 can be developed in parallel (search and render are independent)
- Phase 3 can start in parallel with Phase 1 and 2, but needs to coordinate on shared types (QueueEntry)
- Phase 4 depends on Phase 3 (uses downloader, queue, extractor) and Phase 2 (for filter config types)
- Phase 5 depends on all previous phases being feature-complete
- Phase 6 runs alongside all phases (write tests as features are implemented)
