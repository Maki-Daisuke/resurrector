package util

import (
	"strings"
	"testing"
)

func TestExpandLiteralPassthrough(t *testing.T) {
	t.Parallel()

	got, err := Expand("hello world", func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestExpandDollarEscape(t *testing.T) {
	t.Parallel()

	got, err := Expand("price: $$5", func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "price: $5" {
		t.Fatalf("got %q, want %q", got, "price: $5")
	}
}

func TestExpandEscapedPlaceholder(t *testing.T) {
	t.Parallel()

	got, err := Expand("$${PID}", func(name string) (string, bool) {
		if name == "PID" {
			return "999", true
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// $$ -> $, then {PID} is literal (no leading $), so final output is "${PID}"
	if got != "${PID}" {
		t.Fatalf("got %q, want %q", got, "${PID}")
	}
}

func TestExpandDollarThenPlaceholder(t *testing.T) {
	t.Parallel()

	// $$${PID} — user wants literal $ followed by the pid value.
	got, err := Expand("$$${PID}", func(name string) (string, bool) {
		if name == "PID" {
			return "42", true
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "$42" {
		t.Fatalf("got %q, want %q", got, "$42")
	}
}

func TestExpandPlaceholder(t *testing.T) {
	t.Parallel()

	got, err := Expand("home=${HOME}/bin", func(name string) (string, bool) {
		if name == "HOME" {
			return `C:\Users\alice`, true
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `home=C:\Users\alice/bin`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExpandUndefinedVariable(t *testing.T) {
	t.Parallel()

	_, err := Expand("${NOPE}", func(string) (string, bool) { return "", false })
	if err == nil {
		t.Fatalf("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "NOPE") {
		t.Fatalf("error %q should mention the variable name", err.Error())
	}
}

func TestExpandUnterminatedBrace(t *testing.T) {
	t.Parallel()

	_, err := Expand("${HOME", func(name string) (string, bool) { return "x", true })
	if err == nil {
		t.Fatalf("expected error for unterminated ${")
	}
}

func TestExpandEmptyBraces(t *testing.T) {
	t.Parallel()

	_, err := Expand("${}", func(string) (string, bool) { return "", true })
	if err == nil {
		t.Fatalf("expected error for empty ${}")
	}
}

func TestExpandLoneDollarError(t *testing.T) {
	t.Parallel()

	_, err := Expand("price $5", func(string) (string, bool) { return "", false })
	if err == nil {
		t.Fatalf("expected error for lone $ before regular char")
	}
}

func TestExpandDollarAtEndError(t *testing.T) {
	t.Parallel()

	_, err := Expand("trailing$", func(string) (string, bool) { return "", false })
	if err == nil {
		t.Fatalf("expected error for trailing $")
	}
}

func TestExpandNonRecursive(t *testing.T) {
	t.Parallel()

	// Expanded values must NOT be re-scanned for placeholders.
	got, err := Expand("${FOO}", func(name string) (string, bool) {
		if name == "FOO" {
			return "${BAR}", true
		}
		t.Fatalf("unexpected second lookup for %q", name)
		return "", false
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "${BAR}" {
		t.Fatalf("got %q, want %q", got, "${BAR}")
	}
}

func TestExpandEnvRejectsPID(t *testing.T) {
	t.Parallel()

	_, err := ExpandEnv("prefix-${PID}")
	if err == nil {
		t.Fatalf("expected error: ${PID} must not be allowed in ExpandEnv")
	}
}

func TestExpandEnvLooksUpEnv(t *testing.T) {
	t.Setenv("RESURRECTOR_TEST_VAR", "beacon")

	got, err := ExpandEnv("x=${RESURRECTOR_TEST_VAR}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "x=beacon" {
		t.Fatalf("got %q, want %q", got, "x=beacon")
	}
}

func TestExpandEnvWithPIDExpandsPID(t *testing.T) {
	t.Parallel()

	got, err := ExpandEnvWithPID("pid=${PID}", 4242)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "pid=4242" {
		t.Fatalf("got %q, want %q", got, "pid=4242")
	}
}

func TestValidateTemplateAllowsPID(t *testing.T) {
	t.Parallel()

	if err := ValidateTemplate("${PID}", true); err != nil {
		t.Fatalf("expected ${PID} allowed, got %v", err)
	}
	if err := ValidateTemplate("${PID}", false); err == nil {
		t.Fatalf("expected ${PID} rejected when allowPID=false")
	}
}

func TestValidateTemplateCatchesSyntaxErrors(t *testing.T) {
	t.Parallel()

	if err := ValidateTemplate("oops $", false); err == nil {
		t.Fatalf("expected syntax error for trailing $")
	}
	if err := ValidateTemplate("${unterminated", false); err == nil {
		t.Fatalf("expected syntax error for unterminated ${")
	}
	if err := ValidateTemplate("${}", false); err == nil {
		t.Fatalf("expected syntax error for empty ${}")
	}
}
