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

func TestValidateAndApplyDefaultsPreservesStopCommand(t *testing.T) {
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
		StopArgs:       []string{"/PID", "${PID}"},
		StopTimeoutSec: 5,
	}

	if err := app.ValidateAndApplyDefaults(); err != nil {
		t.Fatalf("ValidateAndApplyDefaults returned error: %v", err)
	}
	// ValidateAndApplyDefaults must not rewrite the user's input to an
	// absolute path; SaveConfig would otherwise persist the resolved value.
	if app.StopCommand != "stop.cmd" {
		t.Fatalf("StopCommand = %q, want %q (unchanged)", app.StopCommand, "stop.cmd")
	}
	// Resolution happens on demand via ResolvedStopCommand.
	resolved, err := app.ResolvedStopCommand()
	if err != nil {
		t.Fatalf("ResolvedStopCommand returned error: %v", err)
	}
	if resolved != stopExe {
		t.Fatalf("ResolvedStopCommand = %q, want %q", resolved, stopExe)
	}
}

func TestValidateAndApplyDefaultsPreservesEnvVarsAndRelativePaths(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "app.exe")
	if err := os.WriteFile(exe, []byte(""), 0644); err != nil {
		t.Fatalf("write exe: %v", err)
	}
	t.Setenv("RESURRECTOR_TEST_DIR", dir)
	t.Setenv("RESURRECTOR_TEST_ARG", "from-env")

	app := &App{
		Command:        "${RESURRECTOR_TEST_DIR}/app.exe",
		Args:           []string{"--value=${RESURRECTOR_TEST_ARG}", "--pid=$$"},
		CWD:            "${RESURRECTOR_TEST_DIR}",
		StopTimeoutSec: 5,
	}

	if err := app.ValidateAndApplyDefaults(); err != nil {
		t.Fatalf("ValidateAndApplyDefaults returned error: %v", err)
	}

	if app.Command != "${RESURRECTOR_TEST_DIR}/app.exe" {
		t.Fatalf("Command was mutated: got %q", app.Command)
	}
	if app.Args[0] != "--value=${RESURRECTOR_TEST_ARG}" {
		t.Fatalf("Args[0] was mutated: got %q", app.Args[0])
	}
	if app.Args[1] != "--pid=$$" {
		t.Fatalf("Args[1] was mutated: got %q", app.Args[1])
	}
	if app.CWD != "${RESURRECTOR_TEST_DIR}" {
		t.Fatalf("CWD was mutated: got %q", app.CWD)
	}

	// Expansion is still available on demand.
	resolvedArgs, err := app.ResolvedArgs()
	if err != nil {
		t.Fatalf("ResolvedArgs: %v", err)
	}
	if resolvedArgs[0] != "--value=from-env" || resolvedArgs[1] != "--pid=$" {
		t.Fatalf("ResolvedArgs = %#v", resolvedArgs)
	}
}
