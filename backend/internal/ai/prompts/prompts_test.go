package prompts_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"watchup/automation/internal/ai/prompts"
)

// repoPromptsDir resolves the real /prompts directory at the repo root,
// independent of the test runner's working directory.
func repoPromptsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	// this file: backend/internal/ai/prompts/prompts_test.go -> repo root/prompts
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "prompts")
}

func TestLoad_RealPromptsDirectory(t *testing.T) {
	dir := repoPromptsDir(t)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("prompts dir not found at %s: %v", dir, err)
	}

	set, err := prompts.Load(dir)
	if err != nil {
		t.Fatalf("load real prompts: %v", err)
	}
	if set.Analysis == nil || set.Email == nil {
		t.Fatal("expected analysis and email templates to load")
	}
	for i, f := range set.Followups {
		if f == nil {
			t.Fatalf("expected followup template %d to load", i+1)
		}
	}
}

func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := prompts.Load(dir); err == nil {
		t.Fatal("expected error for missing prompt files")
	}
}

func TestLoad_InvalidTemplate(t *testing.T) {
	dir := t.TempDir()
	files := []string{"analysis.txt", "email.txt", "followup_1.txt", "followup_2.txt", "followup_3.txt"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("hello {{.Name}}"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	// Break just one file with invalid template syntax.
	if err := os.WriteFile(filepath.Join(dir, "email.txt"), []byte("hello {{.Name"), 0o644); err != nil {
		t.Fatalf("write broken email.txt: %v", err)
	}
	if _, err := prompts.Load(dir); err == nil {
		t.Fatal("expected error for invalid template syntax")
	}
}
