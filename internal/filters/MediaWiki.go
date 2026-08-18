package filters

import "bytes"
import "fmt"
import "net/url"
import "regexp"
import "strings"
import "golang.org/x/net/html"

var media_wiki_skip_actions = map[string]bool{
	"delete": true,
	"edit": true,
	"history": true,
	"info": true,
	"protect": true,
	"purge": true,
	"submit": true,
}

var media_wiki_skip_namespaces = map[string]bool{
	"Talk:": true,
	"User:": true,
	"User_talk:": true,
	"Template:": true,
	"Category:": true,
	"Special:": true,
	"File:": true,
	"MediaWiki:": true,
	"Help:": true,
}

var media_wiki_filter_parameters = map[string]*regexp.Regexp{
	"oldid":     regexp.MustCompile(`(&amp;|&)oldid=\d+`),
	"printable": regexp.MustCompile(`(&amp;|&)printable=yes`),
	"veaction":  regexp.MustCompile(`(&amp;|&)veaction=\w+`),
	"useskin":   regexp.MustCompile(`(&amp;|&)useskin=\w+`),
	"returnto":  regexp.MustCompile(`(&amp;|&)returnto=\w+`),
}

type MediaWiki struct {}

func (filter *MediaWiki) Name() string {
	return "MediaWiki"
}

func (filter *MediaWiki) Description() string {
	return "Filter MediaWiki version control links and rewrite URLs to clean paths"
}

func (filter *MediaWiki) Detect(page_url *url.URL, content []byte) bool {

	if len(content) > 0 {

		if bytes.Contains(content, []byte("class=\"mediawiki\"")) {
			return true
		}

		if bytes.Contains(content, []byte("body class=\"mediawiki")) {
			return true
		}

		document, err := html.Parse(bytes.NewReader(content))

		if err == nil && findHTMLMeta(document, "generator", "mediawiki") {
			return true
		}

	}

	if page_url != nil {
		return strings.Contains(page_url.String(), "index.php?title=")
	}

	return false

}

func (filter *MediaWiki) FilterURL(page_url *url.URL) *url.URL {

	var result *url.URL

	if page_url != nil {

		query := page_url.Query()
		title := query.Get("title")
		action := query.Get("action")

		if title != "" {

			for namespace, _ := range media_wiki_skip_namespaces {

				if strings.HasPrefix(title, namespace) {
					return nil
				}

			}

		}

		if action != "" {

			_, ok := media_wiki_skip_actions[action]
			if ok == true {
				return nil
			}

		}

		changed := false

		for parameter, _ := range media_wiki_filter_parameters {

			if query.Has(parameter) == true {
				query.Del(parameter)
				changed = true
			}

		}

		if changed == true {

			clone := *page_url
			clone.RawQuery = query.Encode()
			result = &clone

		} else {
			result = page_url
		}

	}

	return result

}

var media_wiki_link_regex = regexp.MustCompile(`(href|src)=["']/?index\.php\?title=([^"'\s&]+)(&amp;[^"'\s]*)?["']`)

func (filter *MediaWiki) FilterHTML(_ *url.URL, content []byte) []byte {

	result := []byte(content)

	for _, regex := range media_wiki_filter_parameters {
		result = regex.ReplaceAll(result, []byte{})
	}

	result = media_wiki_link_regex.ReplaceAllFunc(result, func(input []byte) []byte {

		parts := media_wiki_link_regex.FindSubmatch(input)

		if len(parts) >= 3 {

			attribute := string(parts[1])
			title     := string(parts[2])

			tmp        := strings.ReplaceAll(title, "_", " ")
			decoded, _ := url.QueryUnescape(tmp)
			value      := "/" + strings.ReplaceAll(decoded, " ", "_") + ".html"

			return []byte(fmt.Sprintf("%s=\"%s\"", attribute, value))

		} else {
			return input
		}

	})

	return result

}

func (filter *MediaWiki) RewriteURL(page_url *url.URL) (*url.URL, *url.URL) {

	if page_url != nil {

		var result_web_url *url.URL
		var result_zim_url *url.URL

		tmp := page_url.String()

		if strings.Contains(tmp, "index.php?title=") {

			query := page_url.Query()
			title := query.Get("title")

			if title != "" {

				clone := *page_url
				clone.RawQuery = ""
				clone.Path     = "/" + strings.ReplaceAll(title, " ", "_") + ".html"

				result_zim_url = &clone
				result_web_url = page_url

			} else {
				result_zim_url = page_url
				result_web_url = page_url
			}

		} else if strings.HasSuffix(page_url.Path, ".html") {

			title := strings.TrimSuffix(strings.TrimPrefix(page_url.Path, "/"), ".html")

			if title != "" {

				clone := *page_url
				clone.Path = "/index.php"
				clone.RawQuery = fmt.Sprintf("title=%s", url.QueryEscape(strings.ReplaceAll(title, "_", " ")))

				result_zim_url = page_url
				result_web_url = &clone

			} else {
				result_zim_url = page_url
				result_web_url = page_url
			}

		}

		return result_zim_url, result_web_url

	} else {
		return nil, nil
	}

}
