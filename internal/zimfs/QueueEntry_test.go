package zimfs

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func writeCacheEntry(t *testing.T, dir string, entry *QueueEntry, content []byte) {
	t.Helper()

	path := filepath.Join(dir, entry.Path())

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestQueueEntryTitle(t *testing.T) {
	dir := t.TempDir()
	webURL, _ := url.Parse("https://example.com/page")
	entry := NewQueueEntry(webURL, webURL, QueueEntryTypePage, nil)
	entry.folder = dir
	writeCacheEntry(t, dir, entry, []byte("<html><head><title>Hello World</title></head><body>x</body></html>"))

	if title := entry.Title(); title != "Hello World" {
		t.Errorf("expected title %q, got %q", "Hello World", title)
	}
}

func TestQueueEntryTitleFallback(t *testing.T) {
	dir := t.TempDir()
	webURL, _ := url.Parse("https://example.com/about")
	entry := NewQueueEntry(webURL, webURL, QueueEntryTypePage, nil)
	entry.folder = dir
	writeCacheEntry(t, dir, entry, []byte("<html><body>no title</body></html>"))

	if title := entry.Title(); title != "example.com/about" {
		t.Errorf("expected path fallback %q, got %q", "example.com/about", title)
	}
}

func TestQueueEntryTitleAsset(t *testing.T) {
	webURL, _ := url.Parse("https://example.com/style.css")
	entry := NewQueueEntry(webURL, webURL, QueueEntryTypeAsset, nil)

	if title := entry.Title(); title != "style" {
		t.Errorf("expected title %q, got %q", "style", title)
	}
}
