package filters

import "net/url"

func ApplyFilterCSS(filters []Filter, page_url *url.URL, css []byte) []byte {

	result := []byte(css)

	for _, filter := range filters {

		if filter.Detect(page_url, css) == true {
			result = filter.FilterCSS(page_url, result)
		}

	}

	return result

}
