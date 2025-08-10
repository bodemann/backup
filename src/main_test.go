package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
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
	chdir(t, t.TempDir())
	t.Setenv("RESTIC-REPO", "envrepo")
	t.Setenv("RESTIC-REPO-PASSWORD", "envpass")
	cfg := getConfig()
	if cfg.Repo != "envrepo" || cfg.Password != "envpass" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	exp := defaultEmbeddedConfig().Paths
	if fmt.Sprint(cfg.Paths) != fmt.Sprint(exp) {
		t.Fatalf("unexpected paths: %v", cfg.Paths)
	}
}

func TestGetConfigFromPastebin(t *testing.T) {
	chdir(t, t.TempDir())
	unsetEnv(t, "RESTIC-REPO")
	unsetEnv(t, "RESTIC-REPO-PASSWORD")
	restore := withHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"restic-repo":"pb-repo","restic-repo-password":"pb-pass","paths":["/a","/b"]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req}, nil
	}))
	defer restore()
	cfg := getConfig()
	if cfg.Repo != "pb-repo" || cfg.Password != "pb-pass" || fmt.Sprint(cfg.Paths) != fmt.Sprint([]string{"/a", "/b"}) {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestGetConfigEnvOverrides(t *testing.T) {
	chdir(t, t.TempDir())
	t.Setenv("RESTIC-REPO", "envrepo")
	unsetEnv(t, "RESTIC-REPO-PASSWORD")
	restore := withHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"restic-repo":"pb-repo","restic-repo-password":"pb-pass","paths":["/a"]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req}, nil
	}))
	defer restore()
	cfg := getConfig()
	if cfg.Repo != "envrepo" || cfg.Password != "pb-pass" || fmt.Sprint(cfg.Paths) != fmt.Sprint([]string{"/a"}) {
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
	if m["a"].(string) != "b" {
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

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

func TestGetConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	data := config{Repo: "filerepo", Password: "filepass", Paths: []string{"/x", "/y"}}
	b, _ := json.Marshal(data)
	if err := os.WriteFile(configFile, b, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	unsetEnv(t, "RESTIC-REPO")
	unsetEnv(t, "RESTIC-REPO-PASSWORD")
	restore := withHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header), Request: req}, nil
	}))
	defer restore()
	cfg := getConfig()
	if cfg.Repo != "filerepo" || cfg.Password != "filepass" || fmt.Sprint(cfg.Paths) != fmt.Sprint([]string{"/x", "/y"}) {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestEnsureRepoInit(t *testing.T) {
	repoDir := t.TempDir()
	restic := filepath.Join(repoDir, "restic")
	script := `#!/bin/sh
while [ "$1" != "" ]; do
 if [ "$1" = "-r" ]; then shift; repo=$1; fi; shift; done
mkdir -p $repo
touch $repo/config
`
	if err := os.WriteFile(restic, []byte(script), 0755); err != nil {
		t.Fatalf("write restic: %v", err)
	}
	repoPath := filepath.Join(repoDir, "repo")
	if err := ensureRepo(restic, repoPath, "pass"); err != nil {
		t.Fatalf("ensureRepo: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoPath, "config")); err != nil {
		t.Fatalf("config not created: %v", err)
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
