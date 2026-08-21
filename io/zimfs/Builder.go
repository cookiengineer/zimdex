package zimfs

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cookiengineer/gozim/archive/zim"
)

type Builder struct {
	Folder string
}

func NewBuilder(folder string) *Builder {

	return &Builder{
		Folder: folder,
	}

}

func (builder *Builder) Build(queue *Queue) (string, error) {

	downloaded := queue.Query(QueueEntryStatusDownloaded)

	if len(downloaded) == 0 {
		return "", fmt.Errorf("no downloaded entries to build")
	}

	host := "archive"
	if queue.StartURL != nil {
		host = queue.StartURL.Hostname()
	}

	date := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("%s-%s.zim", host, date)
	outputPath := filepath.Join(builder.Folder, filename)

	if _, err := os.Stat(outputPath); err == nil {
		for i := 2; ; i++ {
			alt := fmt.Sprintf("%s-%s-%d.zim", host, date, i)
			if _, err := os.Stat(filepath.Join(builder.Folder, alt)); err != nil {
				filename = alt
				outputPath = filepath.Join(builder.Folder, alt)
				break
			}
		}
	}

	writer := zim.NewWriter()
	writer.SetCompression(zim.CompressionZstd)
	writer.SetIndexing(true, "eng")
	writer.SetMainPath(downloaded[0].Path())

	if err := writer.Create(outputPath); err != nil {
		return "", fmt.Errorf("creating ZIM file: %w", err)
	}

	main_title := ""

	for i := range downloaded {

		entry := &downloaded[i]
		html  := entry.HTML()

		if html == nil {
			continue
		}

		item_title := entry.Title()

		if main_title == "" && entry.Type == QueueEntryTypePage {
			main_title = item_title
		}

		if err := writer.AddItem(zim.NewBytesItem(entry.Path(), entry.MimeType, item_title, html)); err != nil {
			continue
		}

	}

	if main_title != "" {
		writer.AddMetadata("Title", main_title)
	} else {
		writer.AddMetadata("Title", host)
	}

	writer.AddMetadata("Creator", "ZIMdex")
	writer.AddMetadata("Date", date)
	writer.AddMetadata("Language", "eng")

	if queue.StartURL != nil {
		writer.AddMetadata("Source", queue.StartURL.String())
	} else {
		writer.AddMetadata("Source", "")
	}

	writer.AddMetadata("Description", fmt.Sprintf("Archived from %s on %s", host, date))

	if err := writer.Finish(); err != nil {
		return "", fmt.Errorf("finishing ZIM file: %w", err)
	}

	return outputPath, nil

}

func (builder *Builder) BuildAll(queues ...*Queue) ([]string, []error) {

	results := make([]string, len(queues))
	errors := make([]error, len(queues))
	wait_group := sync.WaitGroup{}

	for index, queue := range queues {

		wait_group.Add(1)

		go func(idx int, q *Queue) {

			defer wait_group.Done()

			path, err := builder.Build(q)

			results[idx] = path
			errors[idx] = err

		}(index, queue)

	}

	wait_group.Wait()

	return results, errors

}
