package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("写入清单失败: %v", err)
	}
}

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `{
		"name": "echo",
		"version": "0.1.0",
		"entry": ["python3", "plugin.py"],
		"permissions": ["chat.read"],
		"topics": ["chat"]
	}`)

	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if m.APIVersion != APIVersion {
		t.Fatalf("api_version 应默认填充为 %s，实际 %s", APIVersion, m.APIVersion)
	}
	if m.Name != "echo" || len(m.Entry) != 2 {
		t.Fatalf("清单解析结果不正确: %+v", m)
	}
}

func TestLoadManifestRejects(t *testing.T) {
	cases := map[string]string{
		"缺少名称":    `{"entry":["python3","p.py"]}`,
		"非法名称":    `{"name":"a b","entry":["python3","p.py"]}`,
		"缺少入口":    `{"name":"ok"}`,
		"未知权限":    `{"name":"ok","entry":["x"],"permissions":["root"]}`,
		"错误的接口版本": `{"name":"ok","entry":["x"],"api_version":"v2"}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeManifest(t, dir, body)
			if _, err := LoadManifest(dir); err == nil {
				t.Fatalf("应拒绝该清单")
			}
		})
	}
}

func TestTopicPermission(t *testing.T) {
	cases := map[string]string{
		"chat":    PermChatRead,
		"block":   PermWorldRead,
		"player":  PermWorldRead,
		"session": "",
		"unknown": PermWorldRead,
	}
	for topic, want := range cases {
		if got := topicPermission(topic); got != want {
			t.Errorf("topic %s 需要权限 %q，实际 %q", topic, want, got)
		}
	}
}
