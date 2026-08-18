package filters

import "golang.org/x/net/html"
import "bytes"
import "net/url"
import "strings"

type SanitizerOptions struct {
	Elements   []string
	Attributes []string
	Links      []string
	Metas      []string
}

type Sanitizer struct {
	elements   map[string]struct{}
	attributes map[string]struct{}
	links      map[string]struct{}
	metas      map[string]struct{}
	document   *html.Node
}

func NewSanitizer(content []byte, options SanitizerOptions) *Sanitizer {

	document, err := html.Parse(bytes.NewReader(content))

	if err == nil {

		sanitizer := &Sanitizer{
			document:   document,
			elements:   make(map[string]struct{}),
			attributes: make(map[string]struct{}),
			links:      make(map[string]struct{}),
			metas:      make(map[string]struct{}),
		}

		for _, element := range options.Elements {
			sanitizer.elements[element] = struct{}{}
		}

		for _, attribute := range options.Attributes {
			sanitizer.attributes[attribute] = struct{}{}
		}

		for _, link := range options.Links {
			sanitizer.links[link] = struct{}{}
		}

		for _, meta := range options.Metas {
			sanitizer.metas[meta] = struct{}{}
		}

		return sanitizer

	} else {
		return nil
	}

}

func (sanitizer *Sanitizer) Render() []byte {

	result := make([]byte, 0)

	if sanitizer.document != nil {

		buffer := bytes.Buffer{}
		err := html.Render(&buffer, sanitizer.document)

		if err == nil {
			return buffer.Bytes()
		}

	}

	return result

}

func (sanitizer *Sanitizer) Sanitize() bool {

	if sanitizer.document != nil {

		tmp := sanitizer.walk_and_sanitize(sanitizer.document)

		if tmp != nil {

			sanitizer.document = tmp

			return true

		}

	}

	return false

}

func (sanitizer *Sanitizer) walk_and_sanitize(node *html.Node) *html.Node {

	if node != nil {

		for child := node.FirstChild; child != nil; {

			next := child.NextSibling

			if child.Type == html.CommentNode {

				node.RemoveChild(child)
				child = next
				continue

			} else if child.Type == html.ElementNode {

				tag := child.Data

				// Remove element by tag
				if _, ok := sanitizer.elements[tag]; ok {
					node.RemoveChild(child)
					child = next
					continue
				}

				if tag == "img" {

					width := ""
					height := ""
					src := ""

					for _, attribute := range child.Attr {

						switch strings.ToLower(attribute.Key) {
						case "width":
							width = strings.ToLower(attribute.Val)
						case "height":
							height = strings.ToLower(attribute.Val)
						case "src":
							src = strings.ToLower(attribute.Val)
						}

					}

					if width == "1" && height == "1" {
						node.RemoveChild(child)
						child = next
						continue
					}

					src_url, err := url.Parse(src)

					if err == nil {

						if IsTrackingURL(src_url) == true {
							node.RemoveChild(child)
							child = next
							continue
						}

					}

				}

				if tag == "meta" && len(sanitizer.metas) > 0 {

					name := ""

					for _, attribute := range child.Attr {

						tmp := strings.ToLower(attribute.Key)

						if tmp == "name" || tmp == "property" {
							name = strings.ToLower(attribute.Val)
							break
						}

					}

					if name != "" {

						if _, ok := sanitizer.metas[name]; ok {
							node.RemoveChild(child)
							child = next
							continue
						}

					}

				}

				if tag == "link" {

					rel := ""

					for _, attribute := range child.Attr {

						tmp := strings.ToLower(attribute.Key)

						if tmp == "rel" {
							rel = strings.ToLower(attribute.Val)
							break
						}

					}

					if rel != "" {

						if _, ok := sanitizer.links[rel]; ok {
							node.RemoveChild(child)
							child = next
							continue
						}

					}

				}

				deletions := make([]int, 0)

				for index, attribute := range child.Attr {

					tmp := strings.ToLower(attribute.Key)

					if _, ok := sanitizer.attributes[tmp]; ok {
						deletions = append(deletions, index)
					}

				}

				for d := len(deletions) - 1; d >= 0; d-- {
					idx := deletions[d]
					child.Attr = append(child.Attr[:idx], child.Attr[idx+1:]...)
				}

			}

			sanitizer.walk_and_sanitize(child)
			child = next

		}

		return node

	} else {
		return nil
	}

}
