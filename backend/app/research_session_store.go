package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sys/unix"
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
	root, err := openResearchRoot(s.baseDir)
	if err != nil {
		return err
	}
	defer root.Close()
	names, err := root.Readdirnames(researchMaxRootScanEntries)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		if strings.HasPrefix(name, "deleted_") {
			if err := s.cleanupResearchTombstone(root, name); err != nil {
				log.Printf("[WARN] failed to clean research tombstone %s: %v", name, err)
			}
			continue
		}
		if len(s.sessions) >= researchMaxPersistedSessions {
			break
		}
		if _, err := validateResearchFileComponent(name, "session id", false); err != nil {
			continue
		}
		sessionDir, err := openResearchDirChild(root, name, false)
		if err != nil {
			continue
		}
		payload, err := readResearchFile(sessionDir, "session.json", researchSessionFileMaxBytes)
		_ = sessionDir.Close()
		if err != nil {
			continue
		}
		var session ResearchSession
		if err := json.Unmarshal(payload, &session); err != nil || strings.TrimSpace(session.ID) != name {
			continue
		}
		normalizeResearchSession(&session)
		for format, ref := range session.ArtifactRefs {
			expected, ok := researchArtifactFilename(format)
			if !ok || ref.Name != expected {
				delete(session.ArtifactRefs, format)
				continue
			}
			ref.Path = researchDisplayArtifactPath(s.baseDir, session.ID, expected)
			ref.ContentType = researchArtifactContentType(format)
			ref.Bytes = 0
			ref.SHA256 = ""
			ref.CreatedAt = time.Time{}
			session.ArtifactRefs[format] = ref
		}
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
	s.fsMu.Lock()
	defer s.fsMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) >= researchMaxPersistedSessions {
		return ResearchSession{}, fmt.Errorf("research session limit %d reached", researchMaxPersistedSessions)
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
	s.sessions[session.ID] = &session
	return cloneResearchSession(&session), nil
}

func (s *researchSessionStore) Get(id string) (ResearchSession, error) {
	if err := s.ensureLoaded(); err != nil {
		return ResearchSession{}, err
	}
	id = strings.TrimSpace(id)
	if _, err := validateResearchFileComponent(id, "session id", false); err != nil {
		return ResearchSession{}, os.ErrNotExist
	}
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
	s.fsMu.Lock()
	defer s.fsMu.Unlock()
	id = strings.TrimSpace(id)
	if _, err := validateResearchFileComponent(id, "session id", false); err != nil {
		return os.ErrNotExist
	}
	s.mu.RLock()
	if s.sessions[id] == nil {
		s.mu.RUnlock()
		return os.ErrNotExist
	}
	s.mu.RUnlock()
	root, sessionDir, err := openResearchSession(s.baseDir, id, false)
	if err != nil {
		return err
	}
	defer root.Close()
	defer sessionDir.Close()
	if err := s.preflightResearchSession(sessionDir); err != nil {
		return err
	}
	tombstone := researchGenerateID("deleted")
	if err := unix.Renameat2(int(root.Fd()), id, int(root.Fd()), tombstone, unix.RENAME_NOREPLACE); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
	if err := unix.Fsync(int(root.Fd())); err != nil {
		log.Printf("[WARN] research session %s deleted but root sync failed: %v", id, err)
	}
	if err := s.clearResearchArtifacts(sessionDir); err != nil {
		log.Printf("[WARN] research session %s deleted but tombstone cleanup failed: %v", id, err)
		return nil
	}
	for _, name := range []string{"events.jsonl", "results.json", "session.json"} {
		if err := removeResearchFile(sessionDir, name); err != nil {
			log.Printf("[WARN] research session %s deleted but tombstone cleanup failed: %v", id, err)
			return nil
		}
	}
	if err := sessionDir.Close(); err != nil {
		return err
	}
	if err := removeResearchDirectory(root, tombstone); err != nil {
		log.Printf("[WARN] research session %s deleted but tombstone cleanup failed: %v", id, err)
		return nil
	}
	if err := unix.Fsync(int(root.Fd())); err != nil {
		log.Printf("[WARN] research session %s tombstone removed but root sync failed: %v", id, err)
	}
	return nil
}

func (s *researchSessionStore) ReplaceSessionEvents(id string, events []ResearchEvent, results ResearchResults, filter ResearchSourceFilter, timerange ResearchTimeRange) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	s.fsMu.Lock()
	defer s.fsMu.Unlock()
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
	s.fsMu.Lock()
	defer s.fsMu.Unlock()
	if _, err := s.Get(id); err != nil {
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
	s.fsMu.Lock()
	defer s.fsMu.Unlock()
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
	root, sessionDir, err := openResearchSession(s.baseDir, id, false)
	if err != nil {
		return err
	}
	defer root.Close()
	defer sessionDir.Close()
	artifactDir, err := openResearchArtifacts(sessionDir, true)
	if err != nil {
		return err
	}
	defer artifactDir.Close()
	name := "security-evaluation.json"
	if err := atomicWriteResearchFile(artifactDir, name, payload, researchArtifactMaxBytes); err != nil {
		return err
	}
	path := researchDisplayArtifactPath(s.baseDir, id, name)
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
	s.fsMu.Lock()
	defer s.fsMu.Unlock()
	if _, err := s.Get(id); err != nil {
		return err
	}
	root, sessionDir, err := openResearchSession(s.baseDir, id, false)
	if err != nil {
		return err
	}
	defer root.Close()
	defer sessionDir.Close()
	if err := s.preflightResearchSession(sessionDir); err != nil {
		return err
	}
	if err := s.clearResearchArtifacts(sessionDir); err != nil {
		return err
	}
	for _, name := range []string{"events.jsonl", "results.json"} {
		if err := removeResearchFile(sessionDir, name); err != nil {
			return err
		}
	}
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
	root, sessionDir, err := openResearchSession(s.baseDir, id, false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	defer sessionDir.Close()
	payload, err := readResearchFile(sessionDir, "events.jsonl", researchEventsFileMaxBytes)
	if errors.Is(err, os.ErrNotExist) {
		return []ResearchEvent{}, nil
	}
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	events := make([]ResearchEvent, 0)
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	for {
		var event ResearchEvent
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		events = append(events, event)
		if len(events) > settings.MaxSessionEvents {
			return nil, fmt.Errorf("research event log exceeds %d events", settings.MaxSessionEvents)
		}
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
	root, sessionDir, err := openResearchSession(s.baseDir, id, false)
	if err != nil {
		return ResearchResults{}, err
	}
	defer root.Close()
	defer sessionDir.Close()
	payload, err := readResearchFile(sessionDir, "results.json", researchResultsFileMaxBytes)
	if errors.Is(err, os.ErrNotExist) {
		events, loadErr := s.LoadEvents(id)
		if loadErr != nil {
			return ResearchResults{}, loadErr
		}
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
	s.fsMu.Lock()
	defer s.fsMu.Unlock()
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
	root, sessionDir, err := openResearchSession(s.baseDir, id, false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	defer sessionDir.Close()
	artifactDir, err := openResearchArtifacts(sessionDir, true)
	if err != nil {
		return nil, err
	}
	defer artifactDir.Close()
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
		if int64(len(payload)) > researchArtifactMaxBytes {
			return nil, fmt.Errorf("research artifact %s exceeds size limit", name)
		}
		if err := atomicWriteResearchFile(artifactDir, name, payload, researchArtifactMaxBytes); err != nil {
			return nil, err
		}
		path := researchDisplayArtifactPath(s.baseDir, id, name)
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
		name := "manifest.json"
		if err := atomicWriteResearchFile(artifactDir, name, payload, researchArtifactMaxBytes); err != nil {
			return nil, err
		}
		path := researchDisplayArtifactPath(s.baseDir, id, name)
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
	name, ok := researchArtifactFilename(format)
	if !ok || ref.Name != name {
		return ResearchArtifactRef{}, nil, fmt.Errorf("invalid persisted research artifact reference")
	}
	root, sessionDir, err := openResearchSession(s.baseDir, id, false)
	if err != nil {
		return ResearchArtifactRef{}, nil, err
	}
	defer root.Close()
	defer sessionDir.Close()
	artifactDir, err := openResearchArtifacts(sessionDir, false)
	if errors.Is(err, unix.ENOENT) {
		refs, genErr := s.GenerateExports(id, []string{format})
		if genErr != nil {
			return ResearchArtifactRef{}, nil, genErr
		}
		if len(refs) == 0 {
			return ResearchArtifactRef{}, nil, os.ErrNotExist
		}
		ref = refs[0]
		artifactDir, err = openResearchArtifacts(sessionDir, false)
	}
	if err != nil {
		return ResearchArtifactRef{}, nil, err
	}
	defer artifactDir.Close()
	ref.Path = researchDisplayArtifactPath(s.baseDir, id, name)
	payload, err := readResearchFile(artifactDir, name, researchArtifactMaxBytes)
	if errors.Is(err, os.ErrNotExist) {
		refs, genErr := s.GenerateExports(id, []string{format})
		if genErr != nil {
			return ResearchArtifactRef{}, nil, genErr
		}
		ref = refs[0]
		name, ok = researchArtifactFilename(format)
		if !ok || ref.Name != name {
			return ResearchArtifactRef{}, nil, fmt.Errorf("invalid generated research artifact reference")
		}
		payload, err = readResearchFile(artifactDir, name, researchArtifactMaxBytes)
	}
	if err != nil {
		return ResearchArtifactRef{}, nil, err
	}
	ref.ContentType = researchArtifactContentType(format)
	ref.Bytes = int64(len(payload))
	ref.SHA256 = researchSHA256Hex(payload)
	if info, statErr := researchFileInfo(artifactDir, name); statErr == nil {
		ref.CreatedAt = info.ModTime().UTC()
	}
	return ref, payload, nil
}

func (s *researchSessionStore) CleanupRetention(days int) error {
	if s == nil {
		return nil
	}
	s.fsMu.Lock()
	defer s.fsMu.Unlock()
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
		root, sessionDir, err := openResearchSession(s.baseDir, session.ID, false)
		if err != nil {
			return err
		}
		artifactDir, artifactErr := openResearchArtifacts(sessionDir, false)
		for format, ref := range refs {
			name, ok := researchArtifactFilename(format)
			if !ok || ref.Name != name {
				delete(refs, format)
				changed = true
				continue
			}
			if errors.Is(artifactErr, unix.ENOENT) {
				delete(refs, format)
				changed = true
				continue
			}
			if artifactErr != nil {
				_ = sessionDir.Close()
				_ = root.Close()
				return artifactErr
			}
			info, err := researchFileInfo(artifactDir, name)
			if errors.Is(err, unix.ENOENT) {
				delete(refs, format)
				changed = true
				continue
			}
			if err != nil {
				_ = artifactDir.Close()
				_ = sessionDir.Close()
				_ = root.Close()
				return err
			}
			if info.ModTime().After(cutoff) {
				continue
			}
			if err := removeResearchFile(artifactDir, name); err != nil {
				_ = artifactDir.Close()
				_ = sessionDir.Close()
				_ = root.Close()
				return err
			}
			delete(refs, format)
			changed = true
		}
		if artifactDir != nil {
			_ = artifactDir.Close()
		}
		_ = sessionDir.Close()
		_ = root.Close()
		if changed {
			s.mu.Lock()
			if current := s.sessions[session.ID]; current != nil {
				current.ArtifactRefs = refs
				current.UpdatedAt = time.Now().UTC()
				if err := s.saveSessionLocked(current); err != nil {
					s.mu.Unlock()
					return err
				}
			}
			s.mu.Unlock()
		}
	}
	return nil
}

func (s *researchSessionStore) writeEvents(id string, events []ResearchEvent) error {
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	if len(events) > settings.MaxSessionEvents {
		return fmt.Errorf("research event log exceeds %d events", settings.MaxSessionEvents)
	}
	buf := &researchLimitedBuffer{limit: researchEventsFileMaxBytes}
	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return err
		}
	}
	root, sessionDir, err := openResearchSession(s.baseDir, id, true)
	if err != nil {
		return err
	}
	defer root.Close()
	defer sessionDir.Close()
	return atomicWriteResearchFile(sessionDir, "events.jsonl", buf.buf.Bytes(), researchEventsFileMaxBytes)
}

func (s *researchSessionStore) writeResults(id string, results ResearchResults) error {
	payload, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	root, sessionDir, err := openResearchSession(s.baseDir, id, true)
	if err != nil {
		return err
	}
	defer root.Close()
	defer sessionDir.Close()
	return atomicWriteResearchFile(sessionDir, "results.json", payload, researchResultsFileMaxBytes)
}

func (s *researchSessionStore) saveSessionLocked(session *ResearchSession) error {
	if s == nil || session == nil {
		return errors.New("research session is nil")
	}
	normalizeResearchSession(session)
	root, sessionDir, err := openResearchSession(s.baseDir, session.ID, true)
	if err != nil {
		return err
	}
	defer root.Close()
	defer sessionDir.Close()
	payload, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteResearchFile(sessionDir, "session.json", payload, researchSessionFileMaxBytes)
}

func (s *researchSessionStore) clearResearchArtifacts(sessionDir *os.File) error {
	artifactDir, err := openResearchArtifacts(sessionDir, false)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	names, err := researchDirectoryNames(artifactDir, 64)
	if err != nil {
		_ = artifactDir.Close()
		return err
	}
	for _, name := range names {
		if err := removeResearchFile(artifactDir, name); err != nil {
			_ = artifactDir.Close()
			return err
		}
	}
	if err := artifactDir.Close(); err != nil {
		return err
	}
	return removeResearchDirectory(sessionDir, "artifacts")
}

func (s *researchSessionStore) preflightResearchSession(sessionDir *os.File) error {
	names, err := researchDirectoryNames(sessionDir, 8)
	if err != nil {
		return err
	}
	for _, name := range names {
		switch name {
		case "session.json", "events.jsonl", "results.json":
			if err := validateResearchRegularEntry(sessionDir, name); err != nil {
				return err
			}
		case "artifacts":
			artifactDir, err := openResearchArtifacts(sessionDir, false)
			if err != nil {
				return err
			}
			artifactNames, err := researchDirectoryNames(artifactDir, 64)
			if err != nil {
				_ = artifactDir.Close()
				return err
			}
			for _, artifactName := range artifactNames {
				if err := validateResearchRegularEntry(artifactDir, artifactName); err != nil {
					_ = artifactDir.Close()
					return err
				}
			}
			if err := artifactDir.Close(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected research session entry %q", name)
		}
	}
	return nil
}

func (s *researchSessionStore) cleanupResearchTombstone(root *os.File, name string) error {
	sessionDir, err := openResearchDirChild(root, name, false)
	if err != nil {
		return err
	}
	defer sessionDir.Close()
	if err := s.preflightResearchSession(sessionDir); err != nil {
		return err
	}
	if err := s.clearResearchArtifacts(sessionDir); err != nil {
		return err
	}
	for _, filename := range []string{"events.jsonl", "results.json", "session.json"} {
		if err := removeResearchFile(sessionDir, filename); err != nil {
			return err
		}
	}
	if err := sessionDir.Close(); err != nil {
		return err
	}
	if err := removeResearchDirectory(root, name); err != nil {
		return err
	}
	return unix.Fsync(int(root.Fd()))
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
