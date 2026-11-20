package research

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sys/unix"
)

func (s *researchSessionStore) GenerateExports(id string, formats []string) ([]ResearchArtifactRef, error) {
	return s.GenerateExportsWithCancel(id, formats, nil)
}

func (s *researchSessionStore) GenerateExportsWithCancel(id string, formats []string, entry *researchTaskEntry) ([]ResearchArtifactRef, error) {
	s.fsMu.Lock()
	defer s.fsMu.Unlock()
	settings := snapshotRuntimeSettings().ResearchProcessing
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
		if err := AtomicWriteFile(artifactDir, name, payload, researchArtifactMaxBytes); err != nil {
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
		if err := AtomicWriteFile(artifactDir, name, payload, researchArtifactMaxBytes); err != nil {
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
	payload, err := ReadFile(artifactDir, name, researchArtifactMaxBytes)
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
		payload, err = ReadFile(artifactDir, name, researchArtifactMaxBytes)
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
			if err := RemoveFile(artifactDir, name); err != nil {
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
func (s *researchSessionStore) clearResearchArtifacts(sessionDir *os.File) error {
	artifactDir, err := openResearchArtifacts(sessionDir, false)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	names, err := DirectoryNames(artifactDir, 64)
	if err != nil {
		_ = artifactDir.Close()
		return err
	}
	for _, name := range names {
		if err := RemoveFile(artifactDir, name); err != nil {
			_ = artifactDir.Close()
			return err
		}
	}
	if err := artifactDir.Close(); err != nil {
		return err
	}
	return RemoveDirectory(sessionDir, "artifacts")
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
		if err := RemoveFile(sessionDir, filename); err != nil {
			return err
		}
	}
	if err := sessionDir.Close(); err != nil {
		return err
	}
	if err := RemoveDirectory(root, name); err != nil {
		return err
	}
	return unix.Fsync(int(root.Fd()))
}
