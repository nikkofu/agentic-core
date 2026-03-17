package bus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisPubSub struct {
	client *redis.Client
}

func NewRedisPubSub(client *redis.Client) *RedisPubSub {
	return &RedisPubSub{client: client}
}

func (r *RedisPubSub) Publish(ctx context.Context, channel string, msg Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	// 这里我们同时支持 PUBLISH (广播) 和 LPUSH (队列)
	// 鉴于目前 Consume 的实现，我们优先使用 LPUSH 来支持队列任务
	return r.client.LPush(ctx, channel, data).Err()
}

func (r *RedisPubSub) Consume(ctx context.Context, channel string) (Message, error) {
	// 使用 BRPop 进行阻塞式消费，超时时间稍长一点
	res, err := r.client.BRPop(ctx, 5*time.Second, channel).Result()
	if err != nil {
		if err == redis.Nil {
			return Message{}, ErrNoMessage
		}
		return Message{}, err
	}

	// BRPop 返回的是 [key, value]
	if len(res) < 2 {
		return Message{}, fmt.Errorf("unexpected brpop response length: %d", len(res))
	}

	return ParseMessage([]byte(res[1]))
}

// Subscribe 额外提供一个基于 Redis PubSub 的广播模式支持
func (r *RedisPubSub) Subscribe(ctx context.Context, channel string) <-chan Message {
	pubsub := r.client.Subscribe(ctx, channel)
	ch := make(chan Message)
	go func() {
		defer close(ch)
		defer pubsub.Close()
		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				return
			}
			parsed, err := ParseMessage([]byte(msg.Payload))
			if err == nil {
				ch <- parsed
			}
		}
	}()
	return ch
}

// Broadcast 额外提供一个真正的广播发布
func (r *RedisPubSub) Broadcast(ctx context.Context, channel string, msg Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return r.client.Publish(ctx, channel, data).Err()
}
