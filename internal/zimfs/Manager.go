package zimfs

import (
	"path/filepath"
	"sync"
	"github.com/cookiengineer/gozim/archive/zim"
)

type Manager struct {
	Folder   string
	archives map[string]*zim.Archive
	mutex    sync.RWMutex
}

func NewManager(folder string) *Manager {

	return &Manager{
		Folder:   folder,
		archives: make(map[string]*zim.Archive),
	}

}

func (manager *Manager) Scan() error {

	pattern := filepath.Join(manager.Folder, "*.zim")
	matches, err0 := filepath.Glob(pattern)

	if err0 == nil {

		manager.mutex.Lock()
		defer manager.mutex.Unlock()

		for _, path := range matches {

			filename := filepath.Base(path)

			if _, exists := manager.archives[filename]; exists == false {

				archive, err1 := zim.Open(path)

				if err1 == nil {
					manager.archives[filename] = archive
				}

			}

		}

		return nil

	} else {
		return err0
	}

}

func (manager *Manager) Get(filename string) *zim.Archive {

	manager.mutex.RLock()
	defer manager.mutex.RUnlock()

	archive, ok := manager.archives[filename]

	if ok == true {
		return archive
	}

	return nil

}

func (manager *Manager) List() []ArchiveInfo {

	manager.mutex.RLock()
	defer manager.mutex.RUnlock()

	result := make([]ArchiveInfo, 0, len(manager.archives))

	for filename, archive := range manager.archives {

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

func (manager *Manager) Reload() error {

	err0 := manager.Close()

	if err0 == nil {

		err1 := manager.Scan()

		if err1 == nil {
			return nil
		} else {
			return err1
		}

	} else {
		return err0
	}

}

func (manager *Manager) Which(path string) string {

	for filename, archive := range manager.archives {

		if archive != nil && archive.HasEntry(path) {
			return filename
		}

	}

	return ""

}

func (manager *Manager) Close() error {

	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	for _, archive := range manager.archives {
		archive.Close()
	}

	manager.archives = make(map[string]*zim.Archive)

	return nil

}
