package search

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/cookiengineer/gozim/archive/zim"
)

var ErrSearchPanic = errors.New("search index panic — the ZIM file may be incompatible")

type SuggestionResult struct {
	Title     string `json:"title"`
	Path      string `json:"path"`
	RenderURL string `json:"render_url"`
	Snippet   string `json:"snippet"`
	ZimFile   string `json:"zim_file"`
}

type Searcher struct {
	archives    map[string]*zim.Archive
	zimSearcher *zim.Searcher
	mu          sync.RWMutex
}

func NewSearcher(archives map[string]*zim.Archive) *Searcher {
	s := &Searcher{
		archives: archives,
	}
	s.rebuild()
	return s
}

func (s *Searcher) rebuild() {
	list := make([]*zim.Archive, 0, len(s.archives))
	for _, a := range s.archives {
		list = append(list, a)
	}
	s.zimSearcher = zim.NewSearcher(list...)
}

func (s *Searcher) Search(query string, offset, limit int) (resultSet *zim.SearchResultSet, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	defer func() {
		if r := recover(); r != nil {
			resultSet = nil
			err = fmt.Errorf("%w: %v", ErrSearchPanic, r)
		}
	}()

	return s.zimSearcher.Search(query, offset, limit)
}

func (s *Searcher) Suggest(prefix string, limit int) []SuggestionResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]bool)
	var results []SuggestionResult

	for filename, archive := range s.archives {
		suggester := zim.NewSuggestionSearcher(archive)
		items, err := suggester.Suggest(prefix, limit*2)
		if err != nil {
			continue
		}

		for _, item := range items {
			if seen[item.Path] {
				continue
			}
			seen[item.Path] = true

			results = append(results, SuggestionResult{
				Title:   item.Title,
				Path:    item.Path,
				ZimFile: filename,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if len(results[i].Title) != len(results[j].Title) {
			return len(results[i].Title) < len(results[j].Title)
		}
		return results[i].Title < results[j].Title
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results
}

func (s *Searcher) AddArchive(filename string, archive *zim.Archive) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.archives[filename] = archive
	s.rebuild()
}

func (s *Searcher) RemoveArchive(filename string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.archives, filename)
	s.rebuild()
}

func (s *Searcher) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.archives = make(map[string]*zim.Archive)
	s.zimSearcher = nil
	return nil
}
