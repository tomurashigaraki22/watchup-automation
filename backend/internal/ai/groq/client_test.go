package groq_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"watchup/automation/internal/ai"
	"watchup/automation/internal/ai/groq"
)

func writeTestPrompts(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"analysis.txt":   "Analyze {{.Name}} at {{.Website}}.",
		"email.txt":      "Write to {{.Company.Name}}, contact {{.Contact.Email}}.",
		"followup_1.txt": "Followup 1 for {{.Company.Name}}, re: {{.OriginalSubject}}.",
		"followup_2.txt": "Followup 2 for {{.Company.Name}}, re: {{.OriginalSubject}}.",
		"followup_3.txt": "Followup 3 for {{.Company.Name}}, re: {{.OriginalSubject}}.",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// chatResponse builds a minimal Groq/OpenAI-style chat completion response
// whose message content is the given payload.
func chatResponse(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"},
		},
		"usage": map[string]any{"total_tokens": 321},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func newTestClient(t *testing.T, handler http.HandlerFunc) (*groq.Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	c, err := groq.New("test-key", "llama-3.3-70b-versatile", writeTestPrompts(t))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	c.BaseURL = server.URL
	return c, server
}

func TestNew_RequiresAPIKey(t *testing.T) {
	if _, err := groq.New("", "model", writeTestPrompts(t)); err == nil {
		t.Fatal("expected error for missing api key")
	}
}

func TestNew_RequiresModel(t *testing.T) {
	if _, err := groq.New("key", "", writeTestPrompts(t)); err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestClient_Analyze_ParsesJSONResponse(t *testing.T) {
	payload := `{"summary":"They build tools.","industry":"SaaS","value_proposition":"Fast growth","watchup_angle":"Embed video analytics"}`
	c, server := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("unexpected Authorization header: %q", got)
		}
		chatResponse(w, payload)
	})
	defer server.Close()

	analysis, meta, err := c.Analyze(context.Background(), ai.CompanyContext{Name: "Acme", Website: "https://acme.com"})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if analysis.Summary != "They build tools." || analysis.Industry != "SaaS" {
		t.Errorf("unexpected analysis: %+v", analysis)
	}
	if meta.Tokens != 321 || meta.Model != "llama-3.3-70b-versatile" {
		t.Errorf("unexpected meta: %+v", meta)
	}
	if !strings.Contains(meta.Prompt, "Acme") {
		t.Errorf("expected rendered prompt to reference company name, got %q", meta.Prompt)
	}
}

func TestClient_GenerateEmail_ParsesJSONResponse(t *testing.T) {
	payload := `{"subject":"Quick idea for Acme","body":"Hi there.","cta":"Got 15 min this week?","ps":"Loved your launch post."}`
	c, server := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		chatResponse(w, payload)
	})
	defer server.Close()

	email, meta, err := c.GenerateEmail(context.Background(), ai.EmailContext{
		Company: ai.CompanyContext{Name: "Acme"},
		Contact: ai.ContactContext{Email: "partnership@acme.com"},
	})
	if err != nil {
		t.Fatalf("generate email: %v", err)
	}
	if email.Subject != "Quick idea for Acme" || email.CTA == "" {
		t.Errorf("unexpected email: %+v", email)
	}
	if meta.Tokens != 321 {
		t.Errorf("unexpected meta: %+v", meta)
	}
}

func TestClient_GenerateFollowup_ParsesJSONResponse(t *testing.T) {
	payload := `{"subject":"Following up","body":"Just checking in.","cta":"Worth a quick chat?","ps":"No worries either way."}`
	c, server := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		chatResponse(w, payload)
	})
	defer server.Close()

	for seq := 1; seq <= 3; seq++ {
		email, _, err := c.GenerateFollowup(context.Background(), ai.FollowupContext{
			Company:         ai.CompanyContext{Name: "Acme"},
			OriginalSubject: "Partnership idea",
			Sequence:        seq,
		})
		if err != nil {
			t.Fatalf("sequence %d: generate followup: %v", seq, err)
		}
		if email.Subject != "Following up" {
			t.Errorf("sequence %d: unexpected email: %+v", seq, email)
		}
	}
}

func TestClient_GenerateFollowup_InvalidSequence(t *testing.T) {
	c, server := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not call the API for an invalid sequence")
	})
	defer server.Close()

	for _, seq := range []int{0, 4, -1} {
		if _, _, err := c.GenerateFollowup(context.Background(), ai.FollowupContext{Sequence: seq}); err == nil {
			t.Errorf("sequence %d: expected error", seq)
		}
	}
}

func TestClient_Generate_RetriesThenSucceeds(t *testing.T) {
	var attempts int32
	payload := `{"summary":"ok","industry":"x","value_proposition":"y","watchup_angle":"z"}`
	c, server := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		chatResponse(w, payload)
	})
	defer server.Close()

	_, _, err := c.Analyze(context.Background(), ai.CompanyContext{Name: "Acme"})
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestClient_Generate_AllAttemptsFail(t *testing.T) {
	var attempts int32
	c, server := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer server.Close()

	_, _, err := c.Analyze(context.Background(), ai.CompanyContext{Name: "Acme"})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestClient_Analyze_InvalidJSONResponse(t *testing.T) {
	c, server := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		chatResponse(w, "not valid json")
	})
	defer server.Close()

	if _, _, err := c.Analyze(context.Background(), ai.CompanyContext{Name: "Acme"}); err == nil {
		t.Fatal("expected error for invalid JSON in model response")
	}
}

func TestClient_Analyze_EmptyChoices(t *testing.T) {
	c, server := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[]}`)
	})
	defer server.Close()

	if _, _, err := c.Analyze(context.Background(), ai.CompanyContext{Name: "Acme"}); err == nil {
		t.Fatal("expected error for empty choices")
	}
}
