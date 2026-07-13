package scheduler_test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/queue"
	"watchup/automation/internal/scheduler"
	"watchup/automation/internal/testutil"
)

func TestTick_EnqueuesDiscoverSweepsFollowupsAndReplyScan(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))
	q := queue.NewQueue(testutil.NewRedis(t))
	s := scheduler.NewScheduler(repo, q, zap.NewNop())

	discovered := &models.Company{Name: "A", Website: "https://a.example", Status: models.CompanyStatusDiscovered}
	crawled := &models.Company{Name: "B", Website: "https://b.example", Status: models.CompanyStatusCrawled}
	validated := &models.Company{Name: "C", Website: "https://c.example", Status: models.CompanyStatusValidated}
	analyzed := &models.Company{Name: "D", Website: "https://d.example", Status: models.CompanyStatusAnalyzed}
	for _, c := range []*models.Company{discovered, crawled, validated, analyzed} {
		if err := repo.Companies.Create(ctx, c); err != nil {
			t.Fatalf("seed company: %v", err)
		}
	}

	dueFollowup := &models.Followup{EmailID: 1, Sequence: 1, ScheduledAt: time.Now().Add(-time.Hour)}
	futureFollowup := &models.Followup{EmailID: 1, Sequence: 2, ScheduledAt: time.Now().Add(time.Hour)}
	sentFollowup := &models.Followup{EmailID: 1, Sequence: 3, ScheduledAt: time.Now().Add(-time.Hour), Sent: true}
	canceledFollowup := &models.Followup{EmailID: 2, Sequence: 1, ScheduledAt: time.Now().Add(-time.Hour), Canceled: true}
	for _, f := range []*models.Followup{dueFollowup, futureFollowup, sentFollowup, canceledFollowup} {
		if err := repo.Followups.Create(ctx, f); err != nil {
			t.Fatalf("seed followup: %v", err)
		}
	}

	if err := s.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	var jobs []queue.Job
	for {
		job, ok, err := q.Dequeue(ctx, 200*time.Millisecond)
		if err != nil {
			t.Fatalf("dequeue: %v", err)
		}
		if !ok {
			break
		}
		jobs = append(jobs, job)
	}

	counts := map[queue.JobType]int{}
	for _, j := range jobs {
		counts[j.Type]++
	}

	if counts[queue.JobDiscover] != 1 {
		t.Errorf("expected 1 discover job, got %d", counts[queue.JobDiscover])
	}
	if counts[queue.JobCrawl] != 1 {
		t.Errorf("expected 1 crawl job (for the discovered company), got %d", counts[queue.JobCrawl])
	}
	if counts[queue.JobValidate] != 1 {
		t.Errorf("expected 1 validate job (for the crawled company), got %d", counts[queue.JobValidate])
	}
	if counts[queue.JobAnalyze] != 1 {
		t.Errorf("expected 1 analyze job (for the validated company), got %d", counts[queue.JobAnalyze])
	}
	if counts[queue.JobGenerate] != 0 {
		t.Errorf("analyzed companies aren't swept (workers chain them directly), got %d", counts[queue.JobGenerate])
	}
	if counts[queue.JobFollowup] != 1 {
		t.Errorf("expected exactly 1 due followup enqueued, got %d", counts[queue.JobFollowup])
	}
	if counts[queue.JobReplyScan] != 1 {
		t.Errorf("expected 1 reply scan job, got %d", counts[queue.JobReplyScan])
	}

	total := 0
	for _, n := range counts {
		total += n
	}
	if total != len(jobs) || total != 6 {
		t.Errorf("expected exactly 6 jobs enqueued, got %d: %+v", total, counts)
	}
}
