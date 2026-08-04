package archive

import (
	"net/url"
)

type ScraperOptions struct {
	Folder            string
	URL               *url.URL
	RespectRobots     bool
	IgnoreInsecureSSL bool
	PageWorkers       int
	AssetWorkers      int
	Filters           []string
	PageLimit         int
}
