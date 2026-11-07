package middleware

import (
	"MyGateway/internal/clients"
	"log"
	"net/http"
	"strings"
)

func AuthAndLimit(registry map[string]*clients.Client) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, "Invalid Authorization header", http.StatusUnauthorized)
				return
			}
			key := strings.TrimPrefix(auth, "Bearer ")
			client := registry[key]
			if client == nil {
				http.Error(w, "Invalid client", http.StatusUnauthorized)
				return
			}

			limiter := client.Limiter
			if !limiter.Allow() {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				_, err := w.Write([]byte("Too many requests"))
				if err != nil {
					log.Printf("Error writing response: %v", err)
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
