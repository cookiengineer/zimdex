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

func (filter *TrackingParameters) Detect(_ []byte, _ *url.URL) bool {
	return true
}

func (filter *TrackingParameters) FilterURL(link *url.URL) *url.URL {

	if link != nil {

		clone := *link
		FilterTrackingParameters(&clone)
		return &clone

	} else {
		return nil
	}

}

func (filter *TrackingParameters) FilterHTML(html []byte, self *url.URL) []byte {
	return html
}

func FilterTrackingParameters(link *url.URL) {

	if link != nil && link.RawQuery != "" {

		values  := link.Query()
		changed := false

		for key := range values {

			_, ok := tracking_parameters[strings.ToLower(key)]

			if ok == true {
				values.Del(key)
				changed = true
			}

		}

		if changed == true {
			link.RawQuery = values.Encode()
		}
	}

}

