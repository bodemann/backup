package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func withHTTPClient(rt http.RoundTripper) func() {
	oldClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: rt}
	return func() { http.DefaultClient = oldClient }
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	if val, ok := os.LookupEnv(key); ok {
		t.Cleanup(func() { os.Setenv(key, val) })
	} else {
		t.Cleanup(func() { os.Unsetenv(key) })
	}
	os.Unsetenv(key)
}

func TestGetConfigFromEnv(t *testing.T) {
	t.Setenv("RESTIC-REPO", "envrepo")
	t.Setenv("RESTIC-REPO-PASSWORD", "envpass")
	cfg := getConfig()
	if cfg.Repo != "envrepo" || cfg.Password != "envpass" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestGetConfigFromPastebin(t *testing.T) {
	unsetEnv(t, "RESTIC-REPO")
	unsetEnv(t, "RESTIC-REPO-PASSWORD")
	restore := withHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"restic-repo":"pb-repo","restic-repo-password":"pb-pass"}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req}, nil
	}))
	defer restore()
	cfg := getConfig()
	if cfg.Repo != "pb-repo" || cfg.Password != "pb-pass" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestGetConfigEnvOverrides(t *testing.T) {
	t.Setenv("RESTIC-REPO", "envrepo")
	unsetEnv(t, "RESTIC-REPO-PASSWORD")
	restore := withHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"restic-repo":"pb-repo","restic-repo-password":"pb-pass"}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req}, nil
	}))
	defer restore()
	cfg := getConfig()
	if cfg.Repo != "envrepo" || cfg.Password != "pb-pass" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestFetchPastebinConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"a":"b"}`)
	}))
	defer srv.Close()
	m, err := fetchPastebinConfig(srv.URL)
	if err != nil {
		t.Fatalf("fetchPastebinConfig: %v", err)
	}
	if m["a"] != "b" {
		t.Fatalf("unexpected map: %v", m)
	}
}

func TestFetchPastebinConfigError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := fetchPastebinConfig(srv.URL); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDownloadRestic(t *testing.T) {
	version := "1.0.0"
	asset := fmt.Sprintf("restic_%s_%s_%s.bz2", version, runtime.GOOS, runtime.GOARCH)
	release := fmt.Sprintf(`{"tag_name":"v%s","assets":[{"name":"%s","browser_download_url":"https://downloads/restic.bz2"}]}`, version, asset)
	compressed, err := base64.StdEncoding.DecodeString("QlpoOTFBWSZTWQpuN4IAAAABgAQCAiAgADDNNCGeoEwu5IpwoSAU3G8E")
	if err != nil {
		t.Fatal(err)
	}
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://api.github.com/repos/restic/restic/releases/latest":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(release)), Header: make(http.Header), Request: req}, nil
		case "https://downloads/restic.bz2":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(compressed)), Header: make(http.Header), Request: req}, nil
		default:
			return nil, fmt.Errorf("unexpected URL: %s", req.URL)
		}
	})
	restore := withHTTPClient(rt)
	defer restore()
	dir := t.TempDir()
	path := filepath.Join(dir, "restic")
	if err := downloadRestic(dir, path); err != nil {
		t.Fatalf("downloadRestic: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "dummy" {
		t.Fatalf("unexpected file contents: %q", data)
	}
}
