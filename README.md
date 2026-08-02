# ZIMdex

Offline web scraper and self-hosted search engine. Archives websites into [ZIM files](https://wiki.openzim.org/wiki/ZIM_file_format) and provides a local search interface over the archived content.

## Install

```sh
git clone https://github.com/cookiengineer/zimdex
cd zimdex
CGO_ENABLED=0 go build -o zimdex .
```

## Run

```sh
./zimdex --folder=./data --port=3000
```

Open `http://localhost:3000` in your browser.

## Usage

### Search

The landing page (`/index.html`) searches across all ZIM files in the data folder. Type a query to see results with snippets and links to the archived content.

### Archive a website

1. Go to `http://localhost:3000/archive.html`
2. Enter a seed URL (e.g., `https://buggedplanet.info/index.php?title=Main_Page`)
3. Enable filters as needed (tracking params, MediaWiki cleanup, script removal)
4. Check "Ignore invalid SSL certificates" if the site has a broken cert
5. Click **Start Scraping**
6. Monitor progress in the live activity feed
7. When complete, click **Build ZIM**
8. Search the archived content from the search page

### Filter plugins

Filters are selectable per-scrape in the archive UI:

| Filter | Effect |
|---|---|
| Tracking params | Strips `utm_*`, `fbclid`, `gclid`, and 40+ other tracking parameters from URLs |
| MediaWiki | Skips Talk/User/Special/Template pages; skips edit/history/delete actions; rewrites `?title=X` URLs to clean `X.html` paths |
| Strip scripts | Removes `<script>` tags from HTML before archiving |

### Browse archived content

Click any search result to view the archived page. All links and assets are rewritten to load from the local ZIM file — no internet connection needed.

## Data directory

```
data/
├── example.com-2026-08-02.zim    # Built ZIM archives
├── example.com.json              # Download queue (resumable)
└── example.com/                  # Download cache
    ├── index.html
    ├── logo.png
    └── style.css
```

## Browser integration

Add ZIMdex as a custom search engine in Firefox:
1. Right-click the address bar → **Add "ZIMdex"**
2. Or Settings → Search → Add search engine:
   - URL: `http://localhost:3000/api/search?q=%s`

## License

AGPL 3.0
