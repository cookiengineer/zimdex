package middlewares

import "net/http"
import "strings"

func ContentSecurityPolicy(next http.Handler) http.Handler {

	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {

		response.Header().Set("Content-Security-Policy", strings.Join([]string{
			"default-src 'self'",
			"script-src 'self' 'unsafe-inline'",
			"style-src 'self' 'unsafe-inline'",
			"object-src 'none'",
			"base-uri 'self'",
			"form-action 'self'",
		}, ";"))

		next.ServeHTTP(response, request)

	})

}
