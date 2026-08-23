package filters

import "net/url"

func ApplyFilterHTML(filters []Filter, page_url *url.URL, html []byte) []byte {

	result := []byte(html)

	for _, filter := range filters {

		if filter.Detect(page_url, html) == true {
			result = filter.FilterHTML(page_url, result)
		}

	}

	return result

}
