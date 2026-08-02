package archive

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type robotsRule struct {
	path    string
	allowed bool
}

type RobotsMatcher struct {
	rules []robotsRule
}

func (m *RobotsMatcher) IsAllowed(path string) bool {
	bestLen := -1
	allowed := true

	for _, rule := range m.rules {
		if !strings.HasPrefix(path, rule.path) {
			continue
		}

		if len(rule.path) > bestLen {
			bestLen = len(rule.path)
			allowed = rule.allowed
		}
	}

	return allowed
}

func FetchRobotsTxt(hostname string, client *http.Client) (*RobotsMatcher, []string, time.Duration, error) {
	robotsURL := fmt.Sprintf("https://%s/robots.txt", hostname)

	resp, err := client.Get(robotsURL)
	if err != nil {
		return &RobotsMatcher{}, nil, 0, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &RobotsMatcher{}, nil, 0, nil
	}

	matcher := &RobotsMatcher{}
	var sitemaps []string
	var crawlDelay time.Duration
	currentAgent := ""

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}

		key := strings.TrimSpace(strings.ToLower(line[:idx]))
		value := strings.TrimSpace(line[idx+1:])

		switch key {
		case "user-agent":
			currentAgent = strings.ToLower(value)

		case "disallow":
			if currentAgent == "*" || currentAgent == "zimdex" || currentAgent == "" {
				if value != "" {
					matcher.rules = append(matcher.rules, robotsRule{path: value, allowed: false})
				}
			}

		case "allow":
			if currentAgent == "*" || currentAgent == "zimdex" || currentAgent == "" {
				if value != "" {
					matcher.rules = append(matcher.rules, robotsRule{path: value, allowed: true})
				}
			}

		case "crawl-delay":
			if currentAgent == "*" || currentAgent == "zimdex" || currentAgent == "" {
				if sec, err := strconv.ParseFloat(value, 64); err == nil {
					crawlDelay = time.Duration(sec * float64(time.Second))
				}
			}

		case "sitemap":
			sitemaps = append(sitemaps, value)
		}
	}

	return matcher, sitemaps, crawlDelay, nil
}

func FetchSitemaps(baseHost string, sitemapURLs []string, client *http.Client) []string {
	seen := make(map[string]bool)
	var seeds []string

	for _, url := range sitemapURLs {
		fetchSitemap(url, baseHost, client, &seeds, seen, 0)
	}

	return seeds
}

func FetchDefaultSitemap(hostname string, client *http.Client) []string {
	defaultURL := fmt.Sprintf("https://%s/sitemap.xml", hostname)
	var seeds []string
	seen := make(map[string]bool)
	fetchSitemap(defaultURL, hostname, client, &seeds, seen, 0)
	return seeds
}

func fetchSitemap(sitemapURL, baseHost string, client *http.Client, seeds *[]string, seen map[string]bool, depth int) {
	if depth > 3 {
		return
	}

	canonical, err := normalizeSitemapURL(sitemapURL)
	if err != nil || seen[canonical] {
		return
	}
	seen[canonical] = true

	resp, err := client.Get(sitemapURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	data := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			data = append(data, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	urls, isIndex := parseSitemapXML(string(data))

	if isIndex {
		for _, u := range urls {
			fetchSitemap(u, baseHost, client, seeds, seen, depth+1)
		}
	} else {
		*seeds = append(*seeds, urls...)
	}
}

func parseSitemapXML(xmlBody string) (urls []string, isIndex bool) {
	xmlBody = strings.ToLower(xmlBody)

	if strings.Contains(xmlBody, "<sitemapindex") {
		urls = extractXMLTags(xmlBody, "loc")
		return urls, true
	}

	if strings.Contains(xmlBody, "<urlset") {
		urls = extractXMLTags(xmlBody, "loc")
		return urls, false
	}

	return nil, false
}

func extractXMLTags(xml, tag string) []string {
	var result []string
	openTag := "<" + tag + ">"
	closeTag := "</" + tag + ">"

	pos := 0
	for {
		start := indexAfter(xml, openTag, pos)
		if start < 0 {
			break
		}
		end := indexAfter(xml, closeTag, start)
		if end < 0 {
			break
		}

		value := strings.TrimSpace(xml[start : end-len(closeTag)])
		if value != "" {
			result = append(result, value)
		}
		pos = end
	}

	return result
}

func indexAfter(s, substr string, start int) int {
	idx := strings.Index(s[start:], substr)
	if idx < 0 {
		return -1
	}
	return start + idx + len(substr)
}

func normalizeSitemapURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	u.Fragment = ""
	u.RawFragment = ""
	return u.String(), nil
}
