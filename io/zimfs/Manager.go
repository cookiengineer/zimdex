package zimfs

import "github.com/cookiengineer/gozim/archive/zim"
import "fmt"
import "os"
import "path/filepath"
import "strings"
import "sync"

type Manager struct {
	Folder   string
	archives map[string]*zim.Archive
	searcher *zim.Searcher
	mutex    sync.RWMutex
}

func NewManager(folder string) *Manager {

	return &Manager{
		Folder:   folder,
		searcher: nil,
		archives: make(map[string]*zim.Archive),
		mutex:    sync.RWMutex{},
	}

}

func (manager *Manager) Add(filename string) error {

	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	path := filepath.Join(manager.Folder, filename)
	stat, err0 := os.Stat(path)

	if err0 == nil {

		if stat.IsDir() == false {

			if _, exists := manager.archives[filename]; exists == false {

				archive, err1 := zim.Open(path)

				if err1 == nil {

					manager.archives[filename] = archive

					manager.searcher = nil
					manager.searcher = zim.NewSearcher(manager.archives)

					return nil

				} else {
					return err1
				}

			} else {
				return fmt.Errorf("ZIM Archive %s already opened", filename)
			}

		} else {
			return fmt.Errorf("Could not open ZIM Archive %s", filename)
		}

	} else {
		return err0
	}

}

func (manager *Manager) Close() error {

	for filename, _ := range manager.archives {
		manager.Remove(filename)
	}

	manager.archives = make(map[string]*zim.Archive)

	return nil

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

func (manager *Manager) Remove(filename string) error {

	archive, ok := manager.archives[filename]

	if ok == true {

		err := archive.Close()

		delete(manager.archives, filename)

		manager.searcher = nil
		manager.searcher = zim.NewSearcher(manager.archives)

		if err == nil {
			return nil
		} else {
			return err
		}

	} else {
		return nil
	}

}

func (manager *Manager) Search(query string, offset int, limit int) (*zim.SearchResultSet, error) {

	var result *zim.SearchResultSet
	var err error

	manager.mutex.RLock()
	defer manager.mutex.RUnlock()

	defer func() {

		if reason := recover(); reason != nil {
			result = nil
			err    = fmt.Errorf("ZIM search index panic: %v", reason)
		}

	}()

	result, err = manager.searcher.Search(query, offset, limit)

	return result, err

}

func (manager *Manager) Scan() error {

	pattern := filepath.Join(manager.Folder, "*.zim")
	matches, err0 := filepath.Glob(pattern)

	if err0 == nil {

		failed := make([]string, 0)

		for _, path := range matches {

			err1 := manager.Add(filepath.Base(path))

			if err1 != nil {
				failed = append(failed, path)
			}

		}

		if len(failed) > 0 {
			return fmt.Errorf("Could not open ZIM Archives %s", strings.Join(failed, ", "))
		} else {
			return nil
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

