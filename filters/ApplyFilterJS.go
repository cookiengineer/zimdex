package filters

import "net/url"

func ApplyFilterJS(filters []Filter, page_url *url.URL, js []byte) []byte {

	result := []byte(js)

	for _, filter := range filters {

		if filter.Detect(page_url, js) == true {
			result = filter.FilterJS(page_url, result)
		}

	}

	return result

}
