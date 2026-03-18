package bus

import (
	"context"
	"errors"
	"strings"
	"sync"
)

var ErrNoMessage = errors.New("no message")

type TaskQueue interface {
	Enqueue(ctx context.Context, queue string, msg Message) error
	Dequeue(ctx context.Context, queue string) (<-chan Message, error)
}

type EventBus interface {
	Publish(ctx context.Context, topic string, msg Message) error
	Subscribe(ctx context.Context, topic string) (<-chan Message, error)
}

type HeartbeatBus interface {
	PublishHeartbeat(ctx context.Context, agentID string, status string) error
}

type FakeTransport struct {
	queueMu sync.Mutex
	queues  map[string]chan Message

	eventMu     sync.Mutex
	subscribers map[int]fakeSubscriber
	nextSubID   int
}

type fakeSubscriber struct {
	pattern string
	ch      chan Message
}

func NewFakeTransport() *FakeTransport {
	return &FakeTransport{
		queues:      make(map[string]chan Message),
		subscribers: make(map[int]fakeSubscriber),
	}
}

func (f *FakeTransport) Enqueue(ctx context.Context, queue string, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := msg.Validate(); err != nil {
		return err
	}

	ch := f.getQueue(queue)
	select {
	case ch <- msg:
	default:
	}
	return nil
}

func (f *FakeTransport) Dequeue(ctx context.Context, queue string) (<-chan Message, error) {
	source := f.getQueue(queue)
	out := make(chan Message, 100)

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-source:
				select {
				case out <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

func (f *FakeTransport) Publish(ctx context.Context, topic string, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := msg.Validate(); err != nil {
		return err
	}

	f.eventMu.Lock()
	defer f.eventMu.Unlock()

	for _, sub := range f.subscribers {
		if !matchTopic(sub.pattern, topic) {
			continue
		}
		select {
		case sub.ch <- msg:
		default:
		}
	}

	return nil
}

func (f *FakeTransport) Subscribe(ctx context.Context, topic string) (<-chan Message, error) {
	ch := make(chan Message, 100)

	f.eventMu.Lock()
	id := f.nextSubID
	f.nextSubID++
	f.subscribers[id] = fakeSubscriber{pattern: topic, ch: ch}
	f.eventMu.Unlock()

	go func() {
		<-ctx.Done()
		f.eventMu.Lock()
		defer f.eventMu.Unlock()
		if sub, ok := f.subscribers[id]; ok {
			delete(f.subscribers, id)
			close(sub.ch)
		}
	}()

	return ch, nil
}

func (f *FakeTransport) SubscriberCount() int {
	f.eventMu.Lock()
	defer f.eventMu.Unlock()
	return len(f.subscribers)
}

func (f *FakeTransport) getQueue(queue string) chan Message {
	f.queueMu.Lock()
	defer f.queueMu.Unlock()

	ch, ok := f.queues[queue]
	if !ok {
		ch = make(chan Message, 100)
		f.queues[queue] = ch
	}

	return ch
}

func matchTopic(pattern string, topic string) bool {
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(topic, prefix)
	}
	return pattern == topic
}

type FakeHeartbeatBus struct {
	mu       sync.RWMutex
	statuses map[string]string
}

func NewFakeHeartbeatBus() *FakeHeartbeatBus {
	return &FakeHeartbeatBus{statuses: make(map[string]string)}
}

func (f *FakeHeartbeatBus) PublishHeartbeat(ctx context.Context, agentID string, status string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	f.statuses[agentID] = status
	f.mu.Unlock()
	return nil
}

func (f *FakeHeartbeatBus) LastStatus(agentID string) (string, bool) {
	f.mu.RLock()
	status, ok := f.statuses[agentID]
	f.mu.RUnlock()
	return status, ok
}
