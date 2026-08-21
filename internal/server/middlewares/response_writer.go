package middlewares

import "net/http"

type response_writer struct {
	http.ResponseWriter
	status_code int
}

func (writer *response_writer) WriteHeader(code int) {

	writer.status_code = code
	writer.ResponseWriter.WriteHeader(code)

}

