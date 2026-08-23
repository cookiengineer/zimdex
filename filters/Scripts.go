package filters

import "github.com/grafana/sobek/parser"
import "net/url"

type Scripts struct{}

func (filter *Scripts) Name() string {
	return "Scripts"
}

func (filter *Scripts) Description() string {
	return "Sanitize HTML by removing scripts, embedded content, event handlers, comments, and empty elements"
}

func (filter *Scripts) IsDefault() bool {
	return true
}

func (filter *Scripts) Detect(_ *url.URL, _ []byte) bool {
	return true
}

func (filter *Scripts) FilterURL(page_url *url.URL) *url.URL {
	return page_url
}

func (filter *Scripts) FilterHTML(_ *url.URL, content []byte) []byte {

	sanitizer := NewSanitizer(content, SanitizerOptions{
		Elements: []string{
			"script",
			"iframe",
			"object",
			"embed",
			"applet",
		},
		Attributes: []string{
			"onclick", "ondblclick",
			"onmousedown", "onmouseup", "onmouseover", "onmouseout", "onmousemove",
			"onkeydown", "onkeyup", "onkeypress",
			"onfocus", "onblur",
			"onchange", "onsubmit", "onreset", "onselect",
			"onload", "onunload", "onerror", "onabort",
			"onscroll", "onresize",
			"onbeforeunload", "onhashchange",
		},
	})

	sanitizer.Sanitize()

	return sanitizer.Render()

}

func (filter *Scripts) FilterCSS(_ *url.URL, content []byte) []byte {
	return content
}

func (filter *Scripts) FilterJS(_ *url.URL, content []byte) []byte {

	program, err := parser.ParseFile(nil, "", string(content), 0)

	if err != nil {
		return content
	}

	spoofs := make([]js_spoof, 0)

	for _, statement := range program.Body {
		find_js_functions(statement, &spoofs)
	}

	if len(spoofs) == 0 {
		return content
	}

	result := []byte(content)

	for index := len(spoofs) - 1; index >= 0; index-- {
		spoof := spoofs[index]
		result = js_splice(result, spoof.start, spoof.end, spoof.body)
	}

	return result

}

