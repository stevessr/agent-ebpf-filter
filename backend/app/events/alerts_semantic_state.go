package events

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type SemanticSecretObservation struct {
	SeenAt time.Time
	Target string
}

type SemanticExecObservation struct {
	SeenAt time.Time
	Path   string
	Mode   string
}

type SemanticForkObservation struct {
	WindowStart time.Time
	Count       int
}

type SemanticAgenticLoopObservation struct {
	WindowStart   time.Time
	PromptDigest  string
	PromptRepeats int
	APICalls      int
	FileOps       int
	LastTarget    string
	Alerted       bool
}

type SemanticFileMutationObservation struct {
	SeenAt      time.Time
	LastAlertAt time.Time
	Actor       string
	Op          string
	Path        string
	ContainerID string
}

type SemanticAlertState struct {
	mu                            sync.Mutex
	recentSecrets                 *boundedSemanticStateMap[SemanticSecretObservation]
	recentExecs                   *boundedSemanticStateMap[SemanticExecObservation]
	forkWindows                   *boundedSemanticStateMap[SemanticForkObservation]
	agenticLoopWindows            *boundedSemanticStateMap[SemanticAgenticLoopObservation]
	recentFileMutations           *boundedSemanticStateMap[SemanticFileMutationObservation]
	expiredEvictionsTotal         uint64
	capacityEvictionsTotal        uint64
	truncatedStateValuesTotal     uint64
	ignoredOversizedMetadataTotal uint64
	fileCorrelationAlertsTotal    uint64
	fileCorrelationDedupedTotal   uint64
	fileCorrelationLateTotal      uint64
	lastSweepAt                   time.Time
}

func NewSemanticAlertState() *SemanticAlertState {
	return &SemanticAlertState{
		recentSecrets:       newBoundedSemanticStateMap[SemanticSecretObservation](SemanticStateMaxContextEntries),
		recentExecs:         newBoundedSemanticStateMap[SemanticExecObservation](SemanticStateMaxContextEntries),
		forkWindows:         newBoundedSemanticStateMap[SemanticForkObservation](SemanticStateMaxContextEntries),
		agenticLoopWindows:  newBoundedSemanticStateMap[SemanticAgenticLoopObservation](SemanticStateMaxContextEntries),
		recentFileMutations: newBoundedSemanticStateMap[SemanticFileMutationObservation](SemanticStateMaxFileEntries),
	}
}

func (s *SemanticAlertState) RememberSecret(event *pb.Event, target string, now time.Time) {
	if s == nil {
		return
	}
	key, keyTruncated := semanticAlertContextKeyBounded(event)
	if key == "" {
		return
	}
	target, targetTruncated := boundSemanticStateString(target, SemanticStateMaxValueBytes)
	s.mu.Lock()
	s.ensureMapsLocked()
	s.noteTruncationsLocked(keyTruncated, targetTruncated)
	s.noteCapacityEvictionLocked(s.recentSecrets.Set(key, SemanticSecretObservation{SeenAt: now, Target: target}))
	s.mu.Unlock()
}

func (s *SemanticAlertState) RecentSecretTarget(event *pb.Event, now time.Time) (string, bool) {
	if s == nil {
		return "", false
	}
	key, keyTruncated := semanticAlertContextKeyBounded(event)
	if key == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	s.noteTruncationsLocked(keyTruncated)
	observation, ok := s.recentSecrets.Get(key)
	if !ok {
		return "", false
	}
	if semanticStateExpired(now, observation.SeenAt, SemanticSecretCorrelationTTL) {
		s.recentSecrets.Delete(key)
		s.expiredEvictionsTotal++
		return "", false
	}
	return observation.Target, observation.Target != ""
}

func (s *SemanticAlertState) RememberExecutable(event *pb.Event, path, mode string, now time.Time) {
	if s == nil {
		return
	}
	key, keyTruncated := semanticAlertContextKeyBounded(event)
	if key == "" || strings.TrimSpace(path) == "" {
		return
	}
	path, pathTruncated := boundSemanticStateString(path, SemanticStateMaxPathBytes)
	mode, modeTruncated := boundSemanticStateString(mode, SemanticStateMaxModeBytes)
	s.mu.Lock()
	s.ensureMapsLocked()
	s.noteTruncationsLocked(keyTruncated, pathTruncated, modeTruncated)
	s.noteCapacityEvictionLocked(s.recentExecs.Set(key, SemanticExecObservation{SeenAt: now, Path: filepath.Clean(path), Mode: mode}))
	s.mu.Unlock()
}

func (s *SemanticAlertState) RecentExecutablePath(key, path string, now time.Time) (string, bool) {
	if s == nil || key == "" || strings.TrimSpace(path) == "" {
		return "", false
	}
	key, keyTruncated := boundSemanticStateString(key, SemanticStateMaxContextBytes)
	path, pathTruncated := boundSemanticStateString(path, SemanticStateMaxPathBytes)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	s.noteTruncationsLocked(keyTruncated, pathTruncated)
	observation, ok := s.recentExecs.Get(key)
	if !ok {
		return "", false
	}
	if semanticStateExpired(now, observation.SeenAt, SemanticExecCorrelationTTL) {
		s.recentExecs.Delete(key)
		s.expiredEvictionsTotal++
		return "", false
	}
	cleanPath := filepath.Clean(path)
	return observation.Path, observation.Path == cleanPath
}

func (s *SemanticAlertState) IncrementForkCount(event *pb.Event, now time.Time) int {
	return s.incrementForkCountInScope(event, now, 0)
}

// Clone loop thresholds apply only to a verified clone PID. In particular,
// unrelated commands belonging to the SAME Agent tool/run must continue to
// use the ordinary fork window, rather than inheriting the relaxed threshold
// or advancing a trusted clone's counter.
func (s *SemanticAlertState) incrementForkCountInScope(event *pb.Event, now time.Time, clonePID uint32) int {
	if s == nil || event == nil {
		return 0
	}
	key, keyTruncated := semanticAlertContextKeyBounded(event)
	if key == "" {
		return 0
	}
	if clonePID != 0 {
		key += fmt.Sprintf("/verified_public_clone_pid:%d", clonePID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	s.noteTruncationsLocked(keyTruncated)
	observation, exists := s.forkWindows.Get(key)
	if !exists || semanticStateExpired(now, observation.WindowStart, SemanticForkWindow) {
		if exists {
			s.expiredEvictionsTotal++
		}
		observation = SemanticForkObservation{WindowStart: now, Count: 1}
	} else {
		observation.Count++
	}
	s.noteCapacityEvictionLocked(s.forkWindows.Set(key, observation))
	return observation.Count
}

func (s *SemanticAlertState) ObserveAgenticResourceLoop(event *pb.Event, now time.Time) (string, string, bool) {
	if s == nil || event == nil {
		return "", "", false
	}
	return s.observeAgenticResourceLoop(event, now, isLowValueFileIOEvent(event))
}

func (s *SemanticAlertState) observeAgenticResourceLoop(event *pb.Event, now time.Time, fileIO bool) (string, string, bool) {
	if s == nil || event == nil {
		return "", "", false
	}
	promptDigest, oversizedMetadata := extraInfoFieldBounded(event.GetExtraInfo(), "prompt_digest", SemanticPromptDigestMaxBytes)
	apiLike := isAPILikeNetworkEvent(event)
	if oversizedMetadata {
		s.mu.Lock()
		s.ignoredOversizedMetadataTotal++
		s.mu.Unlock()
	}
	if promptDigest == "" && !apiLike && !fileIO {
		return "", "", false
	}
	key, keyTruncated := semanticAlertContextKeyBounded(event)
	if key == "" {
		return "", "", false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	s.noteTruncationsLocked(keyTruncated)

	observation, exists := s.agenticLoopWindows.Get(key)
	if !exists || semanticStateExpired(now, observation.WindowStart, SemanticAgenticLoopWindow) {
		if exists {
			s.expiredEvictionsTotal++
		}
		observation = SemanticAgenticLoopObservation{WindowStart: now}
	}

	if promptDigest != "" {
		if observation.PromptDigest == promptDigest {
			observation.PromptRepeats++
		} else {
			// The parser returns a view into ExtraInfo. Clone only when a new
			// digest is retained so a small state value cannot pin a large event.
			observation.PromptDigest = strings.Clone(promptDigest)
			observation.PromptRepeats = 1
			observation.Alerted = false
		}
		observation.LastTarget, _ = boundSemanticStateString("prompt:"+promptDigest, SemanticStateMaxValueBytes)
	}
	if apiLike {
		observation.APICalls++
		target, truncated := boundSemanticStateString(
			platform.FirstNonEmpty(event.GetNetEndpoint(), event.GetSni(), event.GetHttpHost(), event.GetDnsName(), event.GetPath()),
			SemanticStateMaxValueBytes,
		)
		s.noteTruncationsLocked(truncated)
		observation.LastTarget = target
	}
	if fileIO {
		observation.FileOps++
		if target := platform.FirstNonEmpty(event.GetPath(), event.GetExtraPath(), event.GetComm()); target != "" {
			boundedTarget, truncated := boundSemanticStateString(target, SemanticStateMaxValueBytes)
			s.noteTruncationsLocked(truncated)
			observation.LastTarget = boundedTarget
		}
	}

	if !observation.Alerted &&
		observation.PromptRepeats >= SemanticPromptLoopThreshold &&
		observation.APICalls >= SemanticAPILoopThreshold &&
		observation.FileOps >= SemanticFileIOLoopThreshold {
		observation.Alerted = true
		s.noteCapacityEvictionLocked(s.agenticLoopWindows.Set(key, observation))
		target := platform.FirstNonEmpty(observation.LastTarget, observation.PromptDigest, key)
		reason := fmt.Sprintf("observed repeated prompt metadata (%d repeats) with %d API egress events and %d low-level file I/O events within %s",
			observation.PromptRepeats, observation.APICalls, observation.FileOps, SemanticAgenticLoopWindow)
		return target, reason, true
	}

	s.noteCapacityEvictionLocked(s.agenticLoopWindows.Set(key, observation))
	return "", "", false
}

// ObserveMultiAgentFileContention detects interleaved mutations of a
// verified pathname by distinct Agent contexts. It does not prove that the
// operations overlapped or that they refer to one inode across mount namespaces.
func (s *SemanticAlertState) ObserveMultiAgentFileContention(event *pb.Event, now time.Time) (string, string, bool) {
	if s == nil || event == nil {
		return "", "", false
	}
	path, pathTruncated, ok := semanticFileMutationPath(event)
	if !ok {
		return "", "", false
	}
	actor, actorTruncated := semanticAgentIdentity(event)
	if actor == "" || actorTruncated {
		return "", "", false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	// Pathnames are scoped by mount/container context. We currently have no
	// mount-namespace inode ID, so separate known containers instead of
	// allowing an unrelated same-named path to evict another actor's evidence.
	// Empty container IDs are a separate unknown scope, not a wildcard.
	containerID := strings.TrimSpace(event.GetContainerId())
	if len(containerID) > 128 || strings.ContainsRune(containerID, 0) {
		return "", "", false
	}
	key := path
	if containerID != "" {
		key = "container:" + containerID + "\x00" + path
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	s.noteTruncationsLocked(pathTruncated, actorTruncated)

	previous, seen := s.recentFileMutations.Get(key)
	// Event replays or asynchronous producers may arrive out of order. An
	// older observation must not replace newer evidence or revive a cooldown
	// window; neither can it establish an ordered interleaving.
	if seen && now.Before(previous.SeenAt) {
		s.fileCorrelationLateTotal++
		return "", "", false
	}
	if seen && semanticStateExpired(now, previous.SeenAt, SemanticFileContentionTTL) {
		s.expiredEvictionsTotal++
		seen = false
	}
	current := SemanticFileMutationObservation{
		SeenAt:      now,
		Actor:       actor,
		Op:          event.GetType(),
		Path:        path,
		ContainerID: containerID,
	}
	if seen {
		current.LastAlertAt = previous.LastAlertAt
	}
	// An Agent run ID and a root PID are different identity namespaces.
	// They are not evidence that two agents are distinct from each other.
	comparable := seen && previous.Actor != "" && previous.Actor != actor &&
		(strings.HasPrefix(previous.Actor, "root_pid:") == strings.HasPrefix(actor, "root_pid:"))
	// Different container IDs may describe different mount namespaces; a
	// shared pathname alone does not establish a shared underlying file.
	if comparable && previous.ContainerID != "" && current.ContainerID != "" &&
		previous.ContainerID != current.ContainerID {
		comparable = false
	}
	cooldown := seen && !previous.LastAlertAt.IsZero() &&
		!semanticStateExpired(now, previous.LastAlertAt, SemanticFileContentionTTL)
	if comparable {
		if cooldown {
			s.fileCorrelationDedupedTotal++
		} else {
			current.LastAlertAt = now
			s.fileCorrelationAlertsTotal++
		}
	}
	s.noteCapacityEvictionLocked(s.recentFileMutations.Set(key, current))
	if !comparable || cooldown {
		return "", "", false
	}

	reason := fmt.Sprintf("two distinct agent contexts (%s and %s) modified the same reported pathname within %s (%s then %s); temporal overlap, identical inode and malicious intent are not established",
		previous.Actor, actor, SemanticFileContentionTTL, previous.Op, event.GetType())
	return path, reason, true
}

func (s *SemanticAlertState) EvictExpired(now time.Time) SemanticAlertStateStatus {
	if s == nil {
		return SemanticAlertStateStatus{MaxEntries: SemanticStateMaxEntries}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	evicted := 0
	evicted += s.recentSecrets.DeleteIf(func(value SemanticSecretObservation) bool {
		return semanticStateExpired(now, value.SeenAt, SemanticSecretCorrelationTTL)
	})
	evicted += s.recentExecs.DeleteIf(func(value SemanticExecObservation) bool {
		return semanticStateExpired(now, value.SeenAt, SemanticExecCorrelationTTL)
	})
	evicted += s.forkWindows.DeleteIf(func(value SemanticForkObservation) bool {
		return semanticStateExpired(now, value.WindowStart, SemanticForkWindow)
	})
	evicted += s.agenticLoopWindows.DeleteIf(func(value SemanticAgenticLoopObservation) bool {
		return semanticStateExpired(now, value.WindowStart, SemanticAgenticLoopWindow)
	})
	evicted += s.recentFileMutations.DeleteIf(func(value SemanticFileMutationObservation) bool {
		return semanticStateExpired(now, value.SeenAt, SemanticFileContentionTTL)
	})
	s.expiredEvictionsTotal += uint64(evicted)
	s.lastSweepAt = now
	return s.statusLocked()
}

func (s *SemanticAlertState) Status() SemanticAlertStateStatus {
	if s == nil {
		return SemanticAlertStateStatus{MaxEntries: SemanticStateMaxEntries}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	return s.statusLocked()
}

func (s *SemanticAlertState) statusLocked() SemanticAlertStateStatus {
	status := SemanticAlertStateStatus{
		RecentSecrets:                 s.recentSecrets.Len(),
		RecentExecutables:             s.recentExecs.Len(),
		ForkWindows:                   s.forkWindows.Len(),
		AgenticLoopWindows:            s.agenticLoopWindows.Len(),
		RecentFileMutations:           s.recentFileMutations.Len(),
		MaxEntries:                    SemanticStateMaxEntries,
		ExpiredEvictionsTotal:         s.expiredEvictionsTotal,
		CapacityEvictionsTotal:        s.capacityEvictionsTotal,
		TruncatedStateValuesTotal:     s.truncatedStateValuesTotal,
		IgnoredOversizedMetadataTotal: s.ignoredOversizedMetadataTotal,
		FileCorrelationAlertsTotal:    s.fileCorrelationAlertsTotal,
		FileCorrelationDedupedTotal:   s.fileCorrelationDedupedTotal,
		FileCorrelationLateTotal:      s.fileCorrelationLateTotal,
		LastSweepAt:                   s.lastSweepAt,
	}
	status.Entries = status.RecentSecrets + status.RecentExecutables + status.ForkWindows + status.AgenticLoopWindows + status.RecentFileMutations
	return status
}

func (s *SemanticAlertState) ensureMapsLocked() {
	if s.recentSecrets == nil {
		s.recentSecrets = newBoundedSemanticStateMap[SemanticSecretObservation](SemanticStateMaxContextEntries)
	}
	if s.recentExecs == nil {
		s.recentExecs = newBoundedSemanticStateMap[SemanticExecObservation](SemanticStateMaxContextEntries)
	}
	if s.forkWindows == nil {
		s.forkWindows = newBoundedSemanticStateMap[SemanticForkObservation](SemanticStateMaxContextEntries)
	}
	if s.agenticLoopWindows == nil {
		s.agenticLoopWindows = newBoundedSemanticStateMap[SemanticAgenticLoopObservation](SemanticStateMaxContextEntries)
	}
	if s.recentFileMutations == nil {
		s.recentFileMutations = newBoundedSemanticStateMap[SemanticFileMutationObservation](SemanticStateMaxFileEntries)
	}
}

func (s *SemanticAlertState) noteCapacityEvictionLocked(evicted bool) {
	if evicted {
		s.capacityEvictionsTotal++
	}
}

func (s *SemanticAlertState) noteTruncationsLocked(values ...bool) {
	for _, truncated := range values {
		if truncated {
			s.truncatedStateValuesTotal++
		}
	}
}
