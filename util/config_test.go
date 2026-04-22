package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigAppliesStopDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	exe := filepath.Join(dir, "app.exe")
	if err := os.WriteFile(exe, []byte(""), 0644); err != nil {
		t.Fatalf("write exe: %v", err)
	}

	configPath := filepath.Join(dir, "config.toml")
	config := `["app"]
command = '` + exe + `'
enabled = true
`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	apps, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	app := apps["app"]
	if app.StopCommand != "" {
		t.Fatalf("StopCommand = %q, want empty string", app.StopCommand)
	}
	if len(app.StopArgs) != 0 {
		t.Fatalf("StopArgs = %#v, want empty slice", app.StopArgs)
	}
	if app.StopTimeoutSec != 5 {
		t.Fatalf("StopTimeoutSec = %d, want 5", app.StopTimeoutSec)
	}
}

func TestValidateAndApplyDefaultsRejectsNegativeStopTimeout(t *testing.T) {
	t.Parallel()

	app := &App{
		Command:        `C:\Windows\System32\cmd.exe`,
		StopTimeoutSec: -1,
	}

	if err := app.ValidateAndApplyDefaults(); err == nil {
		t.Fatalf("expected negative stop_timeout_sec error")
	}
}

func TestValidateAndApplyDefaultsResolvesStopCommand(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "app.exe")
	stopExe := filepath.Join(dir, "stop.cmd")
	if err := os.WriteFile(exe, []byte(""), 0644); err != nil {
		t.Fatalf("write exe: %v", err)
	}
	if err := os.WriteFile(stopExe, []byte(""), 0644); err != nil {
		t.Fatalf("write stop cmd: %v", err)
	}

	pathEnv := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+pathEnv)

	app := &App{
		Command:        exe,
		StopCommand:    "stop.cmd",
		StopArgs:       []string{"/PID", "{pid}"},
		StopTimeoutSec: 5,
	}

	if err := app.ValidateAndApplyDefaults(); err != nil {
		t.Fatalf("ValidateAndApplyDefaults returned error: %v", err)
	}
	if app.StopCommand != stopExe {
		t.Fatalf("StopCommand = %q, want %q", app.StopCommand, stopExe)
	}
}
