package middlewares

import "net/http"

func Join(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {

	for m := len(middlewares) - 1; m >= 0; m-- {
		handler = middlewares[m](handler)
	}

	return handler

}
