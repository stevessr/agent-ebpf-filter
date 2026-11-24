package domainforwardproxy

import (
	"sync"
)

const maxResponsesWSRewriteHistory = 256

type responsesWSPendingSteer struct {
	id       string
	mapping  *modelRewrite
	streamID string
}

type responsesWSRewriteState struct {
	mu                  sync.RWMutex
	streamMappings      map[string][]*modelRewrite
	responseMappings    map[string]*modelRewrite
	responseStreams     map[string]string
	responseOrder       []string
	pendingSteers       map[string][]responsesWSPendingSteer
	heldTerminalStreams map[string]string
}

func newResponsesWSRewriteState() *responsesWSRewriteState {
	return &responsesWSRewriteState{
		streamMappings:      make(map[string][]*modelRewrite),
		responseMappings:    make(map[string]*modelRewrite),
		responseStreams:     make(map[string]string),
		pendingSteers:       make(map[string][]responsesWSPendingSteer),
		heldTerminalStreams: make(map[string]string),
	}
}

func cloneModelRewrite(mapping *modelRewrite) *modelRewrite {
	if mapping == nil {
		return nil
	}
	copy := *mapping
	copy.clientJSON = append([]byte(nil), mapping.clientJSON...)
	copy.upstreamJSON = append([]byte(nil), mapping.upstreamJSON...)
	return &copy
}

func (s *responsesWSRewriteState) enqueueStream(streamID string, mapping *modelRewrite) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamMappings[streamID] = append(s.streamMappings[streamID], cloneModelRewrite(mapping))
}

func (s *responsesWSRewriteState) stream(streamID string) (*modelRewrite, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	queue := s.streamMappings[streamID]
	if len(queue) == 0 {
		return nil, false
	}
	return cloneModelRewrite(queue[0]), true
}

func (s *responsesWSRewriteState) rememberResponse(responseID, streamID string, mapping *modelRewrite) {
	if responseID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.responseMappings[responseID]; !exists {
		s.responseOrder = append(s.responseOrder, responseID)
	}
	s.responseMappings[responseID] = cloneModelRewrite(mapping)
	s.responseStreams[responseID] = streamID
	s.pruneResponseHistoryLocked()
}

func (s *responsesWSRewriteState) pruneResponseHistoryLocked() {
	for len(s.responseOrder) > maxResponsesWSRewriteHistory {
		evictIndex := -1
		for i, responseID := range s.responseOrder {
			if len(s.pendingSteers[responseID]) > 0 {
				continue
			}
			if _, held := s.heldTerminalStreams[responseID]; held {
				continue
			}
			evictIndex = i
			break
		}
		if evictIndex < 0 {
			return
		}
		oldest := s.responseOrder[evictIndex]
		copy(s.responseOrder[evictIndex:], s.responseOrder[evictIndex+1:])
		s.responseOrder[len(s.responseOrder)-1] = ""
		s.responseOrder = s.responseOrder[:len(s.responseOrder)-1]
		delete(s.responseMappings, oldest)
		delete(s.responseStreams, oldest)
	}
}

func (s *responsesWSRewriteState) response(responseID string) (*modelRewrite, bool) {
	if responseID == "" {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	mapping, ok := s.responseMappings[responseID]
	return cloneModelRewrite(mapping), ok
}

func (s *responsesWSRewriteState) streamForResponse(responseID string) string {
	if responseID == "" {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.responseStreams[responseID]
}

func (s *responsesWSRewriteState) enqueueSteer(previousResponseID string, mapping *modelRewrite) {
	if previousResponseID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingSteers[previousResponseID] = append(
		s.pendingSteers[previousResponseID],
		responsesWSPendingSteer{
			mapping:  cloneModelRewrite(mapping),
			streamID: s.responseStreams[previousResponseID],
		},
	)
}

func (s *responsesWSRewriteState) hasPendingSteer(previousResponseID string) bool {
	if previousResponseID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.pendingSteers[previousResponseID]) > 0
}

func (s *responsesWSRewriteState) acknowledgeSteer(previousResponseID, steerID string) bool {
	if previousResponseID == "" || steerID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.pendingSteers[previousResponseID]
	for i := range queue {
		if queue[i].id != "" {
			continue
		}
		queue[i].id = steerID
		s.pendingSteers[previousResponseID] = queue
		return true
	}
	return false
}

func (s *responsesWSRewriteState) holdTerminalResponse(responseID, streamID string) {
	if responseID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.heldTerminalStreams[responseID] = streamID
}

func (s *responsesWSRewriteState) releaseHeldTerminalResponse(responseID string) (string, bool) {
	if responseID == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pendingSteers[responseID]) > 0 {
		return "", false
	}
	streamID, ok := s.heldTerminalStreams[responseID]
	if ok {
		delete(s.heldTerminalStreams, responseID)
		s.pruneResponseHistoryLocked()
	}
	return streamID, ok
}

// bindPendingSteerContinuation attaches an explicit response.create to steering
// input already owned by the server. A pending steer keeps the parent lane
// mapping queued, so appending another stream mapping here would leave stale
// state after the successor completes.
func (s *responsesWSRewriteState) bindPendingSteerContinuation(
	previousResponseID, streamID string,
	mapping *modelRewrite,
) bool {
	if previousResponseID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.pendingSteers[previousResponseID]
	if len(queue) == 0 {
		return false
	}
	expectedStreamID := s.responseStreams[previousResponseID]
	if streamID != expectedStreamID {
		return false
	}
	for i := range queue {
		queue[i].mapping = cloneModelRewrite(mapping)
		queue[i].streamID = streamID
	}
	s.pendingSteers[previousResponseID] = queue
	return true
}

func (s *responsesWSRewriteState) takePendingSteer(previousResponseID string) (responsesWSPendingSteer, bool) {
	if previousResponseID == "" {
		return responsesWSPendingSteer{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.pendingSteers[previousResponseID]
	if len(queue) == 0 {
		return responsesWSPendingSteer{}, false
	}
	entry := queue[0]
	if len(queue) == 1 {
		delete(s.pendingSteers, previousResponseID)
	} else {
		queue[0] = responsesWSPendingSteer{}
		s.pendingSteers[previousResponseID] = queue[1:]
	}
	entry.mapping = cloneModelRewrite(entry.mapping)
	return entry, true
}

// takePendingSteerBatch transfers all steering submissions queued against one
// response into the single automatic successor response. The service may
// accept more than one response.steer before it creates that successor; leaving
// extra entries behind would make a later continuation inherit stale state.
func (s *responsesWSRewriteState) takePendingSteerBatch(previousResponseID string) (responsesWSPendingSteer, bool) {
	if previousResponseID == "" {
		return responsesWSPendingSteer{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.pendingSteers[previousResponseID]
	if len(queue) == 0 {
		return responsesWSPendingSteer{}, false
	}
	entry := queue[0]
	delete(s.pendingSteers, previousResponseID)
	delete(s.heldTerminalStreams, previousResponseID)
	entry.mapping = cloneModelRewrite(entry.mapping)
	s.pruneResponseHistoryLocked()
	return entry, true
}

func (s *responsesWSRewriteState) dropPendingSteer(
	previousResponseID, steerID string,
) (responsesWSPendingSteer, bool) {
	if previousResponseID == "" {
		return responsesWSPendingSteer{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.pendingSteers[previousResponseID]
	if len(queue) == 0 {
		return responsesWSPendingSteer{}, false
	}
	index := -1
	if steerID != "" {
		for i := range queue {
			if queue[i].id == steerID {
				index = i
				break
			}
		}
	} else {
		for i := range queue {
			if queue[i].id == "" {
				index = i
				break
			}
		}
	}
	if index < 0 {
		return responsesWSPendingSteer{}, false
	}
	entry := queue[index]
	copy(queue[index:], queue[index+1:])
	queue[len(queue)-1] = responsesWSPendingSteer{}
	queue = queue[:len(queue)-1]
	if len(queue) == 0 {
		delete(s.pendingSteers, previousResponseID)
	} else {
		s.pendingSteers[previousResponseID] = queue
	}
	entry.mapping = cloneModelRewrite(entry.mapping)
	s.pruneResponseHistoryLocked()
	return entry, true
}

func (s *responsesWSRewriteState) popStream(streamID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.streamMappings[streamID]
	if len(queue) <= 1 {
		delete(s.streamMappings, streamID)
		return
	}
	queue[0] = nil
	s.streamMappings[streamID] = queue[1:]
}
