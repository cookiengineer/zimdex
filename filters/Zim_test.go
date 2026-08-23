package filters

import (
	"net/url"
	"strings"
	"testing"
)

func TestZimName(t *testing.T) {
	filter := &Zim{}
	if got := filter.Name(); got != "Zim" {
		t.Errorf("Name() = %q, want %q", got, "Zim")
	}
}

func TestZimIsDefault(t *testing.T) {
	filter := &Zim{}
	if filter.IsDefault() != false {
		t.Error("IsDefault() = true, want false")
	}
}

func TestZimResolve(t *testing.T) {
	filter := &Zim{File: "example.zim"}
	pageURL, _ := url.Parse("https://example.com/wiki/Page")

	tests := []struct {
		input string
		want  string
	}{
		{"https://cdn.example.com/img.png", "/example.zim/cdn.example.com/img.png"},
		{"//cdn.example.com/img.png", "/example.zim/cdn.example.com/img.png"},
		{"/wiki/Other", "/example.zim/example.com/wiki/Other"},
		{"../img/logo.png", "/example.zim/example.com/img/logo.png"},
		{"#anchor", "#anchor"},
		{"mailto:user@example.com", "mailto:user@example.com"},
		{"data:image/png;base64,AAA", "data:image/png;base64,AAA"},
		{"javascript:alert(1)", "#"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := filter.Resolve(pageURL, tt.input); got != tt.want {
			t.Errorf("Resolve(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestZimFilterHTML(t *testing.T) {
	filter := &Zim{File: "example.zim"}
	pageURL, _ := url.Parse("https://example.com/wiki/Page")

	input := `<html><body>
		<a href="/wiki/Other">link</a>
		<img src="https://cdn.example.com/img.png">
		<img srcset="https://cdn.example.com/a.png 1x, https://cdn.example.com/b.png 2x">
		<script src="https://cdn.example.com/app.js"></script>
		<div style="background: url('/img/bg.png')"></div>
		<style>body { background: url('/img/bg2.png') }</style>
		<a href="#anchor">anchor</a>
		<a href="mailto:user@example.com">mail</a>
	</body></html>`

	got := string(filter.FilterHTML(pageURL, []byte(input)))

	for _, want := range []string{
		"/example.zim/example.com/wiki/Other",
		"/example.zim/cdn.example.com/img.png",
		"/example.zim/cdn.example.com/a.png 1x",
		"/example.zim/cdn.example.com/b.png 2x",
		"/example.zim/cdn.example.com/app.js",
		"/example.zim/example.com/img/bg.png",
		"/example.zim/example.com/img/bg2.png",
		"#anchor",
		"mailto:user@example.com",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("FilterHTML() missing %q in output: %s", want, got)
		}
	}

	if strings.Contains(got, "https://cdn.example.com") {
		t.Errorf("FilterHTML() left absolute URL unrewn: %s", got)
	}
}

func TestZimFilterCSS(t *testing.T) {
	filter := &Zim{File: "example.zim"}
	pageURL, _ := url.Parse("https://example.com/css/main.css")

	input := `body { background: url("../img/bg.png"); }
@import "https://cdn.example.com/base.css";
.icon { background-image: url('data:image/png;base64,AAA'); }`

	got := string(filter.FilterCSS(pageURL, []byte(input)))

	for _, want := range []string{
		"url(/example.zim/example.com/img/bg.png)",
		`@import "/example.zim/cdn.example.com/base.css"`,
		"url('data:image/png;base64,AAA')",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("FilterCSS() missing %q in output: %s", want, got)
		}
	}
}

func TestZimFilterJSImports(t *testing.T) {
	filter := &Zim{File: "example.zim"}
	pageURL, _ := url.Parse("https://example.com/js/app.js")

	input := `import x from "./foo.js";
import * as ns from "https://cdn.example.com/lib.js";
import { a, b } from '../util/helpers.js';
import "./side-effect.js";
export { x } from "./re-export.js";`

	got := string(filter.FilterJS(pageURL, []byte(input)))

	for _, want := range []string{
		"/example.zim/example.com/js/foo.js",
		"/example.zim/cdn.example.com/lib.js",
		"/example.zim/example.com/util/helpers.js",
		"/example.zim/example.com/js/side-effect.js",
		"/example.zim/example.com/js/re-export.js",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("FilterJS() missing %q in output: %s", want, got)
		}
	}

	if strings.Contains(got, "https://cdn.example.com") {
		t.Errorf("FilterJS() left absolute import unrewn: %s", got)
	}
}

func TestZimFilterJSPassThrough(t *testing.T) {
	filter := &Zim{File: "example.zim"}
	input := []byte("function add(a, b) { return a + b; }")

	got := filter.FilterJS(nil, input)

	if string(got) != string(input) {
		t.Errorf("FilterJS() = %q, want unchanged %q", got, input)
	}
}
