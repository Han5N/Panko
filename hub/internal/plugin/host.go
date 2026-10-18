package plugin

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"panko/hub/internal/coreclient"
	"panko/hub/internal/events"
)

// Info 是插件对外的状态快照。
type Info struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	Permissions []string `json:"permissions"`
	Topics      []string `json:"topics"`
	State       State    `json:"state"`
	LastError   string   `json:"last_error,omitempty"`
	StartedAt   int64    `json:"started_at,omitempty"`
}

// Host 管理插件目录下的所有插件实例。
type Host struct {
	ctx    context.Context
	dir    string
	client *coreclient.Client
	bus    *events.Bus
	logger *log.Logger

	mu      sync.Mutex
	plugins map[string]*Instance
	order   []string
}

// NewHost 创建插件宿主。ctx 取消时所有插件停止。
func NewHost(ctx context.Context, dir string, client *coreclient.Client, bus *events.Bus, logger *log.Logger) *Host {
	return &Host{
		ctx:     ctx,
		dir:     dir,
		client:  client,
		bus:     bus,
		logger:  logger,
		plugins: make(map[string]*Instance),
	}
}

// Load 扫描插件目录。单个插件清单损坏只记录日志，不影响其他插件。
func (h *Host) Load() error {
	entries, err := os.ReadDir(h.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			h.logger.Printf("插件目录 %s 不存在，跳过加载", h.dir)
			return nil
		}
		return fmt.Errorf("读取插件目录 %s 失败: %w", h.dir, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		dir := filepath.Join(h.dir, name)
		manifest, err := LoadManifest(dir)
		if err != nil {
			h.logger.Printf("跳过插件 %s: %v", name, err)
			continue
		}

		h.mu.Lock()
		if _, exists := h.plugins[manifest.Name]; exists {
			h.mu.Unlock()
			h.logger.Printf("跳过插件 %s: 名称重复", manifest.Name)
			continue
		}
		h.plugins[manifest.Name] = newInstance(manifest, dir, h)
		h.order = append(h.order, manifest.Name)
		h.mu.Unlock()

		h.logger.Printf("已加载插件 %s %s（%s）", manifest.Name, manifest.Version, manifest.APIVersion)
	}
	return nil
}

// StartAuto 启动所有 auto_start 插件。
func (h *Host) StartAuto() {
	h.mu.Lock()
	list := make([]*Instance, 0, len(h.plugins))
	for _, name := range h.order {
		list = append(list, h.plugins[name])
	}
	h.mu.Unlock()

	for _, p := range list {
		if p.manifest.AutoStart {
			p.Start()
		}
	}
}

// StopAll 停止所有插件。
func (h *Host) StopAll() {
	h.mu.Lock()
	list := make([]*Instance, 0, len(h.plugins))
	for _, p := range h.plugins {
		list = append(list, p)
	}
	h.mu.Unlock()

	var wg sync.WaitGroup
	for _, p := range list {
		wg.Add(1)
		go func(p *Instance) {
			defer wg.Done()
			p.Stop()
		}(p)
	}
	wg.Wait()
}

// List 返回按加载顺序排列的插件状态。
func (h *Host) List() []Info {
	h.mu.Lock()
	names := append([]string(nil), h.order...)
	instances := make([]*Instance, 0, len(names))
	for _, name := range names {
		if p, ok := h.plugins[name]; ok {
			instances = append(instances, p)
		}
	}
	h.mu.Unlock()

	out := make([]Info, 0, len(instances))
	for _, p := range instances {
		out = append(out, p.Info())
	}
	return out
}

// Restart 按名称重启插件。
func (h *Host) Restart(name string) error {
	h.mu.Lock()
	p, ok := h.plugins[name]
	h.mu.Unlock()
	if !ok {
		return fmt.Errorf("插件 %s 不存在", name)
	}
	p.Restart()
	return nil
}

// Start 按名称启动插件。
func (h *Host) Start(name string) error {
	h.mu.Lock()
	p, ok := h.plugins[name]
	h.mu.Unlock()
	if !ok {
		return fmt.Errorf("插件 %s 不存在", name)
	}
	p.Start()
	return nil
}

// Stop 按名称停止插件。
func (h *Host) Stop(name string) error {
	h.mu.Lock()
	p, ok := h.plugins[name]
	h.mu.Unlock()
	if !ok {
		return fmt.Errorf("插件 %s 不存在", name)
	}
	p.Stop()
	return nil
}

// RunFanout 把总线事件分发给所有订阅的插件，直到 ctx 取消。
func (h *Host) RunFanout(ctx context.Context) {
	sub := h.bus.Subscribe(nil, h.bus.LastSeq())
	defer sub.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-sub.C:
			h.mu.Lock()
			list := make([]*Instance, 0, len(h.plugins))
			for _, p := range h.plugins {
				list = append(list, p)
			}
			h.mu.Unlock()

			for _, p := range list {
				p.DeliverEvent(ev)
			}
		}
	}
}

// Info 返回实例状态。StartedAt 使用 Unix 秒，未启动时为 0。
func (p *Instance) Info() Info {
	p.mu.Lock()
	defer p.mu.Unlock()

	topics := make([]string, 0, len(p.topics))
	for t := range p.topics {
		topics = append(topics, t)
	}

	var started int64
	if !p.startedAt.IsZero() {
		started = p.startedAt.Unix()
	}

	return Info{
		Name:        p.manifest.Name,
		Version:     p.manifest.Version,
		Description: p.manifest.Description,
		Author:      p.manifest.Author,
		Permissions: append([]string(nil), p.manifest.Permissions...),
		Topics:      topics,
		State:       p.state,
		LastError:   p.lastErr,
		StartedAt:   started,
	}
}
