// Package mockcore 是一个用于开发和测试的假 botd。它实现与真实核心相同的
// 行分隔 JSON 协议，但不连接任何游戏服务器。
package mockcore

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type request struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type client struct {
	conn  net.Conn
	out   chan any
	done  chan struct{}
	close sync.Once
}

func (c *client) shutdown() {
	c.close.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

// Server 是模拟核心实例。
type Server struct {
	ln net.Listener

	mu     sync.Mutex
	seq    uint64
	ring   []event
	conns  map[*client]struct{}
	closed bool
}

type event struct {
	Seq     uint64 `json:"seq"`
	Topic   string `json:"topic"`
	Time    string `json:"time"`
	Payload any    `json:"payload,omitempty"`
}

// Start 在 addr 上监听。addr 传 "127.0.0.1:0" 可让系统分配端口。
func Start(addr string) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &Server{ln: ln, conns: make(map[*client]struct{})}
	go s.accept()
	return s, nil
}

// Addr 返回实际监听地址。
func (s *Server) Addr() string {
	return s.ln.Addr().String()
}

// Close 关闭监听并断开所有客户端。
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	conns := make([]*client, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	for _, c := range conns {
		c.shutdown()
	}
	return s.ln.Close()
}

// Publish 主动推送一条事件，用于测试和演示。
func (s *Server) Publish(topic string, payload any) uint64 {
	s.mu.Lock()
	s.seq++
	ev := event{
		Seq:     s.seq,
		Topic:   topic,
		Time:    time.Now().UTC().Format(time.RFC3339Nano),
		Payload: payload,
	}
	if len(s.ring) == 256 {
		copy(s.ring, s.ring[1:])
		s.ring[len(s.ring)-1] = ev
	} else {
		s.ring = append(s.ring, ev)
	}
	conns := make([]*client, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	for _, c := range conns {
		select {
		case c.out <- map[string]any{"event": ev}:
		case <-c.done:
		default:
		}
	}
	return ev.Seq
}

func (s *Server) accept() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		c := &client{conn: conn, out: make(chan any, 256), done: make(chan struct{})}
		s.mu.Lock()
		s.conns[c] = struct{}{}
		s.mu.Unlock()

		go s.writeLoop(c)
		go s.readLoop(c)
	}
}

func (s *Server) writeLoop(c *client) {
	enc := json.NewEncoder(c.conn)
	for {
		select {
		case <-c.done:
			return
		case msg := <-c.out:
			if err := enc.Encode(msg); err != nil {
				return
			}
		}
	}
}

func (s *Server) readLoop(c *client) {
	defer func() {
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
		c.shutdown()
	}()

	dec := json.NewDecoder(bufio.NewReaderSize(c.conn, 1<<20))
	for {
		var req request
		if err := dec.Decode(&req); err != nil {
			if err != io.EOF {
				// 客户端断开是正常情况。
			}
			return
		}
		s.handle(c, req)
	}
}

func (s *Server) handle(c *client, req request) {
	switch req.Method {
	case "events.subscribe":
		var params struct {
			Since uint64 `json:"since"`
		}
		_ = json.Unmarshal(req.Params, &params)

		s.mu.Lock()
		for _, ev := range s.ring {
			if ev.Seq > params.Since {
				select {
				case c.out <- map[string]any{"event": ev}:
				default:
				}
			}
		}
		s.mu.Unlock()
		s.reply(c, req.ID, map[string]any{"since": params.Since})

	case "bot.status":
		s.reply(c, req.ID, map[string]any{
			"state":            "online",
			"player_name":      "PankoBot",
			"server_address":   "mock.local:19132",
			"protocol_version": "mock",
		})

	case "bot.say":
		var params struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(req.Params, &params)
		s.Publish("chat", map[string]any{"sender": "PankoBot", "message": params.Text})
		s.reply(c, req.ID, map[string]any{"ok": true})

	case "bot.command":
		var params struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(req.Params, &params)
		s.reply(c, req.ID, map[string]any{"ok": true, "output": "mock: " + params.Command})

	case "world.block":
		s.reply(c, req.ID, map[string]any{
			"loaded": true,
			"block":  map[string]any{"name": "minecraft:stone", "states": map[string]string{}},
		})

	case "world.blocks":
		var params struct {
			Positions []map[string]int `json:"positions"`
		}
		_ = json.Unmarshal(req.Params, &params)
		blocks := make([]map[string]any, 0, len(params.Positions))
		for range params.Positions {
			blocks = append(blocks, map[string]any{"name": "minecraft:stone", "states": map[string]string{}})
		}
		s.reply(c, req.ID, map[string]any{"blocks": blocks})

	case "world.entities":
		s.reply(c, req.ID, map[string]any{"entities": []any{}})

	case "world.place":
		s.reply(c, req.ID, map[string]any{"accepted": 0, "rejected": 0})

	case "raw.send":
		s.reply(c, req.ID, map[string]any{"ok": true})

	default:
		s.replyError(c, req.ID, "method_not_found", "模拟核心不支持 "+req.Method)
	}
}

func (s *Server) reply(c *client, id uint64, v any) {
	data, _ := json.Marshal(v)
	c.out <- map[string]any{"id": id, "result": json.RawMessage(data)}
}

func (s *Server) replyError(c *client, id uint64, code, msg string) {
	c.out <- map[string]any{"id": id, "error": rpcError{Code: code, Message: msg}}
}

// String 便于日志输出。
func (s *Server) String() string {
	return fmt.Sprintf("mockcore(%s)", s.Addr())
}
