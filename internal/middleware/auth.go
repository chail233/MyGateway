package middleware

import (
	"MyGateway/internal/config"
	"crypto/subtle"
	"net/http"
	"strings"
)

func AuthMiddleware(cfg *config.Config) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, "Invalid Authorization header", http.StatusUnauthorized)
				return
			}
			key := strings.TrimPrefix(auth, "Bearer ")
			ok := subtle.ConstantTimeCompare([]byte(key), []byte(cfg.APIKey)) == 1
			if ok {
				next.ServeHTTP(w, r)
			} else {
				http.Error(w, "Invalid API key", http.StatusUnauthorized)
			}
		})
	}
}
