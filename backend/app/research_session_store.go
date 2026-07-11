package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *researchSessionStore) ensureLoaded() error {
	if s == nil {
		return errors.New("research session store is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return nil
	}
	if err := os.MkdirAll(s.baseDir, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(s.baseDir, entry.Name(), "session.json")
		payload, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var session ResearchSession
		if err := json.Unmarshal(payload, &session); err != nil || strings.TrimSpace(session.ID) == "" {
			continue
		}
		normalizeResearchSession(&session)
		s.sessions[session.ID] = &session
	}
	s.loaded = true
	return nil
}

func (s *researchSessionStore) List() ([]ResearchSession, error) {
	if err := s.ensureLoaded(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	out := make([]ResearchSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		out = append(out, cloneResearchSession(session))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (s *researchSessionStore) Create(req researchCreateSessionRequest) (ResearchSession, error) {
	if err := s.ensureLoaded(); err != nil {
		return ResearchSession{}, err
	}
	now := time.Now().UTC()
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Research Session " + now.Format("2006-01-02 15:04:05")
	}
	session := ResearchSession{
		ID:           researchGenerateID("rs"),
		Name:         name,
		Description:  strings.TrimSpace(req.Description),
		Tags:         normalizeResearchTags(req.Tags),
		CreatedAt:    now,
		UpdatedAt:    now,
		SourceFilter: normalizeResearchSourceFilter(req.SourceFilter),
		TimeRange:    normalizeResearchTimeRange(req.TimeRange),
		Status:       researchSessionEmpty,
		Summary:      ResearchSessionSummary{SchemaVersion: researchSchemaVersion},
		ArtifactRefs: map[string]ResearchArtifactRef{},
	}
	if err := s.saveSessionLocked(&session); err != nil {
		return ResearchSession{}, err
	}
	s.mu.Lock()
	s.sessions[session.ID] = &session
	s.mu.Unlock()
	return cloneResearchSession(&session), nil
}

func (s *researchSessionStore) Get(id string) (ResearchSession, error) {
	if err := s.ensureLoaded(); err != nil {
		return ResearchSession{}, err
	}
	id = strings.TrimSpace(id)
	s.mu.RLock()
	session := s.sessions[id]
	s.mu.RUnlock()
	if session == nil {
		return ResearchSession{}, os.ErrNotExist
	}
	return cloneResearchSession(session), nil
}

func (s *researchSessionStore) Delete(id string) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	s.mu.Lock()
	if s.sessions[id] == nil {
		s.mu.Unlock()
		return os.ErrNotExist
	}
	delete(s.sessions, id)
	s.mu.Unlock()
	return os.RemoveAll(filepath.Join(s.baseDir, id))
}

func (s *researchSessionStore) ReplaceSessionEvents(id string, events []ResearchEvent, results ResearchResults, filter ResearchSourceFilter, timerange ResearchTimeRange) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	now := time.Now().UTC()
	s.mu.Lock()
	session := s.sessions[id]
	if session == nil {
		s.mu.Unlock()
		return os.ErrNotExist
	}
	session.Status = researchSessionBuilding
	session.UpdatedAt = now
	session.SourceFilter = normalizeResearchSourceFilter(filter)
	session.TimeRange = normalizeResearchTimeRange(timerange)
	session.LastError = ""
	if err := s.saveSessionLocked(session); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	if err := s.writeEvents(id, events); err != nil {
		s.markSessionError(id, err)
		return err
	}
	if err := s.writeResults(id, results); err != nil {
		s.markSessionError(id, err)
		return err
	}

	s.mu.Lock()
	session = s.sessions[id]
	if session != nil {
		session.Status = researchSessionReady
		if len(events) == 0 {
			session.Status = researchSessionEmpty
		}
		session.UpdatedAt = time.Now().UTC()
		session.Summary = buildResearchSessionSummary(events, results)
		session.ArtifactRefs = map[string]ResearchArtifactRef{}
		if err := s.saveSessionLocked(session); err != nil {
			s.mu.Unlock()
			return err
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *researchSessionStore) SaveResults(id string, results ResearchResults) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	if err := s.writeResults(id, results); err != nil {
		return err
	}
	events, _ := s.LoadEvents(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[id]
	if session == nil {
		return os.ErrNotExist
	}
	session.Summary = buildResearchSessionSummary(events, results)
	session.UpdatedAt = time.Now().UTC()
	session.Status = researchSessionReady
	return s.saveSessionLocked(session)
}

func (s *researchSessionStore) SaveSecurityEvaluation(id string, report ResearchSecurityEvaluationReport) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	if _, err := s.Get(id); err != nil {
		return err
	}
	results, err := s.LoadResults(id)
	if err != nil {
		return err
	}
	report.SessionID = id
	if strings.TrimSpace(report.SchemaVersion) == "" {
		report.SchemaVersion = researchSecurityEvaluationSchemaVersion
	}
	if report.GeneratedAt.IsZero() {
		report.GeneratedAt = time.Now().UTC()
	}
	results.SecurityEvaluation = &report
	if err := s.writeResults(id, results); err != nil {
		return err
	}
	payload, err := researchSecurityEvaluationJSONBytes(&report)
	if err != nil {
		return err
	}
	artifactDir := filepath.Join(s.baseDir, id, "artifacts")
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(artifactDir, "security-evaluation.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return err
	}
	ref := ResearchArtifactRef{Format: "security_json", Name: "security-evaluation.json", Path: path, ContentType: "application/json; charset=utf-8", Bytes: int64(len(payload)), SHA256: researchSHA256Hex(payload), CreatedAt: time.Now().UTC()}
	events, _ := s.LoadEvents(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[id]
	if session == nil {
		return os.ErrNotExist
	}
	if session.ArtifactRefs == nil {
		session.ArtifactRefs = map[string]ResearchArtifactRef{}
	}
	session.ArtifactRefs["security_json"] = ref
	session.Summary = buildResearchSessionSummary(events, results)
	session.UpdatedAt = time.Now().UTC()
	session.Status = researchSessionReady
	return s.saveSessionLocked(session)
}

func (s *researchSessionStore) ResetSession(id string) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	dir := filepath.Join(s.baseDir, id)
	_ = os.Remove(filepath.Join(dir, "events.jsonl"))
	_ = os.Remove(filepath.Join(dir, "results.json"))
	_ = os.RemoveAll(filepath.Join(dir, "artifacts"))
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[id]
	if session == nil {
		return os.ErrNotExist
	}
	session.UpdatedAt = time.Now().UTC()
	session.Status = researchSessionEmpty
	session.Summary = ResearchSessionSummary{SchemaVersion: researchSchemaVersion}
	session.ArtifactRefs = map[string]ResearchArtifactRef{}
	session.LastError = ""
	return s.saveSessionLocked(session)
}

func (s *researchSessionStore) LoadEvents(id string) ([]ResearchEvent, error) {
	if err := s.ensureLoaded(); err != nil {
		return nil, err
	}
	if _, err := s.Get(id); err != nil {
		return nil, err
	}
	path := filepath.Join(s.baseDir, id, "events.jsonl")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []ResearchEvent{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	events := make([]ResearchEvent, 0)
	for {
		var event ResearchEvent
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *researchSessionStore) LoadResults(id string) (ResearchResults, error) {
	if err := s.ensureLoaded(); err != nil {
		return ResearchResults{}, err
	}
	if _, err := s.Get(id); err != nil {
		return ResearchResults{}, err
	}
	path := filepath.Join(s.baseDir, id, "results.json")
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		events, _ := s.LoadEvents(id)
		return buildResearchResults(id, events, nil), nil
	}
	if err != nil {
		return ResearchResults{}, err
	}
	var results ResearchResults
	if err := json.Unmarshal(payload, &results); err != nil {
		return ResearchResults{}, err
	}
	return results, nil
}

func (s *researchSessionStore) GenerateExports(id string, formats []string) ([]ResearchArtifactRef, error) {
	return s.GenerateExportsWithCancel(id, formats, nil)
}

func (s *researchSessionStore) GenerateExportsWithCancel(id string, formats []string, entry *researchTaskEntry) ([]ResearchArtifactRef, error) {
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	formats = normalizeResearchFormats(formats)
	if len(formats) == 0 {
		formats = splitResearchFormats(settings.ExportFormats)
	}
	if err := entry.checkCanceled(); err != nil {
		return nil, err
	}
	session, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	events, err := s.LoadEvents(id)
	if err != nil {
		return nil, err
	}
	if err := entry.checkCanceled(); err != nil {
		return nil, err
	}
	results, err := s.LoadResults(id)
	if err != nil {
		return nil, err
	}
	if err := entry.checkCanceled(); err != nil {
		return nil, err
	}
	artifactDir := filepath.Join(s.baseDir, id, "artifacts")
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		return nil, err
	}
	refs := make([]ResearchArtifactRef, 0, len(formats))
	artifactMap := cloneArtifactRefs(session.ArtifactRefs)
	payloadsByName := map[string][]byte{}
	for _, format := range formats {
		if err := entry.checkCanceled(); err != nil {
			return nil, err
		}
		var payload []byte
		var contentType string
		var name string
		switch format {
		case "jsonl":
			payload = researchEventsJSONLBytes(events)
			contentType = "application/x-ndjson; charset=utf-8"
			name = "events.jsonl"
		case "csv":
			payload, err = researchEventsCSVBytes(events)
			if err != nil {
				return nil, err
			}
			contentType = "text/csv; charset=utf-8"
			name = "events.csv"
		case "json":
			payload, err = json.MarshalIndent(gin.H{"session": session, "results": results, "events": events}, "", "  ")
			if err != nil {
				return nil, err
			}
			contentType = "application/json; charset=utf-8"
			name = "research.json"
		case "security_json":
			payload, err = researchSecurityEvaluationJSONBytes(results.SecurityEvaluation)
			if err != nil {
				return nil, err
			}
			contentType = "application/json; charset=utf-8"
			name = "security-evaluation.json"
		case "security_jsonl":
			if results.SecurityEvaluation == nil {
				return nil, errors.New("security evaluation report is unavailable")
			}
			payload = researchSecurityEvaluationJSONLBytes(results.SecurityEvaluation)
			contentType = "application/x-ndjson; charset=utf-8"
			name = "security-evaluation.jsonl"
		case "security_csv":
			payload, err = researchSecurityEvaluationCSVBytes(results.SecurityEvaluation)
			if err != nil {
				return nil, err
			}
			contentType = "text/csv; charset=utf-8"
			name = "security-evaluation.csv"
		case "bundle":
			payload, err = researchBundleZipBytes(session, events, results, settings)
			if err != nil {
				return nil, err
			}
			contentType = "application/zip"
			name = "research-bundle.zip"
		default:
			return nil, fmt.Errorf("unsupported export format %q", format)
		}
		if err := entry.checkCanceled(); err != nil {
			return nil, err
		}
		path := filepath.Join(artifactDir, name)
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			return nil, err
		}
		payloadsByName[name] = payload
		ref := ResearchArtifactRef{Format: format, Name: name, Path: path, ContentType: contentType, Bytes: int64(len(payload)), SHA256: researchSHA256Hex(payload), CreatedAt: time.Now().UTC()}
		artifactMap[format] = ref
		refs = append(refs, ref)
	}
	if len(payloadsByName) > 0 {
		if err := entry.checkCanceled(); err != nil {
			return nil, err
		}
		manifest := researchBuildManifest(session, events, settings, payloadsByName, artifactMap)
		payload, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return nil, err
		}
		path := filepath.Join(artifactDir, "manifest.json")
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			return nil, err
		}
		ref := ResearchArtifactRef{Format: "manifest", Name: "manifest.json", Path: path, ContentType: "application/json; charset=utf-8", Bytes: int64(len(payload)), SHA256: researchSHA256Hex(payload), CreatedAt: time.Now().UTC()}
		artifactMap["manifest"] = ref
		refs = append(refs, ref)
	}
	if err := entry.checkCanceled(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if current := s.sessions[id]; current != nil {
		current.ArtifactRefs = artifactMap
		current.UpdatedAt = time.Now().UTC()
		if err := s.saveSessionLocked(current); err != nil {
			s.mu.Unlock()
			return nil, err
		}
	}
	s.mu.Unlock()
	return refs, nil
}

func (s *researchSessionStore) ExportArtifact(id, format string) (ResearchArtifactRef, []byte, error) {
	format = normalizeResearchFormat(format)
	if format == "" {
		format = "bundle"
	}
	if _, err := s.Get(id); err != nil {
		return ResearchArtifactRef{}, nil, err
	}
	session, _ := s.Get(id)
	ref, ok := session.ArtifactRefs[format]
	if !ok || strings.TrimSpace(ref.Path) == "" {
		refs, err := s.GenerateExports(id, []string{format})
		if err != nil {
			return ResearchArtifactRef{}, nil, err
		}
		if len(refs) == 0 {
			return ResearchArtifactRef{}, nil, os.ErrNotExist
		}
		ref = refs[0]
	}
	payload, err := os.ReadFile(ref.Path)
	if errors.Is(err, os.ErrNotExist) {
		refs, genErr := s.GenerateExports(id, []string{format})
		if genErr != nil {
			return ResearchArtifactRef{}, nil, genErr
		}
		ref = refs[0]
		payload, err = os.ReadFile(ref.Path)
	}
	if err != nil {
		return ResearchArtifactRef{}, nil, err
	}
	return ref, payload, nil
}

func (s *researchSessionStore) CleanupRetention(days int) error {
	if s == nil {
		return nil
	}
	if days <= 0 {
		days = researchProcessingDefaultArtifactRetentionDays
	}
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	sessions, err := s.List()
	if err != nil {
		return err
	}
	for _, session := range sessions {
		changed := false
		refs := cloneArtifactRefs(session.ArtifactRefs)
		for format, ref := range refs {
			if ref.CreatedAt.IsZero() || ref.CreatedAt.After(cutoff) {
				continue
			}
			if strings.TrimSpace(ref.Path) != "" {
				_ = os.Remove(ref.Path)
			}
			delete(refs, format)
			changed = true
		}
		if changed {
			s.mu.Lock()
			if current := s.sessions[session.ID]; current != nil {
				current.ArtifactRefs = refs
				current.UpdatedAt = time.Now().UTC()
				_ = s.saveSessionLocked(current)
			}
			s.mu.Unlock()
		}
	}
	return nil
}

func (s *researchSessionStore) writeEvents(id string, events []ResearchEvent) error {
	dir := filepath.Join(s.baseDir, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "events.jsonl")
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return err
		}
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

func (s *researchSessionStore) writeResults(id string, results ResearchResults) error {
	payload, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(s.baseDir, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "results.json"), payload, 0o600)
}

func (s *researchSessionStore) saveSessionLocked(session *ResearchSession) error {
	if s == nil || session == nil {
		return errors.New("research session is nil")
	}
	normalizeResearchSession(session)
	dir := filepath.Join(s.baseDir, session.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "session.json"), payload, 0o600)
}

func (s *researchSessionStore) markSessionError(id string, err error) {
	if s == nil || err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if session := s.sessions[id]; session != nil {
		session.Status = researchSessionError
		session.LastError = err.Error()
		session.UpdatedAt = time.Now().UTC()
		_ = s.saveSessionLocked(session)
	}
}
