package main

import (
	"archive/zip"
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
	Repo     string
	Password string
}

var embeddedConfig = config{
	Repo:     "~/tmp/test-backup",
	Password: "test password",
}

const pastebinURL = "https://pastebin.com/raw/example"

func main() {
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
}

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

func getConfig() config {
	cfg := embeddedConfig

	repoEnv, repoEnvSet := os.LookupEnv("RESTIC-REPO")
	passEnv, passEnvSet := os.LookupEnv("RESTIC-REPO-PASSWORD")
	if repoEnvSet {
		cfg.Repo = repoEnv
	}
	if passEnvSet {
		cfg.Password = passEnv
	}

	if repoEnvSet && passEnvSet {
		return cfg
	}

	pb, err := fetchPastebinConfig(pastebinURL)
	if err == nil {
		if !repoEnvSet {
			if v, ok := pb["restic-repo"]; ok {
				cfg.Repo = v
			}
		}
		if !passEnvSet {
			if v, ok := pb["restic-repo-password"]; ok {
				cfg.Password = v
			}
		}
	}

	return cfg
}

func fetchPastebinConfig(url string) (map[string]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}
	var data map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}
