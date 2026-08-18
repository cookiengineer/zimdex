package archive

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cookiengineer/zimdex/internal/zimfs"
)

type ScraperWorker struct {
	Type    zimfs.QueueEntryType
	Queue   *zimfs.Queue
	ctx     context.Context
	scraper *Scraper
	group   *sync.WaitGroup
}

func NewScraperWorker(ctx context.Context, entry_type zimfs.QueueEntryType, queue *zimfs.Queue, scraper *Scraper, wait_group *sync.WaitGroup) *ScraperWorker {

	return &ScraperWorker{
		Type:    entry_type,
		Queue:   queue,
		ctx:     ctx,
		scraper: scraper,
		group:   wait_group,
	}

}

func (worker *ScraperWorker) Run() {

	defer worker.group.Done()

	for {

		select {
		case <-worker.ctx.Done():
			return
		default:
		}

		if worker.scraper.Status() != "running" {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		entry, err := worker.Queue.Get(worker.Type)
		if err != nil && worker.Type == zimfs.QueueEntryTypePage {
			entry, err = worker.Queue.Get(zimfs.QueueEntryTypeExternalPage)
		}

		if err != nil {

			pending := worker.Queue.Count(zimfs.QueueEntryStatusPending)
			active := worker.Queue.Count(zimfs.QueueEntryStatusDownloading)

			if pending == 0 && active == 0 {
				worker.scraper.completeIfRunning()
				worker.Queue.Status = zimfs.QueueStatusIdle
				worker.Queue.Write()
				worker.scraper.log("Scraping complete")
				return
			}

			time.Sleep(500 * time.Millisecond)
			continue
		}

		err = worker.scraper.downloader.Download(worker.ctx, &entry)

		worker.scraper.trackActivity(&entry, 0)

		if err != nil {
			if errors.Is(err, ErrRedirect) {
				redirectURL := extractRedirectURL(err)
				if redirectURL != nil {
					worker.Queue.Add(*zimfs.NewQueueEntry(redirectURL, redirectURL, worker.Type, entry.WebURL))
				}
			} else if errors.Is(err, ErrRetry) {
				entry.Status = zimfs.QueueEntryStatusPending
			} else {
				worker.scraper.log("Failed: %s — %v", entry.WebURL, err)
			}
		}

		worker.Queue.Set(entry)

		if entry.Status == zimfs.QueueEntryStatusDownloaded && (entry.Type == zimfs.QueueEntryTypePage || entry.Type == zimfs.QueueEntryTypeExternalPage) {
			if worker.scraper.Options.PageLimit > 0 && worker.Queue.Count(zimfs.QueueEntryStatusDownloaded) >= worker.scraper.Options.PageLimit {
				worker.scraper.log("Page limit reached (%d)", worker.scraper.Options.PageLimit)
			} else {
				worker.scraper.extractAndEnqueue(&entry)
			}
		}

		worker.Queue.Write()
	}

}

func extractRedirectURL(err error) *url.URL {
	msg := err.Error()
	const prefix = "redirect: "
	if idx := strings.Index(msg, prefix); idx >= 0 {
		u, parseErr := url.Parse(msg[idx+len(prefix):])
		if parseErr == nil {
			return u
		}
	}
	return nil
}
