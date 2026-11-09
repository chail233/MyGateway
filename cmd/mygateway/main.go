package main

import (
	"MyGateway/internal/clients"
	"MyGateway/internal/config"
	"MyGateway/internal/middleware"
	"MyGateway/internal/proxy"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
)

var gatewayClient = &http.Client{}
var cfg *config.Config
var registry map[string]*clients.Client

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

func completionHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}
	gatewayKey, _ := r.Context().Value("client_key").(string)
	client, ok := registry[gatewayKey]
	if !ok {
		http.Error(w, "Unknown client", http.StatusUnauthorized)
		return
	}
	if len(client.Upstream) == 0 {
		http.Error(w, "No upstream configured", http.StatusInternalServerError)
		return
	}

	req, err := proxy.NewCompletionRequest(client.Upstream[0], r, body)
	if err != nil {
		http.Error(w, "Error creating request", http.StatusInternalServerError)
		return
	}
	resp, err := gatewayClient.Do(req)
	if err != nil {
		http.Error(w, "Error making request", http.StatusBadGateway)
		log.Printf("Error making request: %v", err)
		return
	}
	defer resp.Body.Close()

	err = proxy.WriteCompletionResponse(w, resp)
	if err != nil {
		log.Printf("Error writing response: %v", err)
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
	err = cfg.Validate()
	if err != nil {
		log.Fatalf("Config validation error: %v", err)
	}
	registry = make(map[string]*clients.Client, len(cfg.APIKeys))
	for _, k := range cfg.APIKeys {
		registry[k.Key] = &clients.Client{Name: k.Name, Upstream: k.Upstream}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", helloHandler)
	mux.HandleFunc("GET /health", healthHandler)
	mux.Handle("POST /v1/chat/completions", middleware.Chain(http.HandlerFunc(completionHandler), middleware.Auth(registry)))

	log.Printf("MyGateway 启动，监听 http://localhost%s", cfg.Port)

	if err = http.ListenAndServe(cfg.Port, mux); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
