package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/bzip2"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type config struct {
	Repo          string   `json:"repo"`
	Password      string   `json:"password"`
	Paths         []string `json:"paths"`
	PushoverToken string   `json:"pushover-token"`
	PushoverUser  string   `json:"pushover-user"`
	EmailServer   string   `json:"email-server"`
	EmailUser     string   `json:"email-user"`
	EmailPassword string   `json:"email-password"`
	EmailFrom     string   `json:"email-from"`
	EmailTo       string   `json:"email-to"`
}

const (
	pastebinURL = "https://pastebin.com/raw/example"
	configFile  = "config.json"
)

// defaultEmbeddedConfig returns the built-in configuration used when no other
// configuration sources are available.
func defaultEmbeddedConfig() config {
	home, _ := os.UserHomeDir()
	paths := []string{
		filepath.Join(home, "Documents"),
		filepath.Join(home, "Pictures"),
		filepath.Join(home, "Desktop"),
	}
	return config{
		Repo:          "~/tmp/test-backup",
		Password:      "test password",
		Paths:         paths,
		PushoverToken: "",
		PushoverUser:  "",
		EmailServer:   "",
		EmailUser:     "",
		EmailPassword: "",
		EmailFrom:     "",
		EmailTo:       "",
	}
}

// main is the program entry point.
func main() {
	ensureAutoStart()
	binDir := filepath.Join(".", "bin")
	resticName := "restic"
	if runtime.GOOS == "windows" {
		resticName += ".exe"
	}
	resticPath := filepath.Join(binDir, resticName)

	if _, err := os.Stat(resticPath); os.IsNotExist(err) {
		fmt.Println("restic not found, downloading latest release...")
		if err := downloadRestic(binDir, resticPath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to download restic: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("restic downloaded to", resticPath)
	} else {
		fmt.Println("restic found, performing self-update...")
		cmd := exec.Command(resticPath, "self-update")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "restic self-update failed: %v\n", err)
			os.Exit(1)
		}
	}

	cfg := getConfig()
	fmt.Println("repository:", cfg.Repo)
	fmt.Println("password:", cfg.Password)
	if err := ensureRepo(resticPath, cfg.Repo, cfg.Password); err != nil {
		fmt.Fprintf(os.Stderr, "failed to ensure repo: %v\n", err)
		os.Exit(1)
	}
	if err := runBackup(resticPath, cfg, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

// runBackup executes the restic backup command after confirming with the user.
func runBackup(resticPath string, cfg config, in io.Reader, out io.Writer) error {
	fmt.Fprintln(out, "paths to backup:")
	for _, p := range cfg.Paths {
		fmt.Fprintln(out, " -", p)
	}
	fmt.Fprint(out, "proceed with backup? [y/N]: ")
	scanner := bufio.NewScanner(in)
	scanner.Scan()
	resp := strings.TrimSpace(scanner.Text())
	if strings.ToLower(resp) != "y" {
		fmt.Fprintln(out, "backup aborted")
		return nil
	}
	args := append([]string{"-r", expandUser(cfg.Repo), "backup"}, cfg.Paths...)
	cmd := exec.Command(resticPath, args...)
	cmd.Env = append(os.Environ(), "RESTIC_PASSWORD="+cfg.Password)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("restic backup failed: %w", err)
	}
	fmt.Fprintln(out, "backup completed")
	return nil
}

// downloadRestic retrieves the latest restic release for the current platform.
func downloadRestic(binDir, resticPath string) error {
	resp, err := http.Get("https://api.github.com/repos/restic/restic/releases/latest")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return err
	}
	version := strings.TrimPrefix(rel.TagName, "v")
	goos := runtime.GOOS
	arch := runtime.GOARCH
	ext := ".bz2"
	if goos == "windows" {
		ext = ".zip"
	}
	targetName := fmt.Sprintf("restic_%s_%s_%s%s", version, goos, arch, ext)
	var downloadURL string
	for _, a := range rel.Assets {
		if a.Name == targetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("asset %s not found", targetName)
	}

	resp2, err := http.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp2.Status)
	}

	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}

	if ext == ".zip" {
		data, err := io.ReadAll(resp2.Body)
		if err != nil {
			return err
		}
		z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return err
		}
		for _, f := range z.File {
			if f.Name == "restic.exe" {
				rc, err := f.Open()
				if err != nil {
					return err
				}
				out, err := os.OpenFile(resticPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
				if err != nil {
					rc.Close()
					return err
				}
				if _, err := io.Copy(out, rc); err != nil {
					out.Close()
					rc.Close()
					return err
				}
				out.Close()
				rc.Close()
				break
			}
		}
		return nil
	}

	bz2 := bzip2.NewReader(resp2.Body)
	out, err := os.OpenFile(resticPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, bz2); err != nil {
		out.Close()
		return err
	}
	out.Close()
	return nil
}

// getConfig builds the configuration from defaults, environment variables,
// Pastebin and an optional local file. It also reports whether the Pastebin
// configuration was successfully retrieved.
func getConfig() config {
	cfg := defaultEmbeddedConfig()

	repoEnv, repoEnvSet := os.LookupEnv("RESTIC-REPO")
	passEnv, passEnvSet := os.LookupEnv("RESTIC-REPO-PASSWORD")
	if repoEnvSet {
		cfg.Repo = repoEnv
	}
	if passEnvSet {
		cfg.Password = passEnv
	}

	if !(repoEnvSet && passEnvSet) {
		pb, err := fetchPastebinConfig(pastebinURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to fetch pastebin config: %v\n", err)
		} else {
			fmt.Println("pastebin config fetched successfully")
			if !repoEnvSet {
				if v, ok := pb["restic-repo"].(string); ok {
					cfg.Repo = v
				}
			}
			if !passEnvSet {
				if v, ok := pb["restic-repo-password"].(string); ok {
					cfg.Password = v
				}
			}
			if v, ok := pb["paths"].([]any); ok {
				paths := make([]string, 0, len(v))
				for _, p := range v {
					if s, ok := p.(string); ok {
						paths = append(paths, s)
					}
				}
				if len(paths) > 0 {
					cfg.Paths = paths
				}
			}
			if v, ok := pb["pushover-token"].(string); ok {
				cfg.PushoverToken = v
			}
			if v, ok := pb["pushover-user"].(string); ok {
				cfg.PushoverUser = v
			}
			if v, ok := pb["email-server"].(string); ok {
				cfg.EmailServer = v
			}
			if v, ok := pb["email-user"].(string); ok {
				cfg.EmailUser = v
			}
			if v, ok := pb["email-password"].(string); ok {
				cfg.EmailPassword = v
			}
			if v, ok := pb["email-from"].(string); ok {
				cfg.EmailFrom = v
			}
			if v, ok := pb["email-to"].(string); ok {
				cfg.EmailTo = v
			}
		}
	}

	if data, err := os.ReadFile(configFile); err == nil {
		var fcfg config
		if err := json.Unmarshal(data, &fcfg); err == nil {
			if fcfg.Repo != "" {
				cfg.Repo = fcfg.Repo
			}
			if fcfg.Password != "" {
				cfg.Password = fcfg.Password
			}
			if len(fcfg.Paths) > 0 {
				cfg.Paths = fcfg.Paths
			}
		}
	} else if os.IsNotExist(err) {
		data, _ := json.MarshalIndent(cfg, "", "  ")
		_ = os.WriteFile(configFile, data, 0644)
	}

	return cfg
}

// fetchPastebinConfig retrieves JSON configuration from the provided Pastebin URL.
func fetchPastebinConfig(url string) (map[string]any, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}
	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}

// ensureRepo initializes a restic repository if it does not already exist.
func ensureRepo(resticPath, repo, password string) error {
	repoPath := expandUser(repo)
	if _, err := os.Stat(filepath.Join(repoPath, "config")); err == nil {
		fmt.Println("restic repository found at", repoPath)
		return nil
	}
	fmt.Println("initializing restic repository at", repoPath)
	cmd := exec.Command(resticPath, "-r", repoPath, "init")
	cmd.Env = append(os.Environ(), "RESTIC_PASSWORD="+password)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// expandUser expands a leading ~ in p to the user's home directory.
func expandUser(p string) string {
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~/"))
		}
	}
	return p
}
