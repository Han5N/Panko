// Package events 提供带序号的事件总线：有限环形缓冲、断线回放、按 topic 订阅。
package events

import (
	"encoding/json"
	"sync"
	"time"
)

// Event 是核心上报的一条世界变化。Seq 由 botd 生成，全局单调递增。
type Event struct {
	Seq     uint64          `json:"seq"`
	Topic   string          `json:"topic"`
	Time    time.Time       `json:"time"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Subscription 表示一个订阅者。Gap 为 true 表示 since 到当前缓冲之间存在丢失区间。
type Subscription struct {
	C     <-chan Event
	Gap   bool
	close func()
	once  sync.Once
}

// Close 取消订阅。重复调用安全。
func (s *Subscription) Close() {
	s.once.Do(s.close)
}

// Bus 把事件广播给多个订阅者。订阅者队列满时丢弃最旧的一条，
// 保证慢订阅者不会阻塞核心事件流。
type Bus struct {
	mu       sync.Mutex
	capacity int
	ring     []Event
	subs     map[chan Event]struct{}
}

// New 创建事件总线，capacity 为回放缓冲长度。
func New(capacity int) *Bus {
	if capacity <= 0 {
		capacity = 1024
	}
	return &Bus{
		capacity: capacity,
		subs:     make(map[chan Event]struct{}),
	}
}

// Publish 写入一条事件。重复或倒退的 seq 会被忽略，保证订阅者看到单调序列。
func (b *Bus) Publish(ev Event) {
	if ev.Time.IsZero() {
		ev.Time = time.Now().UTC()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if n := len(b.ring); n > 0 && ev.Seq <= b.ring[n-1].Seq {
		return
	}

	if len(b.ring) == b.capacity {
		copy(b.ring, b.ring[1:])
		b.ring[len(b.ring)-1] = ev
	} else {
		b.ring = append(b.ring, ev)
	}

	for ch := range b.subs {
		deliver(ch, ev)
	}
}

// deliver 在队列满时丢弃最旧事件。调用方必须持有 b.mu。
func deliver(ch chan Event, ev Event) {
	select {
	case ch <- ev:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- ev:
	default:
	}
}

// Subscribe 订阅指定 topic（为空表示全部），并从 since 之后开始回放。
// 回放事件在注册订阅时同步入队，避免与实时事件乱序。
func (b *Bus) Subscribe(topics map[string]bool, since uint64) *Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()

	matched := func(ev Event) bool {
		return len(topics) == 0 || topics[ev.Topic]
	}

	var replay []Event
	gap := false
	if len(b.ring) > 0 {
		oldest := b.ring[0].Seq
		if since+1 < oldest {
			gap = true
		}
		for _, ev := range b.ring {
			if ev.Seq > since && matched(ev) {
				replay = append(replay, ev)
			}
		}
	}

	ch := make(chan Event, len(b.ring)+256)
	for _, ev := range replay {
		ch <- ev
	}
	b.subs[ch] = struct{}{}

	return &Subscription{
		C:   ch,
		Gap: gap,
		close: func() {
			b.mu.Lock()
			delete(b.subs, ch)
			b.mu.Unlock()
			close(ch)
		},
	}
}

// LastSeq 返回缓冲区中最新事件的序号。
func (b *Bus) LastSeq() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.ring) == 0 {
		return 0
	}
	return b.ring[len(b.ring)-1].Seq
}
