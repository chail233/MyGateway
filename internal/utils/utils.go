package utils

import (
	"errors"
	"net/http"
	"strings"
)

func GetAuthKey(r *http.Request) (string, error) {
	key := r.Header.Get("Authorization")
	if !strings.HasPrefix(key, "Bearer ") {
		return "", errors.New("invalid authorization header format")
	}
	key = strings.TrimPrefix(key, "Bearer ")
	if key == "" {
		return "", errors.New("authorization header is missing")
	}
	return key, nil
}
