# ZIMdex — Scraping Workflow

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [URL Lifecycle](#url-lifecycle)
3. [Queue State Machine](#queue-state-machine)
4. [Scraper State Machine](#scraper-state-machine)
5. [Worker Lifecycle](#worker-lifecycle)
6. [Filter Plugin Architecture](#filter-plugin-architecture)
7. [Builder Pipeline](#builder-pipeline)
8. [Directory Structure](#directory-structure)
9. [Critical Design Rules](#critical-design-rules)

---

## Architecture Overview

```
User API (handlers.go)
    │
    ▼
Scraper (scraper.go)
    ├── Queue (queue.go)          — JSON-backed persistent download list
    ├── Downloader (downloader.go) — HTTP client with throttling, UA rotation, SSL skip
    ├── Extractor (extractor.go)   — HTML → URL extraction via golang.org/x/net/html
    ├── Filters (filters/)         — Plugin system for URL/HTML transformation
    ├── Sitemap (sitemap.go)       — XML sitemap parser + recursive fetcher
    ├── Robots  (sitemap.go)       — robots.txt parser with path matching
    └── Builder (builder.go)       — Cache → ZIM file via gozim Writer
```

---

## URL Lifecycle

A URL goes through these transformations from discovery to filesystem storage:

```
 1. RAW URL (from HTML, sitemap, or seed)
    │
    ▼
 2. CanonicalizeURL()     — lowercase host, strip fragment, strip default ports,
    │                       filter tracking params (via filters.FilterTrackingParams)
    ▼
 3. FilterURL()           — mediawiki: returns "" to skip action/namespace pages,
    │                       tracking: strips utm_*/fbclid/gclid/etc.
    ▼                       script: no-op (returns same URL)
 4. RewriteURL()          — mediawiki: transforms index.php?title=X → X.html
    │   (newURL,           as canonical URL, keeps original as downloadURL.
    │    downloadURL)       .html → index.php?title=X reverse mapping for
    │                       extracted URLs that were already rewritten in HTML.
    ▼
 5. URLToPath(newURL)     — produces filesystem path and ZIM path
    │   (hostname,            ? and & encoded as %3F/%26 for filesystem safety
    │    localPath,           ZIM path keeps raw ? and &
    │    zimPath)
    ▼
 6. QueueEntry
    ├── URL:         newURL (canonical, used for dedup via urlIndex map)
    ├── DownloadURL: downloadURL (actual URL to fetch from server, if different)
    ├── Path:        localPath (filesystem cache location)
    ├── ZimPath:     zimPath (entry path in ZIM file)
     └── Status:      pending (or downloaded if already cached on disk)

 7. Downloader uses DownloadURL (or URL if DownloadURL empty) for HTTP GET

 8. Response saved to dataDir/Path (filesystem cache)

 9. Builder reads all Status=downloaded entries from cache:
    for each entry:
        data = os.ReadFile(dataDir/entry.Path)
        zim.NewBytesItem(entry.ZimPath, entry.MimeType, title, data)
```

### Key rule: DownloadURL vs URL

| Field | Purpose | Example |
|---|---|---|
| `URL` | Canonical identity, dedup key, displayed in UI | `https://buggedplanet.info/Main_Page.html` |
| `DownloadURL` | Actual HTTP request URL | `https://buggedplanet.info/index.php?title=Main_Page` |
| `Path` | Filesystem cache path | `buggedplanet.info/Main_Page.html` |
| `ZimPath` | Entry path in ZIM file | `buggedplanet.info/Main_Page.html` |

The downloader MUST use `DownloadURL` if set, otherwise `URL`. The `URL` field is NEVER used directly for HTTP requests when `DownloadURL` differs — this was bug #1.

---

## Queue State Machine

Each `QueueEntry` transitions through these states:

```
               ┌──────────┐
     Add() ──► │ pending  │  (or directly → downloaded if cached on disk)
               └────┬─────┘
                    │ PopPending()
               ┌────▼───────┐
               │ downloading │◄──────────────┐
               └────┬───────┘                │
                    │                        │
           ┌────────┼────────┐               │
           ▼        ▼        ▼               │
     downloaded   failed   (retry)           │
           │        │        │               │
           │        │        └── ErrRetry ───┘
           │        │        (worker sets Status=StatusPending)
           │        │
           ▼        ▼
        [final]  [max retries exceeded → final]
```

**Critical rules:**
- Only `pending` entries are returned by `PopPending()`
- `PopPending()` sets status to `downloading` and adjusts stats
- If download fails with `ErrRetry`, the worker MUST set status back to `pending` so it's retried (was bug #2)
- If `retry_count > maxRetries` (3), status becomes `failed` permanently
- `ResetStale()` on scraper restart resets `failed` + `downloading` → `pending` (retry_count = 0)
- Downloaded entries are NEVER reset — they survive restarts

---

## Scraper State Machine

```
              ┌──────────┐
              │   idle   │
              └────┬─────┘
                   │ Start()
              ┌────▼──────┐
       ┌─────│  running  │◄─────┐
       │     └────┬──────┘      │
       │ Pause()  │   Stop()    │ Continue()
       │          ▼             │
       │     ┌──────────┐       │
       └────►│  paused  ├───────┘
             └────┬─────┘
                  │ Stop()
             ┌────▼──────┐
             │  stopped  │
             └───────────┘

  Workers auto-transition to "complete" when:
    PendingCount == 0 AND DownloadingCount == 0
    (both MUST be zero — was bug #3 where only PendingCount was checked)
```

---

## Worker Lifecycle

```
worker(ctx, entryType):
  loop:
    1. Check ctx.Done() → return (shutdown signal)
    2. Check scraper.status != "running" → return (paused/stopped)
    3. entry = queue.PopPending(entryType)
    4. if entry == nil:
         if PendingCount == 0 AND DownloadingCount == 0:
           set status = "complete", save queue, return
         sleep 500ms, continue
    5. downloader.Download(ctx, entry)
       - Uses DownloadURL if set, otherwise URL
       - On success: sets entry.Status = Downloaded, saves to cache
       - On 3xx redirect: returns ErrRedirect → worker enqueues redirect URL
       - On retryable error: returns ErrRetry → worker sets Status = Pending
       - On fatal error: sets Status = Failed
    6. trackActivity(entry) — records in ring buffer for UI
    7. RecalcStats() — syncs queue stats with actual entry statuses
    8. If page AND downloaded:
         extractAndEnqueue(entry):
           a. Read ORIGINAL HTML from cache (not patched)
           b. Extract URLs from original HTML
           c. For each URL:
              - FilterURL (skip if "", keep otherwise)
              - RewriteURL (get newURL + downloadURL)
              - robots.IsAllowed check
              - URLToPath(newURL) → localPath, zimPath
              - queue.Add(entry) — dedup via urlIndex
    9. queue.Save() — atomic write to JSON
```

### Why FilterHTML is NOT applied during extraction

The cache must preserve the ORIGINAL HTML from the server. If `FilterHTML` rewrites
links before `FilterURL` runs, namespace/skip pages get transformed:

```
Original:  index.php?title=Talk:MS
FilterHTML rewrites:  Talk:MS.html
FilterURL checks Talk:MS.html → not a known namespace → keeps it
Entry enqueued with Talk:MS.html → 404 on download
```

Correct flow with FilterHTML only at build time:

```
Scraping phase:
  1. Download index.php?title=Main_Page → cache = original HTML
  2. Extract URLs from original HTML → index.php?title=Talk:MS
  3. FilterURL checks Talk namespace → returns "" → SKIP
  4. Talk:MS never enqueued, never downloaded ✓

Build phase:
  1. Read original HTML from cache (contains index.php?title=Main_Page links)
  2. FilterHTML rewrites links: index.php?title=Main_Page → Main_Page.html
  3. Patched HTML written to ZIM
  4. ZIM has clean HTML with rewritten links ✓
```

---

## Filter Plugin Architecture

```
filters/
├── filter.go      — Filter interface, URLRewriter interface, Registry
├── tracking.go    — TrackingFilter (strips utm_*, fbclid, gclid, ref, etc.)
├── mediawiki.go   — MediaWikiFilter (detects MW pages, rewrites paths, patches HTML)
└── script.go      — ScriptFilter (strips <script> tags from HTML)
```

### Filter Interface

```go
type Filter interface {
    Name() string
    Description() string
    Detect(html []byte, pageURL string) bool   // does filter apply?
    FilterURL(rawURL string) string            // returns "" to skip, or filtered URL
    FilterHTML(html []byte, pageURL string) []byte  // transform cached HTML
}
```

### URLRewriter Interface (optional, filters that implement it)

```go
type URLRewriter interface {
    RewriteURL(rawURL string) (newURL, downloadURL string)
}
```

### Filter execution order during extraction

```
For each extracted URL:
  1. FilterURL(rawURL)          — all enabled filters
     Returns "" → skip entirely
     Returns url → continue

  2. RewriteURL(filteredURL)    — all enabled filters that implement URLRewriter
     Returns (newURL, downloadURL)
     newURL = canonical URL for dedup + path
     downloadURL = actual URL for HTTP request (defaults to newURL)

  3. robots.IsAllowed(url)
  4. URLToPath(newURL) → localPath, zimPath
  5. queue.Add(QueueEntry{URL: newURL, DownloadURL: downloadURL, ...})
```

### Filter execution during seed/sitemap

```
For each seed URL:
  1. Detect(nil, seedURL)       — content-based filters use URL patterns
  2. FilterURL(seedURL)         — skip if ""
  3. RewriteURL(filteredURL)    — transform path
  4. URLToPath(newURL)
  5. queue.Add(...)
```

### MediaWiki filter — full transformation table

| Input URL | FilterURL | newURL | downloadURL | HTML rewrite |
|---|---|---|---|---|
| `index.php?title=Main_Page` | keep | `Main_Page.html` | `index.php?title=Main_Page` | `href="/Main_Page.html"` |
| `index.php?title=Main_Page&oldid=123` | `index.php?title=Main_Page` | `Main_Page.html` | `index.php?title=Main_Page` | `href="/Main_Page.html"` |
| `index.php?title=Talk:Main_Page` | `""` (skip) | — | — | — |
| `index.php?title=Main_Page&action=edit` | `""` (skip) | — | — | — |
| `Main_Page.html` (from HTML) | keep | `Main_Page.html` | `index.php?title=Main_Page` | — |
| `images/0/02/file.pdf` | keep | same | same | — |
| `load.php?lang=en&modules=...` | keep | same | same | — |

---

## Builder Pipeline

```
BuildZIM(dataDir, queue, filterNames):
  1. Get all Status=downloaded entries
  2. Generate filename: {host}-{YYYY-MM-DD}.zim (append -2, -3 if exists)
  3. Create gozim Writer:
     - CompressionZstd
     - SetIndexing(true, "eng")
     - SetMainPath(firstEntry.ZimPath)
  4. For each downloaded entry:
     - Read file from dataDir/entry.Path (ORIGINAL content from cache)
     - Apply FilterHTML to the data (rewrite links, strip scripts, etc.)
     - Extract <title> from HTML pages
     - w.AddItem(zim.NewBytesItem(entry.ZimPath, mimeType, title, PATCHED_data))
  5. Add metadata: Title, Creator="ZIMdex", Date, Language="eng", Source, Description
  6. w.Finish() → closes and finalizes ZIM file
  7. Trigger manager.Reload() to pick up new ZIM
```

Note: Entries span both the primary host and any CDN/external hosts. All are bundled into one ZIM with their hostname-based paths.

---

## Directory Structure

```
<datadir>/                          # --folder CLI arg
│
├── hostname-YYYY-MM-DD.zim         # Built ZIM file
├── hostname-YYYY-MM-DD-2.zim       # Subsequent build (if file exists)
│
├── hostname.json                   # Download queue (atomic write via .tmp rename)
├── hostname/                       # Filesystem cache mirroring URL paths
│   ├── index.php%3Ftitle=...       # URL-encoded paths for filesystem safety
│   ├── Main_Page.html              # MediaWiki-rewritten path
│   ├── images/
│   │   └── 0/02/file.pdf
│   └── ...
│
├── cdn.example.com.json            # Spanned host queue (if cross-host assets)
└── cdn.example.com/                # Spanned host cache
    └── assets/logo.png
```

---

## Critical Design Rules

1. **DownloadURL is authoritative for HTTP**: The downloader MUST use `entry.DownloadURL` if set, never `entry.URL`. `entry.URL` is the canonical identity for dedup only.

2. **Reset pending on retry**: After `ErrRetry`, the worker MUST set `entry.Status = StatusPending` so `PopPending` can pick it up again.

3. **Both counts for completion**: Workers check `PendingCount == 0 AND DownloadingCount == 0` before declaring "complete". Checking only one causes premature exit.

4. **RecalcStats after every download**: The downloader sets entry status directly, bypassing `queue.UpdateEntry()`. The worker calls `queue.RecalcStats()` to sync stats.

5. **FilterHTML only at build time**: HTML must NOT be patched during scraping. The cache preserves the original server response. FilterHTML runs during the ZIM build step, after all URL extraction/filtering is complete. This ensures FilterURL sees unmodified URLs and can correctly detect and skip namespace/action pages.

6. **RewriteURL after FilterURL**: FilterURL decides keep/skip on the ORIGINAL URL. RewriteURL transforms the kept URL for path generation. Both run on every kept URL during extraction. FilterHTML runs during build, not extraction.

7. **Seed URLs need content-based filter detection too**: `filterSeed` must pass the URL to `Detect()` so content-based filters (MediaWiki) can recognize URL patterns without HTML.

8. **Queue stats must be consistent**: `PopPending` adjusts stats (pending--, downloading++). `Downloader` sets entry status directly. `RecalcStats()` reconciles.

9. **Atomic queue saves**: Write to `.tmp` file, then `os.Rename` to avoid corruption on crash.

10. **No `C/` prefix in ZIM paths**: The namespace is a separate dirent field. Entry paths should NOT include the `C/` prefix.

11. **Disk cache detection on Add()**: `queue.Add()` checks if a file already exists at `dataDir/entry.Path` before enqueuing as pending. If the file exists with content (>0 bytes), the entry is added as `downloaded` directly — bypassing the entire download pipeline. On resume/restart, this allows previously downloaded entries to be recognized and the queue to be reconstructed from the filesystem cache.
