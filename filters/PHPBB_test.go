package filters

import (
	"net/url"
	"strings"
	"testing"
)

func TestPHPBBName(t *testing.T) {
	filter := &PHPBB{}
	if got := filter.Name(); got != "PHPBB" {
		t.Errorf("Name() = %q, want %q", got, "PHPBB")
	}
}

func TestPHPBBDetect(t *testing.T) {
	filter := &PHPBB{}

	if filter.Detect(nil, nil) {
		t.Error("Detect(nil, nil) = true, want false")
	}

	if filter.Detect(nil, []byte("<html></html>")) {
		t.Error("Detect(nil, plain html) = true, want false")
	}

	viewforumURL, _ := url.Parse("https://pckf.com/viewforum.php?f=1")
	if !filter.Detect(viewforumURL, nil) {
		t.Error("Detect(viewforum.php URL) = false, want true")
	}

	if !filter.Detect(nil, []byte(`<body id="phpbb" class="nojs notouch section-viewforum ltr">`)) {
		t.Error("Detect(phpbb body id) = false, want true")
	}

	if !filter.Detect(nil, []byte(`phpBB style name: prosilver`)) {
		t.Error("Detect(phpbb style comment) = false, want true")
	}
}

func TestPHPBBFilterURLSkipScripts(t *testing.T) {
	filter := &PHPBB{}

	for _, raw := range []string{
		"https://pckf.com/memberlist.php?mode=viewprofile&u=26",
		"https://pckf.com/posting.php?mode=reply&t=21",
		"https://pckf.com/ucp.php?mode=login",
		"https://pckf.com/download/file.php?id=12345",
	} {
		u, _ := url.Parse(raw)
		if got := filter.FilterURL(u); got != nil {
			t.Errorf("FilterURL(%q) = %v, want nil (skip)", raw, got)
		}
	}
}

func TestPHPBBFilterURLSkipPostPermalink(t *testing.T) {
	filter := &PHPBB{}

	u, _ := url.Parse("https://pckf.com/viewtopic.php?p=201&sid=abc")
	if got := filter.FilterURL(u); got != nil {
		t.Errorf("FilterURL(viewtopic.php?p=) = %v, want nil (skip)", got)
	}
}

func TestPHPBBFilterURLStripsSid(t *testing.T) {
	filter := &PHPBB{}

	u, _ := url.Parse("https://pckf.com/viewforum.php?f=1&sid=abc")
	got := filter.FilterURL(u)

	if got == nil {
		t.Fatal("FilterURL() returned nil")
	}
	if got.String() != "https://pckf.com/viewforum.php?f=1" {
		t.Errorf("FilterURL() = %q, want %q", got.String(), "https://pckf.com/viewforum.php?f=1")
	}
}

func TestPHPBBFilterURLKeepsCleanURL(t *testing.T) {
	filter := &PHPBB{}

	u, _ := url.Parse("https://pckf.com/viewforum.php?f=1")
	got := filter.FilterURL(u)

	if got == nil {
		t.Fatal("FilterURL() returned nil")
	}
	if got.String() != u.String() {
		t.Errorf("FilterURL() = %q, want %q", got.String(), u.String())
	}
}

func TestPHPBBFilterURLExternalUntouched(t *testing.T) {
	filter := &PHPBB{}

	for _, raw := range []string{
		"https://files.keenmodding.org/4keen14.zip",
		"https://i.imgur.com/lIveDpE.png",
		"https://pckf.com/images/smilies/emotikeen-dopefish.gif",
	} {
		u, _ := url.Parse(raw)
		if got := filter.FilterURL(u); got != u {
			t.Errorf("FilterURL(%q) = %v, want same pointer", raw, got)
		}
	}
}

func TestPHPBBRewriteURLForum(t *testing.T) {
	filter := &PHPBB{}

	u, _ := url.Parse("https://pckf.com/viewforum.php?f=1")
	zimURL, webURL := filter.RewriteURL(u)

	if zimURL == nil || webURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != "https://pckf.com/forum/1.html" {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), "https://pckf.com/forum/1.html")
	}
	if webURL.String() != u.String() {
		t.Errorf("RewriteURL() webURL = %q, want %q", webURL.String(), u.String())
	}
}

func TestPHPBBRewriteURLForumPagination(t *testing.T) {
	filter := &PHPBB{}

	u, _ := url.Parse("https://pckf.com/viewforum.php?f=1&start=50")
	zimURL, _ := filter.RewriteURL(u)

	if zimURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != "https://pckf.com/forum/1-50.html" {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), "https://pckf.com/forum/1-50.html")
	}
}

func TestPHPBBRewriteURLTopic(t *testing.T) {
	filter := &PHPBB{}

	u, _ := url.Parse("https://pckf.com/viewtopic.php?t=21")
	zimURL, _ := filter.RewriteURL(u)

	if zimURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != "https://pckf.com/topic/21.html" {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), "https://pckf.com/topic/21.html")
	}
}

func TestPHPBBRewriteURLTopicPagination(t *testing.T) {
	filter := &PHPBB{}

	u, _ := url.Parse("https://pckf.com/viewtopic.php?t=21&start=145")
	zimURL, _ := filter.RewriteURL(u)

	if zimURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != "https://pckf.com/topic/21-145.html" {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), "https://pckf.com/topic/21-145.html")
	}
}

func TestPHPBBRewriteURLSearch(t *testing.T) {
	filter := &PHPBB{}

	u, _ := url.Parse("https://pckf.com/search.php?search_id=unanswered")
	zimURL, _ := filter.RewriteURL(u)

	if zimURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != "https://pckf.com/search/unanswered.html" {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), "https://pckf.com/search/unanswered.html")
	}
}

func TestPHPBBRewriteURLSearchAuthor(t *testing.T) {
	filter := &PHPBB{}

	u, _ := url.Parse("https://pckf.com/search.php?author_id=26&sr=posts")
	zimURL, _ := filter.RewriteURL(u)

	if zimURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != "https://pckf.com/search/author/26.html" {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), "https://pckf.com/search/author/26.html")
	}
}

func TestPHPBBRewriteURLReverseForum(t *testing.T) {
	filter := &PHPBB{}

	u, _ := url.Parse("https://pckf.com/forum/1-50.html")
	zimURL, webURL := filter.RewriteURL(u)

	if zimURL == nil || webURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != u.String() {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), u.String())
	}
	if webURL.String() != "https://pckf.com/viewforum.php?f=1&start=50" {
		t.Errorf("RewriteURL() webURL = %q, want %q", webURL.String(), "https://pckf.com/viewforum.php?f=1&start=50")
	}
}

func TestPHPBBRewriteURLReverseTopic(t *testing.T) {
	filter := &PHPBB{}

	u, _ := url.Parse("https://pckf.com/topic/21.html")
	_, webURL := filter.RewriteURL(u)

	if webURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if webURL.String() != "https://pckf.com/viewtopic.php?t=21" {
		t.Errorf("RewriteURL() webURL = %q, want %q", webURL.String(), "https://pckf.com/viewtopic.php?t=21")
	}
}

func TestPHPBBRewriteURLNil(t *testing.T) {
	filter := &PHPBB{}

	zimURL, webURL := filter.RewriteURL(nil)
	if zimURL != nil || webURL != nil {
		t.Errorf("RewriteURL(nil) = %v, %v, want nil, nil", zimURL, webURL)
	}
}

func TestPHPBBFilterHTMLRewritesForum(t *testing.T) {
	filter := &PHPBB{}

	input := []byte(`<a href="./viewforum.php?f=1&amp;sid=abc">Forum</a>`)
	got := string(filter.FilterHTML(nil, input))

	if strings.Contains(got, "viewforum.php") {
		t.Errorf("FilterHTML() did not rewrite link: %s", got)
	}
	if !strings.Contains(got, `href="/forum/1.html"`) {
		t.Errorf("FilterHTML() = %q, want href=\"/forum/1.html\"", got)
	}
}

func TestPHPBBFilterHTMLRewritesTopicPagination(t *testing.T) {
	filter := &PHPBB{}

	input := []byte(`<a href="./viewtopic.php?t=21&amp;sid=abc&amp;start=15">Topic</a>`)
	got := string(filter.FilterHTML(nil, input))

	if !strings.Contains(got, `href="/topic/21-15.html"`) {
		t.Errorf("FilterHTML() = %q, want href=\"/topic/21-15.html\"", got)
	}
}

func TestPHPBBFilterHTMLRewritesSearch(t *testing.T) {
	filter := &PHPBB{}

	input := []byte(`<a href="./search.php?search_id=unanswered&amp;sid=abc">Unanswered</a>`)
	got := string(filter.FilterHTML(nil, input))

	if !strings.Contains(got, `href="/search/unanswered.html"`) {
		t.Errorf("FilterHTML() = %q, want href=\"/search/unanswered.html\"", got)
	}
}

func TestPHPBBFilterHTMLStripsSid(t *testing.T) {
	filter := &PHPBB{}

	input := []byte(`<a href="./index.php?sid=abc">Board index</a>`)
	got := string(filter.FilterHTML(nil, input))

	if strings.Contains(got, "sid") {
		t.Errorf("FilterHTML() did not strip sid: %s", got)
	}
	if !strings.Contains(got, `href="./index.php"`) {
		t.Errorf("FilterHTML() = %q, want href=\"./index.php\"", got)
	}
}

func TestPHPBBFilterHTMLPostPermalinkOnTopicPage(t *testing.T) {
	filter := &PHPBB{}

	pageURL, _ := url.Parse("https://pckf.com/viewtopic.php?t=21")

	input := []byte(`<a href="./viewtopic.php?p=201&amp;sid=abc#p201">Post</a>`)
	got := string(filter.FilterHTML(pageURL, input))

	if !strings.Contains(got, `href="#p201"`) {
		t.Errorf("FilterHTML() = %q, want href=\"#p201\"", got)
	}
}
