package filters

import (
	"net/url"
	"strings"
	"testing"
)

func TestTrackersName(t *testing.T) {
	filter := &Trackers{}
	if got := filter.Name(); got != "Trackers" {
		t.Errorf("Name() = %q, want %q", got, "Trackers")
	}
}

func TestTrackersDetect(t *testing.T) {
	filter := &Trackers{}
	if !filter.Detect(nil, nil) {
		t.Error("Detect() = false, want true")
	}
}

func TestFilterTrackers(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com/page", "https://example.com/page"},
		{"https://example.com/page?utm_source=web", "https://example.com/page"},
		{"https://example.com/page?utm_source=web&id=42", "https://example.com/page?id=42"},
		{"https://example.com/page?fbclid=abc&gclid=def&name=hello", "https://example.com/page?name=hello"},
		{"https://example.com/page?ref=other&mc_eid=123", "https://example.com/page"},
	}

	for _, tt := range tests {
		u, err := url.Parse(tt.input)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", tt.input, err)
		}

		FilterTrackers(u)

		if got := u.String(); got != tt.expected {
			t.Errorf("FilterTrackers(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestTrackersFilterURL(t *testing.T) {
	filter := &Trackers{}
	u, _ := url.Parse("https://example.com/page?utm_source=web&id=42")

	got := filter.FilterURL(u)

	if got == nil {
		t.Fatal("FilterURL() returned nil")
	}
	if got.String() != "https://example.com/page?id=42" {
		t.Errorf("FilterURL() = %q, want %q", got.String(), "https://example.com/page?id=42")
	}
	if got == u {
		t.Error("FilterURL() should return a clone, not the original pointer")
	}
}

func TestTrackersRemovesTrackingImages(t *testing.T) {
	filter := &Trackers{}
	input := `<body>
		<img src="pixel.gif" width="1" height="1">
		<img src="https://analytics.example.com/t.gif">
		<img src="https://example.com/logo.png" width="200" height="100">
	</body>`

	got := string(filter.FilterHTML(nil, []byte(input)))

	if strings.Contains(got, "pixel.gif") || strings.Contains(got, "analytics.example.com") {
		t.Errorf("output still contains tracking image: %s", got)
	}
	if !strings.Contains(got, "logo.png") {
		t.Errorf("expected normal image to be preserved, got: %s", got)
	}
}

func TestTrackersRemovesTrackingMeta(t *testing.T) {
	filter := &Trackers{}
	input := `<head>
		<meta property="og:title" content="x">
		<meta property="og:image" content="https://example.com/img.png">
		<meta property="fb:app_id" content="123">
		<meta name="twitter:card" content="summary">
		<meta name="msapplication-tilecolor" content="#000000">
		<meta name="apple-mobile-web-app-capable" content="yes">
		<meta name="description" content="keep">
	</head>`

	got := string(filter.FilterHTML(nil, []byte(input)))

	for _, meta := range []string{"og:title", "og:image", "fb:app_id", "twitter:card", "msapplication-tilecolor", "apple-mobile-web-app-capable"} {
		if strings.Contains(got, meta) {
			t.Errorf("output still contains tracking meta %q: %s", meta, got)
		}
	}
	if !strings.Contains(got, "description") {
		t.Errorf("expected non-tracking meta to be preserved, got: %s", got)
	}
}

func TestTrackersRemovesTrackingLinks(t *testing.T) {
	filter := &Trackers{}
	input := `<head>
		<link rel="preconnect" href="https://x">
		<link rel="stylesheet" href="style.css">
	</head>`

	got := string(filter.FilterHTML(nil, []byte(input)))

	if strings.Contains(got, "preconnect") {
		t.Errorf("output still contains preconnect link: %s", got)
	}
	if !strings.Contains(got, "stylesheet") {
		t.Errorf("expected stylesheet link to be preserved, got: %s", got)
	}
}

func TestTrackersFilterHTML(t *testing.T) {
	filter := &Trackers{}
	input := []byte(`<html><body><img src="pixel.gif" width="1" height="1"><p>hi</p></body></html>`)

	got := string(filter.FilterHTML(nil, input))

	if strings.Contains(got, "pixel.gif") {
		t.Errorf("FilterHTML did not remove tracking image: %s", got)
	}
	if !strings.Contains(got, "hi") {
		t.Errorf("FilterHTML dropped content: %s", got)
	}
}
