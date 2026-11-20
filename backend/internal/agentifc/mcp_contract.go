package agentifc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

// MCPToolManifest describes an MCP server tool declaration at discovery/pinning
// time or invocation time.
type MCPToolManifest struct {
	ServerID              string `json:"serverId"`
	ToolName              string `json:"toolName"`
	Description           string `json:"description"`
	ParameterDescriptions string `json:"parameterDescriptions"`
	InputSchemaJSON       string `json:"inputSchemaJson"`
	DeclaredCaps          uint32 `json:"declaredCaps"`
	SpawnBudget           uint32 `json:"spawnBudget"`
}

// PinnedContract records the cryptographic digests and narrowed lease anchored
// when a tool is vetted.
type PinnedContract struct {
	Key        string    `json:"key"`
	SchemaHash string    `json:"schemaHash"`
	DescHash   string    `json:"descHash"`
	Lease      TaskLease `json:"lease"`
}

// ContractCheckResult describes the outcome of PinManifest or VerifyInvocation.
type ContractCheckResult struct {
	Allowed           bool      `json:"allowed"`
	PoisoningDetected bool      `json:"poisoningDetected,omitempty"`
	RugPullDetected   bool      `json:"rugPullDetected,omitempty"`
	EscalationDenied  bool      `json:"escalationDenied,omitempty"`
	Reasons           []string  `json:"reasons,omitempty"`
	Pinned            string    `json:"pinnedDigest,omitempty"`
	Current           string    `json:"currentDigest,omitempty"`
	EffectiveLease    TaskLease `json:"effectiveLease"`
}

// ContractRegistry implements MCP-ContractGuard: screening tool manifests for
// instruction poisoning & hidden Unicode smuggling, pinning SHA-256 digests,
// monotonically narrowing child leases, and quarantining mid-session rug pulls.
type ContractRegistry struct {
	mu          sync.RWMutex
	contracts   map[string]PinnedContract
	quarantined map[string]string
}

// NewContractRegistry initializes an empty MCP-ContractGuard registry.
func NewContractRegistry() *ContractRegistry {
	return &ContractRegistry{
		contracts:   make(map[string]PinnedContract),
		quarantined: make(map[string]string),
	}
}

func contractKey(serverID, toolName string) string {
	return strings.ToLower(strings.TrimSpace(serverID)) + "::" + strings.ToLower(strings.TrimSpace(toolName))
}

func manifestDigests(m MCPToolManifest) (schemaHash, descHash string) {
	sh := sha256.Sum256([]byte(strings.TrimSpace(m.InputSchemaJSON)))
	dh := sha256.Sum256([]byte(strings.TrimSpace(m.Description) + "\x00" + strings.TrimSpace(m.ParameterDescriptions)))
	return hex.EncodeToString(sh[:]), hex.EncodeToString(dh[:])
}

// ScanManifestPoisoning inspects tool descriptions and parameter schemas for
// hidden imperative overrides, covert exfiltration directives, Unicode Tag-Plane
// (U+E0000-U+E007F) invisible instructions, BiDi overrides, and zero-width clusters.
func ScanManifestPoisoning(m MCPToolManifest) []string {
	var reasons []string
	combined := m.Description + "\n" + m.ParameterDescriptions + "\n" + m.InputSchemaJSON
	lower := strings.ToLower(combined)

	for _, marker := range []struct {
		pattern string
		reason  string
	}{
		{"<important>", "hidden_important_tag_directive"},
		{"</important>", "hidden_important_tag_directive"},
		{"<system>", "hidden_system_tag_override"},
		{"ignore previous instructions", "prompt_override_directive"},
		{"ignore all prior", "prompt_override_directive"},
		{"disregard previous", "prompt_override_directive"},
		{"do not tell the user", "covert_user_deception_directive"},
		{"do not mention", "covert_user_deception_directive"},
		{"before calling this tool", "side_effect_prerequisite_hijack"},
		{"pass its contents", "covert_exfiltration_directive"},
		{"~/.ssh", "credential_path_harvesting"},
		{".ssh/id_rsa", "credential_path_harvesting"},
		{"/etc/shadow", "credential_path_harvesting"},
		{"exfiltrate", "explicit_exfiltration_directive"},
		{"sidenote", "hidden_sidenote_directive"},
	} {
		if strings.Contains(lower, marker.pattern) {
			reasons = appendUnique(reasons, marker.reason)
		}
	}

	zwCount := 0
	for _, r := range combined {
		switch {
		case r >= 0xE0000 && r <= 0xE007F:
			reasons = appendUnique(reasons, "unicode_tag_plane_smuggling")
		case (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069):
			reasons = appendUnique(reasons, "bidi_trojan_override")
		case r == 0x200B || r == 0x200C || r == 0x200D || r == 0x200E || r == 0x200F || r == 0xFEFF:
			zwCount++
		}
	}
	if zwCount >= 3 {
		reasons = appendUnique(reasons, "zero_width_steganography")
	}
	return reasons
}

func appendUnique(dst []string, val string) []string {
	for _, existing := range dst {
		if existing == val {
			return dst
		}
	}
	return append(dst, val)
}

// PinManifest validates m against poisoning attacks and parentLease bounds,
// cryptographically pins its digests, and returns a narrowed child TaskLease.
func (r *ContractRegistry) PinManifest(parentLease TaskLease, m MCPToolManifest) ContractCheckResult {
	key := contractKey(m.ServerID, m.ToolName)
	poisonReasons := ScanManifestPoisoning(m)
	if len(poisonReasons) > 0 {
		r.mu.Lock()
		r.quarantined[key] = strings.Join(poisonReasons, ",")
		r.mu.Unlock()
		return ContractCheckResult{
			Allowed:           false,
			PoisoningDetected: true,
			Reasons:           poisonReasons,
			EffectiveLease: TaskLease{
				RunID:      parentLease.RunID,
				ConfTaint:  parentLease.ConfTaint,
				IntegTaint: parentLease.IntegTaint | IntegUntrusted | IntegToolOutput,
				CapMask:    0,
				Flags:      IFCFlagEnforce | IFCFlagTracked,
			},
		}
	}

	budget := m.SpawnBudget
	if budget == 0 {
		budget = parentLease.SpawnBudget
	}
	req := TaskLease{
		RunID:            parentLease.RunID,
		IntegTaint:       IntegToolOutput,
		CapMask:          m.DeclaredCaps,
		SpawnBudget:      budget,
		Flags:            IFCFlagEnforce | IFCFlagTracked,
		RejectEscalation: true,
	}
	narrowed, err := NarrowLease(parentLease, req)
	if err != nil {
		return ContractCheckResult{
			Allowed:          false,
			EscalationDenied: true,
			Reasons:          []string{err.Error()},
			EffectiveLease: TaskLease{
				RunID:      parentLease.RunID,
				ConfTaint:  parentLease.ConfTaint,
				IntegTaint: parentLease.IntegTaint | IntegUntrusted,
				CapMask:    0,
				Flags:      IFCFlagEnforce | IFCFlagTracked,
			},
		}
	}

	schemaHash, descHash := manifestDigests(m)
	r.mu.Lock()
	defer r.mu.Unlock()
	if qReason, isQ := r.quarantined[key]; isQ {
		return ContractCheckResult{
			Allowed:         false,
			RugPullDetected: true,
			Reasons:         []string{"previously_quarantined:" + qReason},
			EffectiveLease: TaskLease{
				RunID:      parentLease.RunID,
				IntegTaint: parentLease.IntegTaint | IntegUntrusted,
				CapMask:    0,
				Flags:      IFCFlagEnforce | IFCFlagTracked,
			},
		}
	}
	r.contracts[key] = PinnedContract{
		Key:        key,
		SchemaHash: schemaHash,
		DescHash:   descHash,
		Lease:      narrowed,
	}
	return ContractCheckResult{
		Allowed:        true,
		Pinned:         descHash[:16] + ":" + schemaHash[:16],
		Current:        descHash[:16] + ":" + schemaHash[:16],
		EffectiveLease: narrowed,
	}
}

// VerifyInvocation checks whether currentManifest matches the previously pinned
// contract and screens again for runtime-injected poisoning ("mid-session rug pull").
// Any drift triggers fail-closed quarantine with zero capabilities.
func (r *ContractRegistry) VerifyInvocation(currentManifest MCPToolManifest) ContractCheckResult {
	key := contractKey(currentManifest.ServerID, currentManifest.ToolName)
	schemaHash, descHash := manifestDigests(currentManifest)
	currDigest := descHash[:16] + ":" + schemaHash[:16]

	r.mu.Lock()
	defer r.mu.Unlock()

	if qReason, isQ := r.quarantined[key]; isQ {
		return ContractCheckResult{
			Allowed:         false,
			RugPullDetected: true,
			Reasons:         []string{"contract_quarantined:" + qReason},
			Current:         currDigest,
			EffectiveLease: TaskLease{
				CapMask:    0,
				IntegTaint: IntegUntrusted | IntegToolOutput,
				Flags:      IFCFlagEnforce | IFCFlagTracked,
			},
		}
	}

	pinned, ok := r.contracts[key]
	if !ok {
		return ContractCheckResult{
			Allowed: false,
			Reasons: []string{fmt.Sprintf("unpinned_mcp_contract:%s", key)},
			Current: currDigest,
			EffectiveLease: TaskLease{
				CapMask:    0,
				IntegTaint: IntegUntrusted | IntegToolOutput,
				Flags:      IFCFlagEnforce | IFCFlagTracked,
			},
		}
	}
	pinnedDigest := pinned.DescHash[:16] + ":" + pinned.SchemaHash[:16]

	if schemaHash != pinned.SchemaHash || descHash != pinned.DescHash {
		r.quarantined[key] = "rug_pull_digest_mismatch"
		return ContractCheckResult{
			Allowed:           false,
			RugPullDetected:   true,
			PoisoningDetected: len(ScanManifestPoisoning(currentManifest)) > 0,
			Reasons:           []string{"mid_session_rug_pull_digest_mismatch"},
			Pinned:            pinnedDigest,
			Current:           currDigest,
			EffectiveLease: TaskLease{
				RunID:      pinned.Lease.RunID,
				ConfTaint:  pinned.Lease.ConfTaint,
				IntegTaint: pinned.Lease.IntegTaint | IntegUntrusted,
				CapMask:    0,
				Flags:      IFCFlagEnforce | IFCFlagTracked,
			},
		}
	}

	if currentManifest.DeclaredCaps != pinned.Lease.CapMask &&
		(currentManifest.DeclaredCaps & ^pinned.Lease.CapMask) != 0 {
		r.quarantined[key] = "rug_pull_cap_escalation"
		return ContractCheckResult{
			Allowed:          false,
			RugPullDetected:  true,
			EscalationDenied: true,
			Reasons:          []string{"mid_session_capability_escalation"},
			Pinned:           pinnedDigest,
			Current:          currDigest,
			EffectiveLease: TaskLease{
				RunID:      pinned.Lease.RunID,
				IntegTaint: pinned.Lease.IntegTaint | IntegUntrusted,
				CapMask:    0,
				Flags:      IFCFlagEnforce | IFCFlagTracked,
			},
		}
	}

	return ContractCheckResult{
		Allowed:        true,
		Pinned:         pinnedDigest,
		Current:        currDigest,
		EffectiveLease: pinned.Lease,
	}
}
