package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"watchup/automation/internal/db/models"
)

// TestLoad_ConcurrentMixedTraffic is a lightweight load test: it fires many
// concurrent requests at a mix of public and protected routes and asserts
// the server stays correct under concurrency — no panics, no wrong status
// codes, no data races (run with `go test -race` to check the last one).
func TestLoad_ConcurrentMixedTraffic(t *testing.T) {
	srv, repos, token := newTestServer(t)
	ctx := context.Background()

	// Seed enough data that list/metrics/search endpoints do real work.
	for i := 0; i < 20; i++ {
		company := &models.Company{
			Name: "LoadCo", Website: httptestUniqueWebsite(i), Status: models.CompanyStatusDiscovered,
		}
		if err := repos.Companies.Create(ctx, company); err != nil {
			t.Fatalf("seed company: %v", err)
		}
	}

	const workers = 20
	const requestsPerWorker = 10
	var wg sync.WaitGroup
	var okCount, failCount int64

	routes := []struct {
		method string
		path   string
		authed bool
	}{
		{http.MethodGet, "/health", false},
		{http.MethodGet, "/api/v1/metrics", true},
		{http.MethodGet, "/api/v1/companies", true},
		{http.MethodGet, "/api/v1/search?status=discovered", true},
	}

	start := time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < requestsPerWorker; i++ {
				route := routes[(worker+i)%len(routes)]
				req := httptest.NewRequest(route.method, route.path, nil)
				if route.authed {
					req = authed(req, token)
				}
				resp, err := srv.App.Test(req, -1)
				if err != nil {
					atomic.AddInt64(&failCount, 1)
					continue
				}
				resp.Body.Close()
				if resp.StatusCode >= 500 {
					atomic.AddInt64(&failCount, 1)
				} else {
					atomic.AddInt64(&okCount, 1)
				}
			}
		}(w)
	}
	wg.Wait()
	elapsed := time.Since(start)

	total := workers * requestsPerWorker
	t.Logf("load test: %d requests in %v (%d ok, %d failed)", total, elapsed, okCount, failCount)

	if failCount > 0 {
		t.Errorf("expected zero 5xx/transport failures under concurrent load, got %d", failCount)
	}
	if int(okCount) != total {
		t.Errorf("expected all %d requests to complete, got %d ok + %d failed", total, okCount, failCount)
	}
}

func httptestUniqueWebsite(i int) string {
	return "https://loadco-" + strconv.Itoa(i) + ".example"
}
