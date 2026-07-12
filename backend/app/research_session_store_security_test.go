package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestResearchJSONRequestLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/research", func(c *gin.Context) {
		var payload map[string]any
		status, err := bindResearchJSON(c, &payload)
		if err != nil {
			c.Status(status)
			return
		}
		c.Status(http.StatusNoContent)
	})
	body := append([]byte(`{"value":"`), bytes.Repeat([]byte("x"), int(researchControlRequestMaxBytes))...)
	body = append(body, []byte(`"}`)...)
	req := httptest.NewRequest(http.MethodPost, "/research", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestResearchStoreIgnoresJunkEntriesWhenApplyingSessionCap(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	for i := 0; i < researchMaxPersistedSessions+32; i++ {
		if err := os.WriteFile(filepath.Join(base, fmt.Sprintf("junk-%04d", i)), nil, 0o600); err != nil {
			t.Fatalf("WriteFile(junk) error = %v", err)
		}
	}
	writeResearchSessionFixture(t, base, ResearchSession{ID: "rs_valid", Name: "valid"})
	store := newResearchSessionStore(base)
	sessions, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "rs_valid" {
		t.Fatalf("valid session was hidden by junk entries: %#v", sessions)
	}
}

func TestResearchStoreRegeneratesMissingArtifactDirectory(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	id := "rs_missing_artifacts"
	writeResearchSessionFixture(t, base, ResearchSession{ID: id, Name: "missing", ArtifactRefs: map[string]ResearchArtifactRef{
		"jsonl": {Format: "jsonl", Name: "events.jsonl", Path: "/tmp/ignored"},
	}})
	store := newResearchSessionStore(base)
	ref, _, err := store.ExportArtifact(id, "jsonl")
	if err != nil {
		t.Fatalf("ExportArtifact() did not regenerate missing directory: %v", err)
	}
	if ref.Name != "events.jsonl" {
		t.Fatalf("unexpected regenerated artifact: %+v", ref)
	}
}

func TestResearchStoreScavengesSafeDeleteTombstones(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	dir := writeResearchSessionFixture(t, base, ResearchSession{ID: "rs_old", Name: "old"})
	tombstone := filepath.Join(base, "deleted_fixture")
	if err := os.Rename(dir, tombstone); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	store := newResearchSessionStore(base)
	if _, err := store.List(); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if _, err := os.Stat(tombstone); !os.IsNotExist(err) {
		t.Fatalf("safe tombstone was not scavenged: %v", err)
	}
}

func TestResearchStoreCapsCreatedSessions(t *testing.T) {
	t.Parallel()
	store := newResearchSessionStore(t.TempDir())
	if err := store.ensureLoaded(); err != nil {
		t.Fatalf("ensureLoaded() error = %v", err)
	}
	store.mu.Lock()
	for i := 0; i < researchMaxPersistedSessions; i++ {
		id := fmt.Sprintf("rs_%04d", i)
		store.sessions[id] = &ResearchSession{ID: id}
	}
	store.mu.Unlock()
	if _, err := store.Create(researchCreateSessionRequest{Name: "overflow"}); err == nil {
		t.Fatal("Create() exceeded persisted session cap")
	}
}

func writeResearchSessionFixture(t *testing.T, base string, session ResearchSession) string {
	t.Helper()
	dir := filepath.Join(base, session.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	payload, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.json"), payload, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return dir
}

func TestResearchStoreRejectsPersistedTraversalAndSymlinkSession(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	writeResearchSessionFixture(t, base, ResearchSession{ID: "rs_mismatch", Name: "mismatch"})
	if err := os.Rename(filepath.Join(base, "rs_mismatch"), filepath.Join(base, "rs_directory")); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}

	out := filepath.Join(t.TempDir(), "outside-session.json")
	payload, _ := json.Marshal(ResearchSession{ID: "rs_link", Name: "linked"})
	if err := os.WriteFile(out, payload, 0o600); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}
	linkDir := filepath.Join(base, "rs_link")
	if err := os.MkdirAll(linkDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(link) error = %v", err)
	}
	if err := os.Symlink(out, filepath.Join(linkDir, "session.json")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	store := newResearchSessionStore(base)
	sessions, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("unsafe persisted sessions were loaded: %#v", sessions)
	}
}

func TestResearchStoreDoesNotTrustPersistedArtifactPath(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}
	id := "rs_safe"
	dir := filepath.Join(base, id)
	artifactDir := filepath.Join(dir, "artifacts")
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	inside := filepath.Join(artifactDir, "events.jsonl")
	if err := os.WriteFile(inside, []byte("inside"), 0o600); err != nil {
		t.Fatalf("WriteFile(inside) error = %v", err)
	}
	writeResearchSessionFixture(t, base, ResearchSession{ID: id, Name: "safe", ArtifactRefs: map[string]ResearchArtifactRef{
		"jsonl": {Format: "jsonl", Name: "events.jsonl", Path: outside, ContentType: "text/html", SHA256: "forged", Bytes: 999, CreatedAt: time.Now().UTC().Add(-72 * time.Hour)},
	}})

	store := newResearchSessionStore(base)
	ref, payload, err := store.ExportArtifact(id, "jsonl")
	if err != nil {
		t.Fatalf("ExportArtifact() error = %v", err)
	}
	if string(payload) != "inside" || ref.Path != inside {
		t.Fatalf("artifact escaped confinement: ref=%+v payload=%q", ref, payload)
	}
	if ref.ContentType != "application/x-ndjson; charset=utf-8" || ref.Bytes != int64(len(payload)) || ref.SHA256 != researchSHA256Hex(payload) {
		t.Fatalf("artifact metadata was not reconstructed: %+v", ref)
	}
	if err := store.CleanupRetention(1); err != nil {
		t.Fatalf("CleanupRetention() error = %v", err)
	}
	if _, err := os.Stat(inside); err != nil {
		t.Fatalf("persisted CreatedAt prematurely removed current artifact: %v", err)
	}
	old := time.Now().UTC().Add(-72 * time.Hour)
	if err := os.Chtimes(inside, old, old); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}
	if err := store.CleanupRetention(1); err != nil {
		t.Fatalf("CleanupRetention(old file) error = %v", err)
	}
	if _, err := os.Stat(inside); !os.IsNotExist(err) {
		t.Fatalf("old inside artifact was not removed: %v", err)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "outside" {
		t.Fatalf("outside artifact changed: %q, %v", data, err)
	}
}

func TestResearchStoreRejectsHardlinkedEventsAndSymlinkedArtifacts(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	store := newResearchSessionStore(base)
	session, err := store.Create(researchCreateSessionRequest{Name: "secure"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	outsideDir := t.TempDir()
	outsideEvents := filepath.Join(outsideDir, "events.jsonl")
	if err := os.WriteFile(outsideEvents, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(events) error = %v", err)
	}
	sessionDir := filepath.Join(base, session.ID)
	if err := os.Link(outsideEvents, filepath.Join(sessionDir, "events.jsonl")); err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if _, err := store.LoadEvents(session.ID); err == nil {
		t.Fatal("hardlinked events file was accepted")
	}
	if err := os.Remove(filepath.Join(sessionDir, "events.jsonl")); err != nil {
		t.Fatalf("Remove(hardlink) error = %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(sessionDir, "artifacts")); err != nil {
		t.Fatalf("Symlink(artifacts) error = %v", err)
	}
	if err := store.SaveSecurityEvaluation(session.ID, ResearchSecurityEvaluationReport{}); err == nil {
		t.Fatal("symlinked artifacts directory was accepted")
	}
	if data, err := os.ReadFile(outsideEvents); err != nil || string(data) != "{}\n" {
		t.Fatalf("outside event changed: %q, %v", data, err)
	}
}

func TestResearchStoreDeleteFailsClosedOnNestedArtifactDirectory(t *testing.T) {
	t.Parallel()
	store := newResearchSessionStore(t.TempDir())
	session, err := store.Create(researchCreateSessionRequest{Name: "nested"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	nested := filepath.Join(store.baseDir, session.ID, "artifacts", "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := store.Delete(session.ID); err == nil {
		t.Fatal("Delete() accepted unsafe nested artifact directory")
	}
	if _, err := store.Get(session.ID); err != nil {
		t.Fatalf("failed delete removed in-memory session: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.baseDir, session.ID, "session.json")); err != nil {
		t.Fatalf("failed delete partially removed session metadata: %v", err)
	}
}
