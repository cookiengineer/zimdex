package filters

import "bytes"
import "fmt"
import "net/url"
import "path/filepath"
import "regexp"
import "strings"
import "golang.org/x/net/html"

var vbulletin_script_names = []string{
	"forumdisplay.php",
	"showthread.php",
	"showpost.php",
	"member.php",
	"memberlist.php",
	"search.php",
	"newreply.php",
	"newthread.php",
	"sendmessage.php",
	"private.php",
	"usercp.php",
	"subscription.php",
	"profile.php",
	"login.php",
	"register.php",
	"sendmail.php",
	"inlinemod.php",
	"report.php",
	"reputation.php",
	"moderator.php",
	"misc.php",
	"external.php",
}

var vbulletin_skip_scripts = map[string]bool{
	"showpost.php":     true,
	"member.php":       true,
	"memberlist.php":   true,
	"search.php":       true,
	"newreply.php":     true,
	"newthread.php":    true,
	"sendmessage.php":  true,
	"private.php":      true,
	"usercp.php":       true,
	"subscription.php": true,
	"profile.php":      true,
	"login.php":        true,
	"register.php":     true,
	"sendmail.php":     true,
	"inlinemod.php":    true,
	"report.php":       true,
	"reputation.php":   true,
	"moderator.php":    true,
	"misc.php":         true,
	"external.php":     true,
}

var vbulletin_filter_parameters = map[string]bool{
	"s":         true, // session id
	"goto":      true, // showthread: goto=newpost
	"highlight": true, // search highlight
	"pp":        true, // posts per page
	"order":     true, // forum display order
	"sort":      true, // sort field
	"daysprune": true, // thread age filter
}

var vbulletin_friendly_forum_regex = regexp.MustCompile(`^/f(\d+)/?$`)
var vbulletin_friendly_forum_page_regex = regexp.MustCompile(`^/f(\d+)/i(\d+)\.html$`)
var vbulletin_friendly_thread_regex = regexp.MustCompile(`^/f(\d+)/(.+)\.html$`)
var vbulletin_friendly_user_regex = regexp.MustCompile(`^/users/\d+/`)
var vbulletin_friendly_any_regex = regexp.MustCompile(`/f\d+|/users/\d+|/attachments/`)

type VBulletin struct{}

func (filter *VBulletin) Name() string {
	return "VBulletin"
}

func (filter *VBulletin) Description() string {
	return "Filter vBulletin session links and rewrite legacy and friendly URLs to clean paths"
}

func (filter *VBulletin) IsDefault() bool {
	return false
}

func (filter *VBulletin) Detect(page_url *url.URL, content []byte) bool {

	if len(content) > 0 {

		if bytes.Contains(content, []byte("clientscript/vbulletin")) {
			return true
		}

		if bytes.Contains(content, []byte(`id="vbulletin`)) {
			return true
		}

		document, err := html.Parse(bytes.NewReader(content))

		if err == nil && findHTMLMeta(document, "generator", "vbulletin") {
			return true
		}

	}

	if page_url != nil {
		return vbulletinIsVBURL(page_url)
	}

	return false

}

func (filter *VBulletin) FilterURL(page_url *url.URL) *url.URL {

	if page_url == nil {
		return nil
	}

	path := strings.ToLower(page_url.Path)

	if vbulletin_friendly_user_regex.MatchString(path) || strings.Contains(path, "/attachments/") {
		return nil
	}

	if vbulletinFriendlyForum(path) || vbulletinFriendlyThread(path) {
		return page_url
	}

	base := filepath.Base(path)

	if vbulletin_skip_scripts[base] {
		return nil
	}

	if base == "showthread.php" {

		query := page_url.Query()

		if query.Get("t") == "" && query.Get("p") != "" {
			return nil
		}

	}

	if base == "forumdisplay.php" || base == "showthread.php" {
		return vbulletinStripParams(page_url)
	}

	return page_url

}

var vbulletin_html_link_regex = regexp.MustCompile(`(?i)(href|src|action)=["']([^"'\s]*)["']`)

func (filter *VBulletin) FilterHTML(page_url *url.URL, content []byte) []byte {

	result := []byte(content)

	result = vbulletin_html_link_regex.ReplaceAllFunc(result, func(input []byte) []byte {

		parts := vbulletin_html_link_regex.FindSubmatch(input)

		if len(parts) < 3 {
			return input
		}

		attribute := string(parts[1])
		raw := string(parts[2])

		rewritten, changed := vbulletinRewriteHTMLURL(page_url, raw)

		if changed == false {
			return input
		}

		return []byte(fmt.Sprintf(`%s="%s"`, attribute, rewritten))

	})

	return result

}

func (filter *VBulletin) RewriteURL(page_url *url.URL) (*url.URL, *url.URL) {

	if page_url == nil {
		return nil, nil
	}

	if zim_url, web_url := vbulletinRewriteLegacy(page_url); zim_url != nil {
		return zim_url, web_url
	}

	if zim_url, web_url := vbulletinRewriteFriendly(page_url); zim_url != nil {
		return zim_url, web_url
	}

	if zim_url, web_url := vbulletinRewriteClean(page_url); zim_url != nil {
		return zim_url, web_url
	}

	return nil, nil

}

func vbulletinRewriteLegacy(page_url *url.URL) (*url.URL, *url.URL) {

	base := filepath.Base(strings.ToLower(page_url.Path))
	query := page_url.Query()

	switch base {

	case "forumdisplay.php":

		if f := query.Get("f"); f != "" {
			return vbulletinCleanClone(page_url, vbulletinForumPath(f, query.Get("page"))), page_url
		}

	case "showthread.php":

		if t := query.Get("t"); t != "" {
			return vbulletinCleanClone(page_url, vbulletinThreadPath(t, query.Get("page"))), page_url
		}

	}

	return nil, nil

}

func vbulletinRewriteFriendly(page_url *url.URL) (*url.URL, *url.URL) {

	path := strings.ToLower(page_url.Path)

	if id, page, ok := vbulletinParseFriendlyForum(path); ok {
		return vbulletinCleanClone(page_url, vbulletinForumPath(id, page)), page_url
	}

	if id, page, ok := vbulletinParseFriendlyThread(path); ok {
		return vbulletinCleanClone(page_url, vbulletinThreadPath(id, page)), page_url
	}

	return nil, nil

}

func vbulletinRewriteClean(page_url *url.URL) (*url.URL, *url.URL) {

	path := page_url.Path

	var web_url *url.URL

	switch {

	case strings.HasPrefix(path, "/forum/"):

		id, page, ok := vbulletinParseDashSuffix(strings.TrimSuffix(strings.TrimPrefix(path, "/forum/"), ".html"))

		if ok == true {
			web_url = vbulletinLegacyClone(page_url, "forumdisplay.php", "f", id, page)
		}

	case strings.HasPrefix(path, "/thread/"):

		id, page, ok := vbulletinParseDashSuffix(strings.TrimSuffix(strings.TrimPrefix(path, "/thread/"), ".html"))

		if ok == true {
			web_url = vbulletinLegacyClone(page_url, "showthread.php", "t", id, page)
		}

	}

	if web_url == nil {
		return nil, nil
	}

	return page_url, web_url

}

func vbulletinIsVBURL(page_url *url.URL) bool {

	if page_url == nil {
		return false
	}

	path := strings.ToLower(page_url.Path)

	if strings.Contains(path, "/attachments/") {
		return true
	}

	if vbulletinFriendlyForum(path) || vbulletinFriendlyThread(path) || vbulletin_friendly_user_regex.MatchString(path) {
		return true
	}

	base := filepath.Base(path)

	for _, script := range vbulletin_script_names {

		if base == script {
			return true
		}

	}

	return false

}

func vbulletinFriendlyForum(path string) bool {
	_, _, ok := vbulletinParseFriendlyForum(path)
	return ok
}

func vbulletinFriendlyThread(path string) bool {
	_, _, ok := vbulletinParseFriendlyThread(path)
	return ok
}

func vbulletinParseFriendlyForum(path string) (string, string, bool) {

	if match := vbulletin_friendly_forum_regex.FindStringSubmatch(path); match != nil {
		return match[1], "", true
	}

	if match := vbulletin_friendly_forum_page_regex.FindStringSubmatch(path); match != nil {
		return match[1], match[2], true
	}

	return "", "", false

}

func vbulletinParseFriendlyThread(path string) (string, string, bool) {

	match := vbulletin_friendly_thread_regex.FindStringSubmatch(path)

	if match == nil {
		return "", "", false
	}

	slug := match[2]

	parts := strings.Split(slug, "-")

	if len(parts) < 2 {
		return "", "", false
	}

	last := parts[len(parts)-1]
	prev := parts[len(parts)-2]

	if vbulletinIsNumeric(last) == true && vbulletinIsNumeric(prev) == true && len(parts) >= 3 {
		return prev, last, true
	}

	if vbulletinIsNumeric(last) == true {
		return last, "", true
	}

	return "", "", false

}

func vbulletinStripParams(page_url *url.URL) *url.URL {

	query := page_url.Query()
	changed := false

	for key := range query {

		if vbulletin_filter_parameters[key] == true {
			query.Del(key)
			changed = true
		}

	}

	if changed == true {

		clone := *page_url
		clone.RawQuery = query.Encode()

		return &clone

	}

	return page_url

}

func vbulletinStripParamsFromURL(parsed *url.URL) (string, bool) {

	query := parsed.Query()
	changed := false

	for key := range query {

		if vbulletin_filter_parameters[key] == true {
			query.Del(key)
			changed = true
		}

	}

	if changed == false {
		return "", false
	}

	result := parsed.Path

	if encoded := query.Encode(); encoded != "" {
		result = result + "?" + encoded
	}

	if parsed.Fragment != "" {
		result = result + "#" + parsed.Fragment
	}

	return result, true

}

func vbulletinCleanClone(page_url *url.URL, path string) *url.URL {

	clone := *page_url
	clone.Path = path
	clone.RawQuery = ""
	clone.Fragment = ""
	clone.RawFragment = ""

	return &clone

}

func vbulletinLegacyClone(page_url *url.URL, script string, parameter string, id string, page string) *url.URL {

	values := url.Values{}
	values.Set(parameter, id)

	if page != "" && page != "1" {
		values.Set("page", page)
	}

	clone := *page_url
	clone.Path = "/" + script
	clone.RawQuery = values.Encode()
	clone.Fragment = ""
	clone.RawFragment = ""

	return &clone

}

func vbulletinForumPath(id string, page string) string {

	if page != "" && page != "1" {
		return "/forum/" + id + "-" + page + ".html"
	}

	return "/forum/" + id + ".html"

}

func vbulletinThreadPath(id string, page string) string {

	if page != "" && page != "1" {
		return "/thread/" + id + "-" + page + ".html"
	}

	return "/thread/" + id + ".html"

}

func vbulletinParseDashSuffix(input string) (string, string, bool) {

	if input == "" {
		return "", "", false
	}

	idx := strings.LastIndex(input, "-")

	if idx == -1 {
		return input, "", vbulletinIsNumeric(input)
	}

	id := input[:idx]
	page := input[idx+1:]

	if vbulletinIsNumeric(id) == true && vbulletinIsNumeric(page) == true {
		return id, page, true
	}

	return "", "", false

}

func vbulletinIsNumeric(input string) bool {

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

func vbulletinLooksLikeVB(raw string) bool {

	lower := strings.ToLower(raw)

	for _, script := range vbulletin_script_names {

		if strings.Contains(lower, script) {
			return true
		}

	}

	if strings.Contains(lower, "/attachments/") {
		return true
	}

	return vbulletin_friendly_any_regex.MatchString(lower)

}

func vbulletinPageIsThread(page_url *url.URL) bool {

	if page_url == nil {
		return false
	}

	path := strings.ToLower(page_url.Path)

	base := filepath.Base(path)

	if base == "showthread.php" && page_url.Query().Get("t") != "" {
		return true
	}

	return vbulletinFriendlyThread(path)

}

func vbulletinRewriteHTMLURL(page_url *url.URL, raw string) (string, bool) {

	if vbulletinLooksLikeVB(raw) == false {
		return raw, false
	}

	plain := strings.ReplaceAll(raw, "&amp;", "&")
	parsed, err := url.Parse(plain)

	if err != nil {
		return raw, false
	}

	path := strings.ToLower(parsed.Path)

	if id, page, ok := vbulletinParseFriendlyForum(path); ok {
		return vbulletinForumPath(id, page), true
	}

	if id, page, ok := vbulletinParseFriendlyThread(path); ok {
		return vbulletinThreadPath(id, page), true
	}

	base := filepath.Base(path)
	query := parsed.Query()

	switch base {

	case "forumdisplay.php":

		if f := query.Get("f"); f != "" {
			return vbulletinForumPath(f, query.Get("page")), true
		}

	case "showthread.php":

		if t := query.Get("t"); t != "" {
			return vbulletinThreadPath(t, query.Get("page")), true
		}

		if query.Get("t") == "" && query.Get("p") != "" {

			if vbulletinPageIsThread(page_url) == true {
				return "#post" + query.Get("p"), true
			}

		}

	}

	if vbulletinIsVBURL(parsed) == true {

		stripped, ok := vbulletinStripParamsFromURL(parsed)

		if ok == true {
			return stripped, true
		}

	}

	return raw, false

}
