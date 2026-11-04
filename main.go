package main

import (
	"MyGateway/internal/config"
	"MyGateway/internal/middleware"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

var client = &http.Client{Timeout: 10 * time.Second}
var cfg *config.Config

func helloHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello from MyGateway! 你访问的是: %s\n", r.URL.Path)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok"}`)
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		cfg.BaseURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "Error creating request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Error making request", http.StatusBadGateway)
		log.Printf("Error making request: %v", err)
		return
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Error reading response body", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, err = w.Write(respBody)
	if err != nil {
		log.Printf("Error writing response body: %v", err)
		return
	}
}

func main() {
	var err error
	cfg, err = config.LoadConfig("./config.json")
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /hello", helloHandler)
	mux.HandleFunc("GET /health", healthHandler)
	mux.Handle("POST /proxy", middleware.AuthMiddleware(cfg, http.HandlerFunc(proxyHandler)))

	log.Printf("MyGateway 启动，监听 http://localhost%s", cfg.Port)

	if err = http.ListenAndServe(cfg.Port, mux); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
