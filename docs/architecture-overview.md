# ZIMdex — Architecture Overview

This guide describes the current codebase and is the canonical onboarding document. It complements `scraping-workflow.md`, which details the scraper's URL lifecycle and state machines.

## What is ZIMdex

ZIMdex is a self-hostable web scraper and offline search engine. It scrapes websites into [ZIM](https://wiki.openzim.org/wiki/ZIM_file_format) archives and serves a local search + browse interface over the archived content. It is a single static Go binary with no C dependencies (`CGO_ENABLED=0`).

Two subsystems:

1. **Scraper / builder** — downloads a website (pages, assets, external pages), stores responses in a filesystem cache, and builds a `.zim` file from the cache.
2. **Server** — hosts a local UI (search + archive management) and serves archived content from ZIM files with offline-safe HTML/CSS/JS rewriting.

## Repository layout

```
zimdex/
├── go.mod / go.sum
├── cmds/
│   └── zimdex/
│       └── main.go                  # Entry point: CLI flags, manager init, signal handling
├── io/
│   └── zimfs/                       # ZIM + queue layer (no external HTTP)
│       ├── Manager.go               # Scan/open/cache *.zim, BM25 search
│       ├── ArchiveInfo.go           # Metadata struct returned by List()
│       ├── Queue.go                 # JSON-backed download queue
│       ├── QueueEntry.go            # Single download unit (page/asset/external-page)
│       ├── QueueEntryType.go        # page | asset | external-page
│       ├── QueueEntryStatus.go      # pending | downloading | downloaded | failed | skipped
│       ├── QueueStatus.go           # idle | running | paused
│       ├── QueueInfo.go             # Counters per status
│       └── Builder.go               # Cache → .zim via gozim Writer
├── internal/
│   ├── archive/                     # Scraping subsystem (network + orchestration)
│   │   ├── Scraper.go               # Orchestrator: workers, pause/continue/stop, retry
│   │   ├── ScraperOptions.go        # Config struct
│   │   ├── ScraperWorker.go         # Goroutine loop: Get → Download → extract → Save
│   │   ├── downloader.go            # HTTP client, UA rotation, per-host throttle
│   │   ├── extractor.go             # HTML → URL extraction + classification
│   │   ├── sitemap.go               # robots.txt + sitemap.xml parsing
│   │   └── Detect.go                # Filter detection for /api/archive/detect
│   └── server/
│       ├── server.go                # Server struct, mux, catch-all router
│       ├── handlers.go              # Archive API handlers (start/status/queue/...)
│       ├── middlewares/             # Recover, Log, ContentSecurityPolicy, Join
│       └── routes/
│           ├── api/                 # Search, Archives, Filters endpoints
│           └── zim/                 # Render + ZIM path helpers (IsPath, split_zim_path)
├── filters/                         # Content filter plugin system (used by scraper AND render)
│   ├── Filter.go                    # Filter interface + URLRewriter interface
│   ├── Registry.go                  # Global registry of available filters
│   ├── Get.go                       # Select filters by name
│   ├── Detect.go                    # Detect(default + content-based) → []names
│   ├── Sanitizer.go                 # HTML node sanitizer (shared by Scripts/Trackers)
│   ├── ApplyFilter*.go              # Run Detect + Filter* across a filter set
│   ├── ApplyRewriteURL.go           # Run RewriteURL across a filter set
│   ├── MediaWiki.go / PHPBB.go / VBulletin.go   # CMS-specific filters
│   ├── Scripts.go / Trackers.go     # Default filters (strip scripts / tracking)
│   └── Zim.go                       # Rewrites URLs to /<zimfile>/<host><path> render paths
├── utils/
│   ├── urls/                        # Canonicalize, GetMimeType
│   └── zim/                         # Render, ResolveEntry, resolve_page_url
├── public/                          # Static UI served by http.FileServer
│   ├── index.html                   # Search view (vanilla JS)
│   ├── archive.html                 # Archive management view
│   └── design/                      # CSS / images
├── samples/                         # Sample ZIM file for development
└── docs/                            # This document and related guides
```

## HTTP routing

All routing is in `internal/server/server.go` (`registerRoutes`). It uses Go 1.22+ `http.ServeMux` with method patterns, plus a single catch-all that dispatches on the request path.

```go
mux.HandleFunc("GET /", func(w, r) {
    if routes_zim.IsPath(r.URL.Path) {              // first segment ends in ".zim"
        if strings.HasSuffix(r.URL.Path, "/") {
            // directory entry → redirect to .../index.html
            http.Redirect(w, r, r.URL.Path + "index.html", http.StatusSeeOther)
        } else {
            routes_zim.Render(manager, w, r)
        }
    } else {
        fileServer.ServeHTTP(w, r)                  // public/ via http.FileServer
    }
})
```

More specific patterns win over `GET /`:

| Pattern | Handler |
|---|---|
| `GET /api/search` | `routes_api.Search` |
| `GET /api/archives` | `routes_api.Archives` |
| `GET /api/filters` | `routes_api.Filters` |
| `POST /api/archive/detect` | `handlers.handleArchiveDetect` |
| `POST /api/archive/start` | `handlers.handleArchiveStart` |
| `GET /api/archive/{hostname}/status` | `handlers.handleArchiveStatus` |
| `GET /api/archive/{hostname}/queue` | `handlers.handleArchiveQueue` |
| `POST /api/archive/{hostname}/pause` | `handlers.handleArchivePause` |
| `POST /api/archive/{hostname}/continue` | `handlers.handleArchiveContinue` |
| `POST /api/archive/{hostname}/stop` | `handlers.handleArchiveStop` |
| `POST /api/archive/{hostname}/build` | `handlers.handleArchiveBuild` |
| `POST /api/archive/{hostname}/retry` | `handlers.handleArchiveRetry` |

**ZIM path detection** (`internal/server/routes/zim/`): `IsPath` reports whether the first URL segment ends in `.zim`; `split_zim_path` splits a render URL like `/file.zim/host/path` into `(file.zim, host/path)`. `Render` uses `split_zim_path` and does not depend on ServeMux path values.

The static `public/` directory is located relative to the source file (`runtime.Caller`) so it works both when run from the repo root and under `go test` (which runs in the package directory).

## Request flows

### Search

```
GET /api/search?q=...&offset=0&limit=25
  → routes_api.Search (internal/server/routes/api/Search.go)
  → manager.Search(query, offset, limit)
  → gozim zim.Searcher (BM25 index over all loaded archives)
  → JSON: {query, offset, limit, estimated, results:[{url, id, file, path, title, score, snippet, word_count}]}
```

The `url` field is a render URL (`/<file>/<path-without-C/-prefix>`), constructed by stripping the `C/` prefix from the ZIM entry path.

### Render (browse archived content)

```
GET /<file>.zim/<host><path>
  → server catch-all → routes_zim.Render (internal/server/routes/zim/Render.go)
  → manager.Get(file.zim)
  → utils/zim.Render (utils/zim/Render.go)
      1. ResolveEntry(path)        # tries path variants: bare, C/, www/no-www, strip first segment
      2. entry.Item(true)          # follow ZIM redirects
      3. resolve_page_url          # reconstruct absolute page URL for relative-URL resolution
      4. apply filters:
         text/html → ApplyFilterHTML([Scripts, Trackers, Zim])
         text/css  → ApplyFilterCSS
         js        → ApplyFilterJS
         other     → raw bytes
  → Content-Type + Content-Length + body
```

`filters.Zim` (set with `File: zim_file`) rewrites `href`/`src`/`srcset`/`action`/`cite`/`longdesc`/`poster`/`data-*` and CSS `url()`/`@import` to `/ <zimfile>/<host><path>` render paths, and strips `javascript:` URLs. `filters.Scripts` removes `<script>`/`<iframe>`/`<object>`/`<embed>`/`<applet>` and event-handler attributes. `filters.Trackers` strips tracking `<meta>`/`<link>` elements and 1×1 tracking images.

### Scraping

```
POST /api/archive/start {url, respect_robots, page_workers, asset_workers, page_limit, filter_names, insecure_skip_verify}
  → handlers.handleArchiveStart
  → archive.NewScraper(ScraperOptions)   # load/create queue, robots.txt + sitemap seeds
  → go scraper.Start()                   # spawn page + asset workers
```

Worker loop (`ScraperWorker.Run`):

```
loop:
  queue.Get(type)                       # pending → downloading (page workers fall back to external-page)
  downloader.Download(entry)            # throttle, HTTP GET, save to cache, set status
  trackActivity(entry)
  queue.Set(entry)
  if page downloaded → extractAndEnqueue # parse HTML, classify + enqueue new URLs
  queue.Write()                          # persist JSON
```

See `scraping-workflow.md` for the full URL lifecycle and queue/state machines.

### Build ZIM

```
POST /api/archive/{hostname}/build
  → handlers.handleArchiveBuild → scraper.BuildZIM()
  → zimfs.Builder.Build(queue)
      1. collect status=downloaded entries
      2. gozim Writer: zstd compression, indexing, main path
      3. for each entry: entry.HTML() (applies FilterHTML at build time) → AddItem
      4. metadata (Title/Creator/Date/Language/Source/Description) → Finish
  → manager.Add(new zim file)
```

## Key data types

```go
// io/zimfs
type Manager struct { Folder string; archives map[string]*zim.Archive; searcher *zim.Searcher; mutex sync.RWMutex }
type Queue struct { Folder string; StartDate time.Time; StartURL *url.URL; Filters []filters.Filter; Status QueueStatus; entries []*QueueEntry; urls map[string]int }
type QueueEntry struct { WebURL, ZimURL *url.URL; MimeType string; Type QueueEntryType; Status QueueEntryStatus; Size int64; Referrer *url.URL; LastModified time.Time; StatusCode, Retries int }
type Builder struct { Folder string }
```

`QueueEntry` is serialized to JSON with `web_url`, `zim_url`, `referrer`, `last_modified` plus the scalar fields; the concrete `*url.URL` fields are reconstructed on `UnmarshalJSON`.

## Filter plugin system

`filters.Filter` is the shared interface used both when scraping (`Queue.Enqueue` applies `ApplyFilterURL`/`ApplyRewriteURL`) and when rendering (`utils/zim.Render` applies `ApplyFilterHTML/CSS/JS`).

```go
type Filter interface {
    Name() string
    Description() string
    IsDefault() bool
    Detect(*url.URL, []byte) bool
    FilterURL(*url.URL) *url.URL        // nil = skip URL
    FilterHTML(*url.URL, []byte) []byte
    FilterCSS(*url.URL, []byte) []byte
    FilterJS(*url.URL, []byte) []byte
}

type URLRewriter interface {            // optional
    RewriteURL(*url.URL) (*url.URL, *url.URL)  // (zimURL, webURL)
}
```

`filters.Registry` = `{ MediaWiki, PHPBB, VBulletin, Scripts, Trackers }`. `Scripts` and `Trackers` are defaults (`IsDefault() == true`). `filters.Zim` is not registered — it is constructed at render time with the current ZIM filename.

`filters.Detect` returns default filter names plus any non-default filters whose `Detect` matches the seed URL/content; `filters.Get(names)` selects a filter set by name.

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/cookiengineer/gozim/archive/zim` | ZIM read/write, BM25 full-text search, entry/metadata access |
| `golang.org/x/net/html` (+ `html/atom`) | HTML parsing and rendering (extractor, sanitizer, Zim rewrite) |
| `github.com/dop251/goja/parser` | JS parsing for the `Scripts` filter |
| Go stdlib | `net/http`, `net/url`, `encoding/json`, `context`, `sync`, etc. |

## Build, run, test

```sh
CGO_ENABLED=0 go build -o zimdex ./cmds/zimdex   # build
./zimdex --folder=./data --port=3000             # run (data dir + UI port)
go test ./...                                    # tests
go vet ./...                                     # vet
```

Dev server against the sample ZIM: `go run ./cmds/zimdex --folder=./samples --port=3000`, then open `http://localhost:3000`.

## Data directory & naming

The `--folder` argument points at a data directory shared by the queue, the download cache, and the built ZIM files.

```
<datadir>/
├── hostname-YYYY-MM-DD.zim       # built ZIM (-2, -3, … on filename collision)
├── hostname_YYYY-MM-DD.json      # download queue (dots in hostname → _)
└── hostname/                     # download cache, mirrors the URL path
    └── <url path>                # ? and & encoded as %3F / %26
```

| Item | Pattern | Example |
|---|---|---|
| ZIM entry path | `<hostname><path>` (no `C/` prefix) | `en.wikipedia.org/wiki/Main_Page` |
| Render URL | `/<zimfile>/<hostname><path>` | `/en.wikipedia.org-2026-08-02.zim/en.wikipedia.org/wiki/Main_Page` |

The cache is organized by hostname so cross-host assets and external pages live alongside the primary host's files. `QueueEntry.Path()` derives the cache path from `ZimURL` (or `WebURL`), encoding `?`/`&` for filesystem safety.

## Adaptive throttling

The downloader keeps a per-host `ThrottleState` (`internal/archive/downloader.go`):

- `MinDelay` 100ms, `MaxDelay` 30s, 15-minute sliding error window, threshold 10.
- `Wait()` — sleep until `CurrentDelay` has elapsed since the last request.
- `RecordSuccess()` — `delay *= 0.9`, floored at `MinDelay`.
- `RecordError()` — append timestamp; when errors in the window exceed the threshold, `delay *= 2` (capped at `MaxDelay`).
- `RecordRetryAfter()` — honor a `429` `Retry-After`.
- `SetCrawlDelay()` — seed from robots.txt `Crawl-Delay`.

The client also rotates 8 browser `User-Agent`s, sets content-type-aware `Accept` headers, and honors robots.txt `Disallow`/`Allow` via `RobotsMatcher`.

## API reference

Search and metadata:

- `GET /api/search?q=…&offset=0&limit=25` → `{query, offset, limit, estimated, results:[{url, id, file, path, title, score, snippet, word_count}]}` (400 if `q` empty).
- `GET /api/archives` → `{archives:[ArchiveInfo…]}`.
- `GET /api/filters` → `{filters:[{name, description, is_default}…]}`.

Archive jobs (all keyed by `{hostname}`):

```jsonc
// POST /api/archive/start
{ "url": "https://…", "respect_robots": true, "insecure_skip_verify": false,
  "page_limit": 0, "page_workers": 2, "asset_workers": 4, "filter_names": ["Scripts", "Trackers"] }
// → 202 { "host", "status": "running", "start_url" }

// GET /api/archive/{hostname}/status
{ "host", "status", "start_url",
  "stats": { "pending", "downloading", "downloaded", "failed", "skipped" },
  "recent_logs": ["…"], "recent_activity": [{ "time", "url", "status" }] }

// POST /api/archive/{hostname}/build → { "host", "zim_file": "host-YYYY-MM-DD.zim" }
// POST /api/archive/{hostname}/retry { "urls": ["…"] } → { "host", "retried": N }
```

Render/static: `GET /` (catch-all) — ZIM paths render or redirect to `index.html`; everything else is served from `public/`.

## Design decisions

| Decision | Rationale |
|---|---|
| Single catch-all `GET /` handler | Dispatch on `request.URL.Path`; no per-file static routes to maintain. |
| `io/zimfs` layer | ZIM + queue types with no networking; `internal/archive` and `internal/server` build on it. |
| Shared `filters.Filter` interface | One plugin system powers both scraping (URL filtering/rewriting) and rendering (HTML/CSS/JS). |
| Static UI in `public/` | Plain HTML + vanilla JS via `http.FileServer`; no template engine or build step. |
| JSON queue files | Human-readable, resumable, zero-dependency, cross-platform. |
| Final ZIM build step | Simpler than incremental; produces well-ordered clusters; re-runnable on the same cache. |
| Three queue entry types | `page` (full extraction), `external-page` (assets only, single-depth), `asset` (terminal). |
| Tracking params stripped, query strings kept | Remove `utm_*`/`fbclid`/etc.; preserve legitimate `?title=`-style queries (encoded on disk). |
| No C dependencies | `CGO_ENABLED=0` → static, portable binary. |

## Security

- **No script execution in render view**: `filters.Scripts` strips `<script>`/`<iframe>`/`<object>`/`<embed>`/`<applet>` and all `on*` attributes; the JS filter neutralizes known browser-spoofing functions.
- **Content-Security-Policy** on every response (`middlewares.ContentSecurityPolicy`): `default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'self'; form-action 'self'`.
- **No outbound requests from archived pages**: `filters.Zim` rewrites every URL to a render path, so the browser never contacts the origin server.
- **Origin isolation**: archived pages run on the ZIMdex origin; no cookies/localStorage leakage from the original domain.
- **Panic recovery** on every request (`middlewares.Recover`).

## Conventions

- One `import` per line; no comments unless required; tabs for indentation.
- Unexported names use `snake_case` (e.g. `split_zim_path`, `refresh_info`); exported names use `PascalCase`.
- `io/zimfs` has no HTTP/networking and no dependency on `internal/archive` — the scraping subsystem builds on `io/zimfs`.
- HTTP handlers keep a plain `if method == http.MethodGet { ... } else { 405 }` shape; content type and status are set explicitly.
