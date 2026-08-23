package api

import "github.com/cookiengineer/zimdex/filters"
import "encoding/json"
import "net/http"
import "strconv"

type filter_result struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}

func Filters(response http.ResponseWriter, request *http.Request) {

	if request.Method == http.MethodGet {

		infos := make([]filter_result, len(filters.Registry))

		for index, filter := range filters.Registry {

			infos[index] = filter_result{
				Name:        filter.Name(),
				Description: filter.Description(),
				IsDefault:   filter.IsDefault(),
			}

		}

		payload, err := json.MarshalIndent(map[string]any{
			"filters": infos,
		}, "", "\t")

		if err == nil {

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
		response.WriteHeader(http.StatusMethodNotAllowed)
		response.Write([]byte{})

	}

}
