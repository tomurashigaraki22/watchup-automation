package testutil

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// NewRedis returns a go-redis client backed by an in-memory miniredis
// server, for hermetic queue/worker tests without a live Redis instance.
func NewRedis(t *testing.T) *redis.Client {
	t.Helper()
	server := miniredis.RunT(t)
	return redis.NewClient(&redis.Options{Addr: server.Addr()})
}
