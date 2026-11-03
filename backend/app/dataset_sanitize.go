package app

import (
	"regexp"
	"strings"
)

var (
	sensitiveValueRegexps = []*regexp.Regexp{
		// Match the entire Authorization credential before the generic
		// "authorization: WORD" matcher; otherwise Bearer tokens leak.
		regexp.MustCompile(`(?i)\bAuthorization\s*[:=]\s*(?:Bearer|Basic)\s+(?:"[^"]+"|'[^']+'|[^\s"'\x60]+)`),
		// Quoted values are still private, even with spaces inside the
		// quotes (e.g. --token="long private value").
		regexp.MustCompile(`(?i)\b(?:password|passwd|pwd|token|secret|apikey|api-key|access[_-]?key|cookie)[\s:=]+(?:"[^"]+"|'[^']+')`),
		regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|apikey|api-key|access[_-]?key|authorization|cookie)[\s:=]+[^\s"'` + "`" + `]+`),
		regexp.MustCompile(`(?i)\b(--?(?:password|passwd|pwd|token|secret|apikey|api-key|access[_-]?key))(?:[=\s]+)[^\s"'` + "`" + `]+`),
		regexp.MustCompile(`(?i)\b(https?://)[^/\s:@]+(?::[^/\s@]+)?@`),
		regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`),
		regexp.MustCompile(`(?i)\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`(?i)\bAuthorization[\s:=]+(?:Bearer\s+|Basic\s+)?[^\s"'` + "`" + `]+(?:\s+[^\s"'` + "`" + `]+)?`),
		regexp.MustCompile(`(?i)\b(Bearer|Basic)\s+[A-Za-z0-9._~+/=-]+`),
		regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`),
	}
	// Unix crypt hashes are data, unlike the canonical /etc/shadow path name.
	// This is a defense in depth for command text that embeds a hash literal.
	unixCryptHashRegexp = regexp.MustCompile(`\$[1256y]\$(?:[A-Za-z0-9./+=-]+\$){1,3}[A-Za-z0-9./+=-]+`)
	sensitivePathRegexps = []*regexp.Regexp{
		// The source identity segment under /home is replaced with
		// __USER_NNNN__ instead of deleting the whole path.
		regexp.MustCompile(`~\/[^\s]+`),
		// Public, globally standardized /etc path names are not personally
		// identifying. Their *contents* remain sensitive, but their names
		// are meaningful ML features. Do not replace these paths.
	}
)

func sanitizeRemoteDatasetRecord(record remoteDatasetRecord) remoteDatasetRecord {
	return sanitizeRemoteDatasetRecordWithIdentity(record, newDatasetIdentityMapper())
}

func sanitizeRemoteDatasetRecordWithIdentity(record remoteDatasetRecord, ids *datasetIdentityMapper) remoteDatasetRecord {
	record.CommandLine = sanitizeRemoteDatasetTextWithIdentity(record.CommandLine, ids)
	if len(record.Args) > 0 {
		record.Args = sanitizeRemoteDatasetArgsWithIdentity(record.Args, ids)
	}
	return record
}

func sanitizeRemoteDatasetRow(row remoteDatasetRow) remoteDatasetRow {
	// Called a second time by the feature extractor, after import.
	// Existing __USER_NNNN__ / __PROJECT_NNNN__ tokens are immutable.
	return sanitizeRemoteDatasetRowWithIdentity(row, newDatasetIdentityMapper())
}

func sanitizeRemoteDatasetRowWithIdentity(row remoteDatasetRow, ids *datasetIdentityMapper) remoteDatasetRow {
	row.CommandLine = sanitizeRemoteDatasetTextWithIdentity(row.CommandLine, ids)
	if len(row.Args) > 0 {
		row.Args = sanitizeRemoteDatasetArgsWithIdentity(row.Args, ids)
	}
	return row
}

// sanitizeRemoteDatasetArgs accounts for argv boundaries: a secret following
// "--token" or "--password" is private even when it is a separate argument.
// Preserve length and ordering so feature extraction can still inspect flags.
func sanitizeRemoteDatasetArgs(args []string) []string {
	return sanitizeRemoteDatasetArgsWithIdentity(args, newDatasetIdentityMapper())
}

func sanitizeRemoteDatasetArgsWithIdentity(args []string, ids *datasetIdentityMapper) []string {
	out := make([]string, len(args))
	redactNext := false
	for i, arg := range args {
		if redactNext {
			out[i] = "***"
			redactNext = false
			continue
		}
		out[i] = sanitizeRemoteDatasetTextWithIdentity(arg, ids)
		switch strings.ToLower(strings.TrimSpace(arg)) {
		case "--password", "--passwd", "--token", "--secret", "--apikey",
			"--api-key", "--access-key", "--access_key", "--authorization", "--cookie",
			"authorization: bearer", "authorization: basic",
			"authorization=bearer", "authorization=basic":
			redactNext = true
		}
	}
	return out
}

func sanitizeRemoteDatasetText(text string) string {
	return sanitizeRemoteDatasetTextWithIdentity(text, newDatasetIdentityMapper())
}

func sanitizeRemoteDatasetTextWithIdentity(text string, ids *datasetIdentityMapper) string {
	if text == "" {
		return text
	}
	out := ids.pseudonymizeDatasetIdentities(text)
	for _, re := range sensitiveValueRegexps {
		out = re.ReplaceAllStringFunc(out, func(match string) string {
			// RFC-defined loopback, unspecified, broadcast and documentation
			// addresses identify no deployment. Preserve them as stable ML
			// features while masking other IP literals (including LAN IPs).
			if isNonIdentifyingIPv4(match) {
				return match
			}
			// git@github.com is a public service account on a globally
			// recognized host. The owner/repository segments in its remote
			// path are pseudonymized separately.
			if strings.EqualFold(match, "git@github.com") ||
				strings.EqualFold(match, "git@gitlab.com") ||
				strings.EqualFold(match, "git@bitbucket.org") {
				return match
			}
			// "grep passwd /etc/passwd" has a public pathname after a
			// search keyword, not a password assignment. Keep these exact
			// path references, but never exempt key=value/CLI secrets.
			if isBarePublicEtcPathReference(match) {
				return match
			}
			if isNonSecretCommandArgument(match) {
				return match
			}
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
			case strings.Contains(lower, "authorization"):
				// Keep the canonical redacted header stable across a second
				// pass; do not strip the separator or leave bearer fragments.
				return "Authorization: ***"
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
				// Keep a recognized project directory with its pseudonym,
				// but do not preserve arbitrary home-private files.
				for _, prefix := range []string{"~/projects/__PROJECT_", "~/project/__PROJECT_",
					"~/workspace/__PROJECT_", "~/workspaces/__PROJECT_", "~/repos/__PROJECT_",
					"~/repo/__PROJECT_"} {
					if strings.HasPrefix(repl, prefix) {
						return repl
					}
				}
				return "~/***"
			case strings.HasPrefix(repl, "/etc/"):
				return "/etc/***"
			default:
				return "***"
			}
		})
	}
	out = unixCryptHashRegexp.ReplaceAllString(out, "***")
	return out
}

// Do not destroy the risk meaning of public passwd flags/standard root name.
// For example, passwd -l root is a security-administration command, not a
// string assignment to a private password value.
func isNonSecretCommandArgument(valueMatch string) bool {
	fields := strings.Fields(valueMatch)
	if len(fields) != 2 || strings.ContainsAny(fields[0], "=:") {
		return false
	}
	switch strings.ToLower(fields[0]) {
	case "passwd":
		return fields[1] == "root" || strings.HasPrefix(fields[1], "-") ||
			isIdentityPlaceholder(fields[1])
	case "pwd":
		return fields[1] == "-P" || fields[1] == "-L"
	}
	return false
}

// isNonIdentifyingIPv4 only preserves globally fixed or documentation-only
// literals. Private networks and real public addresses can identify users.
func isNonIdentifyingIPv4(address string) bool {
	return strings.HasPrefix(address, "127.") ||
		address == "0.0.0.0" || address == "255.255.255.255" ||
		strings.HasPrefix(address, "192.0.2.") ||
		strings.HasPrefix(address, "198.51.100.") ||
		strings.HasPrefix(address, "203.0.113.")
}

// A bare name after a word such as passwd or password is not a credential
// value when the next token is an internationally standardized /etc path.
// Narrowly scope this to unambiguous names, not arbitrary /etc filenames.
func isBarePublicEtcPathReference(valueMatch string) bool {
	fields := strings.Fields(valueMatch)
	if len(fields) != 2 || strings.HasPrefix(fields[0], "-") ||
		strings.ContainsAny(fields[0], "=:") {
		return false
	}
	switch fields[1] {
	case "/etc/passwd", "/etc/shadow", "/etc/sudoers", "/etc/group",
		"/etc/hosts", "/etc/fstab", "/etc/resolv.conf",
		"/etc/ssh/sshd_config":
		return true
	}
	return false
}

func inferRemoteDatasetLabelFromSource(source string) (string, string) {
	lower := strings.ToLower(strings.TrimSpace(source))
	if lower == "" {
		return "", ""
	}
	if strings.Contains(lower, "unsafe") {
		return "BLOCK", "source"
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
