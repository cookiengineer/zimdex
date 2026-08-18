package filters

import (
	"net/url"
	"strings"
)

var tracking_keywords = []string{
	// Generic tracking terms
	"pixel",
	"track",
	"analytics",
	"metrics",
	"telemetry",
	"beacon",
	"pageview",
	"heatmap",
	"conversion",
	"gtag",
	"collect",

	// Self-hostable analytics platforms
	"matomo",
	"piwik",
	"plausible",
	"countly",
	"umami",
	"fathom",
	"goatcounter",
	"ackee",
	"posthog",
	"snowplow",
	"offen",
	"pirsch",
	"shynet",
	"simpleanalytics",
	"openwebanalytics",
	"wideangle",

	// Hosted analytics / marketing trackers
	"statcounter",
	"clicky",
	"mixpanel",
	"amplitude",
	"hotjar",
	"fullstory",
	"mouseflow",
	"crazyegg",
	"metrika",
	"clarity",
	"chartbeat",
}

var tracking_hostnames = []string{
	"google-analytics.com",
	"googletagmanager.com",
	"googleadservices.com",
	"doubleclick.net",
	"facebook.net",
	"fbcdn.net",
	"mc.yandex.ru",
	"hm.baidu.com",
	"cdn.mxpnl.com",
	"cdn.amplitude.com",
	"amplitude.com",
	"cdn.segment.com",
	"segment.com",
	"segment.io",
	"cdn.usefathom.com",
	"zgo.at",
	"clarity.ms",
	"statcounter.com",
	"getclicky.com",
	"crazyegg.com",
	"mouseflow.com",
	"hotjar.com",
	"fullstory.com",
	"hs-scripts.com",
	"hs-analytics.net",
	"snap.licdn.com",
	"px.ads.linkedin.com",
	"static.ads-twitter.com",
	"analytics.twitter.com",
	"s.pinimg.com",
	"ct.pinterest.com",
	"analytics.tiktok.com",
	"www.redditstatic.com",
	"quantserve.com",
	"scorecardresearch.com",
	"chartbeat.com",
	"parsely.com",
	"heapanalytics.com",
}

func IsTrackingURL(link *url.URL) bool {

	if link == nil {
		return false
	}

	hostname := strings.ToLower(link.Hostname())
	path := strings.ToLower(link.Path)

	for _, host := range tracking_hostnames {

		if hostname == host || strings.HasSuffix(hostname, "."+host) {
			return true
		}

	}

	for _, keyword := range tracking_keywords {

		if strings.Contains(hostname, keyword) {
			return true
		}

		if path != "" && strings.Contains(path, keyword) {
			return true
		}

	}

	return false

}
