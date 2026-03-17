package bus

import (
	"context"
	"errors"
	"sync"
)

var ErrNoMessage = errors.New("no message")

type PubSub interface {
	Publish(ctx context.Context, channel string, msg Message) error
	Consume(ctx context.Context, channel string) (Message, error)
}

type HeartbeatBus interface {
	PublishHeartbeat(ctx context.Context, agentID string, status string) error
}

type FakePubSub struct {
	mu       sync.Mutex
	channels map[string][]Message
}

func NewFakePubSub() *FakePubSub {
	return &FakePubSub{channels: make(map[string][]Message)}
}

func (f *FakePubSub) Publish(ctx context.Context, channel string, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := msg.Validate(); err != nil {
		return err
	}
	f.mu.Lock()
	f.channels[channel] = append(f.channels[channel], msg)
	f.mu.Unlock()
	return nil
}

func (f *FakePubSub) Consume(ctx context.Context, channel string) (Message, error) {
	if err := ctx.Err(); err != nil {
		return Message{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	queue := f.channels[channel]
	if len(queue) == 0 {
		return Message{}, ErrNoMessage
	}
	msg := queue[0]
	f.channels[channel] = queue[1:]
	return msg, nil
}

type FakeHeartbeatBus struct {
	mu      sync.RWMutex
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
