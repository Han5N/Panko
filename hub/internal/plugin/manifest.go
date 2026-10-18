// Package plugin 管理外部插件进程：清单校验、权限控制、JSON-RPC 通信与崩溃重启。
package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// APIVersion 是当前插件协议版本。
const APIVersion = "v1"

// Manifest 对应插件目录下的 manifest.json。
type Manifest struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	APIVersion  string   `json:"api_version"`
	Entry       []string `json:"entry"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	Permissions []string `json:"permissions"`
	Topics      []string `json:"topics"`
	AutoStart   bool     `json:"auto_start"`
}

// LoadManifest 读取并校验插件清单。
func LoadManifest(dir string) (Manifest, error) {
	var m Manifest
	path := filepath.Join(dir, "manifest.json")

	data, err := os.ReadFile(path)
	if err != nil {
		return m, fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("解析 %s 失败: %w", path, err)
	}

	if strings.TrimSpace(m.Name) == "" {
		return m, fmt.Errorf("%s 缺少 name", path)
	}
	if err := validName(m.Name); err != nil {
		return m, fmt.Errorf("%s: %w", path, err)
	}
	if len(m.Entry) == 0 {
		return m, fmt.Errorf("%s 缺少 entry", path)
	}
	if m.APIVersion == "" {
		m.APIVersion = APIVersion
	}
	if m.APIVersion != APIVersion {
		return m, fmt.Errorf("%s 的 api_version 为 %q，当前仅支持 %q", path, m.APIVersion, APIVersion)
	}
	for _, p := range m.Permissions {
		if !knownPermission(p) {
			return m, fmt.Errorf("%s 声明了未知权限 %q", path, p)
		}
	}
	return m, nil
}

func validName(name string) error {
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return fmt.Errorf("插件名 %q 只能包含字母、数字、- 和 _", name)
		}
	}
	return nil
}

// 权限名与文档、proto 中的枚举保持一致。
const (
	PermChatRead   = "chat.read"
	PermChatSend   = "chat.send"
	PermCommand    = "command"
	PermWorldRead  = "world.read"
	PermWorldWrite = "world.write"
	PermRawPacket  = "raw.packet"
)

func knownPermission(p string) bool {
	switch p {
	case PermChatRead, PermChatSend, PermCommand, PermWorldRead, PermWorldWrite, PermRawPacket:
		return true
	}
	return false
}

// topicPermission 返回某个事件 topic 所需权限，空字符串表示无需权限。
func topicPermission(topic string) string {
	switch topic {
	case "chat":
		return PermChatRead
	case "block", "player":
		return PermWorldRead
	case "session":
		return ""
	}
	return PermWorldRead
}
