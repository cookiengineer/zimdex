package filters

import (
	"net/url"
	"strings"
)

type TrackingFilter struct{}

func (f *TrackingFilter) Name() string        { return "tracking_params" }
func (f *TrackingFilter) Description() string { return "Strip tracking parameters (utm_*, fbclid, gclid, etc.)" }
func (f *TrackingFilter) Detect(_ []byte, _ string) bool { return true }

var trackingParams = map[string]bool{
	"utm_source": true, "utm_medium": true, "utm_campaign": true,
	"utm_term": true, "utm_content": true, "utm_id": true,
	"utm_source_platform": true, "utm_creative_format": true, "utm_marketing_tactic": true,
	"fbclid": true, "gclid": true, "gclsrc": true, "dclid": true,
	"msclkid": true, "twclid": true, "igshid": true,
	"mc_cid": true, "mc_eid": true,
	"ref": true, "ref_src": true, "ref_url": true,
	"source": true, "sourceid": true,
	"pk_source": true, "pk_medium": true, "pk_campaign": true,
	"pk_keyword": true, "pk_content": true,
	"_ga": true, "_gl": true, "_kx": true,
	"oly_anon_id": true, "oly_enc_id": true,
	"vero_id": true, "vero_conv": true,
	"wickedid": true, "yclid": true,
}

func FilterTrackingParams(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if u.RawQuery == "" {
		return rawURL
	}

	values := u.Query()
	var removed bool
	for key := range values {
		if trackingParams[strings.ToLower(key)] {
			values.Del(key)
			removed = true
		}
	}
	if !removed {
		return rawURL
	}

	u.RawQuery = values.Encode()
	return u.String()
}

func (f *TrackingFilter) FilterURL(rawURL string) string {
	return FilterTrackingParams(rawURL)
}

func (f *TrackingFilter) FilterHTML(html []byte, _ string) []byte {
	return html
}
