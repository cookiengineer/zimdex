package filters

import "net/url"

type URLRewriter interface {
	RewriteURL(*url.URL) (*url.URL, *url.URL)
}

func ApplyRewriteURL(filters []Filter, page_url *url.URL) (*url.URL, *url.URL) {

	var result_web_url *url.URL
	var result_zim_url *url.URL

	result_web_url, _ = url.Parse(page_url.String())

	for _, filter := range filters {

		rewriter, ok := filter.(URLRewriter)

		if ok == true {

			tmp_zim_url, tmp_web_url := rewriter.RewriteURL(page_url)

			if tmp_web_url != nil {
				result_web_url = tmp_web_url
			}

			if tmp_zim_url != nil && tmp_zim_url.String() != page_url.String() {
				result_zim_url = tmp_zim_url
			}

		}

	}

	if result_web_url == nil && result_zim_url != nil {
		result_web_url = result_zim_url
	}

	return result_zim_url, result_web_url

}
