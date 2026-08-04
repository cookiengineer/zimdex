package archive

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cookiengineer/gozim/archive/zim"
	"github.com/cookiengineer/zimdex/internal/archive/filters"
	"golang.org/x/net/html"
)

func BuildZIM(dataDir string, queue *Queue, filterNames []string) (string, error) {
	downloaded := queue.GetByStatus(StatusDownloaded)
	if len(downloaded) == 0 {
		return "", fmt.Errorf("no downloaded entries to build")
	}

	activeFilters := filters.Enabled(filterNames)

	date := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("%s-%s.zim", queue.Host, date)
	outputPath := filepath.Join(dataDir, filename)

	if _, err := os.Stat(outputPath); err == nil {
		for i := 2; ; i++ {
			alt := fmt.Sprintf("%s-%s-%d.zim", queue.Host, date, i)
			if _, err := os.Stat(filepath.Join(dataDir, alt)); err != nil {
				filename = alt
				outputPath = filepath.Join(dataDir, alt)
				break
			}
		}
	}

	w := zim.NewWriter()
	w.SetCompression(zim.CompressionZstd)
	w.SetIndexing(true, "eng")
	w.SetMainPath(downloaded[0].ZimPath)

	err := w.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("creating ZIM file: %w", err)
	}

	title := queue.Host
	mainTitle := ""
	added := 0

	for _, entry := range downloaded {
		cachePath := filepath.Join(dataDir, entry.Path)
		data, err := os.ReadFile(cachePath)
		if err != nil {
			continue
		}

		pageURL := entry.URL
		if entry.DownloadURL != nil {
			pageURL = entry.DownloadURL
		}
		data = filters.ApplyHTMLFilters(data, pageURL, activeFilters)

		itemTitle := entryTitle(entry, cachePath)
		if mainTitle == "" && entry.EntryType == EntryTypePage {
			mainTitle = itemTitle
		}

		err = w.AddItem(zim.NewBytesItem(entry.ZimPath, entry.MimeType, itemTitle, data))
		if err != nil {
			continue
		}
		added++
	}

	w.AddMetadata("Title", title)
	if mainTitle != "" {
		w.AddMetadata("Title", mainTitle)
	}
	w.AddMetadata("Creator", "ZIMdex")
	w.AddMetadata("Date", date)
	w.AddMetadata("Language", "eng")
	if queue.StartURL != nil {
		w.AddMetadata("Source", queue.StartURL.String())
	} else {
		w.AddMetadata("Source", "")
	}
	w.AddMetadata("Description", fmt.Sprintf("Archived from %s on %s", queue.Host, date))

	err = w.Finish()
	if err != nil {
		return "", fmt.Errorf("finishing ZIM file: %w", err)
	}

	return outputPath, nil
}

func entryTitle(entry *QueueEntry, cachePath string) string {
	if entry.EntryType != EntryTypePage {
		ext := filepath.Ext(entry.Path)
		return filepath.Base(entry.Path[:len(entry.Path)-len(ext)])
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return entry.Path
	}

	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return entry.Path
	}

	t := findTitle(doc)
	if t != "" {
		return t
	}

	return entry.Path
}

func findTitle(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "title" {
		if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
			return n.FirstChild.Data
		}
		var buf bytes.Buffer
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				buf.WriteString(c.Data)
			}
		}
		return buf.String()
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if t := findTitle(c); t != "" {
			return t
		}
	}

	return ""
}
