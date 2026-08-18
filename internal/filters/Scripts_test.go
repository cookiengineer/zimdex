package filters

import (
	"net/url"
	"strings"
	"testing"
)

func TestScriptsName(t *testing.T) {
	filter := &Scripts{}
	if got := filter.Name(); got != "Scripts" {
		t.Errorf("Name() = %q, want %q", got, "Scripts")
	}
}

func TestScriptsDescription(t *testing.T) {
	filter := &Scripts{}
	if got := filter.Description(); got == "" {
		t.Error("Description() returned empty string")
	}
}

func TestScriptsDetect(t *testing.T) {
	filter := &Scripts{}
	if !filter.Detect(nil, nil) {
		t.Error("Detect() = false, want true")
	}
}

func TestScriptsFilterURL(t *testing.T) {
	filter := &Scripts{}
	pageURL, _ := url.Parse("https://example.com/page")
	if got := filter.FilterURL(pageURL); got != pageURL {
		t.Errorf("FilterURL() should return the same URL pointer")
	}
}

func TestScriptsRemovesElements(t *testing.T) {
	filter := &Scripts{}
	input := `<html><body>
		<script>alert(1)</script>
		<iframe src="https://example.com"></iframe>
		<object data="x"></object>
		<embed src="x"></embed>
		<applet code="x"></applet>
		<p>keep me</p>
	</body></html>`

	got := string(filter.FilterHTML(nil, []byte(input)))

	for _, tag := range []string{"<script", "<iframe", "<object", "<embed", "<applet"} {
		if strings.Contains(got, tag) {
			t.Errorf("output still contains %s: %s", tag, got)
		}
	}

	if !strings.Contains(got, "keep me") {
		t.Errorf("expected content to be preserved, got: %s", got)
	}
}

func TestScriptsRemovesEventHandlers(t *testing.T) {
	filter := &Scripts{}
	input := `<div onclick="steal()" onload="x()" onerror="y()" class="safe">text</div>`

	got := string(filter.FilterHTML(nil, []byte(input)))

	for _, attr := range []string{"onclick", "onload", "onerror"} {
		if strings.Contains(got, attr) {
			t.Errorf("output still contains %s: %s", attr, got)
		}
	}

	if !strings.Contains(got, "class=\"safe\"") {
		t.Errorf("expected safe attributes to be preserved, got: %s", got)
	}
}

func TestScriptsRemovesComments(t *testing.T) {
	filter := &Scripts{}
	input := `<div><!-- secret --><p>hello</p></div>`

	got := string(filter.FilterHTML(nil, []byte(input)))

	if strings.Contains(got, "secret") {
		t.Errorf("output still contains comment: %s", got)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("expected content to be preserved, got: %s", got)
	}
}

func TestScriptsFilterHTML(t *testing.T) {
	filter := &Scripts{}
	input := []byte(`<html><body><script>alert(1)</script><p onclick="x()">hi</p></body></html>`)

	got := string(filter.FilterHTML(nil, input))

	if strings.Contains(got, "<script") || strings.Contains(got, "onclick") {
		t.Errorf("FilterHTML did not sanitize: %s", got)
	}
	if !strings.Contains(got, "hi") {
		t.Errorf("FilterHTML dropped content: %s", got)
	}
}
