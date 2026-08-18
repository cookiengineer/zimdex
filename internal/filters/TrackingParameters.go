package filters

import (
	"net/url"
	"strings"
)

var tracking_parameters = map[string]bool{
	"utm_source": true,
	"utm_medium": true,
	"utm_campaign": true,
	"utm_term": true,
	"utm_content": true,
	"utm_id": true,
	"utm_source_platform": true,
	"utm_creative_format": true,
	"utm_marketing_tactic": true,
	"fbclid": true,
	"gclid": true,
	"gclsrc": true,
	"dclid": true,
	"msclkid": true,
	"twclid": true,
	"igshid": true,
	"mc_cid": true,
	"mc_eid": true,
	"ref": true,
	"ref_src": true,
	"ref_url": true,
	"source": true,
	"sourceid": true,
	"pk_source": true,
	"pk_medium": true,
	"pk_campaign": true,
	"pk_keyword": true,
	"pk_content": true,
	"_ga": true,
	"_gl": true,
	"_kx": true,
	"oly_anon_id": true,
	"oly_enc_id": true,
	"vero_id": true,
	"vero_conv": true,
	"wickedid": true,
	"yclid": true,
}

type TrackingParameters struct{}

func (filter *TrackingParameters) Name() string {
	return "TrackingParameters"
}

func (filter *TrackingParameters) Description() string {
	return "Strip tracking parameters (utm_*, fbclid, gclid, etc.)"
}

func (filter *TrackingParameters) Detect(_ *url.URL, _ []byte) bool {
	return true
}

func (filter *TrackingParameters) FilterURL(page_url *url.URL) *url.URL {

	if page_url != nil {

		clone := *page_url
		FilterTrackingParameters(&clone)
		return &clone

	} else {
		return nil
	}

}

func (filter *TrackingParameters) FilterHTML(page_url *url.URL, html []byte) []byte {
	return html
}

func FilterTrackingParameters(page_url *url.URL) {

	if page_url != nil && page_url.RawQuery != "" {

		values  := page_url.Query()
		changed := false

		for key := range values {

			_, ok := tracking_parameters[strings.ToLower(key)]

			if ok == true {
				values.Del(key)
				changed = true
			}

		}

		if changed == true {
			page_url.RawQuery = values.Encode()
		}

	}

}

