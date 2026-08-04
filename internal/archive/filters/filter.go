package filters

import "net/url"

type Filter interface {
	Name() string
	Description() string
	Detect(html []byte, pageURL *url.URL) bool
	FilterURL(rawURL *url.URL) *url.URL
	FilterHTML(html []byte, pageURL *url.URL) []byte
}

type URLRewriter interface {
	RewriteURL(rawURL *url.URL) (newURL, downloadURL *url.URL)
}

func ApplyURLRewriter(rawURL *url.URL, filters []Filter) (newURL, downloadURL *url.URL) {
	newURL = rawURL
	for _, f := range filters {
		if rw, ok := f.(URLRewriter); ok {
			n, dl := rw.RewriteURL(rawURL)
			if dl != nil {
				downloadURL = dl
			}
			if n != nil && n != rawURL {
				newURL = n
			}
		}
	}
	if downloadURL == nil {
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

func ApplyHTMLFilters(html []byte, pageURL *url.URL, filters []Filter) []byte {
	for _, f := range filters {
		if f.Detect(html, pageURL) {
			html = f.FilterHTML(html, pageURL)
		}
	}
	return html
}

func ApplyURLFilters(rawURL *url.URL, html []byte, pageURL *url.URL, filters []Filter) *url.URL {
	for _, f := range filters {
		if f.Detect(html, pageURL) {
			filtered := f.FilterURL(rawURL)
			if filtered == nil {
				return nil
			}
			if filtered != rawURL {
				rawURL = filtered
			}
		}
	}
	return rawURL
}
