package coreclient

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"panko/hub/internal/events"
	"panko/hub/internal/mockcore"
)

func waitOnline(t *testing.T, c *Client) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if c.State() == "online" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("核心在 3 秒内未连接")
}

func TestCallAndEventForwarding(t *testing.T) {
	core, err := mockcore.Start("127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动模拟核心失败: %v", err)
	}
	defer core.Close()

	bus := events.New(64)
	client := New(core.Addr(), bus, log.New(io.Discard, "", 0))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go client.Run(ctx)

	waitOnline(t, client)

	var status map[string]any
	if err := client.Do(ctx, "bot.status", nil, &status); err != nil {
		t.Fatalf("bot.status 失败: %v", err)
	}
	if status["state"] != "online" {
		t.Fatalf("状态应为 online，实际 %v", status["state"])
	}

	sub := bus.Subscribe(nil, bus.LastSeq())
	defer sub.Close()

	core.Publish("chat", map[string]any{"sender": "tester", "message": "hello"})

	select {
	case ev := <-sub.C:
		if ev.Topic != "chat" {
			t.Fatalf("topic 应为 chat，实际 %s", ev.Topic)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("2 秒内未收到事件")
	}
}

func TestSayPublishesChatEvent(t *testing.T) {
	core, err := mockcore.Start("127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动模拟核心失败: %v", err)
	}
	defer core.Close()

	bus := events.New(64)
	client := New(core.Addr(), bus, log.New(io.Discard, "", 0))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go client.Run(ctx)

	waitOnline(t, client)

	sub := bus.Subscribe(map[string]bool{"chat": true}, bus.LastSeq())
	defer sub.Close()

	if err := client.Do(ctx, "bot.say", map[string]any{"text": "ping"}, nil); err != nil {
		t.Fatalf("bot.say 失败: %v", err)
	}

	select {
	case ev := <-sub.C:
		if ev.Topic != "chat" {
			t.Fatalf("topic 应为 chat，实际 %s", ev.Topic)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("2 秒内未收到聊天事件")
	}
}

func TestUnavailableWhenOffline(t *testing.T) {
	bus := events.New(4)
	client := New("127.0.0.1:1", bus, log.New(io.Discard, "", 0))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := client.Do(ctx, "bot.status", nil, nil); err != ErrUnavailable {
		t.Fatalf("应返回 ErrUnavailable，实际 %v", err)
	}
}
