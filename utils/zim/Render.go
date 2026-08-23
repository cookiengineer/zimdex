package zim

import "github.com/cookiengineer/gozim/archive/zim"
import "github.com/cookiengineer/zimdex/filters"
import utils_urls "github.com/cookiengineer/zimdex/utils/urls"
import "fmt"
import net_url "net/url"
import "strings"

func Render(archive *zim.Archive, zim_file string, render_path string) ([]byte, string, error) {

	entry, err0 := ResolveEntry(archive, render_path)

	if err0 == nil {

		item, err1 := entry.Item(true)
		page_url, err2 := resolve_page_url(entry, render_path)

		if err1 == nil && err2 == nil {

			mime_type := item.MimeType()

			if mime_type == "application/octet-stream" {
				tmp_url, _ := net_url.Parse("http://localhost" + entry.Path())
				mime_type = utils_urls.GetMimeType(tmp_url)
			}

			payload, err3 := item.DataAll()

			if err3 == nil {

				render_filters := []filters.Filter{
					&filters.Scripts{},
					&filters.Trackers{},
					&filters.Zim{
						File: zim_file,
					},
				}

				switch {
				case strings.HasPrefix(mime_type, "text/html"):
					return filters.ApplyFilterHTML(render_filters, page_url, payload), "text/html; charset=utf-8", nil
				case strings.HasPrefix(mime_type, "text/css"):
					return filters.ApplyFilterCSS(render_filters, page_url, payload), "text/css; charset=utf-8", nil
				case strings.HasPrefix(mime_type, "application/javascript"),
					strings.HasPrefix(mime_type, "text/javascript"):
					return filters.ApplyFilterJS(render_filters, page_url, payload), mime_type, nil
				default:
					return payload, mime_type, nil
				}

			} else {
				return nil, "application/octet-stream", fmt.Errorf("item data read failed: %s", err3.Error())
			}

		} else {
			return nil, "application/octet-stream", fmt.Errorf("item read failed")
		}

	} else {
		return nil, "application/octet-stream", fmt.Errorf("item resolve failed: %s", err0.Error())
	}

}
