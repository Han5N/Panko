// Package coreclient 负责与 botd 之间的连接：断线重连、请求响应、事件转发。
// 传输使用按行分隔的 JSON，方便抓包和跨语言调试。
package coreclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"panko/hub/internal/events"
)

// ErrUnavailable 表示核心尚未连接或连接已断开。
var ErrUnavailable = errors.New("核心未连接")

// RPCError 是 botd 返回的业务错误。
type RPCError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type request struct {
	ID     uint64 `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type response struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

type incoming struct {
	ID     *uint64         `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
	Event  *events.Event   `json:"event,omitempty"`
}

// Client 维护与 botd 的唯一连接。
type Client struct {
	addr string
	bus  *events.Bus
	log  *log.Logger

	mu      sync.Mutex
	conn    net.Conn
	enc     *json.Encoder
	nextID  uint64
	pending map[uint64]chan response
	state   string
	lastSeq uint64
}

// New 创建客户端。调用 Run 才会真正建立连接。
func New(addr string, bus *events.Bus, logger *log.Logger) *Client {
	return &Client{
		addr:    addr,
		bus:     bus,
		log:     logger,
		pending: make(map[uint64]chan response),
		state:   "offline",
	}
}

// State 返回 offline / connecting / online。
func (c *Client) State() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// Run 阻塞运行直到 ctx 取消，期间自动重连。
func (c *Client) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		c.setState("connecting")
		conn, err := net.DialTimeout("tcp", c.addr, 3*time.Second)
		if err != nil {
			c.log.Printf("连接核心 %s 失败: %v", c.addr, err)
			if !sleep(ctx, backoff) {
				break
			}
			backoff = nextBackoff(backoff)
			continue
		}

		backoff = time.Second
		err = c.serve(ctx, conn)
		if ctx.Err() != nil {
			break
		}
		if err != nil {
			c.log.Printf("核心连接断开: %v", err)
		}
		if !sleep(ctx, backoff) {
			break
		}
		backoff = nextBackoff(backoff)
	}
	c.setState("offline")
}

func (c *Client) serve(ctx context.Context, conn net.Conn) error {
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(bufio.NewReaderSize(conn, 1<<20))

	c.mu.Lock()
	c.conn = conn
	c.enc = enc
	c.state = "online"
	c.mu.Unlock()

	c.log.Printf("已连接核心 %s", c.addr)
	defer func() {
		c.mu.Lock()
		c.conn = nil
		c.enc = nil
		c.state = "offline"
		c.mu.Unlock()
		conn.Close()
		c.failPending(ErrUnavailable)
	}()

	// 先启动读取循环，订阅请求的响应才能被分发。
	readErr := make(chan error, 1)
	go func() { readErr <- c.readLoop(dec) }()

	// 订阅事件并请求回放。必须在处理事件之前完成。
	subCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err := c.Do(subCtx, "events.subscribe", map[string]any{"since": c.lastSeq}, nil)
	cancel()
	if err != nil {
		return fmt.Errorf("订阅事件失败: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-readErr:
		return err
	}
}

func (c *Client) readLoop(dec *json.Decoder) error {
	for {
		var msg incoming
		if err := dec.Decode(&msg); err != nil {
			return err
		}
		c.dispatch(msg)
	}
}

func (c *Client) dispatch(msg incoming) {
	if msg.ID != nil {
		c.mu.Lock()
		ch, ok := c.pending[*msg.ID]
		delete(c.pending, *msg.ID)
		c.mu.Unlock()
		if ok {
			ch <- response{ID: *msg.ID, Result: msg.Result, Error: msg.Error}
		}
		return
	}

	if msg.Event != nil {
		c.mu.Lock()
		if msg.Event.Seq > c.lastSeq {
			c.lastSeq = msg.Event.Seq
		}
		c.mu.Unlock()
		c.bus.Publish(*msg.Event)
	}
}

// Do 发起一次调用。ctx 到期或连接断开都会返回错误。
func (c *Client) Do(ctx context.Context, method string, params any, result any) error {
	c.mu.Lock()
	enc := c.enc
	if enc == nil {
		c.mu.Unlock()
		return ErrUnavailable
	}
	c.nextID++
	id := c.nextID
	ch := make(chan response, 1)
	c.pending[id] = ch
	err := enc.Encode(request{ID: id, Method: method, Params: params})
	if err != nil {
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("发送 %s 失败: %w", method, err)
	}
	c.mu.Unlock()

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return resp.Error
		}
		if result != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return fmt.Errorf("解析 %s 响应失败: %w", method, err)
			}
		}
		return nil
	}
}

func (c *Client) failPending(err error) {
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[uint64]chan response)
	c.mu.Unlock()

	for _, ch := range pending {
		ch <- response{Error: &RPCError{Code: "unavailable", Message: err.Error()}}
	}
}

func (c *Client) setState(state string) {
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > 15*time.Second {
		return 15 * time.Second
	}
	return d
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
