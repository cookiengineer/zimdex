package render

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type FilterConfig struct {
	RemoveElements       []string
	RemoveAttributes     []string
	RemoveMetaNames      []string
	RemoveLinkRels       []string
	RemoveComments       bool
	RemoveTrackingImages bool
	RemoveEmptyElements  bool
}

var DefaultFilters = FilterConfig{
	RemoveElements: []string{"script", "iframe", "object", "embed", "applet"},
	RemoveAttributes: []string{
		"onclick", "ondblclick", "onmousedown", "onmouseup", "onmouseover", "onmouseout",
		"onmousemove", "onkeydown", "onkeyup", "onkeypress", "onfocus", "onblur",
		"onchange", "onsubmit", "onreset", "onselect", "onload", "onunload", "onerror",
		"onabort", "onscroll", "onresize", "onbeforeunload", "onhashchange",
	},
	RemoveMetaNames: []string{
		"twitter:*", "og:*", "fb:*", "apple-mobile-web-app-*", "msapplication-*",
	},
	RemoveLinkRels:       []string{"dns-prefetch", "preconnect", "preload", "prefetch", "prerender"},
	RemoveComments:       true,
	RemoveTrackingImages: true,
	RemoveEmptyElements:  true,
}

func isElementRemoved(tag string, remove []string) bool {
	for _, r := range remove {
		if strings.EqualFold(tag, r) {
			return true
		}
	}
	return false
}

func shouldRemoveMeta(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchPattern(name, pattern) {
			return true
		}
	}
	return false
}

func shouldRemoveLink(rel string, rels []string) bool {
	for _, r := range rels {
		if strings.EqualFold(rel, r) {
			return true
		}
	}
	return false
}

func matchPattern(name, pattern string) bool {
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix))
	}
	return strings.EqualFold(name, pattern)
}

func isTrackingImage(node *html.Node) bool {
	if node.DataAtom != atom.Img {
		return false
	}

	width := ""
	height := ""
	src := ""

	for _, attr := range node.Attr {
		switch strings.ToLower(attr.Key) {
		case "width":
			width = attr.Val
		case "height":
			height = attr.Val
		case "src":
			src = strings.ToLower(attr.Val)
		}
	}

	if width == "1" && height == "1" {
		return true
	}

	if strings.Contains(src, "pixel") || strings.Contains(src, "tracking") || strings.Contains(src, "analytics") {
		return true
	}

	return false
}

func applyFilters(node *html.Node, config FilterConfig) *html.Node {
	if node == nil {
		return nil
	}

	var child *html.Node

	for c := node.FirstChild; c != nil; c = child {
		child = c.NextSibling

		if config.RemoveComments && c.Type == html.CommentNode {
			node.RemoveChild(c)
			continue
		}

		if c.Type == html.ElementNode {
			tag := c.Data

			if isElementRemoved(tag, config.RemoveElements) {
				node.RemoveChild(c)
				continue
			}

			if tag == "meta" && config.RemoveMetaNames != nil {
				name := ""
				for _, attr := range c.Attr {
					if strings.EqualFold(attr.Key, "name") {
						name = attr.Val
						break
					}
				}
				if name != "" && shouldRemoveMeta(name, config.RemoveMetaNames) {
					node.RemoveChild(c)
					continue
				}
			}

			if tag == "link" && config.RemoveLinkRels != nil {
				rel := ""
				for _, attr := range c.Attr {
					if strings.EqualFold(attr.Key, "rel") {
						rel = attr.Val
						break
					}
				}
				if rel != "" && shouldRemoveLink(rel, config.RemoveLinkRels) {
					node.RemoveChild(c)
					continue
				}
			}

			if config.RemoveTrackingImages && isTrackingImage(c) {
				node.RemoveChild(c)
				continue
			}

			var del []int
			for i, attr := range c.Attr {
				for _, remove := range config.RemoveAttributes {
					if strings.EqualFold(attr.Key, remove) {
						del = append(del, i)
					}
				}
			}
			for i := len(del) - 1; i >= 0; i-- {
				idx := del[i]
				c.Attr = append(c.Attr[:idx], c.Attr[idx+1:]...)
			}
		}

		if c.Type == html.TextNode {
			if strings.TrimSpace(c.Data) == "" {
				continue
			}
		}

		applyFilters(c, config)

		if config.RemoveEmptyElements && c.Type == html.ElementNode && c.FirstChild == nil {
			isVoid := false
			for _, void := range []atom.Atom{atom.Br, atom.Hr, atom.Img, atom.Input, atom.Meta, atom.Link, atom.Area, atom.Base, atom.Col, atom.Embed, atom.Source, atom.Track, atom.Wbr} {
				if c.DataAtom == void {
					isVoid = true
					break
				}
			}
			if !isVoid {
				node.RemoveChild(c)
			}
		}
	}

	return node
}

func ApplyFilters(doc *html.Node, config FilterConfig) {
	applyFilters(doc, config)
}
