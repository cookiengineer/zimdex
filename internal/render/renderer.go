package render

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cookiengineer/gozim/archive/zim"
	"golang.org/x/net/html"
)

const maxRedirectHops = 5

var extMimeMap = map[string]string{
	".css":  "text/css",
	".js":   "application/javascript",
	".mjs":  "application/javascript",
	".html": "text/html",
	".htm":  "text/html",
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".ico":  "image/x-icon",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":  "font/ttf",
	".eot":  "application/vnd.ms-fontobject",
	".json": "application/json",
	".xml":  "application/xml",
	".txt":  "text/plain",
	".pdf":  "application/pdf",
}

func correctMimeType(mime string, path string) string {
	if mime != "" && mime != "application/octet-stream" {
		return mime
	}
	ext := strings.ToLower(filepath.Ext(path))
	if corrected, ok := extMimeMap[ext]; ok {
		return corrected
	}
	if mime != "" {
		return mime
	}
	return "application/octet-stream"
}

func Render(archive *zim.Archive, zimFile string, renderPath string, filters FilterConfig) ([]byte, string, error) {
	entry, err := resolveEntry(archive, renderPath)
	if err != nil {
		return nil, "", err
	}

	item, err := entry.Item(true)
	if err != nil {
		return nil, "", fmt.Errorf("item read failed: %w", err)
	}

	mimeType := correctMimeType(item.MimeType(), entry.Path())

	var data []byte
	if item.Size() > 100*1024*1024 {
		data, err = item.Data(0, 100*1024*1024)
	} else {
		data, err = item.DataAll()
	}
	if err != nil {
		return nil, "", fmt.Errorf("data read failed: %w", err)
	}

	pageURL := entryPageURL(entry, renderPath)

	rewriter, err := NewRewriter(zimFile, pageURL)
	if err != nil {
		return data, mimeType, nil
	}

	if strings.HasPrefix(mimeType, "text/html") {
		return renderHTML(data, rewriter, filters)
	}

	if strings.HasPrefix(mimeType, "text/css") {
		return renderCSS(data, rewriter)
	}

	return data, mimeType, nil
}

func resolveEntry(archive *zim.Archive, renderPath string) (*zim.Entry, error) {
	variants := pathVariants(renderPath)

	for _, variant := range variants {
		entry, err := archive.EntryByPath(variant)
		if err != nil {
			continue
		}

		for i := 0; i < maxRedirectHops && entry.IsRedirect(); i++ {
			redirectEntry, err := entry.RedirectEntry()
			if err != nil {
				return nil, fmt.Errorf("redirect resolution failed at %q: %w", variant, err)
			}
			entry = redirectEntry
		}

		if entry.IsRedirect() {
			return nil, fmt.Errorf("too many redirect hops for %q", variant)
		}

		return entry, nil
	}

	return nil, fmt.Errorf("entry not found: tried %v", variants)
}

func pathVariants(renderPath string) []string {
	variants := []string{renderPath}

	if !strings.HasPrefix(renderPath, "C/") {
		variants = append(variants, "C/"+renderPath)
	}

	idx := strings.Index(renderPath, "/")
	if idx > 0 {
		host := renderPath[:idx]
		suffix := renderPath[idx:]

		if !strings.HasPrefix(host, "www.") {
			variants = append(variants, "www."+host+suffix)
			variants = append(variants, "C/www."+host+suffix)
		} else {
			noWWW := strings.TrimPrefix(host, "www.")
			variants = append(variants, noWWW+suffix)
			variants = append(variants, "C/"+noWWW+suffix)
		}

		variants = append(variants, suffix[1:])
		variants = append(variants, "C/"+suffix[1:])
	}

	return variants
}

func entryPageURL(entry *zim.Entry, renderPath string) string {
	path := entry.Path()

	if strings.HasPrefix(path, "C/") {
		path = path[2:]
	}

	if strings.Contains(path, "://") {
		idx := strings.Index(path, "://")
		return "https://" + path[idx+3:]
	}

	idx := strings.Index(path, "/")
	if idx > 0 {
		return "https://" + path
	}

	idx = strings.Index(renderPath, "/")
	if idx > 0 {
		return "https://" + renderPath
	}

	return "https://" + renderPath
}

func renderHTML(data []byte, rewriter *Rewriter, filters FilterConfig) ([]byte, string, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return data, "text/html; charset=utf-8", nil
	}

	ApplyFilters(doc, filters)

	RewriteHTML(doc, rewriter)

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return data, "text/html; charset=utf-8", nil
	}

	return buf.Bytes(), "text/html; charset=utf-8", nil
}

func renderCSS(data []byte, rewriter *Rewriter) ([]byte, string, error) {
	css := string(data)
	rewritten := RewriteCSS(css, rewriter)
	return []byte(rewritten), "text/css; charset=utf-8", nil
}
