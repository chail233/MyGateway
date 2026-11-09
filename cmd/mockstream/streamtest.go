package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// mockUpstreamKeys 扮演 OpenAI 侧的真实密钥（sk- 开头，与网关对外的 Key 完全不同）。
// 只有持有这些 Key 的请求才会被接受，用来验证网关确实把「客户端 Key」换成了「上游 Key」。
var mockUpstreamKeys = map[string]string{
	"sk-mock-alice": "alice",
	"sk-mock-bob":   "bob",
}

const mockChatModel = "gpt-4o-mini"

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

// writeJSONError 复刻 OpenAI 的错误响应形状，便于网关原样透传错误。
func writeJSONError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    code,
			"code":    status,
		},
	})
}

type chatRequest struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

// chatHandler 模拟 OpenAI POST /v1/chat/completions：
// 校验上游 Key，并按请求体 stream 字段在「普通 JSON」与「SSE 流」之间分流。
func chatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	auth := r.Header.Get("Authorization")
	owner, ok := mockUpstreamKeys[strings.TrimPrefix(auth, "Bearer ")]
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "invalid_request_error",
			"Incorrect API key provided: 网关没有把客户端 Key 换成上游 Key")
		return
	}
	// 非标准响应头，仅供测试脚本断言「这个请求是用哪个上游 Key 发出来的」。
	w.Header().Set("X-Mock-Upstream-Client", owner)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", "failed to read body")
		return
	}
	var req chatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	if len(req.Messages) == 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", "messages is required")
		return
	}

	model := req.Model
	if model == "" {
		model = mockChatModel
	}
	prompt := req.Messages[len(req.Messages)-1].Content

	if req.Stream {
		writeChatStream(w, model, prompt)
		return
	}
	// 模拟真实上游/CDN 的行为：客户端声明 Accept-Encoding: gzip 时就压缩响应体。
	// 流式分支不做压缩（SSE 实际也很少压缩）。
	useGzip := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
	writeChatJSON(w, model, prompt, useGzip)
}

func writeChatJSON(w http.ResponseWriter, model, prompt string, useGzip bool) {
	payload, err := json.Marshal(map[string]any{
		"id":      chatID(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{map[string]any{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": "mock 回复：你说了「" + prompt + "」"},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 9, "completion_tokens": 12, "total_tokens": 21},
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "server_error", "failed to encode response")
		return
	}
	payload = append(payload, '\n')

	w.Header().Set("Content-Type", "application/json")
	if !useGzip {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
		return
	}

	// 只声明 Content-Encoding，不设 Content-Length（压缩后长度未知，交给 chunked）。
	// 网关若把 Accept-Encoding 透传给上游却没把 Content-Encoding 透传给客户端，
	// 客户端就会拿到不可读的 gzip 字节——这正是测试脚本要暴露的场景。
	w.Header().Set("Content-Encoding", "gzip")
	w.WriteHeader(http.StatusOK)
	gz := gzip.NewWriter(w)
	if _, err := gz.Write(payload); err != nil {
		return
	}
	_ = gz.Close()
}

// writeChatStream 按 OpenAI 的 chunk 格式逐块下发，块之间留 300ms 间隔，
// 以便用 curl.exe -N 肉眼确认网关是「逐字」转发而不是攒完一次吐出。
func writeChatStream(w http.ResponseWriter, model, prompt string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ctrl := http.NewResponseController(w)
	id := chatID()
	created := time.Now().Unix()

	send := func(delta map[string]any, finish any) bool {
		payload, err := json.Marshal(map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   model,
			"choices": []any{map[string]any{
				"index":         0,
				"delta":         delta,
				"finish_reason": finish,
			}},
		})
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
			return false
		}
		return ctrl.Flush() == nil
	}

	if !send(map[string]any{"role": "assistant", "content": ""}, nil) {
		return
	}
	for _, piece := range []string{"mock ", "回复：", "你说了「", prompt, "」"} {
		time.Sleep(300 * time.Millisecond)
		if !send(map[string]any{"content": piece}, nil) {
			return
		}
	}
	if !send(map[string]any{}, "stop") {
		return
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	_ = ctrl.Flush()
}

func chatID() string {
	return "chatcmpl-mock-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

func main() {

	addr := ":5001"
	mux := http.NewServeMux()
	mux.HandleFunc("/stream", streamHandler)
	mux.HandleFunc("/echo", echoHandler)
	mux.HandleFunc("/v1/chat/completions", chatHandler)

	err := http.ListenAndServe(addr, mux)
	if err != nil {
		return
	}
}
