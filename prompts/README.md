# Prompt templates

Gemini prompt templates live here, loaded and Go-templated at runtime by
`internal/ai/gemini` — never hardcoded in source.

- `analysis.txt` — company analysis → `{summary, industry, value_proposition, watchup_angle}`, data = `ai.CompanyContext`
- `email.txt` — initial partnership email → `{subject, body, cta, ps}`, data = `ai.EmailContext`
- `followup_1.txt`, `followup_2.txt`, `followup_3.txt` — Day 5 / 12 / 20 sequence → `{subject, body, cta, ps}`, data = `ai.FollowupContext`
