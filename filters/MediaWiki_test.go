package filters

import (
	"net/url"
	"strings"
	"testing"
)

func TestMediaWikiName(t *testing.T) {
	filter := &MediaWiki{}
	if got := filter.Name(); got != "MediaWiki" {
		t.Errorf("Name() = %q, want %q", got, "MediaWiki")
	}
}

func TestMediaWikiDetect(t *testing.T) {
	filter := &MediaWiki{}

	if filter.Detect(nil, nil) {
		t.Error("Detect(nil, nil) = true, want false")
	}

	if filter.Detect(nil, []byte("<html></html>")) {
		t.Error("Detect(nil, plain html) = true, want false")
	}

	indexURL, _ := url.Parse("https://example.com/w/index.php?title=Main_Page")
	if !filter.Detect(indexURL, nil) {
		t.Error("Detect(index.php URL) = false, want true")
	}

	if !filter.Detect(nil, []byte(`<body class="mediawiki">`)) {
		t.Error("Detect(mediawiki body class) = false, want true")
	}

	if !filter.Detect(nil, []byte(`<meta name="generator" content="MediaWiki 1.39">`)) {
		t.Error("Detect(mediawiki meta generator) = false, want true")
	}
}

func TestMediaWikiFilterURLSkipNamespaces(t *testing.T) {
	filter := &MediaWiki{}

	for _, title := range []string{"Talk:Main_Page", "User:Alice", "Template:Infobox", "Special:RecentChanges"} {
		u, _ := url.Parse("https://example.com/index.php?title=" + title)
		if got := filter.FilterURL(u); got != nil {
			t.Errorf("FilterURL(title=%q) = %v, want nil (skip)", title, got)
		}
	}
}

func TestMediaWikiFilterURLSkipActions(t *testing.T) {
	filter := &MediaWiki{}

	u, _ := url.Parse("https://example.com/index.php?title=Main_Page&action=edit")
	if got := filter.FilterURL(u); got != nil {
		t.Errorf("FilterURL(action=edit) = %v, want nil (skip)", got)
	}
}

func TestMediaWikiFilterURLStripsParameters(t *testing.T) {
	filter := &MediaWiki{}

	u, _ := url.Parse("https://example.com/index.php?title=Main_Page&oldid=123&useskin=vector")
	got := filter.FilterURL(u)

	if got == nil {
		t.Fatal("FilterURL() returned nil")
	}
	if got.String() != "https://example.com/index.php?title=Main_Page" {
		t.Errorf("FilterURL() = %q, want %q", got.String(), "https://example.com/index.php?title=Main_Page")
	}
}

func TestMediaWikiFilterURLKeepsCleanURL(t *testing.T) {
	filter := &MediaWiki{}

	u, _ := url.Parse("https://example.com/index.php?title=Main_Page")
	got := filter.FilterURL(u)

	if got == nil {
		t.Fatal("FilterURL() returned nil")
	}
	if got.String() != u.String() {
		t.Errorf("FilterURL() = %q, want %q", got.String(), u.String())
	}
}

func TestMediaWikiFilterHTMLRewritesLinks(t *testing.T) {
	filter := &MediaWiki{}

	input := []byte(`<a href="/index.php?title=Main_Page">Main</a>`)
	got := string(filter.FilterHTML(nil, input))

	if strings.Contains(got, "index.php?title=") {
		t.Errorf("FilterHTML() did not rewrite link: %s", got)
	}
	if !strings.Contains(got, `href="/Main_Page.html"`) {
		t.Errorf("FilterHTML() = %q, want href=\"/Main_Page.html\"", got)
	}
}

func TestMediaWikiFilterHTMLStripsParameters(t *testing.T) {
	filter := &MediaWiki{}

	input := []byte(`<a href="/index.php?title=Main_Page&amp;oldid=123&amp;printable=yes">Main</a>`)
	got := string(filter.FilterHTML(nil, input))

	if strings.Contains(got, "oldid") || strings.Contains(got, "printable") {
		t.Errorf("FilterHTML() did not strip parameters: %s", got)
	}
	if !strings.Contains(got, `href="/Main_Page.html"`) {
		t.Errorf("FilterHTML() = %q, want href=\"/Main_Page.html\"", got)
	}
}

func TestMediaWikiRewriteURLFromIndexPHP(t *testing.T) {
	filter := &MediaWiki{}

	u, _ := url.Parse("https://example.com/index.php?title=Main_Page")
	zimURL, webURL := filter.RewriteURL(u)

	if zimURL == nil || webURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != "https://example.com/Main_Page.html" {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), "https://example.com/Main_Page.html")
	}
	if webURL.String() != u.String() {
		t.Errorf("RewriteURL() webURL = %q, want %q", webURL.String(), u.String())
	}
}

func TestMediaWikiRewriteURLFromHTML(t *testing.T) {
	filter := &MediaWiki{}

	u, _ := url.Parse("https://example.com/Main_Page.html")
	zimURL, webURL := filter.RewriteURL(u)

	if zimURL == nil || webURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != u.String() {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), u.String())
	}
	if webURL.String() != "https://example.com/index.php?title=Main+Page" {
		t.Errorf("RewriteURL() webURL = %q, want %q", webURL.String(), "https://example.com/index.php?title=Main+Page")
	}
}

func TestMediaWikiRewriteURLNil(t *testing.T) {
	filter := &MediaWiki{}

	zimURL, webURL := filter.RewriteURL(nil)
	if zimURL != nil || webURL != nil {
		t.Errorf("RewriteURL(nil) = %v, %v, want nil, nil", zimURL, webURL)
	}
}
