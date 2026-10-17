package main

import (
	"crypto/sha256"
	"encoding/binary"
	"io"
	"sort"
	"strings"
)

// An alert acknowledgement only changes Renew's current UI. It is neither a
// backend allowlist nor a statement that the event is a false positive.
const personalAlertMuteLimit = 64

type personalAlertGroup struct {
	Event  eventSummary
	Count  int
	Key    [sha256.Size]byte
	Risk   string
	LastMS int64
}

// Delimit fields by their lengths so even unusual process names or targets
// cannot turn unrelated signatures into one group. The digest also avoids
// retaining extra copies of potentially sensitive paths in the mute map.
func personalAlertSignature(e eventSummary) [sha256.Size]byte {
	fields := []string{
		strings.TrimSpace(e.Type),
		strings.TrimSpace(e.Comm),
		strings.TrimSpace(e.Tag),
		strings.TrimSpace(e.Target),
		strings.ToUpper(strings.TrimSpace(e.Decision)),
		eventRisk(e),
	}
	// Without identifying fields, do not accidentally fold separate events
	// into one apparently harmless item.
	if fields[0] == "" && fields[1] == "" && fields[2] == "" && fields[3] == "" {
		fields = append(fields, strings.TrimSpace(e.EventID))
	}
	h := sha256.New()
	var size [binary.MaxVarintLen64]byte
	for _, field := range fields {
		n := binary.PutUvarint(size[:], uint64(len(field)))
		_, _ = h.Write(size[:n])
		_, _ = io.WriteString(h, field)
	}
	var key [sha256.Size]byte
	copy(key[:], h.Sum(nil))
	return key
}

// Group the bounded, already loaded summary window. Severity is part of the
// signature; acknowledging an attention-level group can never hide a later
// BLOCK/DENY or risk escalation to the high-risk tier.
func groupPersonalAlerts(events []eventSummary, muted map[[sha256.Size]byte]struct{}) ([]personalAlertGroup, int) {
	groups := make([]personalAlertGroup, 0, min(len(events), 64))
	indices := make(map[[sha256.Size]byte]int)
	hidden := 0
	for _, event := range events {
		risk := eventRisk(event)
		if risk == "正常" {
			continue
		}
		key := personalAlertSignature(event)
		if risk == "需关注" {
			if _, ok := muted[key]; ok {
				hidden++
				continue
			}
		}
		if index, ok := indices[key]; ok {
			group := &groups[index]
			group.Count++
			if event.ReceivedAtMS > group.LastMS ||
				(event.ReceivedAtMS == group.LastMS && event.EventID > group.Event.EventID) {
				group.Event = event
				group.LastMS = event.ReceivedAtMS
			}
			continue
		}
		indices[key] = len(groups)
		groups = append(groups, personalAlertGroup{
			Event: event, Count: 1, Key: key, Risk: risk, LastMS: event.ReceivedAtMS,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Risk != groups[j].Risk {
			return groups[i].Risk == "高风险"
		}
		if groups[i].LastMS != groups[j].LastMS {
			return groups[i].LastMS > groups[j].LastMS
		}
		return groups[i].Event.EventID > groups[j].Event.EventID
	})
	return groups, hidden
}

// The per-window cache avoids hashing up to 1200 summaries on every MyGo
// frame. Both events and local acknowledgement updates invalidate the cache.
func (a *renewApp) personalAlertGroups() ([]personalAlertGroup, int) {
	if a.personalAlertCacheReady &&
		a.personalAlertCacheEventsVersion == a.eventsVersion &&
		a.personalAlertCacheMuteVersion == a.personalAlertMuteVersion {
		return a.personalAlertCacheRows, a.personalAlertCacheHidden
	}
	rows, hidden := groupPersonalAlerts(a.events, a.personalAlertMuted)
	a.personalAlertCacheRows = rows
	a.personalAlertCacheHidden = hidden
	a.personalAlertCacheEventsVersion = a.eventsVersion
	a.personalAlertCacheMuteVersion = a.personalAlertMuteVersion
	a.personalAlertCacheReady = true
	return rows, hidden
}

func (a *renewApp) mutePersonalAlert(group personalAlertGroup) {
	if group.Risk != "需关注" {
		return
	}
	if a.personalAlertMuted == nil {
		a.personalAlertMuted = make(map[[sha256.Size]byte]struct{})
	}
	if _, exists := a.personalAlertMuted[group.Key]; exists {
		return
	}
	if len(a.personalAlertMuteOrder) == personalAlertMuteLimit {
		oldest := a.personalAlertMuteOrder[0]
		delete(a.personalAlertMuted, oldest)
		copy(a.personalAlertMuteOrder, a.personalAlertMuteOrder[1:])
		a.personalAlertMuteOrder = a.personalAlertMuteOrder[:len(a.personalAlertMuteOrder)-1]
	}
	a.personalAlertMuted[group.Key] = struct{}{}
	a.personalAlertMuteOrder = append(a.personalAlertMuteOrder, group.Key)
	a.personalAlertMuteVersion++
}

func (a *renewApp) restorePersonalAlerts() {
	if len(a.personalAlertMuteOrder) == 0 {
		return
	}
	a.personalAlertMuted = nil
	a.personalAlertMuteOrder = nil
	a.personalAlertMuteVersion++
}
