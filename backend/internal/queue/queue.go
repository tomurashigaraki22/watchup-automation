package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const streamKey = "watchup:jobs"

// Queue is a Redis-list-backed FIFO job queue. Simple by design — sufficient
// for this scale, and swappable for Temporal/streams later without changing
// callers (they only see Enqueue/Dequeue).
type Queue struct {
	rdb *redis.Client
	key string
}

// NewQueue wraps an existing Redis client.
func NewQueue(rdb *redis.Client) *Queue {
	return &Queue{rdb: rdb, key: streamKey}
}

// Enqueue pushes job onto the queue.
func (q *Queue) Enqueue(ctx context.Context, job Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("queue: marshal job: %w", err)
	}
	if err := q.rdb.RPush(ctx, q.key, data).Err(); err != nil {
		return fmt.Errorf("queue: enqueue: %w", err)
	}
	return nil
}

// Dequeue blocks up to timeout waiting for the next job. ok=false means the
// timeout elapsed with no job available (not an error).
func (q *Queue) Dequeue(ctx context.Context, timeout time.Duration) (Job, bool, error) {
	res, err := q.rdb.BLPop(ctx, timeout, q.key).Result()
	if errors.Is(err, redis.Nil) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, fmt.Errorf("queue: dequeue: %w", err)
	}
	if len(res) < 2 {
		return Job{}, false, fmt.Errorf("queue: unexpected BLPOP result shape")
	}
	var job Job
	if err := json.Unmarshal([]byte(res[1]), &job); err != nil {
		return Job{}, false, fmt.Errorf("queue: unmarshal job: %w", err)
	}
	return job, true, nil
}

// Len reports the current queue length (observability/tests).
func (q *Queue) Len(ctx context.Context) (int64, error) {
	return q.rdb.LLen(ctx, q.key).Result()
}
