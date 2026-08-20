package filters

import "net/url"

func Detect(page_url *url.URL, content []byte) []string {

	result := make([]string, 0)

	for _, filter := range Registry {

		if filter.IsDefault() == true {
			result = append(result, filter.Name())
		} else if filter.Detect(page_url, content) == true {
			result = append(result, filter.Name())
		}

	}

	return result

}
