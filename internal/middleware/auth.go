package middleware

import (
	"MyGateway/internal/clients"
	"MyGateway/internal/utils"
	"context"
	"net/http"
)

func Auth(registry map[string]*clients.Client) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key, err := utils.GetAuthKey(r)
			if err != nil {
				http.Error(w, "Invalid client", http.StatusUnauthorized)
				return
			}
			if _, ok := registry[key]; !ok {
				http.Error(w, "Invalid client", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), "client_key", key)))
		})
	}
}
