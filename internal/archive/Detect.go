package archive

import "github.com/cookiengineer/zimdex/filters"
import "crypto/tls"
import "fmt"
import "io"
import "net/http"
import "net/url"
import "time"

func DetectFilters(start_url *url.URL) ([]string, error) {

	if start_url == nil {
		return nil, fmt.Errorf("URL is required")
	}

	if start_url.Scheme != "http" && start_url.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme: %s", start_url.Scheme)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	request, err := http.NewRequest("GET", start_url.String(), nil)

	if err != nil {
		return nil, err
	}

	request.Header.Set("User-Agent", randomUA())
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")

	response, err := client.Do(request)

	if err != nil {
		return filters.Detect(start_url, nil), nil
	}

	defer response.Body.Close()

	content, _ := io.ReadAll(io.LimitReader(response.Body, 10*1024*1024))

	return filters.Detect(start_url, content), nil

}
