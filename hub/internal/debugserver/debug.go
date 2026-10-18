// Package debugserver 提供仅在本机监听的调试接口：SSE 实时日志与健康检查。
package debugserver

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LogHub 把日志行广播给所有 SSE 订阅者。订阅者处理不过来时丢弃最旧的一条，
// 避免调试服务反过来阻塞业务日志。
type LogHub struct {
	mu   sync.Mutex
	subs map[chan string]struct{}
}

// NewLogHub 创建广播中心。
func NewLogHub() *LogHub {
	return &LogHub{subs: make(map[chan string]struct{})}
}

// Write 实现 io.Writer，可以直接交给 log.SetOutput 使用。
func (h *LogHub) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\r\n"), "\n") {
		h.Broadcast(line)
	}
	return len(p), nil
}

// Broadcast 把一行日志推送给所有订阅者。
func (h *LogHub) Broadcast(line string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for ch := range h.subs {
		select {
		case ch <- line:
		default:
			// 队列已满，丢掉最旧的一条再写入，保证订阅者看到的是最新日志。
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- line:
			default:
			}
		}
	}
}

func (h *LogHub) subscribe() chan string {
	ch := make(chan string, 256)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *LogHub) unsubscribe(ch chan string) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	close(ch)
}

// Handler 返回调试服务的路由。
func Handler(hub *LogHub, version string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintf(w, `{"status":"ok","service":"panko-debug","version":%q}`, version)
	})

	mux.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "当前连接不支持流式响应", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()

		ch := hub.subscribe()
		defer hub.unsubscribe(ch)

		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case line, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", line)
				flusher.Flush()
			case <-heartbeat.C:
				fmt.Fprint(w, ": ping\n\n")
				flusher.Flush()
			}
		}
	})

	return mux
}

// 确保 LogHub 在编译期满足 io.Writer。
var _ io.Writer = (*LogHub)(nil)
