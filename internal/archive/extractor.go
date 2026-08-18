package archive

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"

	"github.com/cookiengineer/zimdex/internal/zimfs"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var cssURLRegex = regexp.MustCompile(`url\(\s*["']?(.*?)["']?\s*\)`)
var cssImportRegex = regexp.MustCompile(`@import\s+["'](.*?)["']`)

var knownFileExts = map[string]bool{
	".pdf": true, ".epub": true, ".zip": true, ".tar": true, ".gz": true,
	".bz2": true, ".xz": true, ".7z": true, ".rar": true,
	".mp3": true, ".mp4": true, ".ogg": true, ".ogv": true, ".webm": true,
	".avi": true, ".mov": true, ".wav": true, ".flac": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true,
	".webp": true, ".ico": true, ".bmp": true, ".tiff": true,
	".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true, ".odt": true, ".ods": true, ".odp": true,
}

type ExtractedURL struct {
	URL       *url.URL
	EntryType zimfs.QueueEntryType
}

type Extractor struct {
	baseURL     *url.URL
	primaryHost string
	referrer    *url.URL
	FollowPages bool
}

func NewExtractor(pageURL *url.URL, primaryHost string, referrer *url.URL) (*Extractor, error) {
	return &Extractor{
		baseURL:     pageURL,
		primaryHost: primaryHost,
		referrer:    referrer,
		FollowPages: true,
	}, nil
}

func NewExtractorAssetsOnly(pageURL *url.URL, primaryHost string, referrer *url.URL) (*Extractor, error) {
	ex, err := NewExtractor(pageURL, primaryHost, referrer)
	if err != nil {
		return nil, err
	}
	ex.FollowPages = false
	return ex, nil
}

func (ex *Extractor) Extract(htmlBody []byte) []ExtractedURL {
	doc, err := html.Parse(bytes.NewReader(htmlBody))
	if err != nil {
		return nil
	}

	var urls []ExtractedURL
	ex.walkNode(doc, &urls)
	return urls
}

func (ex *Extractor) walkNode(n *html.Node, urls *[]ExtractedURL) {
	if n.Type == html.ElementNode {
		ex.extractElement(n, urls)

		if n.Data == "style" || n.DataAtom == atom.Style {
			text := ex.getTextContent(n)
			if text != "" {
				ex.extractCSSURLs(text, urls)
			}
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		ex.walkNode(c, urls)
	}
}

func (ex *Extractor) extractElement(n *html.Node, urls *[]ExtractedURL) {
	tag := n.Data
	var href, src, srcset, poster string
	var rel, download string

	for _, attr := range n.Attr {
		switch strings.ToLower(attr.Key) {
		case "href":
			href = attr.Val
		case "src":
			src = attr.Val
		case "srcset":
			srcset = attr.Val
		case "poster":
			poster = attr.Val
		case "rel":
			rel = strings.ToLower(attr.Val)
		case "download":
			download = attr.Val
		}
	}

	switch tag {
	case "img":
		if src != "" {
			ex.addURL(src, zimfs.QueueEntryTypeAsset, urls)
		}
		if srcset != "" {
			for _, candidate := range strings.Split(srcset, ",") {
				parts := strings.Fields(strings.TrimSpace(candidate))
				if len(parts) > 0 {
					ex.addURL(parts[0], zimfs.QueueEntryTypeAsset, urls)
				}
			}
		}

	case "link":
		if rel == "stylesheet" && href != "" {
			ex.addURL(href, zimfs.QueueEntryTypeAsset, urls)
		}

	case "script":
		if src != "" {
			ex.addURL(src, zimfs.QueueEntryTypeAsset, urls)
		}

	case "source":
		if src != "" {
			ex.addURL(src, zimfs.QueueEntryTypeAsset, urls)
		}
		if srcset != "" {
			for _, candidate := range strings.Split(srcset, ",") {
				parts := strings.Fields(strings.TrimSpace(candidate))
				if len(parts) > 0 {
					ex.addURL(parts[0], zimfs.QueueEntryTypeAsset, urls)
				}
			}
		}

	case "video", "audio":
		if src != "" {
			ex.addURL(src, zimfs.QueueEntryTypeAsset, urls)
		}
		if poster != "" {
			ex.addURL(poster, zimfs.QueueEntryTypeAsset, urls)
		}

	case "track":
		if src != "" {
			ex.addURL(src, zimfs.QueueEntryTypeAsset, urls)
		}

	case "object":
		for _, attr := range n.Attr {
			if strings.ToLower(attr.Key) == "data" {
				ex.addURL(attr.Val, zimfs.QueueEntryTypeAsset, urls)
			}
		}

	case "embed":
		if src != "" {
			ex.addURL(src, zimfs.QueueEntryTypeAsset, urls)
		}

	case "a":
		if href == "" || strings.HasPrefix(href, "#") {
			return
		}
		if strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
			return
		}

		if download != "" || hasKnownFileExt(href) {
			ex.addURL(href, zimfs.QueueEntryTypeAsset, urls)
		} else if ex.FollowPages {
			resolved := ex.resolve(href)
			if resolved != nil {
				if resolved.Hostname() == ex.primaryHost {
					ex.addResolvedURL(resolved, zimfs.QueueEntryTypePage, urls)
				} else {
					ex.addResolvedURL(resolved, zimfs.QueueEntryTypeExternalPage, urls)
				}
			}
		}

	case "iframe":
		if src != "" && ex.FollowPages {
			resolved := ex.resolve(src)
			if resolved != nil {
				if resolved.Hostname() == ex.primaryHost {
					ex.addResolvedURL(resolved, zimfs.QueueEntryTypePage, urls)
				} else {
					ex.addResolvedURL(resolved, zimfs.QueueEntryTypeExternalPage, urls)
				}
			}
		}
	}
}

func (ex *Extractor) addURL(raw string, entryType zimfs.QueueEntryType, urls *[]ExtractedURL) {
	resolved := ex.resolve(raw)
	if resolved == nil {
		return
	}
	ex.addResolvedURL(resolved, entryType, urls)
}

func (ex *Extractor) addResolvedURL(resolved *url.URL, entryType zimfs.QueueEntryType, urls *[]ExtractedURL) {
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return
	}

	clone := *resolved
	clone.Fragment = ""
	clone.RawFragment = ""

	*urls = append(*urls, ExtractedURL{
		URL:       &clone,
		EntryType: entryType,
	})
}

func (ex *Extractor) resolve(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	return ex.baseURL.ResolveReference(parsed)
}

func (ex *Extractor) getTextContent(n *html.Node) string {
	var buf strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			buf.WriteString(c.Data)
		}
	}
	return buf.String()
}

func (ex *Extractor) extractCSSURLs(cssContent string, urls *[]ExtractedURL) {
	for _, match := range cssURLRegex.FindAllStringSubmatch(cssContent, -1) {
		if len(match) >= 2 && match[1] != "" {
			u := strings.TrimSpace(match[1])
			if !strings.HasPrefix(u, "data:") {
				ex.addURL(u, zimfs.QueueEntryTypeAsset, urls)
			}
		}
	}

	for _, match := range cssImportRegex.FindAllStringSubmatch(cssContent, -1) {
		if len(match) >= 2 && match[1] != "" {
			u := strings.TrimSpace(match[1])
			ex.addURL(u, zimfs.QueueEntryTypeAsset, urls)
		}
	}
}

func hasKnownFileExt(urlStr string) bool {
	idx := strings.Index(urlStr, "?")
	path := urlStr
	if idx != -1 {
		path = urlStr[:idx]
	}

	for ext := range knownFileExts {
		if strings.HasSuffix(strings.ToLower(path), ext) {
			return true
		}
	}
	return false
}
