package zimfs

import (
	"path/filepath"
	"sync"

	"github.com/cookiengineer/gozim/archive/zim"
)

type ArchiveInfo struct {
	Filename     string `json:"filename"`
	Title        string `json:"title"`
	Date         string `json:"date"`
	Language     string `json:"language"`
	Source       string `json:"source"`
	Description  string `json:"description"`
	EntryCount   uint32 `json:"entry_count"`
	ArticleCount uint32 `json:"article_count"`
	MediaCount   uint32 `json:"media_count"`
	SizeBytes    int64  `json:"size_bytes"`
	Checksum     string `json:"checksum"`
}

type Manager struct {
	Dir      string
	archives map[string]*zim.Archive
	mu       sync.RWMutex
}

func NewManager(dir string) *Manager {
	return &Manager{
		Dir:      dir,
		archives: make(map[string]*zim.Archive),
	}
}

func (m *Manager) Scan() error {
	pattern := filepath.Join(m.Dir, "*.zim")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, path := range matches {
		filename := filepath.Base(path)
		if _, exists := m.archives[filename]; exists {
			continue
		}

		archive, err := zim.Open(path)
		if err != nil {
			continue
		}

		m.archives[filename] = archive
	}

	return nil
}

func (m *Manager) Get(filename string) (*zim.Archive, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	archive, ok := m.archives[filename]
	return archive, ok
}

func (m *Manager) List() []ArchiveInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]ArchiveInfo, 0, len(m.archives))

	for filename, archive := range m.archives {
		info := ArchiveInfo{
			Filename:     filename,
			EntryCount:   archive.EntryCount(),
			ArticleCount: archive.ArticleCount(),
			MediaCount:   archive.MediaCount(),
			Checksum:     archive.Checksum(),
		}

		if val, ok := archive.Metadata("Title"); ok {
			info.Title = val
		}
		if val, ok := archive.Metadata("Date"); ok {
			info.Date = val
		}
		if val, ok := archive.Metadata("Language"); ok {
			info.Language = val
		}
		if val, ok := archive.Metadata("Source"); ok {
			info.Source = val
		}
		if val, ok := archive.Metadata("Description"); ok {
			info.Description = val
		}

		result = append(result, info)
	}

	return result
}

func (m *Manager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, archive := range m.archives {
		archive.Close()
	}
	m.archives = make(map[string]*zim.Archive)
	m.mu.Unlock()

	err := m.Scan()
	m.mu.Lock()

	return err
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, archive := range m.archives {
		archive.Close()
	}
	m.archives = make(map[string]*zim.Archive)

	return nil
}
