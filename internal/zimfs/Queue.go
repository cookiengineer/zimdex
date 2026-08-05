package zimfs

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
	"github.com/cookiengineer/zimdex/internal/utils"
)

type Queue struct {
	Hostname     string         `json:"hostname"`
	StartURL     *url.URL       `json:"-"`
	Info         QueueInfo      `json:"info"`
	LastModified time.Time      `json:"-"`
	Status       QueueStatus    `json:"status"`
	entries      []*QueueEntry  `json:"entries"`
	file         string         `json:"-"`
	urls         map[string]int `json:"-"`
	mutex        sync.Mutex     `json:"-"`


	// TODO: New Queue implementation

}

func NewQueue(file string, hostname string, start_url *url.URL) *Queue {

	queue := &Queue{
		Hostname: hostname,
		StartURL: start_url,
		file:     file,
		entries:  make([]*QueueEntry, 0),
		urls:     make(map[string]int),
		mutex:    sync.Mutex{},
	}

	_, err := os.Stat(file)

	if err == nil {
		queue.ReadFile()
	} else {
		queue.Status = QueueStatusIdle
		queue.WriteFile()
	}

	return queue

}

func (queue *Queue) MarshalJSON() ([]byte, error) {

	type Alias Queue

	return json.Marshal(&struct {
		StartURL     string `json:"start_url"`
		LastModified string `json:"last_modified"`
		*Alias
	}{
		StartURL:     queue.StartURL.String(),
		LastModified: queue.LastModified.Format("2006-01-02T15:04:05Z"),
		Alias:        (*Alias)(queue),
	})

}

func (queue *Queue) UnmarshalJSON(data []byte) error {

	type Alias Queue

	tmp := &struct {
		StartURL     string `json:"start_url"`
		LastModified string `json:"last_modified"`
		*Alias
	}{
		Alias: (*Alias)(queue),
	}

	err0 := json.Unmarshal(data, tmp)

	if err0 == nil {

		start_url,     err1 := url.Parse(tmp.StartURL)
		last_modified, err2 := time.Parse("2006-01-02T15:04:05Z", tmp.LastModified)

		if err1 == nil {
			queue.StartURL = start_url
		} else {
			return err1
		}

		if err2 == nil {
			queue.LastModified = last_modified
		} else {
			return err2
		}

		return nil

	} else {
		return err0
	}

}

func (queue *Queue) ReadFile() error {

	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	data, err0 := os.ReadFile(queue.file)

	if err0 == nil {

		tmp := Queue{}

		err1 := json.Unmarshal(data, &tmp)

		if err1 == nil {

			queue.Hostname     = tmp.Hostname
			queue.StartURL     = tmp.StartURL
			queue.LastModified = tmp.LastModified
			queue.Status       = tmp.Status
			queue.entries      = tmp.entries
			queue.urls         = make(map[string]int)

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

func (queue *Queue) WriteFile() error {

	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	queue.refresh_info()

	data, err0 := json.MarshalIndent(queue, "", "\t")

	if err0 == nil {

		folder := filepath.Dir(queue.file)
		os.MkdirAll(folder, 0755)

		err1 := os.WriteFile(queue.file, data, 0644)

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
