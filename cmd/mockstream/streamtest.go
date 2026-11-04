package main

import (
	"fmt"
	"net/http"
	"time"
)

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

	err := http.ListenAndServe(addr, mux)
	if err != nil {
		return
	}
}
