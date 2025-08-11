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

// roundTripFunc allows mocking HTTP requests in tests.
type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip executes the mocked HTTP request.
func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// withHTTPClient replaces the default HTTP client for the duration of the test.
func withHTTPClient(rt http.RoundTripper) func() {
	oldDefault := http.DefaultClient
	old := httpClient
	c := &http.Client{Transport: rt}
	http.DefaultClient = c
	httpClient = c
	return func() {
		http.DefaultClient = oldDefault
		httpClient = old
	}
}

// unsetEnv unsets an environment variable for a test and restores it afterwards.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	if val, ok := os.LookupEnv(key); ok {
		t.Cleanup(func() { os.Setenv(key, val) })
	} else {
		t.Cleanup(func() { os.Unsetenv(key) })
	}
	os.Unsetenv(key)
}

// TestGetConfigFromEnv ensures environment variables override other config sources.
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

// TestGetConfigFromPastebin verifies configuration retrieval from Pastebin.
func TestGetConfigFromPastebin(t *testing.T) {
	chdir(t, t.TempDir())
	unsetEnv(t, "RESTIC-REPO")
	unsetEnv(t, "RESTIC-REPO-PASSWORD")
	restore := withHTTPClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"restic-repo":"pb-repo","restic-repo-password":"pb-pass","paths":["/a","/b"],"pushover-token":"pt","pushover-user":"pu","email-server":"es","email-user":"eu","email-password":"ep","email-from":"ef","email-to":"et"}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req}, nil
	}))
	defer restore()
	cfg := getConfig()
	if cfg.Repo != "pb-repo" || cfg.Password != "pb-pass" || fmt.Sprint(cfg.Paths) != fmt.Sprint([]string{"/a", "/b"}) || cfg.PushoverToken != "pt" || cfg.PushoverUser != "pu" || cfg.EmailServer != "es" || cfg.EmailUser != "eu" || cfg.EmailPassword != "ep" || cfg.EmailFrom != "ef" || cfg.EmailTo != "et" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

// TestGetConfigEnvOverrides checks environment variables override Pastebin config.
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

// TestFetchPastebinConfig verifies successful fetch from a Pastebin URL.
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

// TestFetchPastebinConfigError ensures errors are returned on HTTP failure.
func TestFetchPastebinConfigError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := fetchPastebinConfig(srv.URL); err == nil {
		t.Fatalf("expected error")
	}
}

// chdir changes the working directory for a test and restores it.
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

// TestGetConfigFromFile loads configuration from a local file.
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

// TestEnsureRepoInit verifies repository initialization when missing.
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

// TestDownloadRestic downloads and extracts the restic binary.
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

// TestRunBackupAbort ensures backup aborts when the user declines.
func TestRunBackupAbort(t *testing.T) {
	dir := t.TempDir()
	restic := filepath.Join(dir, "restic")
	script := fmt.Sprintf("#!/bin/sh\ntouch %s\n", filepath.Join(dir, "executed"))
	if err := os.WriteFile(restic, []byte(script), 0755); err != nil {
		t.Fatalf("write restic: %v", err)
	}
	cfg := config{Repo: filepath.Join(dir, "repo"), Password: "p", Paths: []string{"/a"}}
	var out bytes.Buffer
	ok, err := runBackup(restic, cfg, strings.NewReader("n\n"), &out)
	if err != nil {
		t.Fatalf("runBackup: %v", err)
	}
	if ok {
		t.Fatalf("expected no backup execution")
	}
	if _, err := os.Stat(filepath.Join(dir, "executed")); err == nil {
		t.Fatalf("restic executed despite abort")
	}
	if !strings.Contains(out.String(), "backup aborted") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

// TestRunBackupExec runs the backup command when confirmed.
func TestRunBackupExec(t *testing.T) {
	dir := t.TempDir()
	restic := filepath.Join(dir, "restic")
	argsFile := filepath.Join(dir, "args")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" > %s\nif [ \"$RESTIC_PASSWORD\" != \"pass\" ]; then exit 1; fi\n", argsFile)
	if err := os.WriteFile(restic, []byte(script), 0755); err != nil {
		t.Fatalf("write restic: %v", err)
	}
	repo := filepath.Join(dir, "repo")
	cfg := config{Repo: repo, Password: "pass", Paths: []string{"/a", "/b"}}
	var out bytes.Buffer
	ok, err := runBackup(restic, cfg, strings.NewReader("y\n"), &out)
	if err != nil {
		t.Fatalf("runBackup: %v", err)
	}
	if !ok {
		t.Fatalf("expected backup execution")
	}
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	exp := fmt.Sprintf("-r %s backup /a /b", repo)
	if strings.TrimSpace(string(data)) != exp {
		t.Fatalf("unexpected args: %q", data)
	}
	if !strings.Contains(out.String(), "backup completed") {
		t.Fatalf("expected completion message, got %q", out.String())
	}
}
