package proxy

import (
	"MyGateway/internal/clients"
	"bytes"
	"io"
	"net/http"
	"net/textproto"
	"net/url"
	"slices"
	"strings"
)

type flushWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func newFlushWriter(w http.ResponseWriter) flushWriter {
	return flushWriter{
		w:  w,
		rc: http.NewResponseController(w),
	}
}

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if err != nil {
		return n, err
	}
	if err := fw.rc.Flush(); err != nil {
		return n, err
	}
	return n, nil
}

func NewCompletionRequest(upstream clients.Upstream, r *http.Request, body []byte) (*http.Request, error) {
	completionURL, err := url.JoinPath(upstream.BaseURL, completionPath)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		completionURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = r.Header.Clone()
	req.Header.Del("Accept-Encoding")
	req.Header.Set("Authorization", "Bearer "+upstream.Key)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

func copyUpstreamHeader(dst, src http.Header) {
	drop := slices.Clone(hopByHopHeaders)

	for _, v := range src.Values("Connection") {
		for name := range strings.SplitSeq(v, ",") {
			name = textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(name))
			if name == "" {
				continue
			}
			drop = append(drop, name)
		}
	}
	for name, values := range src {
		if slices.Contains(drop, name) {
			continue
		}
		for _, v := range values {
			dst.Add(name, v)
		}
	}
}

func WriteCompletionResponse(w http.ResponseWriter, resp *http.Response) error {
	copyUpstreamHeader(w.Header(), resp.Header)

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "Error reading response body", http.StatusInternalServerError)
			return err
		}
		w.WriteHeader(resp.StatusCode)
		_, err = w.Write(respBody)
		if err != nil {
			return err
		}
	} else if strings.Contains(contentType, "text/event-stream") {
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(newFlushWriter(w), resp.Body); err != nil {
			return err
		}
	} else {
		w.WriteHeader(resp.StatusCode)
		_, err := io.Copy(w, resp.Body)
		if err != nil {
			return err
		}
	}
	return nil
}
