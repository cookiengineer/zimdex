package utils

import (
	net_url "net/url"
	"strings"
	"github.com/cookiengineer/zimdex/internal/filters"
)

func CanonicalizeURL(url *net_url.URL) *net_url.URL {

	if url != nil {

		clone := *url
		clone.Fragment = ""
		clone.RawFragment = ""

		filters.FilterTrackingParameters(&clone)

		clone.Scheme = strings.ToLower(clone.Scheme)
		clone.Host = strings.ToLower(clone.Host)

		if clone.Scheme == "https" && strings.HasSuffix(clone.Host, ":443") {
			clone.Host = strings.TrimSuffix(clone.Host, ":443")
		} else if clone.Scheme == "http" && strings.HasSuffix(clone.Host, ":80") {
			clone.Host = strings.TrimSuffix(clone.Host, ":80")
		}

		if clone.Path == "" {
			clone.Path = "/"
		} else if len(clone.Path) > 1 && strings.HasSuffix(clone.Path, "/") {
			clone.Path = strings.TrimSuffix(clone.Path, "/")
		}

		return &clone

	} else {
		return nil
	}

}
