package archive

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"
)

type ScraperWorker struct {
	Type    EntryType
	Queue   *Queue
	ctx     context.Context
	scraper *Scraper
	group   *sync.WaitGroup
}

func NewScraperWorker(ctx context.Context, entry_type EntryType, scraper *Scraper, wait_group *sync.WaitGroup) *ScraperWorker {

	return &ScraperWorker{
		Type:    entry_type,
		Queue:   queue,
		context: ctx,
		scraper: scraper,
		group:   wait_group,
	}

}

func (worker *ScraperWorker) Run() {

	defer worker.group.Done()

	for {

		select {
		case <-worker.context.Done():

			// context was stopped
			return

		default:
		}

		status := worker.scraper.Status()

		if status == "running" {

			// TODO: Everything else must go in here





		}


		// TODO: rewrite into something like entry := worker.scraper.GetNewTask()


		entry := worker.Queue.PopPending(worker.Type)
		if entry == nil && worker.Type == EntryTypePage {
			entry = worker.Queue.PopPending(EntryTypeExternalPage)
		}

		if entry == nil {

			// TODO: scraper.UpdateQueueStatus()

			remaining := w.Queue.PendingCount()
			active := w.Queue.DownloadingCount()

			// TODO: This has to happen in Scraper
			if remaining == 0 && active == 0 {

				// status := scraper.Status()
				w.scraper.mu.Lock()
				if w.scraper.status == "running" {
					w.scraper.status = "complete"
				}
				w.scraper.mu.Unlock()
				w.Queue.Status = "complete"
				w.Queue.Save()
				w.scraper.log("Scraping complete")
				return
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}

		err := w.scraper.downloader.Download(w.ctx, entry)

		w.scraper.trackActivity(entry, 0)

		if err != nil {
			if errors.Is(err, ErrRedirect) {
				redirectURL := extractRedirectURL(err)
				if redirectURL != nil {
					_, localPath, zimPath := URLToPath(redirectURL)
					w.Queue.AddIfNew(redirectURL, localPath, zimPath, w.Type, entry.URL)
				}
			} else if errors.Is(err, ErrRetry) {
				entry.Status = StatusPending
			} else {
				w.scraper.log("Failed: %s — %v", entry.URL, err)
			}
		}

		w.Queue.RecalcStats()

		if entry.Status == StatusDownloaded && (entry.EntryType == EntryTypePage || entry.EntryType == EntryTypeExternalPage) {
			if w.scraper.Options.PageLimit > 0 && w.Queue.DownloadedCount() >= w.scraper.Options.PageLimit {
				w.scraper.log("Page limit reached (%d)", w.scraper.Options.PageLimit)
			} else {
				w.scraper.extractAndEnqueue(entry)
			}
		}

		w.Queue.Save()
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
