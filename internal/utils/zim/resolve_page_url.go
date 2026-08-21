package zim

import "github.com/cookiengineer/gozim/archive/zim"
import "fmt"
import "net/url"
import "strings"

func resolve_page_url(entry *zim.Entry, render_path string) (*url.URL, error) {

	path := entry.Path()

	if strings.HasPrefix(path, "C/") {
		path = path[2:]
	}

	if strings.Contains(path, "://") {

		index := strings.Index(path, "://")

		return url.Parse(fmt.Sprintf("https://%s", path[index+3:]))

	} else if strings.Contains(path, "/") {

		return url.Parse(fmt.Sprintf("https://%s", path))

	} else if strings.Contains(render_path, "/") {

		return url.Parse(fmt.Sprintf("https://%s", render_path))

	}

	return url.Parse(fmt.Sprintf("https://%s", render_path))

}
