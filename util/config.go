package util

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2"
)

// App represents the configuration for a single managed application.
type App struct {
	Name              string   `toml:"-"`
	Enabled           bool     `toml:"enabled"`
	Command           string   `toml:"command"`
	Args              []string `toml:"args"`
	StopCommand       string   `toml:"stop_command"`
	StopArgs          []string `toml:"stop_args"`
	CWD               string   `toml:"cwd"`
	RestartDelaySec   int      `toml:"restart_delay_sec"`
	HealthyTimeoutSec int      `toml:"healthy_timeout_sec"`
	HideWindow        bool     `toml:"hide_window"`
	MaxRetries        int      `toml:"max_retries"`
	StopTimeoutSec    int      `toml:"stop_timeout_sec"`
}

// ValidateAndApplyDefaults enforces mandatory fields, fills in default values
// for omitted fields, and validates placeholder syntax and referenced env vars.
//
// Command, Args, CWD, StopCommand, and StopArgs are not mutated: the user's
// original input is preserved so it round-trips through SaveConfig without
// losing ${NAME} references or being rewritten to absolute paths. Expansion
// and path resolution happen on demand via the Resolved* helpers.
func (a *App) ValidateAndApplyDefaults() error {
	if a.Command == "" {
		return fmt.Errorf("command is mandatory")
	}

	if _, err := a.ResolvedCommand(); err != nil {
		return err
	}

	if a.Args == nil {
		a.Args = []string{}
	}
	for i, arg := range a.Args {
		if _, err := ExpandEnv(arg); err != nil {
			return fmt.Errorf("invalid args[%d]: %w", i, err)
		}
	}

	if a.StopArgs == nil {
		a.StopArgs = []string{}
	}
	for i, arg := range a.StopArgs {
		if err := ValidateTemplate(arg, true); err != nil {
			return fmt.Errorf("invalid stop_args[%d]: %w", i, err)
		}
	}

	if a.StopCommand != "" {
		if _, err := a.ResolvedStopCommand(); err != nil {
			return err
		}
	}

	if a.CWD != "" {
		if _, err := ExpandEnv(a.CWD); err != nil {
			return fmt.Errorf("invalid cwd: %w", err)
		}
	}

	if a.StopTimeoutSec < 0 {
		return fmt.Errorf("stop_timeout_sec must be >= 0")
	}
	return nil
}

// ResolvedCommand returns Command with ${NAME} placeholders expanded and the
// path resolved to an absolute path via the system PATH when necessary.
func (a *App) ResolvedCommand() (string, error) {
	expanded, err := ExpandEnv(a.Command)
	if err != nil {
		return "", fmt.Errorf("invalid command: %w", err)
	}
	resolved, err := resolveCommandPath(expanded)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

// ResolvedArgs returns Args with ${NAME} placeholders expanded in each element.
func (a *App) ResolvedArgs() ([]string, error) {
	out := make([]string, len(a.Args))
	for i, arg := range a.Args {
		expanded, err := ExpandEnv(arg)
		if err != nil {
			return nil, fmt.Errorf("invalid args[%d]: %w", i, err)
		}
		out[i] = expanded
	}
	return out, nil
}

// ResolvedStopCommand returns StopCommand with ${NAME} placeholders expanded
// and the path resolved to an absolute path via the system PATH when necessary.
// Returns "" (no error) when StopCommand is empty.
func (a *App) ResolvedStopCommand() (string, error) {
	if a.StopCommand == "" {
		return "", nil
	}
	expanded, err := ExpandEnv(a.StopCommand)
	if err != nil {
		return "", fmt.Errorf("invalid stop_command: %w", err)
	}
	resolved, err := resolveCommandPath(expanded)
	if err != nil {
		return "", fmt.Errorf("invalid stop_command: %w", err)
	}
	return resolved, nil
}

// ResolvedCWD returns CWD with ${NAME} placeholders expanded. If CWD is empty,
// it defaults to the directory containing the resolved Command.
func (a *App) ResolvedCWD() (string, error) {
	if a.CWD != "" {
		expanded, err := ExpandEnv(a.CWD)
		if err != nil {
			return "", fmt.Errorf("invalid cwd: %w", err)
		}
		return expanded, nil
	}
	resolvedCommand, err := a.ResolvedCommand()
	if err != nil {
		return "", err
	}
	return filepath.Dir(resolvedCommand), nil
}

// LoadConfig reads the configuration file from the given path and parses it.
// Returns a map of app name → App config. If the file is invalid TOML,
// an error is returned; the caller should keep the current state unchanged.
func LoadConfig(path string) (map[string]*App, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	var raw map[string]*App
	if err := toml.Unmarshal(b, &raw); err != nil {
		if de, ok := err.(*toml.DecodeError); ok {
			row, col := de.Position()
			return nil, fmt.Errorf("parsing TOML config at line %d, column %d:\n%s\n%w", row, col, de.String(), de)
		}
		return nil, fmt.Errorf("parsing TOML config: %w", err)
	}

	// Track presence of optional fields so we can apply defaults without
	// changing the meaning of explicit zero values.
	//
	// Example: max_retries omitted => default infinite retry (-1).
	//          max_retries = 0     => explicit "no retry".
	//          stop_timeout_sec omitted => default graceful timeout (5).
	var rawTables map[string]map[string]any
	if err := toml.Unmarshal(b, &rawTables); err != nil {
		if de, ok := err.(*toml.DecodeError); ok {
			row, col := de.Position()
			return nil, fmt.Errorf("parsing TOML config at line %d, column %d:\n%s\n%w", row, col, de.String(), de)
		}
		return nil, fmt.Errorf("parsing TOML config: %w", err)
	}

	for name, app := range raw {
		app.Name = name
		if table, ok := rawTables[name]; ok {
			if _, ok := table["enabled"]; !ok {
				app.Enabled = true
			}
			if _, ok := table["max_retries"]; !ok {
				// Default: infinite retries when omitted.
				app.MaxRetries = -1
			}
			if _, ok := table["stop_timeout_sec"]; !ok {
				app.StopTimeoutSec = 5
			}
		} else {
			panic("Should not happen")
		}
		if err := app.ValidateAndApplyDefaults(); err != nil {
			return nil, fmt.Errorf("invalid config for app %q: %w", name, err)
		}
	}

	return raw, nil
}

// SaveConfig writes the given apps map to the config file at path using an
// atomic write (temp file -> sync -> close -> rename) to prevent partial writes.
// The Name field on each App is ignored; the map key is used as the TOML section name.
func SaveConfig(path string, apps map[string]*App) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".resurrector-config-*.toml")
	if err != nil {
		return fmt.Errorf("creating temp file for atomic write: %w", err)
	}
	tmpPath := tmp.Name()

	// Cleanup on failure
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	b, err := toml.Marshal(marshalApps(apps))
	if err != nil {
		return fmt.Errorf("marshaling config to TOML: %w", err)
	}

	if _, err := tmp.Write(b); err != nil {
		return fmt.Errorf("writing temp config file: %w", err)
	}

	// Flush OS buffers before rename to guarantee durability.
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("syncing temp config file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp config file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming temp config file to %s: %w", path, err)
	}

	ok = true
	return nil
}

// marshalApps converts apps into a TOML-ready representation that
//   - omits fields equal to their documented default, so the on-disk config
//     stays minimal (mirrors what LoadConfig treats as "field omitted");
//   - preserves a fixed field order within each table. go-toml/v2 walks
//     struct fields in declaration order, so defining appTOML below fixes
//     the order of keys in the saved file regardless of map iteration.
//
// Command is always written because it is mandatory and has no default.
type appTOML struct {
	Command           string   `toml:"command"`
	Args              []string `toml:"args,omitempty"`
	CWD               *string  `toml:"cwd,omitempty"`
	Enabled           *bool    `toml:"enabled,omitempty"`
	HideWindow        *bool    `toml:"hide_window,omitempty"`
	StopCommand       *string  `toml:"stop_command,omitempty"`
	StopArgs          []string `toml:"stop_args,omitempty"`
	StopTimeoutSec    *int     `toml:"stop_timeout_sec,omitempty"`
	RestartDelaySec   *int     `toml:"restart_delay_sec,omitempty"`
	MaxRetries        *int     `toml:"max_retries,omitempty"`
	HealthyTimeoutSec *int     `toml:"healthy_timeout_sec,omitempty"`
}

func marshalApps(apps map[string]*App) map[string]appTOML {
	out := make(map[string]appTOML, len(apps))
	for name, app := range apps {
		t := appTOML{Command: app.Command}
		if !app.Enabled {
			// Default is true; only persist the explicit "temporarily off" case.
			v := false
			t.Enabled = &v
		}
		if len(app.Args) > 0 {
			t.Args = app.Args
		}
		if app.CWD != "" {
			v := app.CWD
			t.CWD = &v
		}
		if app.HideWindow {
			v := true
			t.HideWindow = &v
		}
		if app.RestartDelaySec != 0 {
			v := app.RestartDelaySec
			t.RestartDelaySec = &v
		}
		if app.HealthyTimeoutSec != 0 {
			v := app.HealthyTimeoutSec
			t.HealthyTimeoutSec = &v
		}
		if app.MaxRetries != -1 {
			v := app.MaxRetries
			t.MaxRetries = &v
		}
		if app.StopCommand != "" {
			v := app.StopCommand
			t.StopCommand = &v
		}
		if len(app.StopArgs) > 0 {
			t.StopArgs = app.StopArgs
		}
		if app.StopTimeoutSec != 5 {
			v := app.StopTimeoutSec
			t.StopTimeoutSec = &v
		}
		out[name] = t
	}
	return out
}

func resolveCommandPath(command string) (string, error) {
	if filepath.IsAbs(command) {
		return command, nil
	}

	fullPath, err := exec.LookPath(command)
	if err != nil {
		return "", fmt.Errorf("command not found in PATH: %s", command)
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("could not get absolute path for command: %s", command)
	}
	return absPath, nil
}

// ParseArgs splits a shell-like argument string into a slice of strings.
// It respects single-quoted and double-quoted strings, and backslash escapes
// inside double-quoted strings.
//
// Examples:
//
//	`-v --debug`            → ["-v", "--debug"]
//	`-c "hello world"`      → ["-c", "hello world"]
//	`-c 'it'\''s fine'`    → ["-c", "it's fine"]
func ParseArgs(s string) ([]string, error) {
	var args []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	for _, r := range s {
		switch {
		case escaped:
			// Inside double-quote escape: only certain chars are special
			current.WriteRune(r)
			escaped = false

		case r == '\\' && inDouble:
			escaped = true

		case r == '\'' && !inDouble:
			inSingle = !inSingle

		case r == '"' && !inSingle:
			inDouble = !inDouble

		case unicode.IsSpace(r) && !inSingle && !inDouble:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}

		default:
			current.WriteRune(r)
		}
	}

	if inSingle {
		return nil, fmt.Errorf("unterminated single quote in args string")
	}
	if inDouble {
		return nil, fmt.Errorf("unterminated double quote in args string")
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args, nil
}

// FormatArgs joins a slice of strings into a single shell-safe string for display.
// Arguments that contain spaces or special shell characters are double-quoted,
// and any internal double-quotes are backslash-escaped.
func FormatArgs(args []string) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// shellQuote quotes a single argument for safe display in a shell-like text box.
func shellQuote(s string) string {
	needsQuote := false
	for _, r := range s {
		if unicode.IsSpace(r) || strings.ContainsRune(`"'\\&|;<>()$`, r) {
			needsQuote = true
			break
		}
	}
	if !needsQuote && s != "" {
		return s
	}
	// Double-quote the argument, escaping any internal double-quotes and backslashes.
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		if r == '"' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}
