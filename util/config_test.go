package util

import (
	"os"
	"path/filepath"
	"strings"
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
	if !app.Enabled {
		t.Fatalf("Enabled = false, want true (default when omitted)")
	}
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

func TestSaveConfigOmitsDefaultValues(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	apps := map[string]*App{
		"minimal": {
			Command:        `C:\bin\app.exe`,
			Enabled:        true, // default
			Args:           []string{},
			StopArgs:       []string{},
			MaxRetries:     -1, // default
			StopTimeoutSec: 5,  // default
		},
		"explicit": {
			Command:           `C:\bin\other.exe`,
			Enabled:           false, // explicit "temporarily off" — must be written
			Args:              []string{"--flag"},
			CWD:               `C:\work`,
			HideWindow:        true,
			RestartDelaySec:   3,
			HealthyTimeoutSec: 60,
			MaxRetries:        0, // explicit "no retry" — must be written
			StopCommand:       "taskkill",
			StopArgs:          []string{"/PID", "${PID}"},
			StopTimeoutSec:    10,
		},
	}

	if err := SaveConfig(configPath, apps); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	b, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	got := string(b)

	// Default-valued keys must not appear in the [minimal] block.
	for _, key := range []string{"args", "cwd", "enabled", "hide_window", "restart_delay_sec", "healthy_timeout_sec", "max_retries", "stop_command", "stop_args", "stop_timeout_sec"} {
		marker := "\n" + key + " ="
		minimalStart := indexAfter(got, "[minimal]")
		minimalEnd := len(got)
		if next := indexAfter(got[minimalStart:], "\n["); next >= 0 {
			minimalEnd = minimalStart + next
		}
		if containsWithin(got, marker, minimalStart, minimalEnd) {
			t.Errorf("default key %q was written in [minimal] block:\n%s", key, got)
		}
	}

	// The explicit entry must round-trip every non-default field.
	expected := []string{
		`command = 'C:\bin\other.exe'`,
		"enabled = false",
		`args = ['--flag']`,
		`cwd = 'C:\work'`,
		"hide_window = true",
		"restart_delay_sec = 3",
		"healthy_timeout_sec = 60",
		"max_retries = 0",
		"stop_command = 'taskkill'",
		`stop_args = ['/PID', '${PID}']`,
		"stop_timeout_sec = 10",
	}
	for _, line := range expected {
		if !strings.Contains(got, line) {
			t.Errorf("missing %q in saved config:\n%s", line, got)
		}
	}

	// Saved file must round-trip through LoadConfig identically to the input.
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig on saved file: %v", err)
	}
	if loaded["minimal"].MaxRetries != -1 {
		t.Errorf("minimal.MaxRetries = %d, want -1", loaded["minimal"].MaxRetries)
	}
	if !loaded["minimal"].Enabled {
		t.Errorf("minimal.Enabled = false, want true (default)")
	}
	if loaded["minimal"].StopTimeoutSec != 5 {
		t.Errorf("minimal.StopTimeoutSec = %d, want 5", loaded["minimal"].StopTimeoutSec)
	}
	if loaded["explicit"].MaxRetries != 0 {
		t.Errorf("explicit.MaxRetries = %d, want 0", loaded["explicit"].MaxRetries)
	}
	if loaded["explicit"].Enabled {
		t.Errorf("explicit.Enabled = true, want false (explicitly disabled)")
	}
	if loaded["explicit"].StopTimeoutSec != 10 {
		t.Errorf("explicit.StopTimeoutSec = %d, want 10", loaded["explicit"].StopTimeoutSec)
	}
}

func TestSaveConfigWritesFieldsInFixedOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	apps := map[string]*App{
		"app": {
			Command:           `C:\bin\app.exe`,
			Enabled:           false, // non-default so it gets written and we can verify order
			Args:              []string{"--flag"},
			CWD:               `C:\work`,
			HideWindow:        true,
			RestartDelaySec:   3,
			HealthyTimeoutSec: 60,
			MaxRetries:        5,
			StopCommand:       "taskkill",
			StopArgs:          []string{"/PID", "${PID}"},
			StopTimeoutSec:    10,
		},
	}
	if err := SaveConfig(configPath, apps); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	b, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(b)

	// The fields must appear in the order declared by appTOML.
	wantOrder := []string{
		"command =",
		"args =",
		"cwd =",
		"enabled =",
		"hide_window =",
		"stop_command =",
		"stop_args =",
		"stop_timeout_sec =",
		"restart_delay_sec =",
		"max_retries =",
		"healthy_timeout_sec =",
	}
	prev := -1
	for _, key := range wantOrder {
		idx := strings.Index(got, key)
		if idx < 0 {
			t.Fatalf("missing key %q in:\n%s", key, got)
		}
		if idx <= prev {
			t.Fatalf("key %q appears out of order:\n%s", key, got)
		}
		prev = idx
	}
}

func indexAfter(s, sub string) int {
	i := strings.Index(s, sub)
	if i < 0 {
		return -1
	}
	return i + len(sub)
}

func containsWithin(s, sub string, start, end int) bool {
	if start < 0 || start >= len(s) || end > len(s) || start >= end {
		return false
	}
	return strings.Contains(s[start:end], sub)
}
