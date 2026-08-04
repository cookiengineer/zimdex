package filters

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

type MediaWikiFilter struct{}

func (f *MediaWikiFilter) Name() string        { return "mediawiki" }
func (f *MediaWikiFilter) Description() string { return "Filter MediaWiki version control links and rewrite URLs to clean paths" }

func (f *MediaWikiFilter) Detect(htmlBody []byte, pageURL *url.URL) bool {
	if len(htmlBody) > 0 {
		if bytes.Contains(htmlBody, []byte(`class="mediawiki"`)) {
			return true
		}
		if bytes.Contains(htmlBody, []byte(`body class="mediawiki`)) {
			return true
		}
		doc, err := html.Parse(bytes.NewReader(htmlBody))
		if err == nil && findMetaGenerator(doc, "mediawiki") {
			return true
		}
		return false
	}
	if pageURL != nil {
		return strings.Contains(pageURL.String(), "index.php?title=")
	}
	return false
}

func findMetaGenerator(n *html.Node, keyword string) bool {
	if n.Type == html.ElementNode && n.Data == "meta" {
		var name, content string
		for _, attr := range n.Attr {
			switch strings.ToLower(attr.Key) {
			case "name":
				name = strings.ToLower(attr.Val)
			case "content":
				content = strings.ToLower(attr.Val)
			}
		}
		if name == "generator" && strings.Contains(content, keyword) {
			return true
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if findMetaGenerator(c, keyword) {
			return true
		}
	}
	return false
}

var mediaWikiSkipActions = map[string]bool{
	"edit": true, "history": true, "submit": true, "delete": true,
	"protect": true, "purge": true, "info": true,
}

var mediaWikiSkipNamespaces = []string{
	"Talk:", "User:", "User_talk:", "Template:", "Category:",
	"Special:", "File:", "MediaWiki:", "Help:",
}

func (f *MediaWikiFilter) FilterURL(rawURL *url.URL) *url.URL {
	if rawURL == nil {
		return nil
	}

	q := rawURL.Query()
	title := q.Get("title")
	action := q.Get("action")

	for _, ns := range mediaWikiSkipNamespaces {
		if title != "" && strings.HasPrefix(title, ns) {
			return nil
		}
	}
	if mediaWikiSkipActions[action] {
		return nil
	}

	changed := false
	if q.Has("oldid") || q.Has("printable") || q.Has("veaction") || q.Has("useskin") || q.Has("returnto") {
		q.Del("oldid")
		q.Del("printable")
		q.Del("veaction")
		q.Del("useskin")
		q.Del("returnto")
		changed = true
	}
	if changed {
		clone := *rawURL
		clone.RawQuery = q.Encode()
		return &clone
	}

	return rawURL
}

func (f *MediaWikiFilter) RewriteURL(rawURL *url.URL) (newURL, downloadURL *url.URL) {
	if rawURL == nil {
		return nil, nil
	}

	rawStr := rawURL.String()

	if strings.Contains(rawStr, "index.php?title=") {
		q := rawURL.Query()
		title := q.Get("title")
		if title == "" {
			return rawURL, rawURL
		}

		newPath := "/" + strings.ReplaceAll(title, " ", "_") + ".html"

		newU := *rawURL
		newU.RawQuery = ""
		newU.Path = newPath
		return &newU, rawURL
	}

	if strings.HasSuffix(rawURL.Path, ".html") {
		title := strings.TrimSuffix(strings.TrimPrefix(rawURL.Path, "/"), ".html")
		title = strings.ReplaceAll(title, "_", " ")

		dlU := *rawURL
		dlU.Path = "/index.php"
		dlU.RawQuery = "title=" + url.QueryEscape(title)
		return rawURL, &dlU
	}

	return rawURL, rawURL
}

var mwOldidRegex = regexp.MustCompile(`(&amp;|&)oldid=\d+`)
var mwPrintableRegex = regexp.MustCompile(`(&amp;|&)printable=yes`)
var mwVeactionRegex = regexp.MustCompile(`(&amp;|&)veaction=\w+`)
var mwUseSkinRegex = regexp.MustCompile(`(&amp;|&)useskin=\w+`)
var mwReturntoRegex = regexp.MustCompile(`(&amp;|&)returnto=\w+`)

var mwLinkRegex = regexp.MustCompile(`(href|src)=["']/?index\.php\?title=([^"'\s&]+)(&amp;[^"'\s]*)?["']`)

func (f *MediaWikiFilter) FilterHTML(htmlBody []byte, _ *url.URL) []byte {
	result := mwOldidRegex.ReplaceAll(htmlBody, []byte{})
	result = mwPrintableRegex.ReplaceAll(result, []byte{})
	result = mwVeactionRegex.ReplaceAll(result, []byte{})
	result = mwUseSkinRegex.ReplaceAll(result, []byte{})
	result = mwReturntoRegex.ReplaceAll(result, []byte{})

	result = mwLinkRegex.ReplaceAllFunc(result, func(b []byte) []byte {
		parts := mwLinkRegex.FindSubmatch(b)
		if len(parts) < 3 {
			return b
		}
		attr := string(parts[1])
		title := string(parts[2])

		decoded := strings.ReplaceAll(title, "_", " ")
		decoded, _ = url.QueryUnescape(decoded)
		newPath := "/" + strings.ReplaceAll(decoded, " ", "_") + ".html"

		return []byte(attr + `="` + newPath + `"`)
	})

	return result
}
