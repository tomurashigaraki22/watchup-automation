package queue

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewRedis connects to Redis and verifies connectivity with a PING.
func NewRedis(ctx context.Context, addr string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("queue: redis ping: %w", err)
	}
	return client, nil
}
