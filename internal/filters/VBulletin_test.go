package filters

import (
	"net/url"
	"strings"
	"testing"
)

func TestVBulletinName(t *testing.T) {
	filter := &VBulletin{}
	if got := filter.Name(); got != "VBulletin" {
		t.Errorf("Name() = %q, want %q", got, "VBulletin")
	}
}

func TestVBulletinDetect(t *testing.T) {
	filter := &VBulletin{}

	if filter.Detect(nil, nil) {
		t.Error("Detect(nil, nil) = true, want false")
	}

	if filter.Detect(nil, []byte("<html></html>")) {
		t.Error("Detect(nil, plain html) = true, want false")
	}

	forumURL, _ := url.Parse("https://example.com/forumdisplay.php?f=362")
	if !filter.Detect(forumURL, nil) {
		t.Error("Detect(forumdisplay.php URL) = false, want true")
	}

	friendlyURL, _ := url.Parse("https://example.com/f1058/")
	if !filter.Detect(friendlyURL, nil) {
		t.Error("Detect(/f1058/ URL) = false, want true")
	}

	if !filter.Detect(nil, []byte(`<meta name="generator" content="vBulletin 3.8.7" />`)) {
		t.Error("Detect(vBulletin generator meta) = false, want true")
	}

	if !filter.Detect(nil, []byte(`<script src="clientscript/vbulletin_global.js">`)) {
		t.Error("Detect(clientscript/vbulletin) = false, want true")
	}
}

func TestVBulletinFilterURLSkipScripts(t *testing.T) {
	filter := &VBulletin{}

	for _, raw := range []string{
		"https://example.com/member.php?u=26",
		"https://example.com/memberlist.php",
		"https://example.com/search.php?do=process",
		"https://example.com/newreply.php?do=newreply&t=21",
		"https://example.com/newthread.php?do=newthread&f=1058",
		"https://example.com/usercp.php",
		"https://example.com/login.php?do=login",
		"https://example.com/register.php",
		"https://example.com/sendmail.php?to=1",
		"https://example.com/inlinemod.php?forumid=1058",
		"https://example.com/showpost.php?p=123",
	} {
		u, _ := url.Parse(raw)
		if got := filter.FilterURL(u); got != nil {
			t.Errorf("FilterURL(%q) = %v, want nil (skip)", raw, got)
		}
	}
}

func TestVBulletinFilterURLSkipFriendlyUser(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/users/1779045/")
	if got := filter.FilterURL(u); got != nil {
		t.Errorf("FilterURL(/users/) = %v, want nil (skip)", got)
	}
}

func TestVBulletinFilterURLSkipAttachments(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/attachments/f1058/719d1495-yumor_jpg")
	if got := filter.FilterURL(u); got != nil {
		t.Errorf("FilterURL(/attachments/) = %v, want nil (skip)", got)
	}
}

func TestVBulletinFilterURLSkipPostPermalink(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/showthread.php?p=201&s=abc")
	if got := filter.FilterURL(u); got != nil {
		t.Errorf("FilterURL(showthread.php?p=) = %v, want nil (skip)", got)
	}
}

func TestVBulletinFilterURLStripsSession(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/forumdisplay.php?f=362&s=abc&order=desc&sort=lastpost&daysprune=-1")
	got := filter.FilterURL(u)

	if got == nil {
		t.Fatal("FilterURL() returned nil")
	}
	if got.String() != "https://example.com/forumdisplay.php?f=362" {
		t.Errorf("FilterURL() = %q, want %q", got.String(), "https://example.com/forumdisplay.php?f=362")
	}
}

func TestVBulletinFilterURLKeepsCleanForum(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/forumdisplay.php?f=362")
	got := filter.FilterURL(u)

	if got == nil {
		t.Fatal("FilterURL() returned nil")
	}
	if got.String() != u.String() {
		t.Errorf("FilterURL() = %q, want %q", got.String(), u.String())
	}
}

func TestVBulletinFilterURLFriendlyUntouched(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/f1058/")
	if got := filter.FilterURL(u); got != u {
		t.Errorf("FilterURL(/f1058/) = %v, want same pointer", got)
	}
}

func TestVBulletinFilterURLExternalUntouched(t *testing.T) {
	filter := &VBulletin{}

	for _, raw := range []string{
		"https://example.com/images/logo.png",
		"https://pbs.twimg.com/media/CxyG7yRXAAAgLZS.jpg",
		"https://www.avito.ru/krasnodar/vakansii/buhgalter",
	} {
		u, _ := url.Parse(raw)
		if got := filter.FilterURL(u); got != u {
			t.Errorf("FilterURL(%q) = %v, want same pointer", raw, got)
		}
	}
}

func TestVBulletinRewriteURLForum(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/forumdisplay.php?f=1058")
	zimURL, webURL := filter.RewriteURL(u)

	if zimURL == nil || webURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != "https://example.com/forum/1058.html" {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), "https://example.com/forum/1058.html")
	}
	if webURL.String() != u.String() {
		t.Errorf("RewriteURL() webURL = %q, want %q", webURL.String(), u.String())
	}
}

func TestVBulletinRewriteURLForumPage(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/forumdisplay.php?f=1058&page=2")
	zimURL, _ := filter.RewriteURL(u)

	if zimURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != "https://example.com/forum/1058-2.html" {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), "https://example.com/forum/1058-2.html")
	}
}

func TestVBulletinRewriteURLThread(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/showthread.php?t=6263615")
	zimURL, _ := filter.RewriteURL(u)

	if zimURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != "https://example.com/thread/6263615.html" {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), "https://example.com/thread/6263615.html")
	}
}

func TestVBulletinRewriteURLThreadPage(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/showthread.php?t=6263615&page=2")
	zimURL, _ := filter.RewriteURL(u)

	if zimURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != "https://example.com/thread/6263615-2.html" {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), "https://example.com/thread/6263615-2.html")
	}
}

func TestVBulletinRewriteURLFriendlyForum(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/f1058/")
	zimURL, _ := filter.RewriteURL(u)

	if zimURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != "https://example.com/forum/1058.html" {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), "https://example.com/forum/1058.html")
	}
}

func TestVBulletinRewriteURLFriendlyForumPage(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/f1058/i2.html")
	zimURL, _ := filter.RewriteURL(u)

	if zimURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != "https://example.com/forum/1058-2.html" {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), "https://example.com/forum/1058-2.html")
	}
}

func TestVBulletinRewriteURLFriendlyThread(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/f1058/yumor_buhgalterskij-6263615.html")
	zimURL, _ := filter.RewriteURL(u)

	if zimURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != "https://example.com/thread/6263615.html" {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), "https://example.com/thread/6263615.html")
	}
}

func TestVBulletinRewriteURLFriendlyThreadPage(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/f1058/yumor_buhgalterskij-6263615-2.html")
	zimURL, _ := filter.RewriteURL(u)

	if zimURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != "https://example.com/thread/6263615-2.html" {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), "https://example.com/thread/6263615-2.html")
	}
}

func TestVBulletinRewriteURLReverseForum(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/forum/1058-2.html")
	zimURL, webURL := filter.RewriteURL(u)

	if zimURL == nil || webURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if zimURL.String() != u.String() {
		t.Errorf("RewriteURL() zimURL = %q, want %q", zimURL.String(), u.String())
	}
	if webURL.String() != "https://example.com/forumdisplay.php?f=1058&page=2" {
		t.Errorf("RewriteURL() webURL = %q, want %q", webURL.String(), "https://example.com/forumdisplay.php?f=1058&page=2")
	}
}

func TestVBulletinRewriteURLReverseThread(t *testing.T) {
	filter := &VBulletin{}

	u, _ := url.Parse("https://example.com/thread/6263615.html")
	_, webURL := filter.RewriteURL(u)

	if webURL == nil {
		t.Fatal("RewriteURL() returned nil")
	}
	if webURL.String() != "https://example.com/showthread.php?t=6263615" {
		t.Errorf("RewriteURL() webURL = %q, want %q", webURL.String(), "https://example.com/showthread.php?t=6263615")
	}
}

func TestVBulletinRewriteURLNil(t *testing.T) {
	filter := &VBulletin{}

	zimURL, webURL := filter.RewriteURL(nil)
	if zimURL != nil || webURL != nil {
		t.Errorf("RewriteURL(nil) = %v, %v, want nil, nil", zimURL, webURL)
	}
}

func TestVBulletinFilterHTMLRewritesForum(t *testing.T) {
	filter := &VBulletin{}

	input := []byte(`<a href="forumdisplay.php?f=362&amp;page=2">Forum</a>`)
	got := string(filter.FilterHTML(nil, input))

	if !strings.Contains(got, `href="/forum/362-2.html"`) {
		t.Errorf("FilterHTML() = %q, want href=\"/forum/362-2.html\"", got)
	}
}

func TestVBulletinFilterHTMLRewritesThread(t *testing.T) {
	filter := &VBulletin{}

	input := []byte(`<a href="showthread.php?t=6263615&amp;s=abc">Thread</a>`)
	got := string(filter.FilterHTML(nil, input))

	if !strings.Contains(got, `href="/thread/6263615.html"`) {
		t.Errorf("FilterHTML() = %q, want href=\"/thread/6263615.html\"", got)
	}
}

func TestVBulletinFilterHTMLRewritesFriendlyThread(t *testing.T) {
	filter := &VBulletin{}

	input := []byte(`<a href="https://example.com/f1058/yumor_buhgalterskij-6263615-2.html">Thread</a>`)
	got := string(filter.FilterHTML(nil, input))

	if !strings.Contains(got, `href="/thread/6263615-2.html"`) {
		t.Errorf("FilterHTML() = %q, want href=\"/thread/6263615-2.html\"", got)
	}
}

func TestVBulletinFilterHTMLStripsSession(t *testing.T) {
	filter := &VBulletin{}

	input := []byte(`<a href="misc.php?do=showrules&amp;s=abc">Rules</a>`)
	got := string(filter.FilterHTML(nil, input))

	if strings.Contains(got, "s=abc") {
		t.Errorf("FilterHTML() did not strip session: %s", got)
	}
}

func TestVBulletinFilterHTMLPostPermalinkOnThreadPage(t *testing.T) {
	filter := &VBulletin{}

	pageURL, _ := url.Parse("https://example.com/showthread.php?t=6263615")

	input := []byte(`<a href="showthread.php?p=44234539#post44234539">Post</a>`)
	got := string(filter.FilterHTML(pageURL, input))

	if !strings.Contains(got, `href="#post44234539"`) {
		t.Errorf("FilterHTML() = %q, want href=\"#post44234539\"", got)
	}
}
