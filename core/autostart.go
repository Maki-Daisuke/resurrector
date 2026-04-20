package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"resurrector/util"
)

const (
	autoStartRegistryPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	autoStartValueName    = "Resurrector"
)

// AutoStartManager manages Windows logon startup registration for the current user.
type AutoStartManager struct {
	executablePath string
	runtimeFlags   util.RuntimeFlags
}

func NewAutoStartManager(runtimeFlags util.RuntimeFlags) (*AutoStartManager, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &AutoStartManager{
		executablePath: executablePath,
		runtimeFlags:   runtimeFlags,
	}, nil
}

func (m *AutoStartManager) Enabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, autoStartRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer key.Close()

	value, _, err := key.GetStringValue(autoStartValueName)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	return registeredExecutableMatches(value, m.executablePath)
}

func (m *AutoStartManager) SetEnabled(enabled bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, autoStartRegistryPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	if !enabled {
		err := key.DeleteValue(autoStartValueName)
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}

	return key.SetStringValue(autoStartValueName, buildAutoStartCommand(m.executablePath, m.runtimeFlags))
}

func buildAutoStartCommand(executablePath string, runtimeFlags util.RuntimeFlags) string {
	args := []string{
		executablePath,
		"-f",
		runtimeFlags.ConfigPath,
	}
	if runtimeFlags.LogFile != "" {
		args = append(args, "-log-file", runtimeFlags.LogFile)
	}
	if runtimeFlags.LogFormat != "" {
		args = append(args, "-log-format", runtimeFlags.LogFormat)
	}

	escaped := make([]string, 0, len(args))
	for _, arg := range args {
		escaped = append(escaped, windows.EscapeArg(arg))
	}

	return strings.Join(escaped, " ")
}

func registeredExecutableMatches(commandLine, executablePath string) (bool, error) {
	if strings.TrimSpace(commandLine) == "" {
		return false, nil
	}

	args, err := windows.DecomposeCommandLine(commandLine)
	if err != nil {
		return false, err
	}
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return false, nil
	}

	return normalizedPathEquals(args[0], executablePath), nil
}

func normalizedPathEquals(left, right string) bool {
	leftPath, ok := normalizePathForComparison(left)
	if !ok {
		return false
	}
	rightPath, ok := normalizePathForComparison(right)
	if !ok {
		return false
	}
	return strings.EqualFold(leftPath, rightPath)
}

func normalizePathForComparison(path string) (string, bool) {
	if strings.TrimSpace(path) == "" {
		return "", false
	}

	fullPath, err := windows.FullPath(path)
	if err == nil {
		return filepath.Clean(fullPath), true
	}

	if filepath.IsAbs(path) {
		return filepath.Clean(path), true
	}

	return "", false
}
