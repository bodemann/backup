package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHealthReport(t *testing.T) {
	dir := t.TempDir()
	restic := filepath.Join(dir, "restic")
	if err := os.WriteFile(restic, []byte("#!/bin/sh\necho restic 0.9.6\n"), 0755); err != nil {
		t.Fatalf("write restic: %v", err)
	}
	dataDir := filepath.Join(dir, "data")
	if err := os.Mkdir(dataDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "f.txt"), []byte("hi"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cfg := config{Paths: []string{dataDir}}
	restore := withHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"a":"b"}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req}, nil
	}))
	defer restore()
	rep := healthReport(restic, cfg)
	if !strings.Contains(rep, "restic version: restic 0.9.6") {
		t.Fatalf("report missing restic version: %s", rep)
	}
	if !strings.Contains(rep, "pushover configured: no") {
		t.Fatalf("report missing pushover info: %s", rep)
	}
	if !strings.Contains(rep, "email configured: no") {
		t.Fatalf("report missing email info: %s", rep)
	}
	if !strings.Contains(rep, "\"a\": \"b\"") {
		t.Fatalf("report missing pastebin content: %s", rep)
	}
	if !strings.Contains(rep, dataDir) {
		t.Fatalf("report missing backup path: %s", rep)
	}
}
