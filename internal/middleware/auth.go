package middleware

import (
	"MyGateway/internal/clients"
	"net/http"
	"strings"
)

func Auth(registry map[string]*clients.Client) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, "Invalid Authorization header", http.StatusUnauthorized)
				return
			}
			key := strings.TrimPrefix(auth, "Bearer ")
			if _, ok := registry[key]; !ok {
				http.Error(w, "Invalid client", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
