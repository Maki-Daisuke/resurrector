package main

import (
	"testing"

	"golang.org/x/sys/windows"

	"resurrector/util"
)

func TestHasIdentityChangedIgnoresStopSettings(t *testing.T) {
	t.Parallel()

	current := util.App{
		Command: `C:\Windows\System32\cmd.exe`,
		CWD:     `C:\Windows\System32`,
	}
	desired := current
	desired.StopCommand = `C:\Windows\System32\taskkill.exe`
	desired.StopArgs = []string{"/PID", "{pid}"}
	desired.StopTimeoutSec = 10

	if hasIdentityChanged(current, desired) {
		t.Fatalf("expected stop settings not to require restart")
	}
}

func TestHasMonitoringParamsChangedIncludesStopSettings(t *testing.T) {
	t.Parallel()

	current := util.App{
		StopTimeoutSec: 5,
	}
	desired := current
	desired.StopTimeoutSec = 10

	if !hasMonitoringParamsChanged(current, desired) {
		t.Fatalf("expected stop_timeout_sec change to hot-reload")
	}

	desired = current
	desired.StopCommand = "taskkill"
	desired.StopArgs = []string{"/PID", "{pid}"}

	if !hasMonitoringParamsChanged(current, desired) {
		t.Fatalf("expected stop_command change to hot-reload")
	}
}

func TestProcessCreationFlagsIncludeProcessGroup(t *testing.T) {
	t.Parallel()

	flags := processCreationFlags()
	want := uint32(windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP)
	if flags != want {
		t.Fatalf("flags = %#x, want %#x", flags, want)
	}
}

func TestExpandStopArgsExpandsPID(t *testing.T) {
	t.Parallel()

	got := expandStopArgs([]string{"/PID", "{pid}", "/FI", "pid eq {pid}"}, 4242)
	want := []string{"/PID", "4242", "/FI", "pid eq 4242"}

	if !stringSlicesEqual(got, want) {
		t.Fatalf("expandStopArgs() = %#v, want %#v", got, want)
	}
}

func TestIsWindowStopTargetClassSkipsConsoleWindow(t *testing.T) {
	t.Parallel()

	if isWindowStopTargetClass(consoleWindowClass) {
		t.Fatalf("expected console window class to be excluded from window stop")
	}

	if isWindowStopTargetClass(pseudoConsoleWindowClass) {
		t.Fatalf("expected pseudo-console window class to be excluded from window stop")
	}

	if !isWindowStopTargetClass("AlertOnWMCloseWindow") {
		t.Fatalf("expected application window class to be included in window stop")
	}
}
