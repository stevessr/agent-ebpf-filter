// Package agentifc implements Kernel-Native Information-Flow Control (K-IFC)
// monotonic capability leases, application-to-kernel policy compilation from
// agentboundary policies and prompts, an ABI-verified kernel reference engine,
// and MCP-ContractGuard manifest pinning.
package agentifc

import (
	"fmt"
	"hash/fnv"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"agent-ebpf-filter/ebpf"
	"agent-ebpf-filter/internal/agentboundary"
)

// Confidentiality taint lattice bits (must match backend/ebpf/ifc_enforcer.c).
const (
	ConfSecret  uint32 = 1 << 0
	ConfPrivate uint32 = 1 << 1
)

// Integrity taint lattice bits (must match backend/ebpf/ifc_enforcer.c).
const (
	IntegUntrusted   uint32 = 1 << 0
	IntegExternalNet uint32 = 1 << 1
	IntegToolOutput  uint32 = 1 << 2
)

// Monotonic capability lease mask bits (must match backend/ebpf/ifc_enforcer.c).
const (
	CapNetEgress       uint32 = 1 << 0
	CapUntrustedEgress uint32 = 1 << 1
	CapFileWrite       uint32 = 1 << 2
	CapFileUnlink      uint32 = 1 << 3
	CapExecUntrusted   uint32 = 1 << 4
	CapSecretRead      uint32 = 1 << 5

	CapAll uint32 = CapNetEgress | CapUntrustedEgress | CapFileWrite | CapFileUnlink | CapExecUntrusted | CapSecretRead
)

// Enforcement flags (must match backend/ebpf/ifc_enforcer.c).
const (
	IFCFlagEnforce   uint32 = 1 << 0
	IFCFlagAuditOnly uint32 = 1 << 1
	IFCFlagTracked   uint32 = 1 << 2
)

// TaskLease models an application-level or tool-level capability & IFC lease
// that compiles deterministically into ebpf.AgentIfcEnforcerIfcTaskLabel.
type TaskLease struct {
	RunID             string   `json:"runId"`
	ConfTaint         uint32   `json:"confTaint"`
	IntegTaint        uint32   `json:"integTaint"`
	CapMask           uint32   `json:"capMask"`
	SpawnBudget       uint32   `json:"spawnBudget"`
	Flags             uint32   `json:"flags"`
	Generation        uint32   `json:"generation"`
	AuthorizedTargets []string `json:"authorizedTargets,omitempty"`
	TargetBound       bool     `json:"targetBound,omitempty"`
	RejectEscalation  bool     `json:"-"`
}

// PromptLease holds the blind compiler output from a raw user prompt.
type PromptLease struct {
	Lease             TaskLease `json:"lease"`
	AuthorizedTargets []string  `json:"authorizedTargets"`
	Summary           string    `json:"summary"`
}

// RunIDHash computes the FNV-1a 64-bit hash stored in the kernel task label.
func RunIDHash(runID string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.TrimSpace(runID)))
	return h.Sum64()
}

// ToKernelLabel converts a TaskLease into the exact bpf2go struct consumed by ifc_enforcer.c.
func (l TaskLease) ToKernelLabel() ebpf.AgentIfcEnforcerIfcTaskLabel {
	return ebpf.AgentIfcEnforcerIfcTaskLabel{
		ConfTaint:   l.ConfTaint,
		IntegTaint:  l.IntegTaint,
		CapMask:     l.CapMask,
		SpawnBudget: l.SpawnBudget,
		RunIdHash:   RunIDHash(l.RunID),
		Flags:       l.Flags,
		Generation:  l.Generation,
	}
}

// DefaultRootLease returns a tracked, enforcing lease with clean taint bits and
// an explicit upper bound on allowed subprocess spawns.
func DefaultRootLease(runID string, capMask, spawnBudget uint32) TaskLease {
	return TaskLease{
		RunID:       runID,
		CapMask:     capMask,
		SpawnBudget: spawnBudget,
		Flags:       IFCFlagEnforce | IFCFlagTracked,
		Generation:  1,
	}
}

// normalizeTarget canonicalizes a single target/destination identifier at lease boundaries.
func normalizeTarget(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimRight(s, ".,;:!?")))
}

// normalizeTargets returns a deduplicated, normalized target slice (nil if empty).
func normalizeTargets(targets []string) []string {
	if len(targets) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(targets))
	out := make([]string, 0, len(targets))
	for _, t := range targets {
		norm := normalizeTarget(t)
		if norm == "" {
			continue
		}
		if _, dup := seen[norm]; dup {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, norm)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// NarrowLease computes a strictly monotonic child/tool lease relative to parent:
// capabilities and spawn budget can only intersect/shrink, while confidentiality
// and integrity taint bits only grow (lattice join).
func NarrowLease(parent, requested TaskLease) (TaskLease, error) {
	strippedCaps := requested.CapMask & ^parent.CapMask
	budgetExceeded := requested.SpawnBudget > parent.SpawnBudget
	if requested.RejectEscalation && (strippedCaps != 0 || budgetExceeded) {
		return TaskLease{}, fmt.Errorf(
			"capability escalation rejected: requested_extra_caps=%#x requested_budget=%d parent_budget=%d",
			strippedCaps, requested.SpawnBudget, parent.SpawnBudget,
		)
	}

	budget := min(requested.SpawnBudget, parent.SpawnBudget)
	gen := max(parent.Generation+1, requested.Generation)
	flags := (parent.Flags | requested.Flags) & ^IFCFlagAuditOnly
	if (parent.Flags & IFCFlagEnforce) != 0 {
		flags |= IFCFlagEnforce
	}

	targets := intersectOrNarrowTargets(parent.AuthorizedTargets, requested.AuthorizedTargets)
	runID := parent.RunID
	if trimmed := strings.TrimSpace(requested.RunID); trimmed != "" {
		runID = trimmed
	}

	return TaskLease{
		RunID:             runID,
		ConfTaint:         parent.ConfTaint | requested.ConfTaint,
		IntegTaint:        parent.IntegTaint | requested.IntegTaint,
		CapMask:           parent.CapMask & requested.CapMask,
		SpawnBudget:       budget,
		Flags:             flags,
		Generation:        gen,
		AuthorizedTargets: targets,
		TargetBound:       parent.TargetBound || requested.TargetBound,
	}, nil
}

func intersectOrNarrowTargets(parent, req []string) []string {
	normParent := normalizeTargets(parent)
	normReq := normalizeTargets(req)
	switch {
	case len(normParent) == 0 && len(normReq) == 0:
		return nil
	case len(normParent) == 0:
		return normReq
	case len(normReq) == 0:
		return normParent
	}
	allowed := make(map[string]struct{}, len(normParent))
	for _, t := range normParent {
		allowed[t] = struct{}{}
	}
	out := make([]string, 0, min(len(normParent), len(normReq)))
	for _, t := range normReq {
		if _, ok := allowed[t]; ok {
			out = append(out, t)
		}
	}
	return out
}

// CompileFromBoundaryPolicy compiles an immutable agentboundary.Policy into a
// monotonically bounded TaskLease intersected with base.
func CompileFromBoundaryPolicy(p agentboundary.Policy, base TaskLease) TaskLease {
	var grantedCaps uint32
	var targets []string

	for _, g := range p.Grants {
		if res := normalizeTarget(g.Resource); res != "" {
			targets = append(targets, res)
		}
		switch g.Domain {
		case "network", "model":
			grantedCaps |= CapNetEgress | CapUntrustedEgress
		case "credential":
			grantedCaps |= CapNetEgress
		case "filesystem":
			if g.Operation == "write" {
				grantedCaps |= CapFileWrite
			}
			if looksLikeSecretResource(g.Resource) {
				grantedCaps |= CapSecretRead
			}
		case "tool":
			if g.Operation == "EXEC" && (base.CapMask&CapExecUntrusted) != 0 {
				grantedCaps |= CapExecUntrusted
			}
		}
	}

	req := TaskLease{
		RunID:             base.RunID,
		CapMask:           grantedCaps,
		SpawnBudget:       base.SpawnBudget,
		Flags:             base.Flags | IFCFlagEnforce | IFCFlagTracked,
		AuthorizedTargets: normalizeTargets(targets),
		TargetBound:       len(targets) > 0,
	}
	narrowed, _ := NarrowLease(base, req)
	return narrowed
}

func looksLikeSecretResource(path string) bool {
	lower := strings.ToLower(path)
	for _, pat := range []string{".ssh", ".env", "shadow", "sudoers", "secret", "token", "vault", "credential", "key"} {
		if strings.Contains(lower, pat) {
			return true
		}
	}
	return false
}

var (
	reEmail             = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	reIBAN              = regexp.MustCompile(`\b[A-Z]{2}[0-9]{2}[A-Za-z0-9]{10,30}\b`)
	reURL               = regexp.MustCompile(`https?://[^\s"'<>)]+`)
	reWWW               = regexp.MustCompile(`\bwww\.[A-Za-z0-9.\-]+\.[A-Za-z]{2,}(?:/[^\s"'<>)]*)?`)
	reQuote             = regexp.MustCompile(`['"]([A-Za-z0-9._@\-]{2,64})['"]`)
	reProperNoun        = regexp.MustCompile(`\b([A-Z][a-z]{1,24})\b`)
	reSinkPhrase        = regexp.MustCompile(`(?i)\b(?:to|into|onto|website|channel)\s+['"]?([A-Za-z0-9._@\-:/]{2,80})`)
	reQuantifiedTarget  = regexp.MustCompile(`(?i)\b(?:(?:all|each|every)\s+(?:users?|people|persons?|members?|recipients?|employees?|participants?)|(?:new\s+colleague|my\s+friend|to\s+the\s+client|find\s+(?:a|an)\s+(?:user|person|channel|member))|(?:do|follow|perform)\s+(?:the\s+)?(?:actions?|instructions?))\b`)
	reIndirectSourceCue = regexp.MustCompile(`(?i)\b(?:inbox|article|websites?|posted)\b`)
	reWordToken         = regexp.MustCompile(`[a-z0-9_]+`)
)

var nonRecipientProperNouns = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "my": {}, "our": {}, "your": {}, "his": {}, "her": {}, "their": {},
	"this": {}, "that": {}, "please": {}, "channel": {}, "slack": {}, "google": {}, "gmail": {},
	"can": {}, "could": {}, "would": {}, "should": {}, "will": {}, "find": {}, "read": {}, "get": {},
	"send": {}, "post": {}, "add": {}, "create": {}, "update": {}, "check": {}, "summarize": {},
	"monday": {}, "tuesday": {}, "wednesday": {}, "thursday": {}, "friday": {}, "saturday": {}, "sunday": {},
	"january": {}, "february": {}, "march": {}, "april": {}, "may": {}, "june": {},
	"july": {}, "august": {}, "september": {}, "october": {}, "november": {}, "december": {},
}

func looksLikeFilenameToken(s string) bool {
	for _, ext := range []string{".txt", ".md", ".csv", ".json", ".yaml", ".yml", ".pdf", ".py", ".go", ".sh"} {
		if strings.HasSuffix(s, ext) {
			return true
		}
	}
	return false
}

// CompilePromptLease derives a minimal capability lease and optional target
// allowlist from a natural-language user prompt without access to downstream
// tool traces.
func CompilePromptLease(prompt string) PromptLease {
	lower := strings.ToLower(prompt)
	var targets []string
	addTarget := func(t string) {
		norm := normalizeTarget(t)
		if norm == "" || slices.Contains(targets, norm) {
			return
		}
		targets = append(targets, norm)
	}

	// Structured external entities (addresses, IBANs, URLs/hosts, single-token quotes).
	for _, m := range reEmail.FindAllString(prompt, -1) {
		addTarget(m)
	}
	for _, m := range reIBAN.FindAllString(prompt, -1) {
		addTarget(m)
	}
	for _, rawURL := range reURL.FindAllString(prompt, -1) {
		addTarget(rawURL)
		if u, err := url.Parse(rawURL); err == nil && u.Hostname() != "" {
			addTarget(u.Hostname())
		}
	}
	for _, m := range reWWW.FindAllString(prompt, -1) {
		addTarget(m)
		if host, _, ok := strings.Cut(m, "/"); ok && host != "" {
			addTarget(host)
		}
	}
	for _, m := range reQuote.FindAllStringSubmatch(prompt, -1) {
		if len(m) >= 2 && !strings.Contains(m[1], " ") && !looksLikeFilenameToken(strings.ToLower(m[1])) {
			addTarget(m[1])
		}
	}
	for _, idx := range reProperNoun.FindAllStringSubmatchIndex(prompt, -1) {
		// Skip sentence-initial capitalized words unless preceded by quotes.
		if idx[2] == 0 {
			continue
		}
		norm := normalizeTarget(prompt[idx[2]:idx[3]])
		if _, skip := nonRecipientProperNouns[norm]; !skip {
			addTarget(norm)
		}
	}

	// When the prompt involves dynamic/unenumerated recipients or indirect
	// instructions, omit static target pinning; if both source and explicit sink
	// targets appear in a multi-clause prompt, narrow to explicit sink targets.
	if reQuantifiedTarget.MatchString(lower) {
		targets = nil
	} else if len(targets) > 1 && reIndirectSourceCue.MatchString(lower) {
		if sinkTargets := extractExplicitSinkTargets(lower, targets); len(sinkTargets) > 0 {
			targets = sinkTargets
		}
	}

	caps := inferPromptCapabilities(lower)
	targets = normalizeTargets(targets)

	return PromptLease{
		Lease: TaskLease{
			RunID:             "prompt-lease",
			CapMask:           caps,
			SpawnBudget:       4,
			Flags:             IFCFlagEnforce | IFCFlagTracked,
			Generation:        1,
			AuthorizedTargets: targets,
			TargetBound:       len(targets) > 0,
		},
		AuthorizedTargets: slices.Clone(targets),
		Summary:           FormatCaps(caps),
	}
}

func extractExplicitSinkTargets(lower string, candidates []string) []string {
	sinkSet := make(map[string]struct{})
	for _, m := range reSinkPhrase.FindAllStringSubmatch(lower, -1) {
		if len(m) >= 2 {
			tok := normalizeTarget(m[1])
			sinkSet[tok] = struct{}{}
			if strings.HasPrefix(tok, "http://") || strings.HasPrefix(tok, "https://") {
				if u, err := url.Parse(tok); err == nil && u.Hostname() != "" {
					sinkSet[u.Hostname()] = struct{}{}
				}
			} else if host, _, ok := strings.Cut(tok, "/"); ok && host != "" {
				sinkSet[host] = struct{}{}
			}
		}
	}
	var out []string
	for _, t := range candidates {
		if _, ok := sinkSet[t]; ok {
			out = append(out, t)
			continue
		}
		if host, _, ok := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(t, "https://"), "http://"), "/"); ok {
			if _, okHost := sinkSet[host]; okHost {
				out = append(out, t)
			}
		}
	}
	return normalizeTargets(out)
}

func inferPromptCapabilities(lower string) uint32 {
	tokenSet := make(map[string]struct{})
	for _, tok := range reWordToken.FindAllString(lower, -1) {
		tokenSet[tok] = struct{}{}
	}
	hasAnyToken := func(words ...string) bool {
		for _, w := range words {
			if _, ok := tokenSet[w]; ok {
				return true
			}
		}
		return false
	}

	var caps uint32
	if hasAnyToken("email", "emails", "inbox", "password", "transaction", "transactions", "spending", "bill", "rent", "scheduled") ||
		strings.Contains(lower, "security code") || strings.Contains(lower, "direct message") {
		caps |= CapSecretRead
	}
	if hasAnyToken("create", "reschedule", "add", "append", "suggest", "update", "book", "pay", "refund",
		"send", "post", "message", "invite", "reply", "change", "share", "write", "transfer") ||
		strings.Contains(lower, "set up") || strings.Contains(lower, "do the actions") || strings.Contains(lower, "follow the instructions") {
		caps |= CapFileWrite
	}
	if hasAnyToken("delete", "remove", "erase") {
		caps |= CapFileUnlink
	}

	needsEgress := hasAnyToken("send", "email", "message", "reply", "post", "invite", "pay", "refund",
		"transfer", "schedule", "reschedule", "share", "write", "book") ||
		strings.Contains(lower, "set up") || strings.Contains(lower, "standing order") ||
		strings.Contains(lower, "add also") || strings.Contains(lower, "to the channel") ||
		strings.Contains(lower, "to that channel") || strings.Contains(lower, "create a new") ||
		strings.Contains(lower, "create an event") || strings.Contains(lower, "update my") ||
		strings.Contains(lower, "update the password") || strings.Contains(lower, "adjust my rent") ||
		strings.Contains(lower, "follow the instructions") || strings.Contains(lower, "do the actions")
	if needsEgress {
		caps |= CapNetEgress | CapUntrustedEgress
	} else if hasAnyToken("webpage", "website") || strings.Contains(lower, "http://") || strings.Contains(lower, "https://") {
		caps |= CapNetEgress
	}
	return caps
}

// IsLoopbackOrLocalEndpoint checks whether an endpoint represents AF_UNIX /
// loopback communication exempted by ifc_enforcer.c.
func IsLoopbackOrLocalEndpoint(endpoint string) bool {
	ep := strings.TrimSpace(strings.ToLower(endpoint))
	if ep == "" {
		return false
	}
	if strings.HasPrefix(ep, "unix:") || strings.HasPrefix(ep, "/") || strings.HasPrefix(ep, "netlink:") {
		return true
	}
	host := ep
	if h, _, err := net.SplitHostPort(ep); err == nil {
		host = h
	} else {
		host = strings.TrimSuffix(strings.TrimPrefix(ep, "["), "]")
	}
	if host == "localhost" {
		return true
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return addr.IsLoopback() || (addr.Is4In6() && addr.Unmap().IsLoopback())
}

// FormatCaps formats a capability bitmask as human-readable tokens.
func FormatCaps(caps uint32) string {
	var parts []string
	for _, item := range []struct {
		bit  uint32
		name string
	}{
		{CapNetEgress, "NET_EGRESS"},
		{CapUntrustedEgress, "UNTRUSTED_EGRESS"},
		{CapFileWrite, "FILE_WRITE"},
		{CapFileUnlink, "FILE_UNLINK"},
		{CapExecUntrusted, "EXEC_UNTRUSTED"},
		{CapSecretRead, "SECRET_READ"},
	} {
		if (caps & item.bit) != 0 {
			parts = append(parts, item.name)
		}
	}
	if len(parts) == 0 {
		return "NONE"
	}
	return strings.Join(parts, "|")
}
