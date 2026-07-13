package sources_test

import (
	"context"
	"strings"
	"testing"

	"watchup/automation/internal/sources"
)

func TestParseCSV_ValidRows(t *testing.T) {
	csv := "name,website,industry,description,employees\n" +
		"Acme,https://acme.com,Tooling,Acme does things,10-50\n" +
		"Beta,beta.io,,,\n"

	companies, err := sources.ParseCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(companies) != 2 {
		t.Fatalf("expected 2 companies, got %d: %+v", len(companies), companies)
	}
	if companies[0].Name != "Acme" || companies[0].Website != "https://acme.com" || companies[0].Industry != "Tooling" {
		t.Fatalf("unexpected first company: %+v", companies[0])
	}
	if companies[1].Website != "beta.io" {
		t.Fatalf("unexpected second company: %+v", companies[1])
	}
}

func TestParseCSV_SkipsRowsWithoutWebsite(t *testing.T) {
	csv := "name,website\nAcme,\nBeta,beta.io\n"
	companies, err := sources.ParseCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(companies) != 1 || companies[0].Name != "Beta" {
		t.Fatalf("expected only Beta, got %+v", companies)
	}
}

func TestParseCSV_MissingWebsiteColumn(t *testing.T) {
	csv := "name,industry\nAcme,Tooling\n"
	_, err := sources.ParseCSV(strings.NewReader(csv))
	if err == nil {
		t.Fatal("expected error for missing website column")
	}
}

func TestParseCSV_EmptyFile(t *testing.T) {
	_, err := sources.ParseCSV(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestCSVSource_Discover(t *testing.T) {
	csv := "name,website\nAcme,https://acme.com\n"
	src := sources.NewCSVSource(strings.NewReader(csv))
	if src.Name() != "csv_import" {
		t.Fatalf("unexpected name: %s", src.Name())
	}
	companies, err := src.Discover(context.Background())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(companies) != 1 {
		t.Fatalf("expected 1 company, got %d", len(companies))
	}
}
