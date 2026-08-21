package api

import "github.com/cookiengineer/zimdex/io/zimfs"
import "encoding/json"
import "fmt"
import "net/http"
import "strconv"
import "strings"

type search_response struct {
	Query     string          `json:"query"`
	Offset    int             `json:"offset"`
	Limit     int             `json:"limit"`
	Estimated int             `json:"estimated"`
	Results   []search_result `json:"results"`
}

type search_result struct {
	// zimdex-specific
	URL       string  `json:"url"`
	// bm25.SearchResult
	DocumentID uint32  `json:"id"`
	File       string  `json:"file"`
	Path       string  `json:"path"`
	Title      string  `json:"title"`
	Score      float64 `json:"score"`
	Snippet    string  `json:"snippet"`
	WordCount  uint64  `json:"word_count"`
}

func Search(manager *zimfs.Manager, response http.ResponseWriter, request *http.Request) {

	if request.Method == http.MethodGet {

		query  := strings.TrimSpace(request.URL.Query().Get("q"))
		offset := 0
		limit  := 25

		if val := request.URL.Query().Get("offset"); val != "" {

			if num, err := strconv.Atoi(val); err == nil && num > 0 {
				offset = num
			}

		}

		if val := request.URL.Query().Get("limit"); val != "" {

			if num, err := strconv.Atoi(val); err == nil && num > 0 {
				limit = num
			}

		}

		if query != "" {

			result_set, err0 := manager.Search(query, offset, limit)

			if err0 == nil {

				results := make([]search_result, len(result_set.Results))

				for index, result := range result_set.Results {

					render_path := result.Path

					if strings.HasPrefix(render_path, "C/") {
						render_path = render_path[2:]
					}

					results[index] = search_result{
						URL:        fmt.Sprintf("/%s/%s", result.File, render_path),
						DocumentID: result.DocumentID,
						File:       result.File,
						Path:       result.Path,
						Title:      result.Title,
						Score:      result.Score,
						Snippet:    result.Snippet,
						WordCount:  result.WordCount,
					}

				}

				payload, err1 := json.MarshalIndent(map[string]any{
					"query":     query,
					"offset":    offset,
					"limit":     limit,
					"estimated": result_set.EstimatedMatches,
					"results":   results,
				}, "", "\t")

				if err1 == nil {

					response.Header().Set("Content-Type", "application/json")
					response.Header().Set("Content-Length", strconv.Itoa(len(payload)))
					response.WriteHeader(http.StatusOK)
					response.Write(payload)

				} else {

					response.Header().Set("Content-Type", "application/json")
					response.WriteHeader(http.StatusInternalServerError)
					response.Write([]byte{})

				}

			} else {

				response.Header().Set("Content-Type", "application/json")
				response.WriteHeader(http.StatusInternalServerError)
				response.Write([]byte{})

			}

		} else {

			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(http.StatusBadRequest)
			response.Write([]byte{})

		}

	} else {

		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusMethodNotAllowed)
		response.Write([]byte{})

	}

}
