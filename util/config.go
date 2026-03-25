package util

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// Config holds the entire application configuration, keyed by app name.
type Config struct {
	Apps map[string]*App
}

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

// LoadConfig reads the configuration file from the given path and parses it.
func LoadConfig(path string) (*Config, error) {
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
	}

	return &Config{Apps: raw}, nil
}
