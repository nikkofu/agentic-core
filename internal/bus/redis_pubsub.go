package bus

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisTransport struct {
	client *redis.Client
}

func NewRedisTransport(client *redis.Client) *RedisTransport {
	return &RedisTransport{client: client}
}

func (r *RedisTransport) Enqueue(ctx context.Context, queue string, msg Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return r.client.LPush(ctx, queue, data).Err()
}

func (r *RedisTransport) Dequeue(ctx context.Context, queue string) (<-chan Message, error) {
	ch := make(chan Message, 100)

	go func() {
		defer close(ch)
		for {
			if err := ctx.Err(); err != nil {
				return
			}

			values, err := r.client.BRPop(ctx, time.Second, queue).Result()
			if err != nil {
				if errors.Is(err, redis.Nil) {
					continue
				}
				if ctx.Err() != nil {
					return
				}
				continue
			}

			if len(values) != 2 {
				continue
			}

			msg, err := ParseMessage([]byte(values[1]))
			if err != nil {
				continue
			}

			select {
			case ch <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

func (r *RedisTransport) Publish(ctx context.Context, topic string, msg Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return r.client.Publish(ctx, topic, data).Err()
}

func (r *RedisTransport) Subscribe(ctx context.Context, topic string) (<-chan Message, error) {
	var pubsub *redis.PubSub
	if strings.Contains(topic, "*") {
		pubsub = r.client.PSubscribe(ctx, topic)
	} else {
		pubsub = r.client.Subscribe(ctx, topic)
	}

	ch := make(chan Message, 100)
	go func() {
		defer close(ch)
		defer pubsub.Close()

		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				return
			}

			parsed, err := ParseMessage([]byte(msg.Payload))
			if err != nil {
				continue
			}

			select {
			case ch <- parsed:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}
