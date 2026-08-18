package zimfs

import (
	"bytes"

	"golang.org/x/net/html"
)

func findHTMLNode(node *html.Node, search string) string {

	if node.Type == html.ElementNode && node.Data == search {

		if node.FirstChild != nil && node.FirstChild.Type == html.TextNode {
			return node.FirstChild.Data
		}

		var buf bytes.Buffer

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.TextNode {
				buf.WriteString(child.Data)
			}
		}

		return buf.String()

	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {

		if title := findHTMLNode(child, search); title != "" {
			return title
		}

	}

	return ""

}

