package agentifc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
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

// FindingClass categorizes MCP manifest screening signals so structural
// smuggling vs imperative hijack directives can be audited distinctly.
type FindingClass string

const (
	FindingStructuralSmuggling FindingClass = "structural_smuggling"
	FindingDirectiveInjection  FindingClass = "directive_injection"
)

// PoisonFinding records a structured screening hit on an MCP tool manifest.
type PoisonFinding struct {
	Class  FindingClass `json:"class"`
	Reason string       `json:"reason"`
}

var directivePatterns = []struct {
	re     *regexp.Regexp
	reason string
}{
	{regexp.MustCompile(`(?i)<\s*/?\s*important\s*>`), "hidden_important_tag_directive"},
	{regexp.MustCompile(`(?i)<\s*/?\s*system\s*>`), "hidden_system_tag_override"},
	{regexp.MustCompile(`(?i)\b(?:ignore|disregard)\s+(?:all\s+)?(?:prior|previous)\s+(?:instructions?|constraints?|rules?)\b`), "prompt_override_directive"},
	{regexp.MustCompile(`(?i)\bdo\s+not\s+(?:tell|inform|notify|alert)\s+(?:the\s+)?user\b`), "covert_user_deception_directive"},
	{regexp.MustCompile(`(?i)\bdo\s+not\s+mention\s+(?:this|that|reading|sending|exfiltrat|the\s+(?:secret|shadow|ssh|token|key))`), "covert_user_deception_directive"},
	{regexp.MustCompile(`(?i)\bbefore\s+calling\s+this\s+tool\b`), "side_effect_prerequisite_hijack"},
	{regexp.MustCompile(`(?i)\bpass\s+its\s+contents\b`), "covert_exfiltration_directive"},
	{regexp.MustCompile(`(?i)\b(?:read|dump|send|upload|steal|copy|pass|exfiltrat\w*)\b[^.\n]{0,80}(?:~/.ssh|\.ssh/id_rsa|/etc/shadow|\.env\b)`), "credential_path_harvesting"},
	{regexp.MustCompile(`(?i)\bexfiltrat(?:e|ing|ion)\s+(?:workspace\s+)?(?:secrets?|credentials?|keys?|tokens?|files?)\b`), "explicit_exfiltration_directive"},
	{regexp.MustCompile(`(?i)["']sidenote["']\s*:|(?:\bin\s+sidenote\b)`), "hidden_sidenote_directive"},
}

// AnalyzeManifestPoisoning returns structured smuggling and imperative-hijack
// findings without blocking benign tools that merely document sensitive paths
// or security terminology.
func AnalyzeManifestPoisoning(m MCPToolManifest) []PoisonFinding {
	var findings []PoisonFinding
	seen := make(map[string]struct{})
	add := func(class FindingClass, reason string) {
		if _, dup := seen[reason]; dup {
			return
		}
		seen[reason] = struct{}{}
		findings = append(findings, PoisonFinding{Class: class, Reason: reason})
	}

	combined := m.Description + "\n" + m.ParameterDescriptions + "\n" + m.InputSchemaJSON
	for _, p := range directivePatterns {
		if p.re.MatchString(combined) {
			class := FindingDirectiveInjection
			if strings.HasPrefix(p.reason, "hidden_") && strings.Contains(p.reason, "_tag_") {
				class = FindingStructuralSmuggling
			}
			add(class, p.reason)
		}
	}

	zwCount := 0
	for _, r := range combined {
		switch {
		case r >= 0xE0000 && r <= 0xE007F:
			add(FindingStructuralSmuggling, "unicode_tag_plane_smuggling")
		case (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069):
			add(FindingStructuralSmuggling, "bidi_trojan_override")
		case r == 0x200B || r == 0x200C || r == 0x200D || r == 0x200E || r == 0x200F || r == 0xFEFF:
			zwCount++
		}
	}
	if zwCount >= 3 {
		add(FindingStructuralSmuggling, "zero_width_steganography")
	}
	return findings
}

// ScanManifestPoisoning inspects tool descriptions and parameter schemas for
// imperative overrides, covert exfiltration directives, Unicode Tag-Plane
// (U+E0000-U+E007F) invisible instructions, BiDi overrides, and zero-width clusters.
func ScanManifestPoisoning(m MCPToolManifest) []string {
	findings := AnalyzeManifestPoisoning(m)
	if len(findings) == 0 {
		return nil
	}
	reasons := make([]string, len(findings))
	for i, f := range findings {
		reasons[i] = f.Reason
	}
	return reasons
}

func quarantinedLease(runID string, conf, integ uint32) TaskLease {
	return TaskLease{
		RunID:      runID,
		ConfTaint:  conf,
		IntegTaint: integ | IntegUntrusted | IntegToolOutput,
		CapMask:    0,
		Flags:      IFCFlagEnforce | IFCFlagTracked,
	}
}

// IsQuarantined reports whether (serverID, toolName) is currently quarantined
// and returns the recorded reason.
func (r *ContractRegistry) IsQuarantined(serverID, toolName string) (string, bool) {
	key := contractKey(serverID, toolName)
	r.mu.RLock()
	defer r.mu.RUnlock()
	reason, ok := r.quarantined[key]
	return reason, ok
}

// ClearQuarantine revokes an active session quarantine for (serverID, toolName)
// after operator/policy re-vetting, returning whether an entry existed.
func (r *ContractRegistry) ClearQuarantine(serverID, toolName string) bool {
	key := contractKey(serverID, toolName)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.quarantined[key]; !ok {
		return false
	}
	delete(r.quarantined, key)
	delete(r.contracts, key)
	return true
}

// RePinManifest clears any prior quarantine after explicit re-authorization and
// pins m monotonically under parentLease.
func (r *ContractRegistry) RePinManifest(parentLease TaskLease, m MCPToolManifest) ContractCheckResult {
	r.ClearQuarantine(m.ServerID, m.ToolName)
	return r.PinManifest(parentLease, m)
}

// PinManifest validates m against poisoning attacks and parentLease bounds,
// cryptographically pins its digests, and returns a narrowed child TaskLease.
func (r *ContractRegistry) PinManifest(parentLease TaskLease, m MCPToolManifest) ContractCheckResult {
	key := contractKey(m.ServerID, m.ToolName)
	if poisonReasons := ScanManifestPoisoning(m); len(poisonReasons) > 0 {
		r.mu.Lock()
		r.quarantined[key] = strings.Join(poisonReasons, ",")
		r.mu.Unlock()
		return ContractCheckResult{
			Allowed:           false,
			PoisoningDetected: true,
			Reasons:           poisonReasons,
			EffectiveLease:    quarantinedLease(parentLease.RunID, parentLease.ConfTaint, parentLease.IntegTaint),
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
			EffectiveLease:   quarantinedLease(parentLease.RunID, parentLease.ConfTaint, parentLease.IntegTaint),
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
			EffectiveLease:  quarantinedLease(parentLease.RunID, parentLease.ConfTaint, parentLease.IntegTaint),
		}
	}
	r.contracts[key] = PinnedContract{
		Key:        key,
		SchemaHash: schemaHash,
		DescHash:   descHash,
		Lease:      narrowed,
	}
	digest := descHash[:16] + ":" + schemaHash[:16]
	return ContractCheckResult{
		Allowed:        true,
		Pinned:         digest,
		Current:        digest,
		EffectiveLease: narrowed,
	}
}

// VerifyInvocation checks whether currentManifest matches the previously pinned
// contract and screens again for runtime-injected poisoning ("mid-session rug pull").
// Drift places the contract in quarantine until explicitly re-vetted via
// ClearQuarantine or RePinManifest.
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
			EffectiveLease:  quarantinedLease("", 0, 0),
		}
	}

	pinned, ok := r.contracts[key]
	if !ok {
		return ContractCheckResult{
			Allowed:        false,
			Reasons:        []string{fmt.Sprintf("unpinned_mcp_contract:%s", key)},
			Current:        currDigest,
			EffectiveLease: quarantinedLease("", 0, 0),
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
			EffectiveLease:    quarantinedLease(pinned.Lease.RunID, pinned.Lease.ConfTaint, pinned.Lease.IntegTaint),
		}
	}

	if (currentManifest.DeclaredCaps & ^pinned.Lease.CapMask) != 0 {
		r.quarantined[key] = "rug_pull_cap_escalation"
		return ContractCheckResult{
			Allowed:          false,
			RugPullDetected:  true,
			EscalationDenied: true,
			Reasons:          []string{"mid_session_capability_escalation"},
			Pinned:           pinnedDigest,
			Current:          currDigest,
			EffectiveLease:   quarantinedLease(pinned.Lease.RunID, pinned.Lease.ConfTaint, pinned.Lease.IntegTaint),
		}
	}

	lease := pinned.Lease
	lease.AuthorizedTargets = slices.Clone(pinned.Lease.AuthorizedTargets)
	return ContractCheckResult{
		Allowed:        true,
		Pinned:         pinnedDigest,
		Current:        currDigest,
		EffectiveLease: lease,
	}
}
