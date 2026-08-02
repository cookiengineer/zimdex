# ZIMdex — Implementation Plan

## Table of Contents

1. [Overview](#overview)
2. [Dependencies](#dependencies)
3. [Module Layout](#module-layout)
4. [Three Views](#three-views)
5. [Scraping Architecture](#scraping-architecture)
6. [Render Architecture](#render-architecture)
7. [Search Architecture](#search-architecture)
8. [Data Model Reference](#data-model-reference)
9. [API Reference](#api-reference)
10. [Design Decisions](#design-decisions)

---

## Overview

ZIMdex is a self-hostable web scraper and search engine. It scrapes websites into `.zim` files and provides an offline search interface over locally archived content. It is a single Go binary (`CGO_ENABLED=0`) that hosts its own API and UI on a local HTTP port (default 3000).

Three modes of operation ("views"):

| View | URL | Purpose |
|---|---|---|
| **Search** | `/index.html` | Full-text search across loaded ZIM files with autocomplete |
| **Archive** | `/archive.html` | Manage scraping jobs: create, pause, continue, build ZIM, retry failures |
| **Render** | `/<zimfile>/<hostname><path>` | Serve archived content from ZIM files with offline-safe HTML rewriting |

---

## Dependencies

| Package | Import | Purpose |
|---|---|---|
| `github.com/cookiengineer/gozim/archive/zim` | `zim` | ZIM file read/write/search (BM25, suggestions, integrity checks) |
| `golang.org/x/net/html` | `html` | HTML5 parser + renderer for scraping asset extraction and offline HTML rewriting |
| `golang.org/x/net/html/atom` | `atom` | HTML tag/attribute name constants for fast comparisons |
| Go stdlib only | — | `net/http`, `encoding/json`, `html/template`, `sync`, `context`, `os`, `io`, `path/filepath`, `net/url`, `log`, `time`, `sort`, `strings`, `strconv`, `encoding/xml`, `compress/gzip`, `crypto/md5` |

Zero C dependencies. Builds with `CGO_ENABLED=0 go build`.

---

## Module Layout

```
zimdex/
├── main.go                          # Entry point
├── go.mod
├── go.sum
├── internal/
│   ├── archive/                     # Scraping subsystem
│   │   ├── scraper.go               # Orchestrator: manages page + asset workers, pause/continue/stop
│   │   ├── downloader.go            # HTTP client pool, adaptive throttling, error tracking
│   │   ├── queue.go                 # JSON-backed download queue per hostname (thread-safe)
│   │   ├── extractor.go             # HTML → asset URL extraction (page-only trigger)
│   │   ├── sitemap.go               # sitemap.xml / sitemap index fetcher + XML parser
│   │   ├── robots.go                # robots.txt fetcher + parser with override flag
│   │   └── builder.go               # Download cache → ZIM file (final build step, post-scrape)
│   ├── render/                      # Offline rendering subsystem
│   │   ├── renderer.go              # Pipeline orchestrator: parse → filter → rewrite → serialize
│   │   ├── rewriter.go              # URL resolution, path remapping, CSS url() rewriting
│   │   └── filters.go               # Configurable HTML element/attribute/comment filters
│   ├── search/                      # Search subsystem
│   │   └── searcher.go              # Multi-archive BM25 search + suggestion wrapper
│   ├── server/                      # HTTP server
│   │   ├── server.go                # Setup, route registration via Go 1.22+ http.ServeMux
│   │   ├── handlers.go              # All HTTP handler functions
│   │   └── middleware.go            # CSP headers, logging, panic recovery
│   └── zimfs/                       # ZIM file management
│       └── manager.go               # Scan directory for .zim files, open/close/cache archives
├── structs/                         # Shared utility types
│   ├── Console.go                   # Thread-safe colorized logging utility (kept from existing)
│   └── ConsoleMessage.go            # Structured log message with caller info (kept from existing)
├── web/
│   └── templates/                   # Go html/template files
│       ├── index.html               # Search view: search bar, results, autocomplete
│       ├── archive.html             # Archive view: job list, create form, progress, retry panel
│       └── render.html              # Chrome wrapper around rendered ZIM articles (optional)
└── public/
    └── design/                      # Static assets (CSS, images)
        └── index.css
```

---

## Three Views

### Search View (`/index.html`)

The default landing page. Features:
- Search input with debounced autocomplete (fetch `/api/suggest?q=prefix&limit=10`)
- Results list rendered from `/api/search?q=query&offset=0&limit=20`
- Each result shows: title (linked to render URL), score, text snippet, source ZIM filename
- External search engine links (optional: Google, DuckDuckGo, Wikipedia) for queries with no local results
- Powered by gozim's BM25 full-text index (in-memory, built on first `Search()` call)

### Archive View (`/archive.html`)

Scraping management dashboard. Features:
- **ZIM files list**: All `.zim` files in data dir with metadata (title, date, language, entry count, media count)
- **New job form**: Enter seed URL, toggle `respect_robots`, configure concurrency, page limit
- **Active jobs**: Progress bar (pending/downloaded/failed/skipped counts), status indicator, log tail
- **Actions**: Pause / Continue / Stop / Build ZIM / Retry All Failed
- **Failed entries panel**: Per-URL retry buttons, error messages, retry counts

### Render View (`/<zimfile>/<hostname><path>`)

Serves archived content from ZIM files. Pipeline:
1. Parse URL path into `{zimfile}`, `{hostname}`, `{path}`
2. Look up ZIM archive by filename in manager
3. Resolve ZIM entry path: `C/{hostname}{path}`
4. Fetch entry + item data from archive
5. If content type is `text/html`: run through filter + rewrite pipeline, serve
6. If content type is CSS: rewrite `url()` references, serve
7. If content type is anything else: serve raw bytes with correct Content-Type

---

## Scraping Architecture

### Queue Model

The scraper maintains three entry types within a single queue JSON file:

| Entry Type | Source of entries | Behavior |
|---|---|---|
| **page** | Seed URL, sitemap URLs, `<a href>` links on same hostname | Downloaded → HTML parsed → assets extracted → new pages + external pages enqueued |
| **external_page** | `<a href>` and `<iframe>` links to other hostnames | Downloaded → assets extracted only (FollowPages=false) → no link following (single-depth) |
| **asset** | `<img>`, `<link>`, `<script>`, `<video>`, `<audio>`, `<source>`, `<track>`, `<a download>`, CSS `url()` / `@import` | Downloaded → terminal (no further extraction) |

**Key rules:**
- Primary host pages (`EntryTypePage`) trigger full extraction: same-host `<a>` links become `page`, external `<a>` links become `external_page`, and all assets become `asset`
- External pages (`EntryTypeExternalPage`) trigger asset-only extraction via `NewExtractorAssetsOnly` (FollowPages=false). Their own page links are NOT followed — preventing unbounded crawling
- Assets from any hostname (including cross-host) are downloaded but do NOT trigger further extraction
- `<a download>` links with file extensions (`.pdf`, `.mp4`, `.zip`, etc.) are treated as assets, not pages

### Scraper State Machine

```
                 ┌─────────┐
                 │  idle   │
                 └────┬────┘
                      │ Start()
                 ┌────▼────┐
         ┌───────│ running │◄──────┐
         │       └────┬────┘       │
         │ Pause()    │ Stop()     │ Continue()
         │       ┌────▼────┐       │
         └──────►│ paused  ├───────┘
                 └────┬────┘
                      │ Stop()
                 ┌────▼────┐
                 │ stopped │
                 └─────────┘
```

State transitions:
- **idle → running**: `Start()` creates context, spawns workers, loads/creates queue JSON
- **running → paused**: `Pause()` cancels worker context, workers drain, queue persists to JSON
- **paused → running**: `Continue()` reloads queue JSON, creates new context, spawns workers
- **running / paused → stopped**: `Stop()` cancels context, workers drain, queue persists
- **running → complete**: All queue entries are `downloaded`/`failed`/`skipped`, no `pending` remain

### Worker Pool Design

```
┌─────────────────────────────────────────────────────────────┐
│                        Scraper                               │
│                                                              │
│  ┌──────────────┐     ┌──────────────────────────────┐      │
│  │  Page Workers │     │       Asset Workers           │      │
│  │  (goroutines) │     │       (goroutines)            │      │
│  │               │     │                               │      │
│  │  ┌─────────┐  │     │  ┌─────────┐  ┌─────────┐    │      │
│  │  │ Worker 1│  │     │  │ Worker 1│  │ Worker 2│    │      │
│  │  └────┬────┘  │     │  └────┬────┘  └────┬────┘    │      │
│  │       │       │     │       │            │          │      │
│  │  ┌────▼────┐  │     │  ┌────▼────┐  ┌────▼────┐    │      │
│  │  │ Worker 2│  │     │  │ Worker 3│  │ Worker 4│    │      │
│  │  └─────────┘  │     │  └─────────┘  └─────────┘    │      │
│  └───────┬───────┘     └──────────────┬───────────────┘      │
│          │                            │                      │
│          │    ┌──────────────────┐    │                      │
│          └────┤  Download Queue  ├────┘                      │
│               │  (JSON file)     │                           │
│               └────────┬─────────┘                           │
│                        │                                      │
│               ┌────────▼─────────┐                           │
│               │   ThrottleState   │                           │
│               │  (per-host,       │                           │
│               │   adaptive delay) │                           │
│               └──────────────────┘                           │
└─────────────────────────────────────────────────────────────┘
```

- Page workers: `N_page` goroutines (configurable, default 2)
- Asset workers: `N_asset` goroutines (configurable, default 4)
- Both pools share the same queue, but page workers only pop entries with `entry_type=page` and asset workers only pop entries with `entry_type=asset`
- Each worker runs through the downloader, which handles throttling per-host

### Download Flow Per URL

```
1. Pop entry from queue (filtered by entry_type for the worker pool)
2. Check robots.txt against URL path
   ├── Disallowed + respect_robots=true → mark "skipped", continue
   └── Allowed or respect_robots=false → proceed
3. Apply per-host throttle delay (adaptive based on error window)
4. HTTP GET with 30s timeout
    ├── 2xx: save body to cache path, detect Content-Type
    │   ├── If entry_type=page: extract assets + page links → enqueue new URLs
    │   ├── If entry_type=external_page: extract assets only via NewExtractorAssetsOnly (FollowPages=false)
    │   └── Mark status "downloaded"
   ├── 3xx: follow redirect (max 5 hops), enqueue final URL if different
   ├── 404: if retry_count < 2, wait 30s, retry. Else mark "failed"
   ├── 429: record error in window, apply Retry-After, retry after delay
   ├── 5xx: if retry_count < 3, wait 30s, retry. Else mark "failed"
   └── Timeout: same as 5xx
5. On any error: record in 15-min error window, update throttle state
6. Persist queue JSON
```

### URL Canonicalization

All URLs are canonicalized before queue insertion:

1. Parse with `net/url.Parse()`
2. Strip fragment (`#...`)
3. Strip query string (`?...`) entirely
4. Strip default ports (443 for https, 80 for http)
5. Decode percent-encoded characters where safe
6. Lowercase scheme and host
7. Remove trailing `/` from paths where possible (but preserve root `/`)
8. Resolve relative URLs against the page's base URL

Normalized URL becomes the deduplication key. Two URLs that normalize to the same canonical form are treated as identical.

### Data Directory Structure

```
<datadir>/                              # --folder CLI arg
│
├── en.wikipedia.org-2026-08-02.zim     # Built ZIM file
├── en.wikipedia.org-2026-08-03.zim     # Subsequent build (different date)
│
├── en.wikipedia.org.json               # Download queue for the entire scrape
│                                       # Contains all entry types (page, external_page, asset)
│                                       # from all hostnames in a single JSON file
│
├── en.wikipedia.org/                   # Primary host download cache
│   ├── wiki/
│   │   ├── Main_Page                   # Raw HTTP response body
│   │   ├── Quantum_Mechanics           #
│   │   └── ...                         #
│   ├── static/
│   │   ├── css/
│   │   │   └── main.css                #
│   │   └── images/
│   │       └── logo.png                #
│   └── ...                             #
│
├── cdn.wikimedia.org/                  # Cross-host asset cache
│   └── static/
│       └── images/
│           └── logo.png                #
│
└── news.example.com/                   # External page cache
    └── 2024/
        └── article.html                #
```

### Naming Convention

| Item | Pattern | Example |
|---|---|---|
| Download cache folder | `<hostname>/` | `en.wikipedia.org/` |
| Queue JSON file | `<hostname>.json` | `en.wikipedia.org.json` |
| Built ZIM file | `<hostname>-YYYY-MM-DD.zim` | `en.wikipedia.org-2026-08-02.zim` |
| ZIM entry path | `C/<hostname><url_path>` | `C/en.wikipedia.org/wiki/Main_Page` |
| Render URL | `/<zimfile>/<hostname><url_path>` | `/en.wikipedia.org-2026-08-02.zim/en.wikipedia.org/wiki/Main_Page` |

### Sitemap Support

The scraper attempts to seed the queue from sitemaps:

1. Fetch `robots.txt` → extract `Sitemap:` directives
2. If no sitemap found in robots.txt, try `https://<hostname>/sitemap.xml`
3. If response is a **sitemap index** (XML with `<sitemapindex>` root), fetch each child sitemap
4. If response is a **URL set** (XML with `<urlset>` root), extract all `<url><loc>` entries
5. Support gzip-compressed sitemaps (`Content-Encoding: gzip`)
6. Add all extracted URLs to the page queue

### robots.txt Support

1. Fetch `https://<hostname>/robots.txt`
2. Parse `User-agent: *` block (and `User-agent: zimdex` if present)
3. Extract `Disallow:` and `Allow:` rules
4. Extract `Crawl-Delay:` → initial throttle delay
5. Extract `Sitemap:` → passed to sitemap parser
6. Build a path matcher function: `func(path string) bool` (returns true if allowed)
7. The `respect_robots` flag controls whether disallowed paths are skipped or downloaded anyway

### Adaptive Throttling

Per-host throttle state tracks errors in a 15-minute sliding window:

```go
type ThrottleState struct {
    Host               string
    LastRequest        time.Time
    MinDelay           time.Duration   // 100ms default, overridden by Crawl-Delay
    CurrentDelay       time.Duration
    MaxDelay           time.Duration   // 30s cap
    ErrorTimestamps    []time.Time     // Ring buffer, pruned beyond 15min window
    WindowDuration     time.Duration   // 15 minutes
    ErrorThreshold     int             // 10 errors per window
}
```

Throttle logic:
- Before each request: `sleep(max(0, CurrentDelay - elapsed_since_last_request))`
- After each error (429, 5xx, timeout): record timestamp, prune window
- If errors in window > threshold: `CurrentDelay = min(CurrentDelay * 2, MaxDelay)`
- After a successful request: `CurrentDelay = max(CurrentDelay * 0.9, MinDelay)` (gradual recovery)

### QueueEntry Data Type

```go
type EntryType string

const (
    EntryTypePage         EntryType = "page"          // HTML page, triggers full extraction
    EntryTypeExternalPage EntryType = "external_page" // External HTML page, triggers asset-only extraction
    EntryTypeAsset        EntryType = "asset"         // Static asset, terminal download
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
    URL          string      `json:"url"`
    Path         string      `json:"path"`
    ZimPath      string      `json:"zim_path"`
    EntryType    EntryType   `json:"entry_type"`
    MimeType     string      `json:"mime_type,omitempty"`
    Status       QueueStatus `json:"status"`
    Size         int64       `json:"size,omitempty"`
    Referrer     string      `json:"referrer,omitempty"`
    DownloadedAt string      `json:"downloaded_at,omitempty"`
    Error        string      `json:"error,omitempty"`
    RetryCount   int         `json:"retry_count"`
}
```

### Queue File Structure

```json
{
  "host": "en.wikipedia.org",
  "start_url": "https://en.wikipedia.org/wiki/Main_Page",
  "respect_robots": true,
  "created_at": "2026-08-02T12:00:00Z",
  "updated_at": "2026-08-02T14:30:00Z",
  "status": "running",
  "page_limit": 0,
  "stats": {
    "pending": 45,
    "downloading": 3,
    "downloaded": 210,
    "failed": 12,
    "skipped": 5
  },
  "entries": [
    {
      "url": "https://en.wikipedia.org/wiki/Main_Page",
      "path": "en.wikipedia.org/wiki/Main_Page",
      "zim_path": "C/en.wikipedia.org/wiki/Main_Page",
      "entry_type": "page",
      "mime_type": "text/html",
      "status": "downloaded",
      "size": 245678,
      "referrer": "",
      "downloaded_at": "2026-08-02T12:00:05Z",
      "retry_count": 0
    },
    {
      "url": "https://en.wikipedia.org/static/css/main.css",
      "path": "en.wikipedia.org/static/css/main.css",
      "zim_path": "C/en.wikipedia.org/static/css/main.css",
      "entry_type": "asset",
      "mime_type": "text/css",
      "status": "downloaded",
      "size": 12345,
      "referrer": "https://en.wikipedia.org/wiki/Main_Page",
      "downloaded_at": "2026-08-02T12:00:08Z",
      "retry_count": 0
    }
  ]
}
```

### Asset Extraction (extractor.go)

HTML pages are parsed with `golang.org/x/net/html`. The extractor walks the node tree looking for:

| Tag | Attribute | Entry Type | Condition |
|---|---|---|---|
| `<img>` | `src`, `srcset` | asset | Always |
| `<link>` | `href` | asset | `rel="stylesheet"` only |
| `<link>` | `href` | ignored | `rel="dns-prefetch"`, `preconnect`, `preload`, `canonical`, `alternate` |
| `<script>` | `src` | asset | Always (but scripts downloaded, not executed) |
| `<video>` | `src`, `poster` | asset | Always |
| `<audio>` | `src` | asset | Always |
| `<source>` | `src`, `srcset` | asset | Always |
| `<track>` | `src` | asset | Always |
| `<object>` | `data` | asset | Always |
| `<embed>` | `src` | asset | Always |
| `<a>` | `href` | page | Same hostname, no `download`, no file extension |
| `<a>` | `href` | external_page | Different hostname, no `download`, no file extension |
| `<a>` | `href` | asset | Has `download` attribute OR path ends with known file extension |
| `<iframe>` | `src` | page | Same hostname |
| `<iframe>` | `src` | external_page | Different hostname |
| CSS `url()` | inside `<style>` or fetched stylesheets | asset | Always |
| CSS `@import` | inside `<style>` or fetched stylesheets | asset | Always |

**Known file extensions** for asset classification of `<a>` links:
`.pdf`, `.epub`, `.zip`, `.tar`, `.gz`, `.bz2`, `.xz`, `.7z`, `.rar`,
`.mp3`, `.mp4`, `.ogg`, `.ogv`, `.webm`, `.avi`, `.mov`, `.wav`, `.flac`,
`.png`, `.jpg`, `.jpeg`, `.gif`, `.svg`, `.webp`, `.ico`, `.bmp`, `.tiff`,
`.doc`, `.docx`, `.xls`, `.xlsx`, `.ppt`, `.pptx`, `.odt`, `.ods`, `.odp`

**CSS asset extraction**: When a stylesheet is downloaded (or inline `<style>` is encountered during page download), the content is scanned for:
- `url(...)` or `url('...')` or `url("...")` — extract the inner URL, resolve against stylesheet URL
- `@import url(...)` or `@import "..."` — extract URL, resolve, enqueue as asset

### ZIM Builder (builder.go)

The builder runs as a final step after scraping is complete (or when user manually triggers "Build ZIM"):

1. Create a `zim.Writer` configured with:
   - `SetCompression(zim.CompressionZstd)`
   - `SetClusterSize(2 * zim.Megabyte)`
   - `SetIndexing(true, "eng")` — or detect language from HTML `<html lang>` attribute
   - `SetMainPath(zimPathOfStartURL)`
2. Iterate all queue entries with `status == "downloaded"`:
    - Includes primary host pages, external pages, and assets from ALL hostnames
    - Read file bytes from download cache at `dataDir/entry.Path`
    - Create `zim.NewBytesItem(zimPath, mimeType, title, data)`
    - Call `writer.AddItem(item)`
3. Add metadata:
   - `Title`: from `<title>` of start page or hostname
   - `Creator`: "ZIMdex"
   - `Date`: `YYYY-MM-DD`
   - `Language`: detected from HTML or "eng"
   - `Source`: start URL
   - `Description`: "Archived from {hostname} on {date}"
4. Call `writer.Finish()` to finalize the ZIM file
5. Output filename: `<datadir>/<hostname>-YYYY-MM-DD.zim`

If builder is re-run on the same cache, it produces a new ZIM with the current date suffix. Old ZIM files are not overwritten.

---

## Render Architecture

### URL Scheme

```
/<zimfile>/<hostname><path>

Examples:
/en.wikipedia.org-2026-08-02.zim/en.wikipedia.org/wiki/Main_Page
/en.wikipedia.org-2026-08-02.zim/en.wikipedia.org/static/css/main.css
/en.wikipedia.org-2026-08-02.zim/cdn.wikipedia.org/assets/foo.png
```

### Resolution Algorithm

```
Input: URL path = "/en.wikipedia.org-2026-08-02.zim/en.wikipedia.org/wiki/Main_Page"

1. Find first "/" after ".zim"
   zimfile = "en.wikipedia.org-2026-08-02.zim"
   remainder = "en.wikipedia.org/wiki/Main_Page"

2. Look up zimfile in ZIM Manager

3. Build ZIM entry path: "C/" + remainder
   entryPath = "C/en.wikipedia.org/wiki/Main_Page"

4. Look up entry in ZIM archive
   entry = archive.EntryByPath(entryPath)
   if entry.IsRedirect():
       entry = entry.RedirectEntry()

5. Get item data
   item = entry.Item(true)       // true = follow redirects
   data = item.DataAll()
   mimeType = item.MimeType()

6. If mimeType == "text/html":
       run filter + rewrite pipeline
       serve rewritten HTML
   elif mimeType starts with "text/css":
       rewrite CSS url() references
       serve rewritten CSS
   else:
       serve raw data with Content-Type header
```

### HTML Rewriting Pipeline

```
Raw HTML bytes
    │
    ▼
┌─────────────────┐
│  html.Parse()   │  Parse into token tree (golang.org/x/net/html)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  filters.Apply()│  Walk node tree, remove disallowed elements/attributes
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ rewriter.Rewrite│  Walk node tree, remap all URLs to render paths
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  html.Render()  │  Serialize node tree back to HTML bytes
└────────┬────────┘
         │
         ▼
    Rewritten HTML bytes (served to browser)
```

### URL Rewriting Rules (rewriter.go)

For every element in the node tree, iterate its attributes:

| Attribute | Action |
|---|---|
| `href` | Resolve against page base URL → construct render path → replace |
| `src` | Resolve against page base URL → construct render path → replace |
| `srcset` | Split by `,`, rewrite each candidate URL, preserve descriptors |
| `data-src` | Same as `src` (lazy loading) |
| `poster` | Same as `src` |
| `action` | Resolve → construct render path → replace (forms) |
| `cite` | Resolve → construct render path → replace (blockquotes) |
| `longdesc` | Resolve → construct render path → replace |

**Resolution logic per URL**:

1. Parse URL with `net/url.Parse()`
2. If scheme is `http://` or `https://`:
   - Extract hostname
   - Determine which ZIM file contains this hostname (the current one, by default)
   - Construct: `/<zimfile>/<hostname><path>`
3. If scheme is `//` (protocol-relative):
   - Same as above, using `https://` as default scheme
4. If it's a relative path (no scheme):
   - Resolve against the page's absolute URL
   - Construct render path
5. If scheme is `data:`, `javascript:`, `mailto:`, `tel:`:
   - Pass through unchanged (or strip for `javascript:` based on filter config)
6. If it's a fragment-only URL (`#anchor`):
   - Pass through unchanged

**CSS rewriting**: When serving CSS files from the render view, scan for `url(...)` and `@import` references. Resolve each URL against the stylesheet's original absolute URL, then construct the render path.

### HTML Filters (filters.go)

Filters are configurable rules applied during the render pipeline. Default ruleset:

```
[RemoveElements]
<script>                # All scripts (security: no JS in offline mode)
<iframe>                # All iframes
<object>                # All objects
<embed>                 # All embeds
<applet>                # All applets (legacy)

[RemoveAttributes]
onclick, ondblclick, onmousedown, onmouseup, onmouseover, onmouseout,
onmousemove, onkeydown, onkeyup, onkeypress, onfocus, onblur,
onchange, onsubmit, onreset, onselect, onload, onunload, onerror,
onabort, onscroll, onresize, onbeforeunload, onhashchange

[RemoveMetaNames]
twitter:*, og:*, fb:*, apple-mobile-web-app-*, msapplication-*

[RemoveLinkRels]
dns-prefetch, preconnect, preload, prefetch, prerender

[RemoveComments]
true    # Strip all HTML comments (<!-- ... -->)

[RemoveTrackingImages]
true    # Remove <img> with dimensions exactly 1x1 and src containing "pixel" or "tracking"

[RemoveEmptyElements]
true    # Remove elements that became empty after other filters (e.g., empty <div>)
```

Filters can be extended per-project via a JSON config file, which is merged with defaults.

---

## Search Architecture

### Multi-Archive Search (searcher.go)

```go
type Searcher struct {
    archives       []*zim.Archive
    zimSearcher    *zim.Searcher
    suggesters     map[string]*zim.SuggestionSearcher   // keyed by archive path
    mu             sync.RWMutex
}
```

Methods:

```go
func NewSearcher(archives []*zim.Archive) *Searcher
func (s *Searcher) Search(query string, offset, limit int) (*zim.SearchResultSet, error)
func (s *Searcher) Suggest(prefix string, limit int) ([]SuggestionResult, error)
func (s *Searcher) AddArchive(a *zim.Archive)
func (s *Searcher) RemoveArchive(path string)
func (s *Searcher) Close() error
```

**Search flow**:
1. Call `zim.NewSearcher(archives...)` — gozim builds in-memory BM25 index
2. On `Search(query, offset, limit)`: delegate to `zimSearcher.Search(query, offset, limit)`
3. Results include: `Path`, `Title`, `Score`, `Snippet` (200-char highlight), `WordCount`

**Suggest flow**:
1. For each archive, query `zim.NewSuggestionSearcher(archive).Suggest(prefix, limit)`
2. Merge results from all archives
3. Deduplicate by path
4. Sort by relevance (shorter titles first, then alphabetical)
5. Return top N

### Search API Response Format

`GET /api/search?q=quantum+physics&offset=0&limit=20`

```json
{
  "query": "quantum physics",
  "offset": 0,
  "limit": 20,
  "estimated": 156,
  "results": [
    {
      "title": "Quantum Mechanics",
      "path": "C/en.wikipedia.org/wiki/Quantum_Mechanics",
      "zim_file": "en.wikipedia.org-2026-08-02.zim",
      "render_url": "/en.wikipedia.org-2026-08-02.zim/en.wikipedia.org/wiki/Quantum_Mechanics",
      "score": 12.45,
      "snippet": "...of energy levels in atoms led to the development of <b>quantum</b> <b>mechanics</b> in the early 20th century...",
      "word_count": 45210
    }
  ]
}
```

### Suggest API Response Format

`GET /api/suggest?q=quant&limit=10`

```json
{
  "query": "quant",
  "suggestions": [
    {
      "title": "Quantum Mechanics",
      "path": "C/en.wikipedia.org/wiki/Quantum_Mechanics",
      "render_url": "/en.wikipedia.org-2026-08-02.zim/en.wikipedia.org/wiki/Quantum_Mechanics",
      "snippet": "Quantum Mechanics"
    }
  ]
}
```

---

## Data Model Reference

### Scraper (scraper.go)

```go
type Scraper struct {
    Host           string           // Primary hostname
    StartURL       string           // Seed URL
    DataDir        string           // Root data directory
    Queue          *Queue           // Download queue
    Robots         *RobotsMatcher   // robots.txt path matcher (nil if respect_robots=false)
    Throttle       *ThrottleState   // Per-host adaptive throttle
    Filters        FilterConfig     // HTML filter rules (used for scraping decisions)
    
    PageWorkerCount  int            // Goroutine count for page downloads
    AssetWorkerCount int            // Goroutine count for asset downloads
    PageLimit        int            // Max pages to download (0 = unlimited)
    MaxRetries       int            // Max retries per URL (3)
    RetryDelay       time.Duration  // Delay between retries (30s)
    
    ctx    context.Context
    cancel context.CancelFunc
    
    mu     sync.RWMutex
    status string                   // idle, running, paused, stopped, error
    
    logBuf []LogEntry               // Ring buffer of recent log lines
}
```

### Queue (queue.go)

```go
type Queue struct {
    Host         string        `json:"host"`
    StartURL     string        `json:"start_url"`
    RespectRobots bool         `json:"respect_robots"`
    CreatedAt    string        `json:"created_at"`
    UpdatedAt    string        `json:"updated_at"`
    Status       string        `json:"status"`
    PageLimit    int           `json:"page_limit"`
    Stats        QueueStats    `json:"stats"`
    Entries      []QueueEntry  `json:"entries"`

    filePath string
    dataDir  string
    mu       sync.Mutex
    urlIndex map[string]int    // canonical URL → index in Entries slice
}

type QueueStats struct {
    Pending     int `json:"pending"`
    Downloading int `json:"downloading"`
    Downloaded  int `json:"downloaded"`
    Failed      int `json:"failed"`
    Skipped     int `json:"skipped"`
}
```

Key methods:
```go
func NewQueue(filePath, host, startURL string, respectRobots bool, pageLimit int) *Queue
func (q *Queue) Load(filePath string) error
func (q *Queue) Save() error
func (q *Queue) Add(entry QueueEntry) bool         // returns true if added (not duplicate)
func (q *Queue) GetByStatus(status QueueStatus) []*QueueEntry
func (q *Queue) HasURL(canonicalURL string) bool
func (q *Queue) UpdateEntry(canonicalURL string, updater func(*QueueEntry)) // thread-safe update
func (q *Queue) PendingCount() int
func (q *Queue) RecalcStats()                       // recount all statuses
```

### ZIM Manager (zimfs/manager.go)

```go
type Manager struct {
    Dir      string                    // Data directory to scan
    Archives map[string]*zim.Archive   // Filename → Archive
    
    mu       sync.RWMutex
}

func NewManager(dir string) *Manager
func (m *Manager) Scan() error                     // Scan dir for *.zim, open all
func (m *Manager) Get(filename string) (*zim.Archive, bool)
func (m *Manager) List() []ArchiveInfo             // Return metadata for all loaded ZIMs
func (m *Manager) Reload() error                   // Rescan (for new ZIMs added while running)
func (m *Manager) Close() error                    // Close all archives
```

### ArchiveInfo

```go
type ArchiveInfo struct {
    Filename    string `json:"filename"`
    Title       string `json:"title"`
    Date        string `json:"date"`
    Language    string `json:"language"`
    Source      string `json:"source"`
    Description string `json:"description"`
    EntryCount  uint32 `json:"entry_count"`
    ArticleCount uint32 `json:"article_count"`
    MediaCount  uint32 `json:"media_count"`
    Size        int64  `json:"size_bytes"`
    Checksum    string `json:"checksum"`
}
```

---

## API Reference

### Search Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/search` | Full-text search. Query params: `q` (required), `offset` (default 0), `limit` (default 20) |
| `GET` | `/api/suggest` | Title autocomplete. Query params: `q` (required), `limit` (default 10) |

### Archive Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/archives` | List all loaded ZIM files with metadata |
| `POST` | `/api/archive/start` | Start a new scrape job. Body: `{url, respect_robots, page_limit, page_workers, asset_workers}` |
| `POST` | `/api/archive/{hostname}/stop` | Stop an active scrape |
| `POST` | `/api/archive/{hostname}/pause` | Pause an active scrape |
| `POST` | `/api/archive/{hostname}/continue` | Resume a paused scrape |
| `GET` | `/api/archive/{hostname}/status` | Get job status, progress, throttle state, recent logs |
| `GET` | `/api/archive/{hostname}/queue` | Get full download queue. Query params: `status` (filter), `offset`, `limit` |
| `POST` | `/api/archive/{hostname}/build` | Build ZIM file from completed downloads |
| `POST` | `/api/archive/{hostname}/retry` | Retry all failed entries. Body: optional `{urls: [...]}` for specific retries |

### Render Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/{zimfile}/{remainder...}` | Render content from ZIM file. `{zimfile}` must end with `.zim`. `{remainder}` is `<hostname><path>` |

### Status Response Example

`GET /api/archive/en.wikipedia.org/status`

```json
{
  "host": "en.wikipedia.org",
  "start_url": "https://en.wikipedia.org/wiki/Main_Page",
  "status": "running",
  "respect_robots": true,
  "created_at": "2026-08-02T12:00:00Z",
  "updated_at": "2026-08-02T14:30:00Z",
  "stats": {
    "pending": 45,
    "downloading": 3,
    "downloaded": 210,
    "failed": 12,
    "skipped": 5
  },
  "throttle": {
    "host": "en.wikipedia.org",
    "current_delay_ms": 250,
    "errors_last_15min": 2,
    "error_threshold": 10
  },
  "recent_logs": [
    "[14:29:55] Downloaded: /wiki/Quantum_Mechanics (245KB)",
    "[14:29:58] Downloaded: /static/css/main.css (12KB)",
    "[14:30:01] Failed: /wiki/Deleted_Page (404 Not Found)"
  ]
}
```

---

## Design Decisions

| Decision | Rationale |
|---|---|
| **Go 1.22+ `http.ServeMux`** for routing | Supports `{param}` and `{remainder...}` patterns natively. No external router needed. |
| **JSON queue files** | Human-readable, resumable, zero-dependency, works cross-platform. Can be inspected and manually edited. |
| **Download cache mirrors URL paths** | `en.wikipedia.org/wiki/Main_Page` stored at `<datadir>/en.wikipedia.org/wiki/Main_Page`. Natural, debuggable, tool-independent. |
| **ZIM path = `C/<hostname><url_path>`** | Simple 1:1 mapping from render URL path to ZIM entry. No hashing, no encoding tricks. |
| **All render URLs are absolute** | No `<base>` tag needed. Every link is self-contained with the ZIM filename context. |
| **Final ZIM build step** | Simpler than incremental; produces optimally ordered clusters; allows re-running on same cache. |
| **Query strings always stripped** | Prevents duplicate content from tracking params (utm_*, fbclid, etc.). Simplifies deduplication. |
| **Two queue types (page vs asset)** | Pages trigger asset extraction; assets are terminal. Matches real browser behavior. |
| **`html/template` for UI** | Server-side rendering with minimal vanilla JS for interactivity. No framework bloat. |
| **Error window for adaptive throttling** | 15-min sliding window detects server throttling without complex heuristics. Backs off on errors, recovers on success. |
| **Pause = cancel context + persist JSON** | Clean goroutine lifecycle. Resume = reload JSON + new context. Zero state loss. |
| **Per-host throttle state** | Different hosts have different rate limits. En.wikipedia.org can handle more than a small blog. |
| **crawl-delay from robots.txt** | Respects server-advertised rate limits as initial throttle delay. Good web citizen. |
| **No C dependencies** | `CGO_ENABLED=0` means static binary, portable across Linux/macOS/Windows without libc or system deps. |

---

## Security Considerations

- **No JavaScript execution** in render view: All `<script>` tags are stripped. Event handler attributes removed. User's browser cannot run JS from archived pages.
- **Content-Security-Policy headers** on all responses: `default-src 'self'; script-src 'none'; object-src 'none'; base-uri 'self'; form-action 'self'`
- **No outbound requests** from render view: All asset URLs are rewritten to point into ZIM files. The browser never contacts the original server.
- **No cookies/localStorage leakage**: Archived pages run in the ZIMdex origin, isolated from the original domain.
- **Input sanitization**: Search queries, URL paths, and API parameters are validated and sanitized before use.
