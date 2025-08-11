package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ensureAutoStart configures the program to run at user login for the current OS.
func ensureAutoStart() {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "auto-start: executable: %v\n", err)
		return
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "auto-start: abs: %v\n", err)
		return
	}

	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("reg", "add",
			`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
			"/v", "backup",
			"/t", "REG_SZ",
			"/d", exePath,
			"/f",
		)
		_ = cmd.Run()
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "auto-start: home: %v\n", err)
			return
		}
		dir := filepath.Join(home, "Library", "LaunchAgents")
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "auto-start: mkdir: %v\n", err)
			return
		}
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>com.example.backup</string>
    <key>ProgramArguments</key>
    <array><string>%s</string></array>
    <key>RunAtLoad</key><true/>
</dict>
</plist>
`, exePath)
		plistPath := filepath.Join(dir, "com.example.backup.plist")
		_ = os.WriteFile(plistPath, []byte(plist), 0644)
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "auto-start: home: %v\n", err)
			return
		}
		dir := filepath.Join(home, ".config", "autostart")
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "auto-start: mkdir: %v\n", err)
			return
		}
		desktop := fmt.Sprintf(`[Desktop Entry]
Type=Application
Exec=%s
Hidden=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
Name=backup
Comment=Backup program
`, exePath)
		desktopPath := filepath.Join(dir, "backup.desktop")
		_ = os.WriteFile(desktopPath, []byte(desktop), 0644)
	}
}
