// Package config 负责加载并校验 hub 的运行时配置。
package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
)

// Listener 描述一个 HTTP 服务的监听地址。
type Listener struct {
	Addr string `json:"addr"`
}

// Config 是 Panko 控制面的完整配置。
type Config struct {
	Debug       bool     `json:"debug"`
	API         Listener `json:"api"`
	WebUI       Listener `json:"webui"`
	DebugServer Listener `json:"debug_server"`
	Core        Listener `json:"core"`
	WebDir      string   `json:"web_dir"`
	PluginsDir  string   `json:"plugins_dir"`
}

// Default 返回默认配置。端口与用途见 README。
func Default() Config {
	return Config{
		Debug:       false,
		API:         Listener{Addr: "127.0.0.1:3030"},
		WebUI:       Listener{Addr: "127.0.0.1:3031"},
		DebugServer: Listener{Addr: "127.0.0.1:3032"},
		Core:        Listener{Addr: "127.0.0.1:3039"},
		WebDir:      "web",
		PluginsDir:  "plugins",
	}
}

// Load 读取配置文件。缺失字段保留默认值，但文件本身必须存在，避免用户
// 以为改了配置却实际跑在默认端口上。
func Load(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("读取配置 %s 失败: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("解析配置 %s 失败: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate 检查所有监听地址是否合法。端口冲突留给实际监听时报错，
// 因为调试端口只在 debug 开启时才会占用。
func (c Config) Validate() error {
	listeners := map[string]Listener{
		"api":          c.API,
		"webui":        c.WebUI,
		"debug_server": c.DebugServer,
		"core":         c.Core,
	}
	for name, l := range listeners {
		if l.Addr == "" {
			return fmt.Errorf("配置项 %s.addr 不能为空", name)
		}
		if _, _, err := net.SplitHostPort(l.Addr); err != nil {
			return fmt.Errorf("配置项 %s.addr 无效 (%q): %w", name, l.Addr, err)
		}
	}
	if c.WebDir == "" {
		return fmt.Errorf("配置项 web_dir 不能为空")
	}
	if c.PluginsDir == "" {
		return fmt.Errorf("配置项 plugins_dir 不能为空")
	}
	return nil
}
