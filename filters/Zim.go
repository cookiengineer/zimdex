package filters

import "bytes"
import "fmt"
import "net/url"
import "regexp"
import "strings"
import "golang.org/x/net/html"

type Zim struct {
	File string
}

func (filter *Zim) Name() string {
	return "Zim"
}

func (filter *Zim) Description() string {
	return "Rewrite URLs in HTML, CSS and JS to offline render paths"
}

func (filter *Zim) IsDefault() bool {
	return false
}

func (filter *Zim) Detect(_ *url.URL, _ []byte) bool {
	return true
}

func (filter *Zim) FilterURL(page_url *url.URL) *url.URL {
	return page_url
}

func (filter *Zim) Resolve(page_url *url.URL, raw_url string) string {

	raw_url = strings.TrimSpace(raw_url)

	if raw_url == "" {
		return ""
	}

	if strings.HasPrefix(raw_url, "#") {
		return raw_url
	}

	link, err := url.Parse(raw_url)

	if err != nil {
		return raw_url
	}

	if link.Scheme == "data" || link.Scheme == "mailto" || link.Scheme == "tel" {
		return raw_url
	}

	if link.Scheme == "javascript" {
		return "#"
	}

	if strings.HasPrefix(raw_url, "//") {

		link = &url.URL{
			Scheme: "https",
			Host:   link.Host,
			Path:   link.Path,
		}

	} else if link.Scheme == "" {
		link = page_url.ResolveReference(link)
	}

	hostname := link.Hostname()

	if hostname == "" {
		return raw_url
	}

	path := link.Path

	if path == "" {
		path = "/"
	}

	return fmt.Sprintf("/%s/%s%s", filter.File, hostname, path)

}

func (filter *Zim) FilterHTML(page_url *url.URL, content []byte) []byte {

	document, err := html.Parse(bytes.NewReader(content))

	if err != nil {
		return content
	}

	filter.rewriteHTML(document, page_url)

	buffer := bytes.Buffer{}

	if html.Render(&buffer, document) == nil {
		return buffer.Bytes()
	}

	return content

}

func (filter *Zim) rewriteHTML(node *html.Node, page_url *url.URL) {

	var walk func(*html.Node)

	walk = func(current *html.Node) {

		if current.Type == html.ElementNode {
			filter.rewriteElement(current, page_url)
		}

		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}

	}

	walk(node)

}

var offline_url_attributes = map[string]bool{
	"href":      true,
	"src":       true,
	"action":    true,
	"cite":      true,
	"longdesc":  true,
	"poster":    true,
	"data-src":  true,
	"data-href": true,
}

func (filter *Zim) rewriteElement(node *html.Node, page_url *url.URL) {

	for index, attribute := range node.Attr {

		key := strings.ToLower(attribute.Key)

		if key == "srcset" {
			node.Attr[index].Val = filter.rewriteSrcSet(attribute.Val, page_url)
			continue
		}

		if key == "style" {
			node.Attr[index].Val = filter.rewrite_css(attribute.Val, page_url)
			continue
		}

		if offline_url_attributes[key] == true {

			resolved := filter.Resolve(page_url, attribute.Val)

			if resolved != attribute.Val {
				node.Attr[index].Val = resolved
			}

		}

	}

	if node.Data == "style" {

		text := getTextContent(node)

		if text != "" {

			rewritten := filter.rewrite_css(text, page_url)

			for child := node.FirstChild; child != nil; {
				next := child.NextSibling
				node.RemoveChild(child)
				child = next
			}

			node.AppendChild(&html.Node{
				Type: html.TextNode,
				Data: rewritten,
			})

		}

	}

}

func (filter *Zim) rewriteSrcSet(srcset string, page_url *url.URL) string {

	candidates := strings.Split(srcset, ",")
	rewritten := make([]string, 0)

	for _, candidate := range candidates {

		candidate = strings.TrimSpace(candidate)

		parts := strings.Fields(candidate)

		if len(parts) == 0 {
			continue
		}

		url_str := parts[0]
		resolved := filter.Resolve(page_url, url_str)
		parts[0] = resolved

		rewritten = append(rewritten, strings.Join(parts, " "))

	}

	return strings.Join(rewritten, ", ")

}

var offline_css_url_regex = regexp.MustCompile(`url\(\s*["']?(.*?)["']?\s*\)`)
var offline_css_import_regex = regexp.MustCompile(`@import\s+["'](.*?)["']`)

func (filter *Zim) FilterCSS(page_url *url.URL, content []byte) []byte {
	return []byte(filter.rewrite_css(string(content), page_url))
}

func (filter *Zim) rewrite_css(css string, page_url *url.URL) string {

	css = offline_css_url_regex.ReplaceAllStringFunc(css, func(match string) string {

		submatch := offline_css_url_regex.FindStringSubmatch(match)

		if len(submatch) < 2 {
			return match
		}

		raw_url := strings.TrimSpace(submatch[1])

		if raw_url == "" || strings.HasPrefix(raw_url, "data:") {
			return match
		}

		resolved := filter.Resolve(page_url, raw_url)

		return "url(" + resolved + ")"

	})

	css = offline_css_import_regex.ReplaceAllStringFunc(css, func(match string) string {

		submatch := offline_css_import_regex.FindStringSubmatch(match)

		if len(submatch) < 2 {
			return match
		}

		raw_url := strings.TrimSpace(submatch[1])

		if raw_url == "" || strings.HasPrefix(raw_url, "data:") {
			return match
		}

		resolved := filter.Resolve(page_url, raw_url)

		return `@import "` + resolved + `"`

	})

	return css

}

func (filter *Zim) FilterJS(page_url *url.URL, content []byte) []byte {
	return rewrite_js_imports(content, func(raw_url string) string {
		return filter.Resolve(page_url, raw_url)
	})
}

func getTextContent(node *html.Node) string {

	var buffer strings.Builder

	for child := node.FirstChild; child != nil; child = child.NextSibling {

		if child.Type == html.TextNode {
			buffer.WriteString(child.Data)
		}

	}

	return buffer.String()

}
