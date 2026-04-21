package main

import (
	"fmt"

	"resurrector/util"
)

// AppConfig is the DTO exposed to the frontend via the Wails bridge.
// It mirrors util.App but with Args represented as a pre-formatted shell string
// so the frontend can display it in a single text box.
type AppConfig struct {
	Name              string `json:"name"`
	Enabled           bool   `json:"enabled"`
	Command           string `json:"command"`
	Args              string `json:"args"` // Shell-formatted, e.g. `/c "hello world" --debug`
	StopCommand       string `json:"stopCommand"`
	CWD               string `json:"cwd"`
	RestartDelaySec   int    `json:"restartDelaySec"`
	HealthyTimeoutSec int    `json:"healthyTimeoutSec"`
	HideWindow        bool   `json:"hideWindow"`
	MaxRetries        int    `json:"maxRetries"`
	StopTimeoutSec    int    `json:"stopTimeoutSec"`
}

// GetFullConfig reads config.toml and returns all app entries as AppConfig DTOs.
// Args is formatted as a shell-like string for display in the UI text box.
func (a *App) GetFullConfig() (map[string]AppConfig, error) {
	if a.configPath == "" {
		return nil, fmt.Errorf("config path is not set")
	}
	apps, err := util.LoadConfig(a.configPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	result := make(map[string]AppConfig, len(apps))
	for name, app := range apps {
		result[name] = AppConfig{
			Name:              name,
			Enabled:           app.Enabled,
			Command:           app.Command,
			Args:              util.FormatArgs(app.Args),
			StopCommand:       util.FormatArgs(app.StopCommand),
			CWD:               app.CWD,
			RestartDelaySec:   app.RestartDelaySec,
			HealthyTimeoutSec: app.HealthyTimeoutSec,
			HideWindow:        app.HideWindow,
			MaxRetries:        app.MaxRetries,
			StopTimeoutSec:    app.StopTimeoutSec,
		}
	}
	return result, nil
}

// UpdateAppConfig saves the given AppConfig to config.toml.
// If oldName != newName, the entry is renamed (old key deleted, new key written).
// Args is parsed from the shell-formatted string sent by the frontend.
func (a *App) UpdateAppConfig(oldName string, cfg AppConfig) error {
	if a.configPath == "" {
		return fmt.Errorf("config path is not set")
	}

	parsedArgs, err := util.ParseArgs(cfg.Args)
	if err != nil {
		return fmt.Errorf("parsing args: %w", err)
	}
	parsedStopCommand, err := util.ParseArgs(cfg.StopCommand)
	if err != nil {
		return fmt.Errorf("parsing stop command: %w", err)
	}

	// Load the current config to preserve all other entries.
	apps, err := util.LoadConfig(a.configPath)
	if err != nil {
		return fmt.Errorf("loading current config: %w", err)
	}

	// If the name changed, remove the old key.
	if oldName != cfg.Name {
		delete(apps, oldName)
	}

	apps[cfg.Name] = &util.App{
		Name:              cfg.Name,
		Enabled:           cfg.Enabled,
		Command:           cfg.Command,
		Args:              parsedArgs,
		StopCommand:       parsedStopCommand,
		CWD:               cfg.CWD,
		RestartDelaySec:   cfg.RestartDelaySec,
		HealthyTimeoutSec: cfg.HealthyTimeoutSec,
		HideWindow:        cfg.HideWindow,
		MaxRetries:        cfg.MaxRetries,
		StopTimeoutSec:    cfg.StopTimeoutSec,
	}

	if err := util.SaveConfig(a.configPath, apps); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	return nil
}

// DeleteAppConfig removes the entry with the given name from config.toml.
func (a *App) DeleteAppConfig(name string) error {
	if a.configPath == "" {
		return fmt.Errorf("config path is not set")
	}

	apps, err := util.LoadConfig(a.configPath)
	if err != nil {
		return fmt.Errorf("loading current config: %w", err)
	}

	if _, ok := apps[name]; !ok {
		return fmt.Errorf("app %q not found in config", name)
	}
	delete(apps, name)

	if err := util.SaveConfig(a.configPath, apps); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	return nil
}
