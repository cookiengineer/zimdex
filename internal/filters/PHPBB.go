package filters

import "bytes"
import "fmt"
import "net/url"
import "path/filepath"
import "regexp"
import "strings"

var phpbb_script_names = []string{
	"viewforum.php",
	"viewtopic.php",
	"index.php",
	"search.php",
	"memberlist.php",
	"posting.php",
	"ucp.php",
}

var phpbb_skip_scripts = map[string]bool{
	"memberlist.php":    true,
	"posting.php":       true,
	"ucp.php":           true,
	"download/file.php": true,
}

type PHPBB struct{}

func (filter *PHPBB) Name() string {
	return "PHPBB"
}

func (filter *PHPBB) Description() string {
	return "Filter phpBB session links and rewrite legacy URLs to clean paths"
}

func (filter *PHPBB) IsDefault() bool {
	return false
}

func (filter *PHPBB) Detect(page_url *url.URL, content []byte) bool {

	if len(content) > 0 {

		if bytes.Contains(content, []byte(`id="phpbb"`)) {
			return true
		}

		if bytes.Contains(content, []byte(`phpBB style name:`)) {
			return true
		}

	}

	if page_url != nil {
		return phpbbPathToken(page_url) != ""
	}

	return false

}

func (filter *PHPBB) FilterURL(page_url *url.URL) *url.URL {

	if page_url == nil {
		return nil
	}

	token := phpbbPathToken(page_url)

	if phpbb_skip_scripts[token] {
		return nil
	}

	if token == "viewtopic.php" {

		query := page_url.Query()

		if query.Get("t") == "" && query.Get("p") != "" {
			return nil
		}

	}

	if token != "" {
		return phpbbStripSession(page_url)
	}

	return page_url

}

var phpbb_html_link_regex = regexp.MustCompile(`(?i)(href|src|action)=["']([^"'\s]*)["']`)

func (filter *PHPBB) FilterHTML(page_url *url.URL, content []byte) []byte {

	result := []byte(content)

	result = phpbb_html_link_regex.ReplaceAllFunc(result, func(input []byte) []byte {

		parts := phpbb_html_link_regex.FindSubmatch(input)

		if len(parts) < 3 {
			return input
		}

		attribute := string(parts[1])
		raw := string(parts[2])

		rewritten, changed := phpbbRewriteHTMLURL(page_url, raw)

		if changed == false {
			return input
		}

		return []byte(fmt.Sprintf(`%s="%s"`, attribute, rewritten))

	})

	return result

}

func (filter *PHPBB) RewriteURL(page_url *url.URL) (*url.URL, *url.URL) {

	if page_url == nil {
		return nil, nil
	}

	if zim_url, web_url := phpbbRewriteLegacy(page_url); zim_url != nil {
		return zim_url, web_url
	}

	if zim_url, web_url := phpbbRewriteClean(page_url); zim_url != nil {
		return zim_url, web_url
	}

	return nil, nil

}

func phpbbRewriteLegacy(page_url *url.URL) (*url.URL, *url.URL) {

	token := phpbbPathToken(page_url)

	query := page_url.Query()

	switch token {

	case "viewforum.php":

		if f := query.Get("f"); f != "" {
			return phpbbCleanClone(page_url, phpbbForumPath(f, query.Get("start"))), page_url
		}

	case "viewtopic.php":

		if t := query.Get("t"); t != "" {
			return phpbbCleanClone(page_url, phpbbTopicPath(t, query.Get("start"))), page_url
		}

	case "search.php":

		if id := query.Get("search_id"); id != "" {
			return phpbbCleanClone(page_url, "/search/"+id+".html"), page_url
		}

		if author := query.Get("author_id"); author != "" {
			return phpbbCleanClone(page_url, "/search/author/"+author+".html"), page_url
		}

	}

	return nil, nil

}

func phpbbRewriteClean(page_url *url.URL) (*url.URL, *url.URL) {

	path := page_url.Path

	var web_url *url.URL

	switch {

	case strings.HasPrefix(path, "/forum/"):

		id, start, ok := phpbbParseDashSuffix(strings.TrimSuffix(strings.TrimPrefix(path, "/forum/"), ".html"))

		if ok == true {
			web_url = phpbbLegacyClone(page_url, "viewforum.php", "f", id, start)
		}

	case strings.HasPrefix(path, "/topic/"):

		id, start, ok := phpbbParseDashSuffix(strings.TrimSuffix(strings.TrimPrefix(path, "/topic/"), ".html"))

		if ok == true {
			web_url = phpbbLegacyClone(page_url, "viewtopic.php", "t", id, start)
		}

	case strings.HasPrefix(path, "/search/author/"):

		author := strings.TrimSuffix(strings.TrimPrefix(path, "/search/author/"), ".html")

		if phpbbIsNumeric(author) == true {
			web_url = phpbbLegacySearchAuthorClone(page_url, author)
		}

	case strings.HasPrefix(path, "/search/"):

		id := strings.TrimSuffix(strings.TrimPrefix(path, "/search/"), ".html")

		if id != "" {
			web_url = phpbbLegacySearchClone(page_url, id)
		}

	}

	if web_url == nil {
		return nil, nil
	}

	return page_url, web_url

}

func phpbbPathToken(page_url *url.URL) string {

	if page_url == nil {
		return ""
	}

	path := strings.ToLower(page_url.Path)

	if strings.Contains(path, "download/file.php") {
		return "download/file.php"
	}

	if strings.Contains(path, "app.php") {
		return "app.php"
	}

	base := filepath.Base(path)

	for _, script := range phpbb_script_names {

		if base == script {
			return base
		}

	}

	return ""

}

func phpbbStripSession(page_url *url.URL) *url.URL {

	query := page_url.Query()

	if query.Has("sid") == false {
		return page_url
	}

	query.Del("sid")

	clone := *page_url
	clone.RawQuery = query.Encode()

	return &clone

}

func phpbbCleanClone(page_url *url.URL, path string) *url.URL {

	clone := *page_url
	clone.Path = path
	clone.RawQuery = ""
	clone.Fragment = ""
	clone.RawFragment = ""

	return &clone

}

func phpbbLegacyClone(page_url *url.URL, script string, parameter string, id string, start string) *url.URL {

	values := url.Values{}
	values.Set(parameter, id)

	if start != "" {
		values.Set("start", start)
	}

	clone := *page_url
	clone.Path = "/" + script
	clone.RawQuery = values.Encode()
	clone.Fragment = ""
	clone.RawFragment = ""

	return &clone

}

func phpbbLegacySearchClone(page_url *url.URL, search_id string) *url.URL {

	values := url.Values{}
	values.Set("search_id", search_id)

	clone := *page_url
	clone.Path = "/search.php"
	clone.RawQuery = values.Encode()
	clone.Fragment = ""
	clone.RawFragment = ""

	return &clone

}

func phpbbLegacySearchAuthorClone(page_url *url.URL, author string) *url.URL {

	values := url.Values{}
	values.Set("author_id", author)
	values.Set("sr", "posts")

	clone := *page_url
	clone.Path = "/search.php"
	clone.RawQuery = values.Encode()
	clone.Fragment = ""
	clone.RawFragment = ""

	return &clone

}

func phpbbForumPath(id string, start string) string {

	if start != "" {
		return "/forum/" + id + "-" + start + ".html"
	}

	return "/forum/" + id + ".html"

}

func phpbbTopicPath(id string, start string) string {

	if start != "" {
		return "/topic/" + id + "-" + start + ".html"
	}

	return "/topic/" + id + ".html"

}

func phpbbParseDashSuffix(input string) (string, string, bool) {

	if input == "" {
		return "", "", false
	}

	idx := strings.LastIndex(input, "-")

	if idx == -1 {
		return input, "", phpbbIsNumeric(input)
	}

	id := input[:idx]
	start := input[idx+1:]

	if phpbbIsNumeric(id) == true && phpbbIsNumeric(start) == true {
		return id, start, true
	}

	return "", "", false

}

func phpbbIsNumeric(input string) bool {

	if input == "" {
		return false
	}

	for _, char := range input {

		if char < '0' || char > '9' {
			return false
		}

	}

	return true

}

func phpbbLooksLikeScript(raw string) bool {

	lower := strings.ToLower(raw)

	for _, script := range phpbb_script_names {

		if strings.Contains(lower, script) {
			return true
		}

	}

	if strings.Contains(lower, "download/file.php") {
		return true
	}

	if strings.Contains(lower, "app.php") {
		return true
	}

	return false

}

func phpbbPageIsTopic(page_url *url.URL) bool {
	return page_url != nil && phpbbPathToken(page_url) == "viewtopic.php"
}

func phpbbRewriteHTMLURL(page_url *url.URL, raw string) (string, bool) {

	if phpbbLooksLikeScript(raw) == false {
		return raw, false
	}

	plain := strings.ReplaceAll(raw, "&amp;", "&")
	parsed, err := url.Parse(plain)

	if err != nil {
		return raw, false
	}

	token := phpbbPathToken(parsed)

	query := parsed.Query()

	switch token {

	case "viewforum.php":

		if f := query.Get("f"); f != "" {
			return phpbbForumPath(f, query.Get("start")), true
		}

	case "viewtopic.php":

		if t := query.Get("t"); t != "" {
			return phpbbTopicPath(t, query.Get("start")), true
		}

		if query.Get("t") == "" && query.Get("p") != "" {

			if phpbbPageIsTopic(page_url) == true {

				if parsed.Fragment != "" {
					return "#" + parsed.Fragment, true
				}

				return "#p" + query.Get("p"), true

			}

		}

	case "search.php":

		if id := query.Get("search_id"); id != "" {
			return "/search/" + id + ".html", true
		}

		if author := query.Get("author_id"); author != "" {
			return "/search/author/" + author + ".html", true
		}

	}

	if token != "" {

		stripped, ok := phpbbStripSessionFromURL(parsed)

		if ok == true {
			return stripped, true
		}

	}

	return raw, false

}

func phpbbStripSessionFromURL(parsed *url.URL) (string, bool) {

	query := parsed.Query()

	if query.Has("sid") == false {
		return "", false
	}

	query.Del("sid")

	result := parsed.Path

	if encoded := query.Encode(); encoded != "" {
		result = result + "?" + encoded
	}

	if parsed.Fragment != "" {
		result = result + "#" + parsed.Fragment
	}

	return result, true

}
