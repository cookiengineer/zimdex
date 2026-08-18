package zimfs

import "github.com/cookiengineer/zimdex/internal/filters"
import utils_urls "github.com/cookiengineer/zimdex/internal/utils/urls"
import "golang.org/x/net/html"
import "bytes"
import "encoding/json"
import "fmt"
import "net/url"
import "os"
import "path/filepath"
import "strings"
import "time"

type QueueEntry struct {
	WebURL       *url.URL         `json:"-"` // via Alias
	ZimURL       *url.URL         `json:"-"` // via Alias
	MimeType     string           `json:"mime_type"`
	Type         QueueEntryType   `json:"type"`
	Status       QueueEntryStatus `json:"status"`
	Size         int64            `json:"size,omitempty"`
	Referrer     *url.URL         `json:"-"` // via Alias
	LastModified time.Time        `json:"-"` // via Alias
	StatusCode   int              `json:"status_code"`
	Retries      int              `json:"retries"`

	folder  string           `json:"-"`
	filters []filters.Filter `json:"-"`
}

func NewQueueEntry(web_url *url.URL, zim_url *url.URL, typ QueueEntryType, referrer *url.URL) *QueueEntry {

	entry := QueueEntry{
		WebURL:       utils_urls.Canonicalize(web_url),
		ZimURL:       zim_url,
		MimeType:     utils_urls.GetMimeType(web_url),
		Type:         typ,
		Status:       QueueEntryStatusPending,
		Size:         0,
		Referrer:     referrer,
		LastModified: time.Time{},
		StatusCode:   0,
		Retries:      0,
	}

	return &entry

}

func (entry *QueueEntry) Path() (string) {

	link := entry.WebURL

	if entry.ZimURL != nil {
		link = entry.ZimURL
	}

	if link != nil {

		hostname := link.Hostname()
		path     := link.Path

		if path == "" || path == "/" {
			path = "/index.html"
		} else if strings.HasSuffix(path, "/") {
			path = fmt.Sprintf("%sindex.html", path)
		}

		if link.RawQuery != "" {
			path = fmt.Sprintf("%s?%s", path, link.RawQuery)
		}

		safe_path := path
		safe_path  = strings.ReplaceAll(safe_path, "?", "%3F")
		safe_path  = strings.ReplaceAll(safe_path, "&", "%26")

		return filepath.Join(hostname, filepath.FromSlash(strings.TrimPrefix(safe_path, "/")))

	} else {
		return ""
	}

}

func (entry *QueueEntry) HTML() []byte {

	data, err := os.ReadFile(filepath.Join(entry.folder, entry.Path()))

	if err != nil {
		return nil
	}

	return filters.ApplyFilterHTML(entry.filters, entry.WebURL, data)

}

func (entry *QueueEntry) Title() string {

	if entry.Type != QueueEntryTypePage {

		ext := filepath.Ext(entry.Path())

		if ext != "" {
			return filepath.Base(entry.Path()[:len(entry.Path())-len(ext)])
		}

		return filepath.Base(entry.Path())

	}

	data, err := os.ReadFile(filepath.Join(entry.folder, entry.Path()))

	if err != nil {
		return entry.Path()
	}

	doc, err := html.Parse(bytes.NewReader(data))

	if err != nil {
		return entry.Path()
	}

	if title := findHTMLNode(doc, "title"); title != "" {
		return title
	}

	return entry.Path()

}

func (entry *QueueEntry) filterQueueEntryURL(raw_url *url.URL, html_body []byte, referrer *url.URL) (new_url, download_url *url.URL) {

	filtered := filters.ApplyFilterURL(entry.filters, raw_url, html_body, referrer)

	if filtered == nil {
		return nil, nil
	}

	return filters.ApplyRewriteURL(entry.filters, filtered)

}

func (entry QueueEntry) MarshalJSON() ([]byte, error) {

	type Alias QueueEntry

	return json.Marshal(&struct {
		WebURL       string `json:"web_url"`
		ZimURL       string `json:"zim_url"`
		Referrer     string `json:"referrer"`
		LastModified string `json:"last_modified"`
		*Alias
	}{
		WebURL:       entry.WebURL.String(),
		ZimURL:       entry.ZimURL.String(),
		Referrer:     entry.Referrer.String(),
		LastModified: entry.LastModified.Format("2006-01-02T15:04:05Z"),
		Alias:        (*Alias)(&entry),
	})

}

func (entry *QueueEntry) UnmarshalJSON(data []byte) error {

	type Alias QueueEntry

	tmp := &struct {
		WebURL       string `json:"web_url"`
		ZimURL       string `json:"zim_url"`
		Referrer     string `json:"referrer"`
		LastModified string `json:"last_modified"`
		*Alias
	}{
		Alias: (*Alias)(entry),
	}

	err0 := json.Unmarshal(data, tmp)

	if err0 == nil {

		web_url,       err1 := url.Parse(tmp.WebURL)
		zim_url,       err2 := url.Parse(tmp.ZimURL)
		referrer,      err3 := url.Parse(tmp.Referrer)
		last_modified, err4 := time.Parse("2006-01-02T15:04:05Z", tmp.LastModified)

		if err1 == nil {
			entry.WebURL = web_url
		} else {
			return err1
		}

		if err2 == nil {
			entry.ZimURL = zim_url
		} else {
			return err2
		}

		if err3 == nil {
			entry.Referrer = referrer
		} else {
			return err3
		}

		if err4 == nil {
			entry.LastModified = last_modified
		} else {
			return err4
		}

		return nil

	} else {
		return err0
	}

}
