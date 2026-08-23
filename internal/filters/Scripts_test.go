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

func TestScriptsFilterJSFarbleAsync(t *testing.T) {
	filter := &Scripts{}
	input := `async function trackUser() { await fetch('https://analytics.example.com/x'); return true; }`

	got := string(filter.FilterJS(nil, []byte(input)))

	if !strings.Contains(got, "async function trackUser() { return true; }") {
		t.Errorf("FilterJS() = %q, want farbled async signature", got)
	}
	if strings.Contains(got, "fetch") {
		t.Errorf("FilterJS() left network call intact: %s", got)
	}
}

func TestScriptsFilterJSFarbleFetch(t *testing.T) {
	filter := &Scripts{}
	input := `function sendEvent() { fetch('/track'); console.log('done'); }`

	got := string(filter.FilterJS(nil, []byte(input)))

	if !strings.Contains(got, "function sendEvent() { return true; }") {
		t.Errorf("FilterJS() = %q, want farbled body", got)
	}
	if strings.Contains(got, "fetch") || strings.Contains(got, "console.log") {
		t.Errorf("FilterJS() left network code intact: %s", got)
	}
}

func TestScriptsFilterJSFarbleXHR(t *testing.T) {
	filter := &Scripts{}
	input := `function load() { var xhr = new XMLHttpRequest(); xhr.open('GET', '/data'); xhr.send(); }`

	got := string(filter.FilterJS(nil, []byte(input)))

	if !strings.Contains(got, "function load() { return true; }") {
		t.Errorf("FilterJS() = %q, want farbled XHR body", got)
	}
	if strings.Contains(got, "XMLHttpRequest") {
		t.Errorf("FilterJS() left XHR intact: %s", got)
	}
}

func TestScriptsFilterJSPreservesNonNetwork(t *testing.T) {
	filter := &Scripts{}
	input := `function add(a, b) { return a + b; }`

	got := string(filter.FilterJS(nil, []byte(input)))

	if got != input {
		t.Errorf("FilterJS() = %q, want unchanged %q", got, input)
	}
}

func TestScriptsFilterJSFarbleNested(t *testing.T) {
	filter := &Scripts{}
	input := `function outer() { function inner() { fetch('/x'); } inner(); return 1; }`

	got := string(filter.FilterJS(nil, []byte(input)))

	if !strings.Contains(got, "function inner() { return true; }") {
		t.Errorf("FilterJS() did not farble nested function: %s", got)
	}
	if !strings.Contains(got, "return 1;") {
		t.Errorf("FilterJS() should preserve non-network outer body: %s", got)
	}
}

func TestScriptsFilterJSInvalidInput(t *testing.T) {
	filter := &Scripts{}
	input := []byte("this is not valid javascript {{{")

	got := filter.FilterJS(nil, input)

	if string(got) != string(input) {
		t.Errorf("FilterJS() = %q, want unchanged for invalid input", got)
	}
}
