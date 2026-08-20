package filters

import (
	"net/url"
	"strings"
)

var tracking_parameters = map[string]bool{
	"utm_source":           true,
	"utm_medium":           true,
	"utm_campaign":         true,
	"utm_term":             true,
	"utm_content":          true,
	"utm_id":               true,
	"utm_source_platform":  true,
	"utm_creative_format":  true,
	"utm_marketing_tactic": true,
	"fbclid":               true,
	"gclid":                true,
	"gclsrc":               true,
	"dclid":                true,
	"msclkid":              true,
	"twclid":               true,
	"igshid":               true,
	"mc_cid":               true,
	"mc_eid":               true,
	"ref":                  true,
	"ref_src":              true,
	"ref_url":              true,
	"source":               true,
	"sourceid":             true,
	"pk_source":            true,
	"pk_medium":            true,
	"pk_campaign":          true,
	"pk_keyword":           true,
	"pk_content":           true,
	"_ga":                  true,
	"_gl":                  true,
	"_kx":                  true,
	"oly_anon_id":          true,
	"oly_enc_id":           true,
	"vero_id":              true,
	"vero_conv":            true,
	"wickedid":             true,
	"yclid":                true,
}

type Trackers struct{}

func (filter *Trackers) Name() string {
	return "Trackers"
}

func (filter *Trackers) Description() string {
	return "Strip tracking parameters from URLs and tracking elements from HTML"
}

func (filter *Trackers) IsDefault() bool {
	return true
}

func (filter *Trackers) Detect(_ *url.URL, _ []byte) bool {
	return true
}

func (filter *Trackers) FilterURL(page_url *url.URL) *url.URL {

	if page_url != nil {

		clone := *page_url
		FilterTrackers(&clone)
		return &clone

	} else {
		return nil
	}

}

func (filter *Trackers) FilterHTML(_ *url.URL, content []byte) []byte {

	sanitizer := NewSanitizer(content, SanitizerOptions{
		Metas: []string{
			// Open Graph
			"og:article:author",
			"og:article:expiration_time",
			"og:article:modified_time",
			"og:article:published_time",
			"og:article:section",
			"og:article:tag",
			"og:audio",
			"og:audio:secure_url",
			"og:audio:type",
			"og:audio:url",
			"og:book:author",
			"og:book:isbn",
			"og:book:release_date",
			"og:book:tag",
			"og:business:contact_data:country_name",
			"og:business:contact_data:locality",
			"og:business:contact_data:postal_code",
			"og:business:contact_data:region",
			"og:business:contact_data:street_address",
			"og:country-name",
			"og:description",
			"og:determiner",
			"og:email",
			"og:fax_number",
			"og:image",
			"og:image:alt",
			"og:image:height",
			"og:image:secure_url",
			"og:image:type",
			"og:image:url",
			"og:image:width",
			"og:latitude",
			"og:locale",
			"og:locale:alternate",
			"og:locality",
			"og:longitude",
			"og:music:album",
			"og:music:album:disc",
			"og:music:album:track",
			"og:music:creator",
			"og:music:duration",
			"og:music:musician",
			"og:music:playlist",
			"og:music:playlist:disc",
			"og:music:playlist:track",
			"og:music:playlist:song",
			"og:music:release_date",
			"og:music:song",
			"og:music:song:disc",
			"og:music:song:track",
			"og:phone_number",
			"og:postal-code",
			"og:product:availability",
			"og:product:brand",
			"og:product:category",
			"og:product:color",
			"og:product:condition",
			"og:product:gtin",
			"og:product:material",
			"og:product:mfr_part_no",
			"og:product:pattern",
			"og:product:price:amount",
			"og:product:price:currency",
			"og:product:retailer_item_id",
			"og:product:size",
			"og:profile:first_name",
			"og:profile:gender",
			"og:profile:last_name",
			"og:profile:username",
			"og:region",
			"og:restrictions:age",
			"og:restrictions:content",
			"og:restrictions:country:allowed",
			"og:restrictions:country:disallowed",
			"og:restrictions:location:allowed",
			"og:restrictions:location:disallowed",
			"og:restrictions:relationship:allowed",
			"og:restrictions:relationship:disallowed",
			"og:see_also",
			"og:site_name",
			"og:street-address",
			"og:title",
			"og:ttl",
			"og:type",
			"og:updated_time",
			"og:url",
			"og:video",
			"og:video:actor",
			"og:video:actor:role",
			"og:video:director",
			"og:video:duration",
			"og:video:height",
			"og:video:release_date",
			"og:video:secure_url",
			"og:video:series",
			"og:video:tag",
			"og:video:type",
			"og:video:url",
			"og:video:width",
			"og:video:writer",

			// Facebook
			"fb:admins",
			"fb:app_id",
			"fb:pages",
			"fb:profile_id",

			// Twitter
			"twitter:app:id:googleplay",
			"twitter:app:id:ipad",
			"twitter:app:id:iphone",
			"twitter:app:name:googleplay",
			"twitter:app:name:ipad",
			"twitter:app:name:iphone",
			"twitter:app:url:googleplay",
			"twitter:app:url:ipad",
			"twitter:app:url:iphone",
			"twitter:card",
			"twitter:creator",
			"twitter:creator:id",
			"twitter:data1",
			"twitter:data2",
			"twitter:description",
			"twitter:domain",
			"twitter:image",
			"twitter:image:alt",
			"twitter:image:src",
			"twitter:label1",
			"twitter:label2",
			"twitter:player",
			"twitter:player:height",
			"twitter:player:stream",
			"twitter:player:stream:content_type",
			"twitter:player:width",
			"twitter:site",
			"twitter:site:id",
			"twitter:title",

			// Apple
			"apple-mobile-web-app-capable",
			"apple-mobile-web-app-orientation",
			"apple-mobile-web-app-status-bar-style",
			"apple-mobile-web-app-title",

			// Microsoft
			"msapplication-badge",
			"msapplication-config",
			"msapplication-navbutton-color",
			"msapplication-notification",
			"msapplication-square150x150logo",
			"msapplication-square310x310logo",
			"msapplication-square70x70logo",
			"msapplication-starturl",
			"msapplication-task",
			"msapplication-tilecolor",
			"msapplication-tileimage",
			"msapplication-tooltip",
			"msapplication-wide310x150logo",
		},
		Links: []string{
			"dns-prefetch",
			"preconnect",
			"preload",
			"prefetch",
			"prerender",
		},
	})

	sanitizer.Sanitize()

	return sanitizer.Render()

}

func FilterTrackers(page_url *url.URL) {

	if page_url != nil && page_url.RawQuery != "" {

		values := page_url.Query()
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
