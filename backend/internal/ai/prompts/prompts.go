// Package prompts loads and renders the Gemini/Groq prompt templates from
// /prompts. Shared across AI provider implementations so prompt content
// stays defined once, never hardcoded, and identical regardless of which
// provider is active.
package prompts

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

// Set holds the parsed prompt templates loaded from the prompts directory.
type Set struct {
	Analysis  *template.Template
	Email     *template.Template
	Followups [3]*template.Template // index 0/1/2 -> Day 5 / 12 / 20
}

// Load parses analysis.txt, email.txt, and followup_1/2/3.txt from dir.
func Load(dir string) (*Set, error) {
	load := func(name string) (*template.Template, error) {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		tmpl, err := template.New(name).Parse(string(data))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		return tmpl, nil
	}

	analysis, err := load("analysis.txt")
	if err != nil {
		return nil, err
	}
	email, err := load("email.txt")
	if err != nil {
		return nil, err
	}
	f1, err := load("followup_1.txt")
	if err != nil {
		return nil, err
	}
	f2, err := load("followup_2.txt")
	if err != nil {
		return nil, err
	}
	f3, err := load("followup_3.txt")
	if err != nil {
		return nil, err
	}

	return &Set{Analysis: analysis, Email: email, Followups: [3]*template.Template{f1, f2, f3}}, nil
}

// Render executes t against data.
func Render(t *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render %s: %w", t.Name(), err)
	}
	return buf.String(), nil
}
