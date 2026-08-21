package middlewares

import "log"
import "net/http"
import "time"

func Log(next http.Handler) http.Handler {

	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {

		start   := time.Now()
		handler := &response_writer{
			ResponseWriter: response,
			status_code:    http.StatusOK,
		}

		next.ServeHTTP(handler, request)

		duration := time.Since(start)

		log.Printf("%s %s: %d (%s)", request.Method, request.URL.Path, handler.status_code, duration.Round(time.Millisecond))

	})

}
