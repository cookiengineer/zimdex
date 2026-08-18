package filters

import "golang.org/x/net/html"
import "strings"

func findHTMLMeta(node *html.Node, name string, content string) bool {

	if node.Type == html.ElementNode && node.Data == "meta" {

		node_name    := ""
		node_content := ""

		for _, attribute := range node.Attr {
			switch strings.ToLower(attribute.Key) {
			case "name":
				node_name = strings.ToLower(attribute.Val)
			case "content":
				node_content = strings.ToLower(attribute.Val)
			}
		}

		if name == node_name && strings.Contains(node_content, content) {
			return true
		}

	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {

		if findHTMLMeta(child, name, content) {
			return true
		}

	}

	return false

}
