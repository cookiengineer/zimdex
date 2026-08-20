package filters

import "net/url"

type Filter interface {
	Name() string
	Description() string
	IsDefault() bool
	Detect(*url.URL, []byte) bool
	FilterURL(*url.URL) *url.URL
	FilterHTML(*url.URL, []byte) []byte
}

