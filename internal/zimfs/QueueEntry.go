package zimfs

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"github.com/cookiengineer/zimdex/internal/utils"
)

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
}

func NewQueueEntry(web_url *url.URL, zim_url *url.URL, typ QueueEntryType, referrer *url.URL) *QueueEntry {

	entry := QueueEntry{
		WebURL:       utils.CanonicalizeURL(web_url),
		ZimURL:       zim_url,
		MimeType:     utils.GetMimeType(web_url),
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

	if entry.WebURL != nil {

		hostname := entry.WebURL.Hostname()
		path     := entry.WebURL.Path

		if path == "" || path == "/" {
			path = "/index.html"
		} else if strings.HasSuffix(path, "/") {
			path = fmt.Sprintf("%sindex.html", path)
		}

		if entry.WebURL.RawQuery != "" {
			path = fmt.Sprintf("%s?%s", path, entry.WebURL.RawQuery)
		}

		safe_path := path
		safe_path  = strings.ReplaceAll(safe_path, "?", "%3F")
		safe_path  = strings.ReplaceAll(safe_path, "&", "%26")

		return filepath.Join(hostname, filepath.FromSlash(strings.TrimPrefix(safe_path, "/")))

	} else {
		return ""
	}

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
