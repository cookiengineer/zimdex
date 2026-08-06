package zimfs

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"github.com/cookiengineer/zimdex/internal/utils"
)

type Queue struct {
	Folder    string         `json:"folder"`
	StartDate time.Time      `json:"-"`
	StartURL  *url.URL       `json:"-"`
	Info      QueueInfo      `json:"info"`
	Status    QueueStatus    `json:"status"`
	entries   []*QueueEntry  `json:"entries"`
	urls      map[string]int `json:"-"`
	mutex     sync.RWMutex   `json:"-"`
}

func NewQueue(folder string, start_url *url.URL) *Queue {

	queue := &Queue{
		Folder:    folder,
		StartDate: time.Now(),
		StartURL:  start_url,
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
		StartDate string `json:"start_date"`
		StartURL  string `json:"start_url"`
		*Alias
	}{
		StartDate: queue.StartDate.Format("2006-01-02"),
		StartURL:  queue.StartURL.String(),
		Alias:     (*Alias)(queue),
	})

}

func (queue *Queue) UnmarshalJSON(data []byte) error {

	type Alias Queue

	tmp := &struct {
		StartDate string `json:"start_date"`
		StartURL  string `json:"start_url"`
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

		return nil

	} else {
		return err0
	}

}

func (queue *Queue) Add(entry QueueEntry) bool {

	canonicalized := utils.CanonicalizeURL(entry.WebURL)

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

			queue.Info.Downloaded += 1

		} else {
			queue.Info.Pending += 1
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
		result = queue.Info.Pending
	case QueueEntryStatusDownloading:
		result = queue.Info.Downloading
	case QueueEntryStatusDownloaded:
		result = queue.Info.Downloaded
	case QueueEntryStatusFailed:
		result = queue.Info.Failed
	case QueueEntryStatusSkipped:
		result = queue.Info.Skipped
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

			queue.Info.Pending     -= 1
			queue.Info.Downloading += 1

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

	canonicalized := utils.CanonicalizeURL(link)
	_, ok         := queue.urls[canonicalized.String()]

	if ok == true {
		return true
	}

	return false

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

				canonicalized := utils.CanonicalizeURL(entry.WebURL)

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

	canonicalized := utils.CanonicalizeURL(entry.WebURL)

	index, ok := queue.urls[canonicalized.String()]

	if ok == true {

		switch queue.entries[index].Status {
		case QueueEntryStatusPending:
			queue.Info.Pending -= 1
		case QueueEntryStatusDownloading:
			queue.Info.Downloading -= 1
		case QueueEntryStatusDownloaded:
			queue.Info.Downloaded -= 1
		case QueueEntryStatusFailed:
			queue.Info.Failed -= 1
		case QueueEntryStatusSkipped:
			queue.Info.Skipped -= 1
		}

		queue.entries[index] = &entry

		switch queue.entries[index].Status {
		case QueueEntryStatusPending:
			queue.Info.Pending += 1
		case QueueEntryStatusDownloading:
			queue.Info.Downloading += 1
		case QueueEntryStatusDownloaded:
			queue.Info.Downloaded += 1
		case QueueEntryStatusFailed:
			queue.Info.Failed += 1
		case QueueEntryStatusSkipped:
			queue.Info.Skipped += 1
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

	queue.Info = info

}
