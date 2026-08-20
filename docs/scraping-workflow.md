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
    │                       filter tracking params (via filters.FilterTrackers)
    ▼
 3. FilterURL()           — mediawiki: returns nil to skip action/namespace pages,
    │                       phpbb: skips memberlist/posting/ucp/download/post
    │                       permalinks; tracking: strips utm_*/fbclid/gclid/etc.
    ▼                       script: no-op (returns same URL)
 4. RewriteURL()          — mediawiki: transforms index.php?title=X → X.html
    │   (zimURL,           as canonical path, keeps original as the download URL.
    │    webURL)            .html → index.php?title=X reverse mapping for
    │                       URLs that were already rewritten in HTML.
    ▼
 5. QueueEntry.Path()     — derives the filesystem cache path and ZIM path
    │   (hostname,            ? and & encoded as %3F/%26 for filesystem safety
    │    localPath,           ZIM path keeps raw ? and &
    │    zimPath)
    ▼
 6. QueueEntry
    ├── WebURL:      webURL (actual URL to fetch; also the dedup key)
    ├── ZimURL:      zimURL (clean path for filesystem/ZIM; may be nil)
    ├── MimeType:    resolved from webURL extension or response header
     └── Status:      pending (or downloaded if already cached on disk)

 7. Downloader uses entry.WebURL for HTTP GET

 8. Response saved to dataDir/entry.Path() (filesystem cache)

 9. Builder reads all Status=downloaded entries from cache:
    for each entry:
        html = entry.HTML()  (applies FilterHTML at build time only)
        zim.NewBytesItem(entry.Path(), entry.MimeType, title, html)
```

### Key rule: WebURL vs ZimURL

| Field | Purpose | Example |
|---|---|---|
| `WebURL` | Actual HTTP request URL, dedup key | `https://buggedplanet.info/index.php?title=Main_Page` |
| `ZimURL` | Clean path for filesystem/ZIM storage | `https://buggedplanet.info/Main_Page.html` |
| `Path()` | Filesystem cache + ZIM path (uses ZimURL if set, else WebURL) | `buggedplanet.info/Main_Page.html` |

The downloader MUST use `WebURL` for HTTP requests. `ZimURL` is only a storage-path alias produced by `RewriteURL` and is never fetched directly — this was bug #1.

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
       Page workers also try PopPending(ExternalPage) if no primary pages available
    4. if entry == nil:
         if PendingCount == 0 AND DownloadingCount == 0:
           set status = "complete", save queue, return
         sleep 500ms, continue
     5. downloader.Download(ctx, entry)
       - Uses entry.WebURL for the HTTP GET
       - On success: sets entry.Status = Downloaded, saves to cache
       - On 3xx redirect: returns ErrRedirect → worker enqueues redirect URL
       - On retryable error: returns ErrRetry → worker sets Status = Pending
       - On fatal error: sets Status = Failed
    6. trackActivity(entry) — records in ring buffer for UI
    7. RecalcStats() — syncs queue stats with actual entry statuses
    8. If page (EntryTypePage or EntryTypeExternalPage) AND downloaded:
         extractAndEnqueue(entry):
           a. Read ORIGINAL HTML from cache (not patched)
           b. Create Extractor:
              - Primary host pages: NewExtractor (FollowPages=true)
                → extracts assets + same-host page links + external page links
              - External host pages: NewExtractorAssetsOnly (FollowPages=false)
                → extracts assets only (no page link following)
           c. For each URL:
               - FilterURL (skip if nil, keep otherwise)
               - RewriteURL (get zimURL + webURL)
               - robots.IsAllowed check
               - queue.EnqueueURL(rawURL, html, referrer, type) — dedup via urlIndex
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
├── Filter.go         — Filter interface, URLRewriter interface, Registry
├── ApplyFilterURL.go — runs Detect + FilterURL across enabled filters
├── ApplyRewriteURL.go— runs RewriteURL across enabled filters
├── MediaWiki.go      — MediaWikiFilter (detects MW pages, rewrites paths, patches HTML)
├── PHPBB.go          — PHPBBFilter (detects phpBB pages, rewrites view*/search paths)
├── Scripts.go        — ScriptFilter (strips <script> tags from HTML)
├── Trackers.go       — TrackingFilter (strips utm_*, fbclid, gclid, ref, etc.)
└── Sanitizer.go      — HTML node sanitizer used by Scripts/Trackers
```

### Filter Interface

```go
type Filter interface {
    Name() string
    Description() string
    Detect(*url.URL, []byte) bool               // does filter apply?
    FilterURL(*url.URL) *url.URL                // returns nil to skip, or filtered URL
    FilterHTML(*url.URL, []byte) []byte         // transform cached HTML
}
```

### URLRewriter Interface (optional, filters that implement it)

```go
type URLRewriter interface {
    RewriteURL(*url.URL) (*url.URL, *url.URL)   // returns (zimURL, webURL)
}
```

### Filter execution order during extraction

```
For each extracted URL:
  1. FilterURL(rawURL)          — all enabled filters
     Returns nil → skip entirely
     Returns url  → continue (e.g. sid stripped)

  2. RewriteURL(filteredURL)    — all enabled filters that implement URLRewriter
     Returns (zimURL, webURL)
     zimURL = clean path for filesystem/ZIM storage
     webURL = actual URL for HTTP request (defaults to filteredURL if nil)

  3. robots.IsAllowed(url)
  4. queue.EnqueueURL(rawURL, html, referrer, type)
     → QueueEntry{WebURL: webURL, ZimURL: zimURL, ...}
```

### Filter execution during seed/sitemap

```
For each seed URL:
  1. queue.EnqueueURL(seedURL, nil, seedURL, Page)
  2. FilterURL(seedURL)         — skip if nil
  3. RewriteURL(filteredURL)    — transform path
  4. queue.Add(entry)
```

### MediaWiki filter — full transformation table

| Input URL | FilterURL | zimURL | webURL | HTML rewrite |
|---|---|---|---|---|
| `index.php?title=Main_Page` | keep | `Main_Page.html` | `index.php?title=Main_Page` | `href="/Main_Page.html"` |
| `index.php?title=Main_Page&oldid=123` | `index.php?title=Main_Page` | `Main_Page.html` | `index.php?title=Main_Page` | `href="/Main_Page.html"` |
| `index.php?title=Talk:Main_Page` | `nil` (skip) | — | — | — |
| `index.php?title=Main_Page&action=edit` | `nil` (skip) | — | — | — |
| `Main_Page.html` (from HTML) | keep | `Main_Page.html` | `index.php?title=Main_Page` | — |
| `images/0/02/file.pdf` | keep | same | same | — |
| `load.php?lang=en&modules=...` | keep | same | same | — |

### PHPBB filter — full transformation table

| Input URL | FilterURL | zimURL | webURL | HTML rewrite |
|---|---|---|---|---|
| `viewforum.php?f=1` | keep | `forum/1.html` | `viewforum.php?f=1` | `href="/forum/1.html"` |
| `viewforum.php?f=1&start=50` | keep | `forum/1-50.html` | `viewforum.php?f=1&start=50` | `href="/forum/1-50.html"` |
| `viewtopic.php?t=21&sid=...` | `viewtopic.php?t=21` | `topic/21.html` | `viewtopic.php?t=21` | `href="/topic/21.html"` |
| `viewtopic.php?t=21&start=145` | keep | `topic/21-145.html` | `viewtopic.php?t=21&start=145` | `href="/topic/21-145.html"` |
| `viewtopic.php?p=201#p201` | `nil` (skip) | — | — | `#p201` (on topic pages) |
| `search.php?search_id=unanswered` | keep | `search/unanswered.html` | `search.php?search_id=unanswered` | `href="/search/unanswered.html"` |
| `search.php?author_id=26&sr=posts` | keep | `search/author/26.html` | `search.php?author_id=26&sr=posts` | `href="/search/author/26.html"` |
| `memberlist.php?mode=viewprofile&u=26` | `nil` (skip) | — | — | sid stripped |
| `posting.php?mode=reply&t=21` | `nil` (skip) | — | — | sid stripped |
| `download/file.php?id=12345` | `nil` (skip) | — | — | sid stripped |
| `index.php` | keep (sid stripped) | same | same | `href="./index.php"` |

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
├── hostname.json                   # Download queue for the entire scrape job
│                                   # Contains all entries: primary host pages,
│                                   # assets from any host, and external pages
│
├── hostname/                       # Primary host's filesystem cache
│   ├── index.php%3Ftitle=...       # URL-encoded paths for filesystem safety
│   ├── Main_Page.html              # MediaWiki-rewritten path
│   ├── images/
│   │   └── 0/02/file.pdf
│   └── ...
│
├── cdn.example.com/                # Cross-host asset cache (within same queue)
│   └── assets/logo.png
│
└── external-site.org/              # External page cache (within same queue)
    └── article.html
```

---

## Critical Design Rules

1. **WebURL is authoritative for HTTP**: The downloader MUST use `entry.WebURL` for the HTTP GET, never `entry.ZimURL`. `ZimURL` is only a storage-path alias produced by `RewriteURL` and is never fetched directly.

2. **Reset pending on retry**: After `ErrRetry`, the worker MUST set `entry.Status = StatusPending` so `PopPending` can pick it up again.

3. **Both counts for completion**: Workers check `PendingCount == 0 AND DownloadingCount == 0` before declaring "complete". Checking only one causes premature exit.

4. **RecalcStats after every download**: The downloader sets entry status directly, bypassing `queue.UpdateEntry()`. The worker calls `queue.RecalcStats()` to sync stats.

5. **FilterHTML only at build time**: HTML must NOT be patched during scraping. The cache preserves the original server response. FilterHTML runs during the ZIM build step, after all URL extraction/filtering is complete. This ensures FilterURL sees unmodified URLs and can correctly detect and skip namespace/action pages.

6. **RewriteURL after FilterURL**: FilterURL decides keep/skip on the ORIGINAL URL. RewriteURL transforms the kept URL for path generation. Both run on every kept URL during extraction. FilterHTML runs during build, not extraction.

7. **Seed URLs need URL-based detection too**: `queue.EnqueueURL` passes the seed URL as the referrer to `ApplyFilterURL`, so content-based filters (MediaWiki, PHPBB) fall back to URL patterns when no HTML is available.

8. **Queue stats must be consistent**: `PopPending` adjusts stats (pending--, downloading++). `Downloader` sets entry status directly. `RecalcStats()` reconciles.

9. **Atomic queue saves**: Write to `.tmp` file, then `os.Rename` to avoid corruption on crash.

10. **No `C/` prefix in ZIM paths**: The namespace is a separate dirent field. Entry paths should NOT include the `C/` prefix.

11. **Disk cache detection on Add()**: `queue.Add()` checks if a file already exists at `dataDir/entry.Path` before enqueuing as pending. If the file exists with content (>0 bytes), the entry is added as `downloaded` directly — bypassing the entire download pipeline. On resume/restart, this allows previously downloaded entries to be recognized and the queue to be reconstructed from the filesystem cache.

12. **External page extraction is assets-only**: When an external page (different hostname from the primary scrape target) is downloaded, its assets (images, CSS, JS, etc.) are extracted via `NewExtractorAssetsOnly` (FollowPages=false). Its own page links are NOT followed — external pages are single-depth only. This preserves linked articles (e.g., from news sites or wiki references) without unbounded crawling. External `<a>` and `<iframe>` links from primary host pages are classified as `EntryTypeExternalPage` rather than being discarded.
