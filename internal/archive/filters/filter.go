package filters

type Filter interface {
	Name() string
	Description() string
	Detect(html []byte, pageURL string) bool
	FilterURL(rawURL string) string
	FilterHTML(html []byte, pageURL string) []byte
}

type URLRewriter interface {
	RewriteURL(rawURL string) (newURL, downloadURL string)
}

func ApplyURLRewriter(rawURL string, filters []Filter) (newURL, downloadURL string) {
	newURL = rawURL
	for _, f := range filters {
		if rw, ok := f.(URLRewriter); ok {
			n, dl := rw.RewriteURL(rawURL)
			if dl != "" {
				downloadURL = dl
			}
			if n != "" && n != rawURL {
				newURL = n
			}
		}
	}
	if downloadURL == "" {
		downloadURL = newURL
	}
	return
}

var Registry = []Filter{
	&TrackingFilter{},
	&MediaWikiFilter{},
	&ScriptFilter{},
}

func Enabled(names []string) []Filter {
	if len(names) == 0 {
		return nil
	}
	wanted := make(map[string]bool, len(names))
	for _, n := range names {
		wanted[n] = true
	}
	var result []Filter
	for _, f := range Registry {
		if wanted[f.Name()] {
			result = append(result, f)
		}
	}
	return result
}

func AllNames() []string {
	names := make([]string, len(Registry))
	for i, f := range Registry {
		names[i] = f.Name()
	}
	return names
}

func ApplyHTMLFilters(html []byte, pageURL string, filters []Filter) []byte {
	for _, f := range filters {
		if f.Detect(html, pageURL) {
			html = f.FilterHTML(html, pageURL)
		}
	}
	return html
}

func ApplyURLFilters(rawURL string, html []byte, pageURL string, filters []Filter) string {
	for _, f := range filters {
		if f.Detect(html, pageURL) {
			filtered := f.FilterURL(rawURL)
			if filtered == "" {
				return ""
			}
			if filtered != rawURL {
				rawURL = filtered
			}
		}
	}
	return rawURL
}
