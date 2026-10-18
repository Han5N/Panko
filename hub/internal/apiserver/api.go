// Package apiserver 提供 Panko 的对外 HTTP API。
package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"panko/hub/internal/coreclient"
	"panko/hub/internal/events"
	"panko/hub/internal/plugin"
)

const callTimeout = 5 * time.Second

// Server 持有 API 依赖。
type Server struct {
	version string
	client  *coreclient.Client
	bus     *events.Bus
	plugins *plugin.Host
}

// New 创建 API 路由。
func New(version string, client *coreclient.Client, bus *events.Bus, plugins *plugin.Host) *Server {
	return &Server{version: version, client: client, bus: bus, plugins: plugins}
}

// Handler 返回完整的 API 路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /v1/status", s.status)
	mux.HandleFunc("GET /v1/plugins", s.listPlugins)
	mux.HandleFunc("POST /v1/plugins/{name}/{action}", s.pluginAction)
	mux.HandleFunc("POST /v1/say", s.say)
	mux.HandleFunc("POST /v1/command", s.command)
	mux.HandleFunc("GET /v1/blocks", s.getBlock)
	mux.HandleFunc("POST /v1/blocks", s.getBlocks)
	mux.HandleFunc("GET /v1/entities", s.listEntities)
	mux.HandleFunc("GET /v1/events", s.events)

	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "panko-api",
		"version": s.version,
		"core":    s.client.State(),
	})
}

type botStatus struct {
	State           string `json:"state"`
	PlayerName      string `json:"player_name"`
	ServerAddress   string `json:"server_address"`
	ProtocolVersion string `json:"protocol_version"`
	OnlineSince     int64  `json:"online_since,omitempty"`
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	if s.client.State() != "online" {
		writeJSON(w, http.StatusOK, map[string]any{
			"state": "offline",
			"core":  s.client.State(),
		})
		return
	}

	var st botStatus
	if err := s.call(r.Context(), "bot.status", nil, &st); err != nil {
		writeCoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) say(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if strings.TrimSpace(body.Text) == "" {
		writeError(w, http.StatusBadRequest, "empty_text", "text 不能为空")
		return
	}

	var out map[string]any
	if err := s.call(r.Context(), "bot.say", map[string]any{"text": body.Text}, &out); err != nil {
		writeCoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) command(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Command string `json:"command"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if strings.TrimSpace(body.Command) == "" {
		writeError(w, http.StatusBadRequest, "empty_command", "command 不能为空")
		return
	}

	var out map[string]any
	if err := s.call(r.Context(), "bot.command", map[string]any{"command": body.Command}, &out); err != nil {
		writeCoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getBlock(w http.ResponseWriter, r *http.Request) {
	pos, err := parsePos(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_position", err.Error())
		return
	}

	var out map[string]any
	if err := s.call(r.Context(), "world.block", pos, &out); err != nil {
		writeCoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getBlocks(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Positions []map[string]int `json:"positions"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if len(body.Positions) == 0 {
		writeError(w, http.StatusBadRequest, "empty_positions", "positions 不能为空")
		return
	}
	if len(body.Positions) > 4096 {
		writeError(w, http.StatusBadRequest, "too_many_positions", "一次最多查询 4096 个方块")
		return
	}

	var out map[string]any
	if err := s.call(r.Context(), "world.blocks", map[string]any{"positions": body.Positions}, &out); err != nil {
		writeCoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listEntities(w http.ResponseWriter, r *http.Request) {
	pos, err := parsePos(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_position", err.Error())
		return
	}

	radius, err := strconv.ParseFloat(r.URL.Query().Get("radius"), 64)
	if err != nil || radius <= 0 || radius > 256 {
		writeError(w, http.StatusBadRequest, "invalid_radius", "radius 必须是 0 到 256 之间的数字")
		return
	}

	params := map[string]any{"x": pos["x"], "y": pos["y"], "z": pos["z"], "radius": radius}
	var out map[string]any
	if err := s.call(r.Context(), "world.entities", params, &out); err != nil {
		writeCoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listPlugins(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"plugins": s.plugins.List()})
}

func (s *Server) pluginAction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	action := r.PathValue("action")

	var err error
	switch action {
	case "start":
		err = s.plugins.Start(name)
	case "stop":
		err = s.plugins.Stop(name)
	case "restart":
		err = s.plugins.Restart(name)
	default:
		writeError(w, http.StatusNotFound, "unknown_action", "未知操作 "+action)
		return
	}
	if err != nil {
		writeError(w, http.StatusNotFound, "plugin_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "no_stream", "当前连接不支持流式响应")
		return
	}

	var topics map[string]bool
	if raw := r.URL.Query().Get("topics"); raw != "" {
		topics = make(map[string]bool)
		for _, t := range strings.Split(raw, ",") {
			if t = strings.TrimSpace(t); t != "" {
				topics[t] = true
			}
		}
	}

	since := s.bus.LastSeq()
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		if v, err := strconv.ParseUint(raw, 10, 64); err == nil {
			since = v
		}
	}
	if raw := r.URL.Query().Get("since"); raw != "" {
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_since", "since 必须是非负整数")
			return
		}
		since = v
	}

	sub := s.bus.Subscribe(topics, since)
	defer sub.Close()

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	if sub.Gap {
		fmt.Fprint(w, "event: gap\ndata: {\"message\":\"部分事件已丢失，请重新获取快照\"}\n\n")
	}
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-sub.C:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "id: %d\ndata: %s\n\n", ev.Seq, data)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) call(ctx context.Context, method string, params any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	return s.client.Do(ctx, method, params, out)
}

func parsePos(r *http.Request) (map[string]int, error) {
	q := r.URL.Query()
	out := make(map[string]int, 3)
	for _, axis := range []string{"x", "y", "z"} {
		v, err := strconv.Atoi(q.Get(axis))
		if err != nil {
			return nil, fmt.Errorf("%s 必须是整数", axis)
		}
		out[axis] = v
	}
	return out, nil
}

func readJSON(r *http.Request, dst any) error {
	body := http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("请求体为空")
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func writeCoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, coreclient.ErrUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "core_unavailable", "核心未连接")
		return
	}
	var rpcErr *coreclient.RPCError
	if errors.As(err, &rpcErr) {
		writeError(w, http.StatusBadGateway, rpcErr.Code, rpcErr.Message)
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		writeError(w, http.StatusGatewayTimeout, "core_timeout", "核心响应超时")
		return
	}
	writeError(w, http.StatusBadGateway, "core_error", err.Error())
}
