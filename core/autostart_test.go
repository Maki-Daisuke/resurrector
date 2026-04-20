package main

import (
	"testing"

	"resurrector/util"
)

func TestBuildAutoStartCommandIncludesQuotedExecutableAndFlags(t *testing.T) {
	t.Parallel()

	command := buildAutoStartCommand(
		`C:\Program Files\Resurrector\resurrector.exe`,
		util.RuntimeFlags{
			ConfigPath: `C:\Users\Daisu\AppData\Roaming\Resurrector\config file.toml`,
			LogFile:    `C:\Logs\resurrector log.txt`,
			LogFormat:  "json",
		},
	)

	want := `"C:\Program Files\Resurrector\resurrector.exe" -f "C:\Users\Daisu\AppData\Roaming\Resurrector\config file.toml" -log-file "C:\Logs\resurrector log.txt" -log-format json`
	if command != want {
		t.Fatalf("unexpected command line:\nwant: %s\ngot:  %s", want, command)
	}
}

func TestBuildAutoStartCommandOmitsEmptyOptionalFlags(t *testing.T) {
	t.Parallel()

	command := buildAutoStartCommand(
		`C:\Resurrector\resurrector.exe`,
		util.RuntimeFlags{
			ConfigPath: `C:\Users\Daisu\.config\resurrector\config.toml`,
			LogFormat:  "text",
		},
	)

	want := `C:\Resurrector\resurrector.exe -f C:\Users\Daisu\.config\resurrector\config.toml -log-format text`
	if command != want {
		t.Fatalf("unexpected command line:\nwant: %s\ngot:  %s", want, command)
	}
}

func TestRegisteredExecutableMatchesIgnoresArguments(t *testing.T) {
	t.Parallel()

	command := `"C:\Program Files\Resurrector\resurrector.exe" -f C:\different-config.toml -log-format json`
	matches, err := registeredExecutableMatches(command, `C:\Program Files\Resurrector\resurrector.exe`)
	if err != nil {
		t.Fatalf("registeredExecutableMatches returned error: %v", err)
	}
	if !matches {
		t.Fatalf("expected executable paths to match")
	}
}

func TestRegisteredExecutableMatchesRejectsDifferentExecutable(t *testing.T) {
	t.Parallel()

	command := `"C:\Program Files\Old Resurrector\resurrector.exe" -f C:\config.toml`
	matches, err := registeredExecutableMatches(command, `C:\Program Files\Resurrector\resurrector.exe`)
	if err != nil {
		t.Fatalf("registeredExecutableMatches returned error: %v", err)
	}
	if matches {
		t.Fatalf("expected executable paths not to match")
	}
}

func TestRegisteredExecutableMatchesRejectsEmptyValue(t *testing.T) {
	t.Parallel()

	matches, err := registeredExecutableMatches("   ", `C:\Program Files\Resurrector\resurrector.exe`)
	if err != nil {
		t.Fatalf("registeredExecutableMatches returned error: %v", err)
	}
	if matches {
		t.Fatalf("expected empty value not to match")
	}
}
