package gateway

import (
	"fmt"
	"io"
	"net/http"
)

// SetSSEHeaders 设置 SSE 响应头
func SetSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
}

// WriteSSEFrame 写入符合 OpenAI SSE 规范的帧
func WriteSSEFrame(w io.Writer, event string, data []byte) error {
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", string(data)); err != nil {
		return err
	}
	
	// 如果是 http.Flusher，则立即推送
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// WriteDoneFrame 写入 SSE 终止帧
func WriteDoneFrame(w io.Writer) error {
	return WriteSSEFrame(w, "done", []byte("[DONE]"))
}
