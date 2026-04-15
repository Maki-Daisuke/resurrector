package util

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// RuntimeFlags contains common runtime options used by core and UI binaries.
type RuntimeFlags struct {
	ConfigPath string
	LogFile    string
	LogFormat  string
}

// ParseRuntimeFlags parses common CLI options.
func ParseRuntimeFlags() (RuntimeFlags, error) {
	var options RuntimeFlags
	flag.StringVar(&options.ConfigPath, "f", "", "Path to config.toml")
	flag.StringVar(&options.LogFile, "log-file", "", "Path to log file (optional, append mode). If empty, logs go to stderr")
	flag.StringVar(&options.LogFormat, "log-format", "text", "Log format: text or json")
	flag.Parse()

	if options.ConfigPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return RuntimeFlags{}, fmt.Errorf("getting user home dir: %w", err)
		}
		options.ConfigPath = filepath.Join(home, ".config", "resurrector", "config.toml")
	}

	absoluteConfigPath, err := filepath.Abs(options.ConfigPath)
	if err != nil {
		return RuntimeFlags{}, fmt.Errorf("resolving config path %q: %w", options.ConfigPath, err)
	}
	options.ConfigPath = absoluteConfigPath

	return options, nil
}
