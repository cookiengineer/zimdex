package zim

import "github.com/cookiengineer/zimdex/io/zimfs"
import utils_zim "github.com/cookiengineer/zimdex/internal/utils/zim"
import "encoding/json"
import "net/http"
import "strconv"

func Render(manager *zimfs.Manager, response http.ResponseWriter, request *http.Request) {

	if request.Method == http.MethodGet {

		zim_file := request.PathValue("zimfile")
		zim_path := request.PathValue("zimpath")

		if strings.HasSuffix(zim_file, ".zim") {

			if request.URL.RawQuery != "" {
				zim_path = fmt.Sprintf("%s?%s", zim_path, request.URL.RawQuery)
			}

			archive := manager.Get(zim_file)

			if archive != nil {

				payload, mime_type, err := utils_zim.Render(archive, zim_file, zim_path)

				if err == nil {

					response.Header().Set("Content-Type", mime_type)
					response.Header().Set("Content-Length", strconv.Itoa(len(payload)))
					response.WriteHeader(http.StatusOK)
					response.Write(payload)

				} else {

					response.Header().Set("Content-Type", mime_type)

					if strings.Contains(strings.ToLower(err.Error()), "not found") {
						response.WriteHeader(http.StatusNotFound)
					} else {
						response.WriteHeader(http.StatusInternalServerError)
					}

					response.Write([]byte{})

				}

			} else {

				response.Header().Set("Content-Type", "application/octet-stream")
				response.WriteHeader(http.StatusNotFound)
				response.Write([]byte{})

			}

		} else {

			response.Header().Set("Content-Type", "application/octet-stream")
			response.WriteHeader(http.StatusBadRequest)
			response.Write([]byte{})

		}

	} else {

		response.Header().Set("Content-Type", "application/octet-stream")
		response.WriteHeader(http.StatusMethodNotAllowed)
		response.Write([]byte{})

	}

}
