package main

import (
	"MyGateway/internal/config"
	"MyGateway/internal/middleware"
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

var transport = &http.Transport{
	ResponseHeaderTimeout: 5 * time.Second,
}
var client = &http.Client{Transport: transport}
var cfg *config.Config

func helloHandler(w http.ResponseWriter, r *http.Request) {
	_, err := fmt.Fprintf(w, "Hello from MyGateway! 你访问的是: %s\n", r.URL.Path)
	if err != nil {
		log.Printf("Error writing response: %v", err)
		return
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err := fmt.Fprint(w, `{"status":"ok"}`)
	if err != nil {
		log.Printf("Error writing response: %v", err)
	}
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

func streamHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		"http://127.0.0.1:5001/stream", bytes.NewReader(body))
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

	scanner := bufio.NewScanner(resp.Body)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher := http.NewResponseController(w)
	for scanner.Scan() {
		line := scanner.Text()
		_, err := fmt.Fprintf(w, "%s\n\n", line)
		if err != nil {
			http.Error(w, "Error writing to client", http.StatusInternalServerError)
			return
		}
		err = flusher.Flush()
		if err != nil {
			http.Error(w, "Error flushing response", http.StatusInternalServerError)
			return
		}
	}
	if err := scanner.Err(); err != nil {
		http.Error(w, "Error reading response body", http.StatusInternalServerError)
		return
	}
}

func main() {
	configPath := flag.String("config", "./config.json", "Path to the config file")
	flag.Parse()
	var err error
	cfg, err = config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /hello", helloHandler)
	mux.HandleFunc("GET /health", healthHandler)
	mux.Handle("POST /proxy", middleware.RateLimit(middleware.AuthMiddleware(cfg, http.HandlerFunc(proxyHandler))))
	mux.Handle("POST /stream", middleware.RateLimit(middleware.AuthMiddleware(cfg, http.HandlerFunc(streamHandler))))

	log.Printf("MyGateway 启动，监听 http://localhost%s", cfg.Port)

	if err = http.ListenAndServe(cfg.Port, mux); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
