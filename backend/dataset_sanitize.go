package main

import (
	"regexp"
	"strings"
)

var (
	sensitiveValueRegexps = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|apikey|api-key|access[_-]?key|authorization|cookie)[\s:=]+[^\s"'` + "`" + `]+`),
		regexp.MustCompile(`(?i)\b(--?(?:password|passwd|pwd|token|secret|apikey|api-key|access[_-]?key))(?:[=\s]+)[^\s"'` + "`" + `]+`),
		regexp.MustCompile(`(?i)\b(https?://)[^/\s:@]+(?::[^/\s@]+)?@`),
		regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`),
		regexp.MustCompile(`(?i)\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`(?i)\bAuthorization[\s:=]+(?:Bearer\s+|Basic\s+)?[^\s"'` + "`" + `]+(?:\s+[^\s"'` + "`" + `]+)?`),
		regexp.MustCompile(`(?i)\b(Bearer|Basic)\s+[A-Za-z0-9._~+/=-]+`),
		regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`),
	}
	sensitivePathRegexps = []*regexp.Regexp{
		regexp.MustCompile(`/home/[^/\s]+`),
		regexp.MustCompile(`~\/[^\s]+`),
		regexp.MustCompile(`/etc/(passwd|shadow|sudoers)`),
	}
)

func sanitizeRemoteDatasetRecord(record remoteDatasetRecord) remoteDatasetRecord {
	record.CommandLine = sanitizeRemoteDatasetText(record.CommandLine)
	if len(record.Args) > 0 {
		sanitizedArgs := make([]string, 0, len(record.Args))
		for _, arg := range record.Args {
			sanitizedArgs = append(sanitizedArgs, sanitizeRemoteDatasetText(arg))
		}
		record.Args = sanitizedArgs
	}
	return record
}

func sanitizeRemoteDatasetRow(row remoteDatasetRow) remoteDatasetRow {
	row.CommandLine = sanitizeRemoteDatasetText(row.CommandLine)
	if len(row.Args) > 0 {
		sanitizedArgs := make([]string, 0, len(row.Args))
		for _, arg := range row.Args {
			sanitizedArgs = append(sanitizedArgs, sanitizeRemoteDatasetText(arg))
		}
		row.Args = sanitizedArgs
	}
	return row
}

func sanitizeRemoteDatasetText(text string) string {
	if text == "" {
		return text
	}
	out := text
	for _, re := range sensitiveValueRegexps {
		out = re.ReplaceAllStringFunc(out, func(match string) string {
			lower := strings.ToLower(match)
			switch {
			case strings.Contains(lower, "http://") || strings.Contains(lower, "https://"):
				if idx := strings.Index(match, "://"); idx >= 0 {
					return match[:idx+3] + "***@"
				}
				return "***"
			case strings.Contains(lower, "@") && !strings.Contains(lower, "authorization"):
				return "***@***"
			case strings.Contains(lower, "akia"):
				return "AKIA****************"
			case strings.Contains(lower, "password"), strings.Contains(lower, "passwd"), strings.Contains(lower, "pwd"),
				strings.Contains(lower, "token"), strings.Contains(lower, "secret"), strings.Contains(lower, "apikey"),
				strings.Contains(lower, "api-key"), strings.Contains(lower, "access_key"), strings.Contains(lower, "access-key"),
				strings.Contains(lower, "authorization"), strings.Contains(lower, "cookie"):
				if strings.Contains(lower, "bearer ") || strings.Contains(lower, "basic ") {
					return "Authorization: ***"
				}
				if idx := strings.IndexAny(match, " =:"); idx >= 0 {
					return match[:idx+1] + "***"
				}
				return "***"
			default:
				if strings.Contains(match, ".") && strings.Count(match, ".") == 3 {
					return "***.***.***.**"
				}
				return "***"
			}
		})
	}
	for _, re := range sensitivePathRegexps {
		out = re.ReplaceAllStringFunc(out, func(repl string) string {
			switch {
			case strings.HasPrefix(repl, "/home/"):
				return "/home/***"
			case strings.HasPrefix(repl, "~/"):
				return "~/***"
			case strings.HasPrefix(repl, "/etc/"):
				return "/etc/***"
			default:
				return "***"
			}
		})
	}
	return out
}

func inferRemoteDatasetLabelFromSource(source string) (string, string) {
	lower := strings.ToLower(strings.TrimSpace(source))
	if lower == "" {
		return "", ""
	}
	switch {
	case strings.Contains(lower, "benign"),
		strings.Contains(lower, "normal"),
		strings.Contains(lower, "allow"),
		strings.Contains(lower, "safe"),
		strings.Contains(lower, "norm"):
		return "ALLOW", "source"
	case strings.Contains(lower, "malicious"),
		strings.Contains(lower, "mixed_malicious"),
		strings.Contains(lower, "attack"),
		strings.Contains(lower, "exploit"),
		strings.Contains(lower, "backdoor"),
		strings.Contains(lower, "trojan"),
		strings.Contains(lower, "ransomware"),
		strings.Contains(lower, "botnet"),
		strings.Contains(lower, "worm"),
		strings.Contains(lower, "cmdi"),
		strings.Contains(lower, "sqli"),
		strings.Contains(lower, "xss"),
		strings.Contains(lower, "path-traversal"),
		strings.Contains(lower, "path_traversal"),
		strings.Contains(lower, "anom"),
		strings.Contains(lower, "anomali"):
		return "BLOCK", "source"
	}
	return "", ""
}
