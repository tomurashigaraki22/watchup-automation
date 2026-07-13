package workers

import (
	"context"
	"math/rand"
	"time"

	"go.uber.org/zap"

	"watchup/automation/internal/db/models"
	"watchup/automation/internal/queue"
)

// Worker pulls jobs from a queue and dispatches them to Handlers, writing an
// audit log entry per job. Multiple Workers can run concurrently against the
// same queue for horizontal scaling.
type Worker struct {
	queue        *queue.Queue
	handlers     *Handlers
	sendDelayMin time.Duration
	sendDelayMax time.Duration
	log          *zap.Logger
}

// NewWorker builds a Worker. sendDelayMin/Max enforce the PRD's randomized
// 45-240s pacing between actual email dispatches (JobSend/JobFollowup) —
// applied only to those job types, not the whole pipeline.
func NewWorker(q *queue.Queue, h *Handlers, sendDelayMin, sendDelayMax time.Duration, log *zap.Logger) *Worker {
	return &Worker{queue: q, handlers: h, sendDelayMin: sendDelayMin, sendDelayMax: sendDelayMax, log: log}
}

// Run blocks, processing jobs until ctx is canceled.
func (w *Worker) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		job, ok, err := w.queue.Dequeue(ctx, 5*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			w.log.Error("worker: dequeue failed", zap.Error(err))
			continue
		}
		if !ok {
			continue
		}
		w.process(ctx, job)
	}
}

func (w *Worker) process(ctx context.Context, job queue.Job) {
	err := w.handlers.Process(ctx, job)

	detail := "ok"
	if err != nil {
		detail = err.Error()
		w.log.Error("worker: job failed", zap.String("type", string(job.Type)), zap.Error(err))
	}
	audit := &models.AuditLog{
		Actor:  "worker",
		Action: "job." + string(job.Type),
		Entity: "job",
		Detail: detail,
	}
	if createErr := w.handlers.Repo.AuditLogs.Create(ctx, audit); createErr != nil {
		w.log.Error("worker: failed to write audit log", zap.Error(createErr))
	}

	if job.Type == queue.JobSend || job.Type == queue.JobFollowup {
		w.paceSend(ctx)
	}
}

// paceSend sleeps a random duration in [sendDelayMin, sendDelayMax] so
// consecutive sends never happen at identical timing, per the PRD.
func (w *Worker) paceSend(ctx context.Context) {
	if w.sendDelayMax <= 0 {
		return
	}
	d := w.sendDelayMin
	if w.sendDelayMax > w.sendDelayMin {
		d += time.Duration(rand.Int63n(int64(w.sendDelayMax - w.sendDelayMin)))
	}
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}
