package render

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

type Rewriter struct {
	ZimFile  string
	PageURL  *url.URL
}

func NewRewriter(zimFile, pageURL string) (*Rewriter, error) {
	u, err := url.Parse(pageURL)
	if err != nil {
		return nil, fmt.Errorf("invalid page URL %q: %w", pageURL, err)
	}

	if u.Scheme == "" {
		return nil, fmt.Errorf("page URL must have a scheme: %q", pageURL)
	}

	return &Rewriter{
		ZimFile: zimFile,
		PageURL: u,
	}, nil
}

func (rw *Rewriter) Resolve(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)

	if rawURL == "" {
		return ""
	}

	if strings.HasPrefix(rawURL, "#") {
		return rawURL
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	if u.Scheme == "data" || u.Scheme == "mailto" || u.Scheme == "tel" {
		return rawURL
	}

	if u.Scheme == "javascript" {
		return "#"
	}

	if strings.HasPrefix(rawURL, "//") {
		u = &url.URL{
			Scheme: "https",
			Host:   u.Host,
			Path:   u.Path,
		}
	} else if u.Scheme == "" {
		u = rw.PageURL.ResolveReference(u)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return rawURL
	}

	path := u.Path
	if path == "" {
		path = "/"
	}

	return fmt.Sprintf("/%s/%s%s", rw.ZimFile, hostname, path)
}

func RewriteHTML(doc *html.Node, rewriter *Rewriter) {
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			rewriteElement(n, rewriter)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
}

var urlAttrs = map[string]bool{
	"href":     true,
	"src":      true,
	"action":   true,
	"cite":     true,
	"longdesc": true,
	"poster":   true,
	"data-src": true,
	"data-href": true,
}

func rewriteElement(n *html.Node, rewriter *Rewriter) {
	for i, attr := range n.Attr {
		key := strings.ToLower(attr.Key)

		if key == "srcset" {
			n.Attr[i].Val = rewriteSrcSet(attr.Val, rewriter)
			continue
		}

		if key == "style" {
			n.Attr[i].Val = rewriteInlineCSS(attr.Val, rewriter)
			continue
		}

		if urlAttrs[key] {
			resolved := rewriter.Resolve(attr.Val)
			if resolved != attr.Val {
				n.Attr[i].Val = resolved
			}
		}
	}

	if n.DataAtom.String() == "style" {
		text := getTextContent(n)
		if text != "" {
			rewritten := rewriteInlineCSS(text, rewriter)
			n.FirstChild = nil
			n.AppendChild(&html.Node{
				Type: html.TextNode,
				Data: rewritten,
			})
		}
	}
}

func rewriteSrcSet(srcset string, rewriter *Rewriter) string {
	candidates := strings.Split(srcset, ",")
	var rewritten []string

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		parts := strings.Fields(candidate)
		if len(parts) == 0 {
			continue
		}

		urlStr := parts[0]
		resolved := rewriter.Resolve(urlStr)
		parts[0] = resolved
		rewritten = append(rewritten, strings.Join(parts, " "))
	}

	return strings.Join(rewritten, ", ")
}

var cssURLRegex = regexp.MustCompile(`url\(\s*["']?(.*?)["']?\s*\)`)
var cssImportRegex = regexp.MustCompile(`@import\s+["'](.*?)["']`)

func RewriteCSS(cssContent string, rewriter *Rewriter) string {
	cssContent = cssURLRegex.ReplaceAllStringFunc(cssContent, func(match string) string {
		submatch := cssURLRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		rawURL := strings.TrimSpace(submatch[1])

		if rawURL == "" || strings.HasPrefix(rawURL, "data:") {
			return match
		}

		resolved := rewriter.Resolve(rawURL)
		return "url(" + resolved + ")"
	})

	cssContent = cssImportRegex.ReplaceAllStringFunc(cssContent, func(match string) string {
		submatch := cssImportRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		rawURL := strings.TrimSpace(submatch[1])

		if rawURL == "" || strings.HasPrefix(rawURL, "data:") {
			return match
		}

		resolved := rewriter.Resolve(rawURL)
		return `@import "` + resolved + `"`
	})

	return cssContent
}

func rewriteInlineCSS(cssContent string, rewriter *Rewriter) string {
	return RewriteCSS(cssContent, rewriter)
}

func getTextContent(n *html.Node) string {
	var buf strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			buf.WriteString(c.Data)
		}
	}
	return buf.String()
}
