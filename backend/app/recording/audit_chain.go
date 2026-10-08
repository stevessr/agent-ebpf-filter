package recording

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"sync"

	"agent-ebpf-filter/app/events"
	"google.golang.org/protobuf/proto"
)

const AuditChainVersion = "sha256-envelope-v1"

var auditChainDomain = []byte("agent-ebpf-filter/audit-chain/v1\x00")

type AuditChainStatus struct {
	Version  string `json:"version"`
	ChainID  string `json:"chainId"`
	Sequence uint64 `json:"sequence"`
	LastHash string `json:"lastHash,omitempty"`
}

type AuditChain struct {
	mu       sync.RWMutex
	chainID  string
	sequence uint64
	lastHash [sha256.Size]byte
	hasLast  bool
}

func NewAuditChain() (*AuditChain, error) {
	seed := make([]byte, 16)
	if _, err := rand.Read(seed); err != nil {
		return nil, fmt.Errorf("generate audit chain id: %w", err)
	}
	return &AuditChain{chainID: hex.EncodeToString(seed)}, nil
}

func (c *AuditChain) Status() AuditChainStatus {
	if c == nil {
		return AuditChainStatus{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	status := AuditChainStatus{Version: AuditChainVersion, ChainID: c.chainID, Sequence: c.sequence}
	if c.hasLast {
		status.LastHash = hex.EncodeToString(c.lastHash[:])
	}
	return status
}

func (c *AuditChain) MarshalRecord(record CapturedEventRecord) ([]byte, error) {
	if c == nil {
		return MarshalRecord(record)
	}
	if record.Event == nil {
		return nil, errors.New("event recording record has no event")
	}
	record = events.NormalizeCapturedEventRecord(record)

	c.mu.Lock()
	defer c.mu.Unlock()
	sequence := c.sequence + 1
	previous := ""
	if c.hasLast {
		previous = hex.EncodeToString(c.lastHash[:])
	}
	digest, err := auditRecordHash(record, c.chainID, sequence, previous)
	if err != nil {
		return nil, err
	}
	record.AuditChainVersion = AuditChainVersion
	record.AuditChainID = c.chainID
	record.AuditSequence = sequence
	record.AuditPrevHash = previous
	record.AuditHash = hex.EncodeToString(digest[:])

	payload, err := marshalNormalizedRecord(record)
	if err != nil {
		return nil, err
	}
	c.sequence = sequence
	c.lastHash = digest
	c.hasLast = true
	return payload, nil
}

func auditRecordHash(record CapturedEventRecord, chainID string, sequence uint64, previous string) ([sha256.Size]byte, error) {
	record = events.NormalizeCapturedEventRecord(record)
	if record.Envelope == nil {
		return [sha256.Size]byte{}, errors.New("audit chain record has no envelope")
	}
	envelopeBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(record.Envelope)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("marshal audit envelope: %w", err)
	}
	h := sha256.New()
	_, _ = h.Write(auditChainDomain)
	writeAuditPart(h, []byte(chainID))
	writeAuditUint64(h, sequence)
	writeAuditPart(h, []byte(previous))
	writeAuditUint64(h, uint64(record.ReceivedAt.UTC().UnixNano()))
	writeAuditPart(h, envelopeBytes)
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}

func writeAuditUint64(h hash.Hash, value uint64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], value)
	_, _ = h.Write(buf[:])
}

func writeAuditPart(h hash.Hash, value []byte) {
	writeAuditUint64(h, uint64(len(value)))
	_, _ = h.Write(value)
}

type AuditVerification struct {
	Valid          bool   `json:"valid"`
	CheckedRecords int    `json:"checkedRecords"`
	LegacyRecords  int    `json:"legacyRecords"`
	Segments       int    `json:"segments"`
	LastHash       string `json:"lastHash,omitempty"`
	ErrorIndex     int    `json:"errorIndex"`
	Error          string `json:"error,omitempty"`
}

func VerifyAuditChain(records []CapturedEventRecord) AuditVerification {
	result := AuditVerification{Valid: true, ErrorIndex: -1}
	var chainID string
	var sequence uint64
	var lastHash string
	active := false

	fail := func(index int, err error) AuditVerification {
		result.Valid = false
		result.ErrorIndex = index
		result.Error = err.Error()
		return result
	}

	for index, record := range records {
		if record.AuditChainVersion == "" {
			result.LegacyRecords++
			active = false
			chainID, sequence, lastHash = "", 0, ""
			continue
		}
		if record.AuditChainVersion != AuditChainVersion {
			return fail(index, fmt.Errorf("unsupported audit chain version %q", record.AuditChainVersion))
		}
		if record.AuditChainID == "" || record.AuditSequence == 0 || record.AuditHash == "" {
			return fail(index, errors.New("incomplete audit chain metadata"))
		}
		if record.AuditSequence == 1 {
			if record.AuditPrevHash != "" {
				return fail(index, errors.New("audit chain genesis has previous hash"))
			}
			result.Segments++
			chainID, sequence, lastHash = record.AuditChainID, 0, ""
			active = true
		} else {
			if !active || record.AuditChainID != chainID {
				return fail(index, errors.New("audit chain segment starts without genesis"))
			}
			if record.AuditSequence != sequence+1 {
				return fail(index, fmt.Errorf("audit sequence gap: got %d want %d", record.AuditSequence, sequence+1))
			}
			if record.AuditPrevHash != lastHash {
				return fail(index, errors.New("audit previous hash mismatch"))
			}
		}
		digest, err := auditRecordHash(record, record.AuditChainID, record.AuditSequence, record.AuditPrevHash)
		if err != nil {
			return fail(index, err)
		}
		expected := hex.EncodeToString(digest[:])
		if record.AuditHash != expected {
			return fail(index, errors.New("audit record hash mismatch"))
		}
		result.CheckedRecords++
		chainID = record.AuditChainID
		sequence = record.AuditSequence
		lastHash = record.AuditHash
		result.LastHash = lastHash
		active = true
	}
	return result
}
