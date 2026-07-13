package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"watchup/automation/internal/api"
	"watchup/automation/internal/config"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/queue"
	"watchup/automation/internal/testutil"
)

const testAdminPassword = "test-pass"

// newTestServer builds a fully-wired Server (DB, queue, auth) for tests and
// returns a valid bearer token for authenticated requests.
func newTestServer(t *testing.T) (*api.Server, *repository.Repositories, string) {
	t.Helper()
	gdb := testutil.NewDB(t)
	repos := repository.NewRepositories(gdb)
	q := queue.NewQueue(testutil.NewRedis(t))
	cfg := &config.Config{
		AppEnv: "test", AIProvider: "groq", SendMode: "manual", DailyLimit: 1,
		JWTSecret: "test-secret", AdminUsername: "admin", AdminPassword: testAdminPassword,
		RateLimitPerMin: 10000,
	}
	srv := api.New(cfg, gdb, repos, q, zap.NewNop())
	return srv, repos, loginToken(t, srv)
}

func loginToken(t *testing.T, srv *api.Server) string {
	t.Helper()
	body := `{"username":"admin","password":"` + testAdminPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login failed: status %d", resp.StatusCode)
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if result.Token == "" {
		t.Fatal("expected non-empty token")
	}
	return result.Token
}

// authed sets the Authorization header for a protected-route request.
func authed(req *http.Request, token string) *http.Request {
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}
