package filters

import (
	"net/url"
	"testing"
)

func TestIsTrackingURLNil(t *testing.T) {
	if IsTrackingURL(nil) {
		t.Error("IsTrackingURL(nil) = true, want false")
	}
}

func TestIsTrackingURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		// Known hosted tracking domains
		{"google-analytics", "https://www.google-analytics.com/collect", true},
		{"googletagmanager", "https://www.googletagmanager.com/gtm.js", true},
		{"facebook-pixel", "https://connect.facebook.net/en_US/fbevents.js", true},
		{"doubleclick", "https://ad.doubleclick.net/pixel", true},
		{"yandex-metrica", "https://mc.yandex.ru/metrika/watch.js", true},
		{"baidu-analytics", "https://hm.baidu.com/hm.js", true},
		{"segment", "https://cdn.segment.com/analytics.js/v1/x/analytics.min.js", true},
		{"tiktok-pixel", "https://analytics.tiktok.com/i18n/pixel/events.js", true},

		// Self-hostable analytics by hostname
		{"matomo-hostname", "https://matomo.example.com/matomo.js", true},
		{"piwik-hostname", "https://piwik.example.com/piwik.php", true},
		{"plausible-hostname", "https://plausible.example.com/js/script.js", true},
		{"countly-hostname", "https://countly.example.com/sdk/web/countly.js", true},
		{"umami-hostname", "https://umami.example.com/script.js", true},

		// Self-hostable analytics by path
		{"matomo-path", "https://example.com/matomo/matomo.js", true},
		{"piwik-path", "https://example.com/piwik.php", true},
		{"plausible-path", "https://example.com/js/plausible.js", true},
		{"countly-path", "https://example.com/countly/countly.js", true},
		{"umami-path", "https://example.com/umami.js", true},
		{"fathom-path", "https://example.com/fathom.js", true},
		{"posthog-path", "https://example.com/static/posthog.js", true},
		{"gtag-path", "https://example.com/js/gtag.js", true},
		{"analytics-path", "https://example.com/analytics/collect", true},
		{"pixel-path", "https://example.com/static/pixel.gif", true},

		// Generic tracking terms in hostname
		{"analytics-hostname", "https://analytics.example.com/collect", true},
		{"track-hostname", "https://track.example.com/event", true},

		// Non-tracking URLs
		{"plain-image", "https://example.com/images/logo.png", false},
		{"plain-page", "https://example.com/about", false},
		{"plain-script", "https://example.com/js/app.js", false},
		{"stylesheet", "https://example.com/style.css", false},
		{"static-host", "https://static.example.com/assets/main.js", false},
		{"github", "https://github.com/cookiengineer", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.raw)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", tt.raw, err)
			}

			if got := IsTrackingURL(u); got != tt.want {
				t.Errorf("IsTrackingURL(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}
