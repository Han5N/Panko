package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"

	"panko/hub/internal/coreclient"
	"panko/hub/internal/events"
)

// State 是插件进程的当前状态。
type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateBackoff  State = "backoff"
	StateFailed   State = "failed"
)

const (
	maxFailures   = 5
	eventQueue    = 256
	maxBackoff    = 30 * time.Second
	requestTimout = 5 * time.Second
)

// Instance 是一个已经加载、可独立启停的插件进程。
type Instance struct {
	manifest Manifest
	dir      string
	host     *Host
	logger   *log.Logger

	mu        sync.Mutex
	state     State
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	out       chan any
	done      chan struct{}
	stopped   bool
	failures  int
	lastErr   string
	startedAt time.Time
	topics    map[string]bool
}

func newInstance(m Manifest, dir string, host *Host) *Instance {
	topics := make(map[string]bool, len(m.Topics))
	for _, t := range m.Topics {
		topics[t] = true
	}
	return &Instance{
		manifest: m,
		dir:      dir,
		host:     host,
		logger:   loggerWith(host.logger, m.Name),
		state:    StateStopped,
		topics:   topics,
	}
}

func loggerWith(parent *log.Logger, name string) *log.Logger {
	return log.New(parent.Writer(), "["+name+"] ", parent.Flags())
}

// Start 启动插件。已经运行时为空操作。
func (p *Instance) Start() {
	p.mu.Lock()
	if p.state == StateRunning || p.state == StateStarting {
		p.mu.Unlock()
		return
	}
	p.stopped = false
	p.failures = 0
	p.state = StateStarting
	p.mu.Unlock()

	go p.supervise()
}

// Stop 停止插件并阻止自动重启。
func (p *Instance) Stop() {
	p.mu.Lock()
	p.stopped = true
	cmd := p.cmd
	done := p.done
	p.mu.Unlock()

	if cmd != nil && cmd.Process != nil && done != nil {
		p.notify("panko.shutdown", nil)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
		}
	}

	p.mu.Lock()
	if p.state != StateRunning {
		p.state = StateStopped
	}
	p.mu.Unlock()
}

// Restart 重启插件。
func (p *Instance) Restart() {
	p.Stop()
	p.Start()
}

func (p *Instance) supervise() {
	for {
		if p.isStopped() {
			p.setState(StateStopped)
			return
		}

		err := p.launchOnce()

		if p.isStopped() {
			p.setState(StateStopped)
			return
		}

		p.mu.Lock()
		p.failures++
		failures := p.failures
		if err != nil {
			p.lastErr = err.Error()
		} else {
			p.lastErr = "进程自行退出"
		}
		p.state = StateBackoff
		p.mu.Unlock()

		p.logger.Printf("进程退出: %v", p.lastErr)

		if failures >= maxFailures {
			p.setState(StateFailed)
			p.logger.Printf("连续失败 %d 次，已停止自动重启", failures)
			return
		}

		delay := time.Duration(1<<(failures-1)) * time.Second
		if delay > maxBackoff {
			delay = maxBackoff
		}
		p.logger.Printf("%s 后重启", delay)
		select {
		case <-time.After(delay):
		case <-p.host.ctx.Done():
			p.setState(StateStopped)
			return
		}
	}
}

func (p *Instance) launchOnce() error {
	cmd := exec.Command(p.manifest.Entry[0], p.manifest.Entry[1:]...)
	cmd.Dir = p.dir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("创建 stdin 失败: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建 stdout 失败: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("创建 stderr 失败: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动失败: %w", err)
	}

	out := make(chan any, eventQueue)
	done := make(chan struct{})

	p.mu.Lock()
	p.cmd = cmd
	p.stdin = stdin
	p.out = out
	p.done = done
	p.state = StateRunning
	p.startedAt = time.Now().UTC()
	p.lastErr = ""
	p.mu.Unlock()

	p.logger.Printf("已启动 (pid %d)", cmd.Process.Pid)

	go p.writeLoop(stdin, out, done)
	go p.readLoop(bufio.NewReaderSize(stdout, 1<<20))
	go p.readStderr(stderr)
	go p.notifyReady()

	err = cmd.Wait()

	p.mu.Lock()
	close(done)
	p.cmd = nil
	p.stdin = nil
	p.state = StateStopped
	p.mu.Unlock()
	_ = stdin.Close()

	return err
}

func (p *Instance) writeLoop(stdin io.WriteCloser, out chan any, done chan struct{}) {
	enc := json.NewEncoder(stdin)
	for {
		select {
		case <-done:
			return
		case msg := <-out:
			if err := enc.Encode(msg); err != nil {
				p.logger.Printf("写入失败: %v", err)
				return
			}
		}
	}
}

func (p *Instance) readLoop(r io.Reader) {
	dec := json.NewDecoder(r)
	for {
		var req rpcRequest
		if err := dec.Decode(&req); err != nil {
			if !errors.Is(err, io.EOF) {
				p.logger.Printf("读取失败: %v", err)
			}
			return
		}
		go p.handle(req)
	}
}

func (p *Instance) readStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		p.logger.Printf("%s", scanner.Text())
	}
}

// notifyReady 在进程启动后发送初始化信息。插件可以据此完成握手。
func (p *Instance) notifyReady() {
	p.send(rpcNotification{
		JSONRPC: "2.0",
		Method:  "panko.init",
		Params: map[string]any{
			"api_version": APIVersion,
			"plugin":      p.manifest.Name,
			"permissions": p.manifest.Permissions,
			"topics":      p.manifest.Topics,
		},
	}, true)
}

func (p *Instance) handle(req rpcRequest) {
	if req.Method == "" {
		return
	}

	result, rerr := p.call(req)
	if len(req.ID) == 0 {
		return
	}
	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	if rerr != nil {
		resp.Error = rerr
	} else {
		resp.Result = result
	}
	p.send(resp, false)
}

func (p *Instance) call(req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "panko.ping":
		return map[string]any{"pong": time.Now().UTC()}, nil

	case "panko.status":
		return map[string]any{"core": p.host.client.State()}, nil

	case "panko.log":
		var params struct {
			Level   string `json:"level"`
			Message string `json:"message"`
		}
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, err
		}
		p.logger.Printf("%s: %s", params.Level, params.Message)
		return map[string]any{}, nil

	case "panko.subscribe":
		var params struct {
			Topics []string `json:"topics"`
		}
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, err
		}
		for _, t := range params.Topics {
			if perm := topicPermission(t); perm != "" && !p.has(perm) {
				return nil, &rpcError{Code: errPermission, Message: fmt.Sprintf("缺少权限 %s，无法订阅 %s", perm, t)}
			}
			p.mu.Lock()
			p.topics[t] = true
			p.mu.Unlock()
		}
		return map[string]any{"topics": params.Topics}, nil

	case "panko.say":
		if !p.has(PermChatSend) {
			return nil, &rpcError{Code: errPermission, Message: "缺少权限 " + PermChatSend}
		}
		return p.forward("bot.say", req.Params)

	case "panko.command":
		if !p.has(PermCommand) {
			return nil, &rpcError{Code: errPermission, Message: "缺少权限 " + PermCommand}
		}
		return p.forward("bot.command", req.Params)

	case "panko.block.get":
		if !p.has(PermWorldRead) {
			return nil, &rpcError{Code: errPermission, Message: "缺少权限 " + PermWorldRead}
		}
		return p.forward("world.block", req.Params)

	case "panko.blocks.get":
		if !p.has(PermWorldRead) {
			return nil, &rpcError{Code: errPermission, Message: "缺少权限 " + PermWorldRead}
		}
		return p.forward("world.blocks", req.Params)

	case "panko.entities.list":
		if !p.has(PermWorldRead) {
			return nil, &rpcError{Code: errPermission, Message: "缺少权限 " + PermWorldRead}
		}
		return p.forward("world.entities", req.Params)

	case "panko.place_blocks":
		if !p.has(PermWorldWrite) {
			return nil, &rpcError{Code: errPermission, Message: "缺少权限 " + PermWorldWrite}
		}
		return p.forward("world.place", req.Params)

	case "panko.raw.send":
		if !p.has(PermRawPacket) {
			return nil, &rpcError{Code: errPermission, Message: "缺少权限 " + PermRawPacket}
		}
		return p.forward("raw.send", req.Params)
	}

	return nil, &rpcError{Code: errMethodNotFound, Message: "未知方法 " + req.Method}
}

func (p *Instance) forward(method string, params json.RawMessage) (any, *rpcError) {
	var payload any
	if len(params) > 0 {
		if err := json.Unmarshal(params, &payload); err != nil {
			return nil, &rpcError{Code: errInvalidParams, Message: "参数不是合法 JSON"}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimout)
	defer cancel()

	var result json.RawMessage
	if err := p.host.client.Do(ctx, method, payload, &result); err != nil {
		var rpcErr *coreclient.RPCError
		if errors.As(err, &rpcErr) {
			return nil, &rpcError{Code: errCore, Message: rpcErr.Error()}
		}
		return nil, &rpcError{Code: errCore, Message: err.Error()}
	}
	return result, nil
}

// DeliverEvent 把事件通知给插件。插件未运行或队列满时直接丢弃，绝不阻塞总线。
func (p *Instance) DeliverEvent(ev events.Event) {
	p.mu.Lock()
	running := p.state == StateRunning
	out := p.out
	done := p.done
	wants := p.wantsLocked(ev.Topic)
	p.mu.Unlock()

	if !running || out == nil || !wants {
		return
	}

	msg := rpcNotification{JSONRPC: "2.0", Method: "panko.event", Params: ev}
	select {
	case out <- msg:
	case <-done:
	default:
		// 队列满：丢弃事件。插件可以通过 since 重新订阅补齐。
	}
}

func (p *Instance) wantsLocked(topic string) bool {
	if perm := topicPermission(topic); perm != "" && !p.hasLocked(perm) {
		return false
	}
	if len(p.topics) == 0 {
		return true
	}
	return p.topics[topic]
}

func (p *Instance) notify(method string, params any) {
	p.send(rpcNotification{JSONRPC: "2.0", Method: method, Params: params}, false)
}

// send 把消息放入出站队列。dropIfFull 为 true 时队列满直接丢弃。
func (p *Instance) send(msg any, dropIfFull bool) {
	p.mu.Lock()
	out := p.out
	done := p.done
	running := p.state == StateRunning
	p.mu.Unlock()

	if !running || out == nil {
		return
	}

	select {
	case out <- msg:
	case <-done:
	default:
		if dropIfFull {
			return
		}
		// 响应类消息不能让插件一直等，但也不能无限阻塞总线。
		select {
		case out <- msg:
		case <-done:
		case <-time.After(2 * time.Second):
			p.logger.Printf("出站队列已满，丢弃 %T 消息", msg)
		}
	}
}

func (p *Instance) has(perm string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hasLocked(perm)
}

func (p *Instance) hasLocked(perm string) bool {
	for _, item := range p.manifest.Permissions {
		if item == perm {
			return true
		}
	}
	return false
}

func (p *Instance) isStopped() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopped
}

func (p *Instance) setState(s State) {
	p.mu.Lock()
	p.state = s
	p.mu.Unlock()
}

func decodeParams(raw json.RawMessage, dst any) *rpcError {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return &rpcError{Code: errInvalidParams, Message: "参数格式错误: " + err.Error()}
	}
	return nil
}
