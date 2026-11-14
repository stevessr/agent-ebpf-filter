package app

import (
    "strings"
    "testing"
)

func TestDatasetIdentityMapperPreservesStableReferences(t *testing.T) {
    ids := newDatasetIdentityMapper()
    cases := []struct {input, want string}{
        {"/home/alice/projects/secret-app/src", "/home/__USER_0001__/projects/__PROJECT_0001__/src"},
        {"/home/alice/projects/secret-app/test", "/home/__USER_0001__/projects/__PROJECT_0001__/test"},
        {"/home/bob/projects/another-app", "/home/__USER_0002__/projects/__PROJECT_0002__"},
        {"USER=alice PROJECT_NAME=secret-app", "USER=__USER_0001__ PROJECT_NAME=__PROJECT_0001__"},
        {"sudo -u alice --project=secret-app", "sudo -u __USER_0001__ --project=__PROJECT_0001__"},
        {"passwd alice", "passwd __USER_0001__"},
        {"passwd -l root", "passwd -l root"},
        {"git clone git@github.com:alice/secret-app.git", "git clone git@github.com:__USER_0001__/__PROJECT_0001__.git"},
        {"git clone https://github.com/bob/another-app.git", "git clone https://github.com/__USER_0002__/__PROJECT_0002__.git"},
        {"git clone git@bitbucket.org:alice/secret-app.git", "git clone git@bitbucket.org:__USER_0001__/__PROJECT_0001__.git"},
        {"curl --user alice:secret-value", "curl --user ***"},
        {"cat /etc/shadow", "cat /etc/shadow"},
        {"cd /tmp/unclassified-name", "cd /tmp/unclassified-name"},
        {"--project=\"secret project\"", "--project=\"__PROJECT_0003__\""},
        {"alice@example.com", "__USER_0001__@***"},
    }
    for _, tc := range cases {
        got := sanitizeRemoteDatasetTextWithIdentity(tc.input, ids)
        if got != tc.want {
            t.Errorf("identity sanitization of %q: got %q, want %q", tc.input, got, tc.want)
        }
        if again := sanitizeRemoteDatasetTextWithIdentity(got, ids); again != got {
            t.Errorf("not idempotent: initial %q, second %q", got, again)
        }
    }
}

func TestDatasetIdentityMapperWindowsPaths(t *testing.T) {
    ids := newDatasetIdentityMapper()
    got := sanitizeRemoteDatasetTextWithIdentity("C:\\Users\\alice\\source\\repos\\secret-app\\main.go", ids)
    want := "C:\\Users\\__USER_0001__\\source\\repos\\__PROJECT_0001__\\main.go"
    if got != want {
        t.Fatalf("windows path sanitization got %q want %q", got, want)
    }
}

func TestDatasetIdentityMapperConsistentAcrossImportedRows(t *testing.T) {
    content := strings.Join([]string{
        "{\"command\":\"cat /home/alice/projects/secret-app/README.md\",\"label\":\"ALLOW\"}",
        "{\"command\":\"cat /home/alice/projects/secret-app/main.go\",\"label\":\"ALLOW\"}",
        "{\"command\":\"ls /home/bob/projects/another-app\",\"label\":\"ALLOW\"}",
    }, "\n") + "\n"
    resp, err := pullRemoteDataset(remoteDatasetRequest{
        Content: content, SourceName: "pseudonym-fixture", Format: "jsonl",
        LabelMode: "preserve", ImportAll: true, CleanSensitive: true,
    })
    if err != nil {
        t.Fatal(err)
    }
    if len(resp.Rows) != 3 {
        t.Fatalf("unexpected row count %d", len(resp.Rows))
    }
    for i, row := range resp.Rows {
        if strings.Contains(row.CommandLine, "alice") || strings.Contains(row.CommandLine, "bob") ||
            strings.Contains(row.CommandLine, "secret-app") || strings.Contains(row.CommandLine, "another-app") {
            t.Errorf("private identifier leaked in row %d: %+v", i, row)
        }
    }
    for _, row := range resp.Rows[:2] {
        if !strings.Contains(row.CommandLine, "/home/__USER_0001__/projects/__PROJECT_0001__/") {
            t.Errorf("first user/project ID changed between rows: %+v", row)
        }
        twice := sanitizeRemoteDatasetRow(row)
        if twice.CommandLine != row.CommandLine || strings.Join(twice.Args, "|") != strings.Join(row.Args, "|") {
            t.Errorf("double sanitization changed identity references: first=%+v twice=%+v", row, twice)
        }
    }
    if !strings.Contains(resp.Rows[2].CommandLine, "/home/__USER_0002__/projects/__PROJECT_0002__") {
        t.Errorf("different user/project collapsed into first identity: %+v", resp.Rows[2])
    }
}

func TestDatasetIdentityMapperIndependentImportsAreNotCrossLinked(t *testing.T) {
    first := sanitizeRemoteDatasetTextWithIdentity("/home/alice/projects/private-a", newDatasetIdentityMapper())
    second := sanitizeRemoteDatasetTextWithIdentity("/home/bob/projects/private-b", newDatasetIdentityMapper())
    if first != second {
        t.Fatalf("two isolated source-local identity maps must not encode original names: %q != %q", first, second)
    }
}

func TestDatasetIdentityMapperArgvSharesOneIdentityMap(t *testing.T) {
    ids := newDatasetIdentityMapper()
    cleaned := sanitizeRemoteDatasetArgsWithIdentity([]string{
        "/home/alice/projects/secret-app", "--project", "secret-app",
        "--user", "alice", "--token", "private-secret", "/etc/sudoers",
    }, ids)
    if strings.Join(cleaned, "|") != "/home/__USER_0001__/projects/__PROJECT_0001__|--project|__PROJECT_0001__|--user|__USER_0001__|--token|***|/etc/sudoers" {
        t.Fatalf("argv identity and secret handling failed: %q", cleaned)
    }
}
func TestDatasetIdentityMapperSplitArgvUsesCommandContext(t *testing.T) {
    ids := newDatasetIdentityMapper()
    row := sanitizeRemoteDatasetRowWithIdentity(remoteDatasetRow{
        CommandLine: "sudo -u alice cat /etc/passwd",
        Args: []string{"-u", "alice", "cat", "/etc/passwd"},
    }, ids)
    if row.Args[1] != "__USER_0001__" || strings.Contains(strings.Join(row.Args, " "), "alice") {
        t.Fatalf("split command and argv disagree: %+v", row)
    }
    unknown := sanitizeRemoteDatasetArgsWithCommand([]string{"-u", "plain-file"}, "head", newDatasetIdentityMapper())
    if unknown[1] != "plain-file" {
        t.Fatalf("ambiguous -u was incorrectly treated as username: %q", unknown)
    }
}

func TestDatasetIdentityMapperDoesNotFlattenStructuredFlagPaths(t *testing.T) {
    ids := newDatasetIdentityMapper()
    value := sanitizeRemoteDatasetTextWithIdentity("--project=/home/alice/projects/private-app", ids)
    if value != "--project=/home/__USER_0001__/projects/__PROJECT_0001__" {
        t.Fatalf("structured --project path got flattened: %q", value)
    }
    args := sanitizeRemoteDatasetArgsWithCommand([]string{"--project", "/home/alice/projects/private-app"}, "tool", ids)
    if args[1] != "/home/__USER_0001__/projects/__PROJECT_0001__" {
        t.Fatalf("split structured project argument got flattened: %q", args)
    }
}

func TestDatasetIdentityMapperRecognizesNamedTildeHome(t *testing.T) {
    actual := sanitizeRemoteDatasetText("cat ~alice/.ssh/config")
    if actual != "cat ~__USER_0001__/.ssh/config" {
        t.Fatalf("named shell home should be replaced, not removed: %q", actual)
    }
}
