package queue_test

import (
	"context"
	"testing"
	"time"

	"watchup/automation/internal/queue"
	"watchup/automation/internal/testutil"
)

func TestQueue_EnqueueDequeue_FIFO(t *testing.T) {
	ctx := context.Background()
	q := queue.NewQueue(testutil.NewRedis(t))

	if err := q.Enqueue(ctx, queue.Job{Type: queue.JobCrawl, CompanyID: 1}); err != nil {
		t.Fatalf("enqueue 1: %v", err)
	}
	if err := q.Enqueue(ctx, queue.Job{Type: queue.JobCrawl, CompanyID: 2}); err != nil {
		t.Fatalf("enqueue 2: %v", err)
	}

	job1, ok, err := q.Dequeue(ctx, time.Second)
	if err != nil || !ok {
		t.Fatalf("dequeue 1: ok=%v err=%v", ok, err)
	}
	if job1.CompanyID != 1 {
		t.Errorf("expected FIFO order, got company_id=%d first", job1.CompanyID)
	}

	job2, ok, err := q.Dequeue(ctx, time.Second)
	if err != nil || !ok {
		t.Fatalf("dequeue 2: ok=%v err=%v", ok, err)
	}
	if job2.CompanyID != 2 {
		t.Errorf("expected company_id=2 second, got %d", job2.CompanyID)
	}
}

func TestQueue_Dequeue_TimeoutReturnsNotOK(t *testing.T) {
	ctx := context.Background()
	q := queue.NewQueue(testutil.NewRedis(t))

	_, ok, err := q.Dequeue(ctx, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false on empty queue timeout")
	}
}

func TestQueue_Len(t *testing.T) {
	ctx := context.Background()
	q := queue.NewQueue(testutil.NewRedis(t))

	if err := q.Enqueue(ctx, queue.Job{Type: queue.JobDiscover}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := q.Enqueue(ctx, queue.Job{Type: queue.JobReplyScan}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	n, err := q.Len(ctx)
	if err != nil {
		t.Fatalf("len: %v", err)
	}
	if n != 2 {
		t.Errorf("expected len 2, got %d", n)
	}
}

func TestQueue_PreservesAllFields(t *testing.T) {
	ctx := context.Background()
	q := queue.NewQueue(testutil.NewRedis(t))

	want := queue.Job{Type: queue.JobFollowup, CompanyID: 7, EmailID: 9, CampaignID: 3, FollowupID: 11}
	if err := q.Enqueue(ctx, want); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	got, ok, err := q.Dequeue(ctx, time.Second)
	if err != nil || !ok {
		t.Fatalf("dequeue: ok=%v err=%v", ok, err)
	}
	if got != want {
		t.Errorf("expected %+v, got %+v", want, got)
	}
}
