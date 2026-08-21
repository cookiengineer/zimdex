package api

import "github.com/cookiengineer/zimdex/io/zimfs"
import "encoding/json"
import "net/http"
import "strconv"

func Archives(manager *zimfs.Manager, response http.ResponseWriter, request *http.Request) {

	if request.Method == http.MethodGet {

		archives     := manager.List()
		payload, err := json.MarshalIndent(map[string]any{
			"archives": archives,
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
