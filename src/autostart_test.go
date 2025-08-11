package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnsureAutoStartLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	ensureAutoStart()
	desktop := filepath.Join(home, ".config", "autostart", "backup.desktop")
	data, err := os.ReadFile(desktop)
	if err != nil {
		t.Fatalf("read desktop: %v", err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	if !strings.Contains(string(data), exe) {
		t.Fatalf("desktop missing executable: %s", data)
	}
}
