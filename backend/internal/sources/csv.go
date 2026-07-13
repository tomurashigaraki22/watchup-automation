package sources

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// CSVSource discovers companies from a manually imported CSV file. Expected
// header columns (case-insensitive): name, website, industry, description,
// employees. Only "website" is required; missing optional columns are left
// blank, and rows without a website are skipped.
type CSVSource struct {
	reader io.Reader
}

// NewCSVSource wraps r as a CompanySource.
func NewCSVSource(r io.Reader) *CSVSource {
	return &CSVSource{reader: r}
}

func (s *CSVSource) Name() string { return "csv_import" }

func (s *CSVSource) Discover(_ context.Context) ([]Company, error) {
	return ParseCSV(s.reader)
}

// ParseCSV reads a header-based CSV of companies.
func ParseCSV(r io.Reader) ([]Company, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("sources: csv: empty file")
		}
		return nil, fmt.Errorf("sources: csv: read header: %w", err)
	}

	col := make(map[string]int, len(header))
	for i, h := range header {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	websiteIdx, ok := col["website"]
	if !ok {
		return nil, fmt.Errorf(`sources: csv: missing required "website" column`)
	}

	var companies []Company
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("sources: csv: read row: %w", err)
		}
		if websiteIdx >= len(row) || strings.TrimSpace(row[websiteIdx]) == "" {
			continue
		}
		companies = append(companies, Company{
			Name:        csvField(row, col, "name"),
			Website:     strings.TrimSpace(row[websiteIdx]),
			Industry:    csvField(row, col, "industry"),
			Description: csvField(row, col, "description"),
			Employees:   csvField(row, col, "employees"),
		})
	}
	return companies, nil
}

func csvField(row []string, col map[string]int, key string) string {
	idx, ok := col[key]
	if !ok || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}
