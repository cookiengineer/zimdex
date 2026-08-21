package middlewares

import "log"
import "net/http"
import "runtime/debug"

func Recover(next http.Handler) http.Handler {

	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {

		defer func() {

			if reason := recover(); reason != nil {

				log.Printf("PANIC: %v\n%s", reason, debug.Stack())

				http.Error(response, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

			}

		}()

		next.ServeHTTP(response, request)

	})

}

