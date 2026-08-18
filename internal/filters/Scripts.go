package filters

import "net/url"
import "regexp"

var scripts_tag_regex = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)

type Scripts struct {}

func (filter *Scripts) Name() string {
	return "Scripts"
}

func (filter *Scripts) Description() string {
	return "Remove script elements and and inline JavaScript from HTML"
}

func (filter *Scripts) Detect(_ *url.URL, _ []byte) bool {
	return true
}

func (filter *Scripts) FilterURL(page_url *url.URL) *url.URL {
	return page_url
}

func (filter *Scripts) FilterHTML(_ *url.URL, content []byte) []byte {
	return scripts_tag_regex.ReplaceAll(content, []byte{})
}
