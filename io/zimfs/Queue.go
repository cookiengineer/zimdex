package zimfs

import "github.com/cookiengineer/zimdex/internal/filters"
import utils_urls "github.com/cookiengineer/zimdex/internal/utils/urls"
import "encoding/json"
import "fmt"
import "net/url"
import "os"
import "path/filepath"
import "strings"
import "sync"
import "time"

type Queue struct {
	Folder    string           `json:"folder"`
	StartDate time.Time        `json:"-"`
	StartURL  *url.URL         `json:"-"`
	Filters   []filters.Filter `json:"-"`
	info      QueueInfo        `json:"-"`
	Status    QueueStatus      `json:"status"`
	entries   []*QueueEntry    `json:"-"`
	urls      map[string]int   `json:"-"`
	mutex     sync.RWMutex     `json:"-"`
}

func NewQueue(folder string, start_url *url.URL, filter_names []string) *Queue {

	queue := &Queue{
		Folder:    folder,
		StartDate: time.Now(),
		StartURL:  start_url,
		Filters:   filters.Get(filter_names),
		entries:   make([]*QueueEntry, 0),
		urls:      make(map[string]int),
		mutex:     sync.RWMutex{},
	}

	host := strings.ReplaceAll(queue.StartURL.Hostname(), ".", "_")
	date := queue.StartDate.Format("2006-01-02")
	path := filepath.Join(queue.Folder, fmt.Sprintf("%s_%s.json", host, date))

	_, err := os.Stat(path)

	if err == nil {
		queue.Read()
	} else {
		queue.Status = QueueStatusIdle
		queue.Write()
	}

	return queue

}

func (queue *Queue) MarshalJSON() ([]byte, error) {

	type Alias Queue

	return json.Marshal(&struct {
		StartDate string        `json:"start_date"`
		StartURL  string        `json:"start_url"`
		Info      QueueInfo     `json:"info"`
		Entries   []*QueueEntry `json:"entries"`
		*Alias
	}{
		StartDate: queue.StartDate.Format("2006-01-02"),
		StartURL:  queue.StartURL.String(),
		Info:      queue.info,
		Entries:   queue.entries,
		Alias:     (*Alias)(queue),
	})

}

func (queue *Queue) UnmarshalJSON(data []byte) error {

	type Alias Queue

	tmp := &struct {
		StartDate string        `json:"start_date"`
		StartURL  string        `json:"start_url"`
		Info      QueueInfo     `json:"info"`
		Entries   []*QueueEntry `json:"entries"`
		*Alias
	}{
		Alias: (*Alias)(queue),
	}

	err0 := json.Unmarshal(data, tmp)

	if err0 == nil {

		start_date, err1 := time.Parse("2006-01-02", tmp.StartDate)
		start_url,  err2 := url.Parse(tmp.StartURL)

		if err1 == nil {
			queue.StartDate = start_date
		} else {
			return err1
		}

		if err2 == nil {
			queue.StartURL = start_url
		} else {
			return err2
		}

		queue.info = tmp.Info
		queue.entries = tmp.Entries

		return nil

	} else {
		return err0
	}

}

func (queue *Queue) Add(entry QueueEntry) bool {

	entry.folder = queue.Folder
	entry.filters = queue.Filters

	canonicalized := utils_urls.Canonicalize(entry.WebURL)

	queue.mutex.RLock()
	_, exists_already := queue.urls[canonicalized.String()]
	queue.mutex.RUnlock()

	if exists_already == false {

		cache_path := filepath.Join(queue.Folder, entry.Path())
		info, err := os.Stat(cache_path)

		queue.mutex.Lock()

		if err == nil && info.Size() > 0 {

			entry.Status = QueueEntryStatusDownloaded
			entry.Size   = info.Size()

			queue.info.Downloaded += 1

		} else {
			queue.info.Pending += 1
		}

		queue.urls[canonicalized.String()] = len(queue.entries)
		queue.entries = append(queue.entries, &entry)

		queue.mutex.Unlock()

		return true

	} else {
		return false
	}

}

func (queue *Queue) Count(status QueueEntryStatus) int {

	result := int(0)

	queue.mutex.RLock()
	defer queue.mutex.RUnlock()

	switch status {
	case QueueEntryStatusPending:
		result = queue.info.Pending
	case QueueEntryStatusDownloading:
		result = queue.info.Downloading
	case QueueEntryStatusDownloaded:
		result = queue.info.Downloaded
	case QueueEntryStatusFailed:
		result = queue.info.Failed
	case QueueEntryStatusSkipped:
		result = queue.info.Skipped
	}

	return result

}

func (queue *Queue) Get(typ QueueEntryType) (QueueEntry, error) {

	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	result := QueueEntry{}
	found  := false

	for _, entry := range queue.entries {

		if entry.Status == QueueEntryStatusPending && entry.Type == typ {

			entry.Status = QueueEntryStatusDownloading

			queue.info.Pending     -= 1
			queue.info.Downloading += 1

			// Clone value
			result = *entry
			found  = true
			break

		}

	}

	if found == true {
		return result, nil
	} else {
		return result, fmt.Errorf("No QueueEntry with Type %s found", typ)
	}

}

func (queue *Queue) Has(link *url.URL) bool {

	queue.mutex.RLock()
	defer queue.mutex.RUnlock()

	canonicalized := utils_urls.Canonicalize(link)
	_, ok         := queue.urls[canonicalized.String()]

	if ok == true {
		return true
	}

	return false

}

func (queue *Queue) Info() QueueInfo {

	queue.mutex.RLock()
	defer queue.mutex.RUnlock()

	return QueueInfo{
		Pending:     queue.info.Pending,
		Downloading: queue.info.Downloading,
		Downloaded:  queue.info.Downloaded,
		Failed:      queue.info.Failed,
		Skipped:     queue.info.Skipped,
	}

}

func (queue *Queue) Query(status QueueEntryStatus) []QueueEntry {

	queue.mutex.RLock()
	defer queue.mutex.RUnlock()

	result := make([]QueueEntry, 0)

	for _, entry := range queue.entries {

		if entry.Status == status {
			result = append(result, *entry)
		}

	}

	return result

}

func (queue *Queue) Enqueue(raw_url *url.URL, referrer *url.URL, typ QueueEntryType) bool {

	entry := NewQueueEntry(raw_url, nil, typ, referrer)
	entry.folder = queue.Folder
	entry.filters = queue.Filters

	filtered := filters.ApplyFilterURL(queue.Filters, raw_url, nil, referrer)

	if filtered == nil {
		return false
	}

	new_url, download_url := filters.ApplyRewriteURL(queue.Filters, filtered)

	if download_url == nil {
		download_url = new_url
	}

	entry.WebURL = utils_urls.Canonicalize(download_url)
	entry.ZimURL = new_url
	entry.MimeType = utils_urls.GetMimeType(download_url)

	return queue.Add(*entry)

}

func (queue *Queue) Read() error {

	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	host := strings.ReplaceAll(queue.StartURL.Hostname(), ".", "_")
	date := queue.StartDate.Format("2006-01-02")
	path := filepath.Join(queue.Folder, fmt.Sprintf("%s_%s.json", host, date))

	data, err0 := os.ReadFile(path)

	if err0 == nil {

		tmp := Queue{}

		err1 := json.Unmarshal(data, &tmp)

		if err1 == nil {

			queue.StartDate = tmp.StartDate
			queue.StartURL  = tmp.StartURL
			queue.Status    = tmp.Status
			queue.entries   = tmp.entries
			queue.urls      = make(map[string]int)

			for index, entry := range queue.entries {

				entry.folder = queue.Folder
				entry.filters = queue.Filters

				canonicalized := utils_urls.Canonicalize(entry.WebURL)

				if canonicalized.String() != "" {
					queue.urls[canonicalized.String()] = index
				}

			}

			queue.refresh_info()

			return nil

		} else {
			return err1
		}

	} else {
		return err0
	}

}

func (queue *Queue) Reset() bool {

	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	found := false

	for _, entry := range queue.entries {

		if entry.Status == QueueEntryStatusFailed || entry.Status == QueueEntryStatusDownloading {
			entry.Status     = QueueEntryStatusPending
			entry.Size       = 0
			entry.StatusCode = 0
			entry.Retries    = 0
			found = true
		}

	}

	if found == true {
		queue.refresh_info()
	}

	return found

}

func (queue *Queue) Set(entry QueueEntry) bool {

	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	entry.folder = queue.Folder
	entry.filters = queue.Filters

	canonicalized := utils_urls.Canonicalize(entry.WebURL)

	index, ok := queue.urls[canonicalized.String()]

	if ok == true {

		switch queue.entries[index].Status {
		case QueueEntryStatusPending:
			queue.info.Pending -= 1
		case QueueEntryStatusDownloading:
			queue.info.Downloading -= 1
		case QueueEntryStatusDownloaded:
			queue.info.Downloaded -= 1
		case QueueEntryStatusFailed:
			queue.info.Failed -= 1
		case QueueEntryStatusSkipped:
			queue.info.Skipped -= 1
		}

		queue.entries[index] = &entry

		switch queue.entries[index].Status {
		case QueueEntryStatusPending:
			queue.info.Pending += 1
		case QueueEntryStatusDownloading:
			queue.info.Downloading += 1
		case QueueEntryStatusDownloaded:
			queue.info.Downloaded += 1
		case QueueEntryStatusFailed:
			queue.info.Failed += 1
		case QueueEntryStatusSkipped:
			queue.info.Skipped += 1
		}

		return true

	} else {
		return false
	}

}

func (queue *Queue) Write() error {

	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	queue.refresh_info()

	data, err0 := json.MarshalIndent(queue, "", "\t")

	if err0 == nil {

		os.MkdirAll(queue.Folder, 0755)

		host := strings.ReplaceAll(queue.StartURL.Hostname(), ".", "_")
		date := queue.StartDate.Format("2006-01-02")
		path := filepath.Join(queue.Folder, fmt.Sprintf("%s_%s.json", host, date))
		err1 := os.WriteFile(path, data, 0644)

		if err1 == nil {
			return nil
		} else {
			return err1
		}

	} else {
		return err0
	}

}

func (queue *Queue) refresh_info() {

	info := QueueInfo{}

	for _, entry := range queue.entries {

		switch entry.Status {
		case QueueEntryStatusPending:
			info.Pending += 1
		case QueueEntryStatusDownloading:
			info.Downloading += 1
		case QueueEntryStatusDownloaded:
			info.Downloaded += 1
		case QueueEntryStatusFailed:
			info.Failed += 1
		case QueueEntryStatusSkipped:
			info.Skipped += 1
		default:
			// Do Nothing
		}

	}

	queue.info = info

}
