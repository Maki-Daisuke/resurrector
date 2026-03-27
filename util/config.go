package util

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// App represents the configuration for a single managed application.
type App struct {
	Name              string   `toml:"-"`
	Enabled           bool     `toml:"enabled"`
	Command           string   `toml:"command"`
	Args              []string `toml:"args"`
	CWD               string   `toml:"cwd"`
	RestartDelaySec   int      `toml:"restart_delay_sec"`
	HealthyTimeoutSec int      `toml:"healthy_timeout_sec"`
	HideWindow        bool     `toml:"hide_window"`
	MaxRetries        int      `toml:"max_retries"`
}

// ValidateAndApplyDefaults enforces mandatory fields and sets default values.
func (a *App) ValidateAndApplyDefaults() error {
	if a.Command == "" {
		return fmt.Errorf("command is mandatory")
	}

	// Try to resolve the full path if it's not absolute
	if !filepath.IsAbs(a.Command) {
		fullPath, err := exec.LookPath(a.Command)
		if err != nil {
			return fmt.Errorf("command not found in PATH: %s", a.Command)
		}
		absPath, err := filepath.Abs(fullPath)
		if err != nil {
			return fmt.Errorf("could not get absolute path for command: %s", a.Command)
		}
		a.Command = absPath
	}

	if a.Args == nil {
		a.Args = []string{}
	}
	if a.CWD == "" {
		a.CWD = filepath.Dir(a.Command)
	}
	return nil
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
			return nil, fmt.Errorf("parsing TOML config:\n%s", de.String())
		}
		return nil, fmt.Errorf("parsing TOML config: %w", err)
	}

	for name, app := range raw {
		app.Name = name
		if err := app.ValidateAndApplyDefaults(); err != nil {
			return nil, fmt.Errorf("invalid config for app %q: %w", name, err)
		}
	}

	return raw, nil
}
