package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// echoHandler mimics httpbin's /post response shape: {"json": <parsed body>, "headers": {k: first value}}
func echoHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	flat := make(map[string]string, len(r.Header))
	for k, v := range r.Header {
		if len(v) > 0 {
			flat[k] = v[0]
		}
	}
	resp := map[string]any{"headers": flat}
	if len(body) > 0 {
		var parsed any
		if json.Unmarshal(body, &parsed) == nil {
			resp["json"] = parsed
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		return
	}
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher := http.NewResponseController(w)
	for i := 1; i <= 14; i++ {
		_, err := fmt.Fprintf(w, "data: %d\n\n", i)
		if err != nil {
			http.Error(w, "Error writing to client", http.StatusInternalServerError)
			return
		}
		err = flusher.Flush()
		if err != nil {
			http.Error(w, "Error flushing response", http.StatusInternalServerError)
			return
		}
		time.Sleep(time.Millisecond * 500)
	}
}

func main() {

	addr := ":5001"
	mux := http.NewServeMux()
	mux.HandleFunc("/stream", streamHandler)
	mux.HandleFunc("/echo", echoHandler)

	err := http.ListenAndServe(addr, mux)
	if err != nil {
		return
	}
}
