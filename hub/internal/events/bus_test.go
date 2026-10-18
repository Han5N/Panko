package events

import (
	"encoding/json"
	"testing"
)

func event(seq uint64, topic string) Event {
	return Event{Seq: seq, Topic: topic, Payload: json.RawMessage(`{}`)}
}

func TestPublishAndReplay(t *testing.T) {
	bus := New(8)
	for i := uint64(1); i <= 3; i++ {
		bus.Publish(event(i, "chat"))
	}

	sub := bus.Subscribe(nil, 1)
	defer sub.Close()

	if sub.Gap {
		t.Fatalf("不应报告断档")
	}
	if got := <-sub.C; got.Seq != 2 {
		t.Fatalf("第一条回放应为 2，实际 %d", got.Seq)
	}
	if got := <-sub.C; got.Seq != 3 {
		t.Fatalf("第二条回放应为 3，实际 %d", got.Seq)
	}
}

func TestTopicFilter(t *testing.T) {
	bus := New(8)
	bus.Publish(event(1, "chat"))
	bus.Publish(event(2, "block"))
	bus.Publish(event(3, "chat"))

	sub := bus.Subscribe(map[string]bool{"block": true}, 0)
	defer sub.Close()

	if got := <-sub.C; got.Seq != 2 {
		t.Fatalf("只应收到 block 事件，实际 seq=%d", got.Seq)
	}
}

func TestGapDetection(t *testing.T) {
	bus := New(2)
	for i := uint64(1); i <= 4; i++ {
		bus.Publish(event(i, "chat"))
	}

	// 缓冲区只保留 3、4，从 0 开始订阅必然断档。
	sub := bus.Subscribe(nil, 0)
	defer sub.Close()

	if !sub.Gap {
		t.Fatalf("应报告断档")
	}
	if got := <-sub.C; got.Seq != 3 {
		t.Fatalf("最旧事件应为 3，实际 %d", got.Seq)
	}
}

func TestMonotonicSeq(t *testing.T) {
	bus := New(8)
	bus.Publish(event(2, "chat"))
	bus.Publish(event(1, "chat")) // 倒退，应忽略
	bus.Publish(event(2, "chat")) // 重复，应忽略

	if bus.LastSeq() != 2 {
		t.Fatalf("LastSeq 应为 2，实际 %d", bus.LastSeq())
	}
}

func TestSlowSubscriberDropsOldest(t *testing.T) {
	bus := New(4)
	sub := bus.Subscribe(nil, 0)
	defer sub.Close()

	for i := uint64(1); i <= 300; i++ {
		bus.Publish(event(i, "chat"))
	}

	// 队列容量是 4+256，超出的部分丢弃最旧的，最后一条必须保留。
	last := uint64(0)
	for {
		select {
		case ev := <-sub.C:
			last = ev.Seq
			continue
		default:
		}
		break
	}
	if last != 300 {
		t.Fatalf("最后一条应为 300，实际 %d", last)
	}
}

func TestUnsubscribeIsIdempotent(t *testing.T) {
	bus := New(4)
	sub := bus.Subscribe(nil, 0)
	sub.Close()
	sub.Close()
}
