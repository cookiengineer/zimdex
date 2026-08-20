package filters

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
