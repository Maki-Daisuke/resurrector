package util

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Lookup resolves a placeholder variable name to its value.
// Returning ok == false causes Expand to return an "undefined variable" error.
type Lookup func(name string) (string, bool)

// Expand performs one-pass placeholder expansion on s.
//
// Rules:
//   - ${NAME} is replaced with lookup(NAME); if lookup returns !ok, it is an error.
//   - $$ is replaced with a literal $.
//   - A lone $ (not followed by $ or {) is a syntax error.
//   - An unterminated ${ is a syntax error.
//   - An empty ${} is a syntax error.
//
// Expansion is non-recursive: values returned by lookup are inserted verbatim
// and are not re-scanned for placeholders.
func Expand(s string, lookup Lookup) (string, error) {
	var b strings.Builder
	b.Grow(len(s))

	i := 0
	for i < len(s) {
		c := s[i]
		if c != '$' {
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 >= len(s) {
			return "", fmt.Errorf("unescaped $ at end of input (use $$ for a literal dollar sign)")
		}
		next := s[i+1]
		if next == '$' {
			b.WriteByte('$')
			i += 2
			continue
		}
		if next != '{' {
			return "", fmt.Errorf("unescaped $ before %q (use $$ for a literal dollar sign, or ${NAME} for a variable)", next)
		}
		// ${...}
		rest := s[i+2:]
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			return "", fmt.Errorf("unterminated ${ in input")
		}
		name := rest[:end]
		if name == "" {
			return "", fmt.Errorf("empty ${} placeholder")
		}
		value, ok := lookup(name)
		if !ok {
			return "", fmt.Errorf("undefined variable: ${%s}", name)
		}
		b.WriteString(value)
		i += 2 + end + 1
	}
	return b.String(), nil
}

// ExpandEnv expands ${NAME} placeholders using environment variables and
// converts $$ to a literal $. The reserved name ${PID} is rejected here
// because it is only meaningful inside stop_args.
func ExpandEnv(s string) (string, error) {
	return Expand(s, func(name string) (string, bool) {
		if name == "PID" {
			return "", false
		}
		return os.LookupEnv(name)
	})
}

// ExpandEnvWithPID is like ExpandEnv but additionally expands ${PID}
// to the decimal string form of pid. Used for stop_args at stop time.
func ExpandEnvWithPID(s string, pid int) (string, error) {
	return Expand(s, func(name string) (string, bool) {
		if name == "PID" {
			return strconv.Itoa(pid), true
		}
		return os.LookupEnv(name)
	})
}

// ValidateTemplate checks that s is a syntactically well-formed template
// (balanced ${}, no stray $, no empty ${}) without actually substituting
// any values. ${PID} is accepted only if allowPID is true.
func ValidateTemplate(s string, allowPID bool) error {
	_, err := Expand(s, func(name string) (string, bool) {
		if name == "PID" {
			return "", allowPID
		}
		// Pretend every other variable is defined for syntax-only validation.
		return "", true
	})
	return err
}
