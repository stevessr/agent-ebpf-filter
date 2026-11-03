package app

import (
	"strings"
	"testing"
)

func TestSanitizeRemoteDatasetTextPreservesPublicEtcPathNames(t *testing.T) {
	// Globally documented system path names are not secret. They are also
	// crucial risk-model features, even when the *file contents* are secret.
	for _, command := range []string{
		"cat /etc/passwd",
		"cat /etc/shadow",
		"cat /etc/sudoers",
		"grep root /etc/group",
		"head -n 5 /etc/hosts",
		"cat /etc/resolv.conf",
		"cat /etc/fstab",
		"cat /etc/ssh/sshd_config",
		"ls /etc/sudoers.d /etc/shadow- /etc/passwd.bak",
		"grep passwd /etc/passwd",
		"grep password /etc/shadow",
		"passwd -l root",
		"passwd root",
		"pwd -P",
		"cat ../../etc/shadow",
		"echo 'root ALL=(ALL) NOPASSWD: ALL' >> /etc/sudoers",
	} {
		t.Run(command, func(t *testing.T) {
			if got := sanitizeRemoteDatasetText(command); got != command {
				t.Fatalf("redacted public pathname or shell syntax:\n got %q\nwant %q", got, command)
			}
		})
	}
}

func TestSanitizeRemoteDatasetTextMasksGenuineSensitiveValues(t *testing.T) {
	cases := []struct {
		input, mustContain string
		forbidden          []string
	}{
		{"cat /etc/shadow --token=not-for-logs", "--token=***", []string{"not-for-logs"}},
		{"password=correct-horse-battery", "password=***", []string{"correct-horse-battery"}},
		{"--token=\"private multi word value\"", "token=***", []string{"private multi word value"}},
		{"Authorization: Bearer eyJhbGciOi.some-token", "Authorization: ***", []string{"eyJhbGciOi", "some-token"}},
		{"Authorization: Basic dXNlcjpwYXNzd29yZA==", "Authorization: ***", []string{"dXNlcjpwYXNzd29yZA"}},
		{"Authorization: Bearer \"credential with spaces\"", "Authorization: ***", []string{"credential with spaces"}},
		{"cat /home/alice/.ssh/id_rsa", "/home/***/.ssh/id_rsa", []string{"alice"}},
		{"cat ~/.ssh/id_rsa", "~/***", []string{"id_rsa"}},
		{"printf 'root:$6$salt$hash:19000:0:99999:7:::'", "***", []string{"$6$salt$hash"}},
		{"ssh 192.168.42.13", "***", []string{"192.168.42.13"}},
		{"curl http://alice:pass123@example.net/a", "http://***@", []string{"alice:pass123"}},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := sanitizeRemoteDatasetText(tc.input)
			if !strings.Contains(got, tc.mustContain) {
				t.Errorf("sanitized %q: missing %q (got %q)", tc.input, tc.mustContain, got)
			}
			for _, secret := range tc.forbidden {
				if strings.Contains(got, secret) {
					t.Errorf("secret %q leaked in sanitized %q", secret, got)
				}
			}
		})
	}
}

func TestSanitizeRemoteDatasetArgsMasksSplitSecrets(t *testing.T) {
	args := []string{"-H", "Authorization: Bearer", "raw-auth-credential", "--token",
		"raw-api-credential", "--password", "raw-password", "/etc/shadow"}
	got := sanitizeRemoteDatasetArgs(args)
	if len(got) != len(args) {
		t.Fatalf("sanitized argv changed arity: %d != %d", len(got), len(args))
	}
	if got[2] != "***" || got[4] != "***" || got[6] != "***" {
		t.Fatalf("split argv secret remained visible: %q", got)
	}
	if got[7] != "/etc/shadow" {
		t.Fatalf("public sensitive-target name was lost: %q", got)
	}
	// Repeat cleaning should not restore data or mutate argument boundaries.
	again := sanitizeRemoteDatasetArgs(got)
	if strings.Join(got, "|") != strings.Join(again, "|") {
		t.Fatalf("split argv sanitizer is not idempotent: %q != %q", got, again)
	}
}

func TestSanitizeRemoteDatasetRecordMasksSplitCredentials(t *testing.T) {
	record := sanitizeRemoteDatasetRecord(remoteDatasetRecord{
		CommandLine: "cat /etc/shadow --token real-raw-token",
		Comm:        "cat",
		Args:        []string{"/etc/shadow", "--token", "real-raw-token"},
	})
	if record.Args[0] != "/etc/shadow" || record.Args[2] != "***" {
		t.Fatalf("record cleaning lost target or leaked token: %+v", record)
	}
	if strings.Contains(record.CommandLine, "real-raw-token") {
		t.Fatalf("command-line cleaning leaked token: %+v", record)
	}
}

func TestSanitizeRemoteDatasetTextPreservesPublicReferenceIPs(t *testing.T) {
	input := "curl 127.0.0.1 0.0.0.0 255.255.255.255 192.0.2.24 198.51.100.5 203.0.113.9 10.2.3.4 8.8.8.8"
	got := sanitizeRemoteDatasetText(input)
	for _, ip := range []string{"127.0.0.1", "0.0.0.0", "255.255.255.255", "192.0.2.24", "198.51.100.5", "203.0.113.9"} {
		if !strings.Contains(got, ip) {
			t.Errorf("public non-identifying IPv4 %q was redacted: %q", ip, got)
		}
	}
	for _, ip := range []string{"10.2.3.4", "8.8.8.8"} {
		if strings.Contains(got, ip) {
			t.Errorf("deployment IP %q was not redacted: %q", ip, got)
		}
	}
}

func TestSanitizeRemoteDatasetKeepsPathIdentityAcrossRowAndSampleCleaning(t *testing.T) {
	row := sanitizeRemoteDatasetRow(remoteDatasetRow{
		CommandLine: "cat /etc/shadow --token=private-token",
		Comm:        "cat",
		Args:        []string{"/etc/shadow", "--token=private-token"},
	})
	if !strings.Contains(row.CommandLine, "/etc/shadow") || row.Args[0] != "/etc/shadow" {
		t.Fatalf("cleaning must keep the globally documented path: %+v", row)
	}
	if strings.Contains(row.CommandLine, "private-token") || strings.Contains(strings.Join(row.Args, " "), "private-token") {
		t.Fatalf("secret leaked into cleaned row: %+v", row)
	}
	// Feature extraction cleans each parsed row a second time. Keep the
	// transformation stable so path features survive both sanitizations.
	twice := sanitizeRemoteDatasetRow(row)
	if row.CommandLine != twice.CommandLine || strings.Join(row.Args, "|") != strings.Join(twice.Args, "|") {
		t.Fatalf("sanitization is not idempotent: first=%+v second=%+v", row, twice)
	}
}
