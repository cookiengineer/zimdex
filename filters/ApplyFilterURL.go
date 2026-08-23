package filters

import "net/url"

func ApplyFilterURL(filters []Filter, page_url *url.URL, html []byte, referrer *url.URL) *url.URL {

	var result *url.URL = page_url

	for _, filter := range filters {

		if filter.Detect(referrer, html) == true {

			filtered := filter.FilterURL(page_url)

			if filtered == nil {
				result = nil
				break
			} else if filtered.String() != page_url.String() {
				result = filtered
			}

		}

	}

	return result

}

