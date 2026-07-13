package workers_test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"watchup/automation/internal/queue"
	"watchup/automation/internal/workers"
)

func TestWorker_ProcessesJobAndWritesAuditLog(t *testing.T) {
	h := newHarness(t, "", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := h.queue.Enqueue(context.Background(), queue.Job{Type: queue.JobReplyScan}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	w := workers.NewWorker(h.queue, h.handlers, time.Millisecond, 2*time.Millisecond, zap.NewNop())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if h.replies.calls > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if h.replies.calls != 1 {
		t.Fatalf("expected worker to process the job, got %d scanner calls", h.replies.calls)
	}

	logs, err := h.repo.AuditLogs.List(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Action != "job.reply_scan" {
		t.Fatalf("expected 1 audit log for job.reply_scan, got %+v", logs)
	}

	cancel()
	<-done
}
