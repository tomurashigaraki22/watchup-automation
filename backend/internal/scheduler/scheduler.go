// Package scheduler runs the hourly pipeline tick: kick off discovery,
// catch companies stuck at each pipeline stage (a resilience net alongside
// the workers' own stage-to-stage chaining), enqueue due followups, and
// scan for replies/bounces/unsubscribes.
package scheduler

import (
	"context"
	"time"

	"go.uber.org/zap"

	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/queue"
)

// Scheduler enqueues the periodic pipeline.
type Scheduler struct {
	repo  *repository.Repositories
	queue *queue.Queue
	log   *zap.Logger
}

// NewScheduler builds a Scheduler.
func NewScheduler(repo *repository.Repositories, q *queue.Queue, log *zap.Logger) *Scheduler {
	return &Scheduler{repo: repo, queue: q, log: log}
}

// stageSweep maps a company status to the job that advances it, for the
// resilience sweep (workers normally chain these automatically as each
// stage completes; this catches anything that got stuck).
var stageSweep = []struct {
	status  string
	jobType queue.JobType
}{
	{models.CompanyStatusDiscovered, queue.JobCrawl},
	{models.CompanyStatusCrawled, queue.JobValidate},
	{models.CompanyStatusValidated, queue.JobAnalyze},
}

// Tick runs one scheduling pass. Intended to be called hourly by cmd/scheduler.
func (s *Scheduler) Tick(ctx context.Context) error {
	if err := s.queue.Enqueue(ctx, queue.Job{Type: queue.JobDiscover}); err != nil {
		return err
	}

	for _, stage := range stageSweep {
		if err := s.sweepStage(ctx, stage.status, stage.jobType); err != nil {
			return err
		}
	}

	if err := s.enqueueDueFollowups(ctx); err != nil {
		return err
	}

	if err := s.queue.Enqueue(ctx, queue.Job{Type: queue.JobReplyScan}); err != nil {
		return err
	}

	s.log.Info("scheduler: tick complete")
	return nil
}

func (s *Scheduler) sweepStage(ctx context.Context, status string, jobType queue.JobType) error {
	companies, err := s.repo.Companies.ListWhere(ctx, 0, 0, "status = ?", status)
	if err != nil {
		return err
	}
	for _, c := range companies {
		if err := s.queue.Enqueue(ctx, queue.Job{Type: jobType, CompanyID: c.ID}); err != nil {
			s.log.Error("scheduler: enqueue failed", zap.Uint("company_id", c.ID), zap.String("job_type", string(jobType)), zap.Error(err))
		}
	}
	return nil
}

func (s *Scheduler) enqueueDueFollowups(ctx context.Context) error {
	followups, err := s.repo.Followups.ListWhere(ctx, 0, 0, "sent = ? AND canceled = ?", false, false)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, f := range followups {
		if f.ScheduledAt.After(now) {
			continue
		}
		if err := s.queue.Enqueue(ctx, queue.Job{Type: queue.JobFollowup, FollowupID: f.ID}); err != nil {
			s.log.Error("scheduler: enqueue followup failed", zap.Uint("followup_id", f.ID), zap.Error(err))
		}
	}
	return nil
}
