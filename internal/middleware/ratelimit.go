package middleware

import (
	"log"
	"net/http"

	"golang.org/x/time/rate"
)

var limiter = rate.NewLimiter(2, 3)

func RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, err := w.Write([]byte("Too many requests"))
			if err != nil {
				log.Printf("Error writing response body: %v", err)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}
