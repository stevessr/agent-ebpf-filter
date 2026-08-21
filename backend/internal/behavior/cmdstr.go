// Package behavior — shared command-line string utilities.
//
// These helpers normalize, split, join and key raw command lines for both
// the event pipeline (package app) and the behavior-ML algorithms
// (package app/ml). They must stay dependency-free (stdlib only).
package behavior

import "strings"

// NormalizeCommandInput derives (comm, args) from a raw command line,
// falling back to trimming comm/args when the line is empty.
func NormalizeCommandInput(commandLine string, comm string, args []string) (string, []string) {
	parts := SplitCommandLine(commandLine)
	if len(parts) > 0 {
		return parts[0], parts[1:]
	}

	comm = strings.TrimSpace(comm)
	if comm == "" {
		return "", nil
	}
	cleanArgs := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.TrimSpace(arg) == "" {
			continue
		}
		cleanArgs = append(cleanArgs, arg)
	}
	return comm, cleanArgs
}

// SplitCommandLine splits a raw command line on whitespace while honoring
// single/double quotes and backslash escapes. NUL bytes are treated as
// separators (proc cmdline style).
func SplitCommandLine(commandLine string) []string {
	commandLine = strings.TrimSpace(strings.ReplaceAll(commandLine, "\x00", " "))
	if commandLine == "" {
		return nil
	}

	var parts []string
	var b strings.Builder
	inSingle := false
	inDouble := false
	escaped := false
	emit := func() {
		if b.Len() == 0 {
			return
		}
		parts = append(parts, b.String())
		b.Reset()
	}

	for _, r := range commandLine {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\' && !inSingle:
			escaped = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case (r == ' ' || r == '\t' || r == '\n' || r == '\r') && !inSingle && !inDouble:
			emit()
		default:
			b.WriteRune(r)
		}
	}
	if escaped {
		b.WriteRune('\\')
	}
	emit()
	return parts
}

// JoinCommandLine renders (comm, args) as a display line, dropping blanks.
func JoinCommandLine(comm string, args []string) string {
	parts := append([]string{strings.TrimSpace(comm)}, args...)
	compact := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		compact = append(compact, part)
	}
	return strings.Join(compact, " ")
}

// CommandKey builds a stable dedup key for a (comm, args) pair.
func CommandKey(comm string, args []string) string {
	return comm + "\x00" + strings.Join(args, "\x00")
}

// SameStringSlice reports element-wise slice equality.
func SameStringSlice(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
