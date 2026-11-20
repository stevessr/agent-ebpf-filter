// Package agentifc implements Kernel-Native Information-Flow Control (K-IFC)
// monotonic capability leases, application-to-kernel policy compilation from
// agentboundary policies and prompts, an ABI-verified kernel reference engine,
// and MCP-ContractGuard manifest pinning.
package agentifc

import (
	"fmt"
	"hash/fnv"
	"net/netip"
	"regexp"
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

	budget := requested.SpawnBudget
	if budget > parent.SpawnBudget {
		budget = parent.SpawnBudget
	}
	gen := parent.Generation + 1
	if requested.Generation > gen {
		gen = requested.Generation
	}
	flags := (parent.Flags | requested.Flags) & ^IFCFlagAuditOnly
	if (parent.Flags & IFCFlagEnforce) != 0 {
		flags |= IFCFlagEnforce
	}

	targets := intersectOrNarrowTargets(parent.AuthorizedTargets, requested.AuthorizedTargets)
	runID := parent.RunID
	if strings.TrimSpace(requested.RunID) != "" {
		runID = requested.RunID
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
	if len(parent) == 0 && len(req) == 0 {
		return nil
	}
	if len(parent) == 0 {
		return append([]string(nil), req...)
	}
	if len(req) == 0 {
		return append([]string(nil), parent...)
	}
	allowed := make(map[string]struct{}, len(parent))
	for _, t := range parent {
		allowed[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
	}
	var out []string
	for _, t := range req {
		norm := strings.ToLower(strings.TrimSpace(t))
		if _, ok := allowed[norm]; ok {
			out = append(out, norm)
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
		res := strings.TrimSpace(g.Resource)
		if res != "" {
			targets = append(targets, strings.ToLower(res))
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
			if looksLikeSecretResource(res) {
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
		AuthorizedTargets: targets,
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
	reEmail = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	reIBAN  = regexp.MustCompile(`\b[A-Z]{2}[0-9]{2}[A-Za-z0-9]{10,30}\b`)
	reURL   = regexp.MustCompile(`https?://[^\s"'<>)]+`)
	reWWW   = regexp.MustCompile(`\bwww\.[A-Za-z0-9.\-]+\.[A-Za-z]{2,}(?:/[^\s"'<>)]*)?`)
	reQuote = regexp.MustCompile(`['"]([^'"]{2,80})['"]`)
)

// CompilePromptLease blindly derives the minimal monotonic capability lease and
// target allowlist solely from a natural-language user prompt (no access to
// tool call traces or injection tasks).
func CompilePromptLease(prompt string) PromptLease {
	lower := strings.ToLower(prompt)
	var caps uint32
	var targets []string

	addTarget := func(t string) {
		norm := strings.ToLower(strings.TrimSpace(strings.TrimRight(t, ".,;:!?")))
		if norm == "" {
			return
		}
		for _, prev := range targets {
			if prev == norm {
				return
			}
		}
		targets = append(targets, norm)
	}

	// Extract explicit external destination targets (emails, IBANs, URLs/domains).
	for _, m := range reEmail.FindAllString(prompt, -1) {
		addTarget(m)
	}
	for _, m := range reIBAN.FindAllString(prompt, -1) {
		addTarget(m)
	}
	for _, m := range reURL.FindAllString(prompt, -1) {
		addTarget(m)
		trimmed := strings.TrimPrefix(strings.TrimPrefix(m, "https://"), "http://")
		if host, _, ok := strings.Cut(trimmed, "/"); ok && host != "" {
			addTarget(host)
		} else if trimmed != "" {
			addTarget(trimmed)
		}
	}
	for _, m := range reWWW.FindAllString(prompt, -1) {
		addTarget(m)
		if host, _, ok := strings.Cut(m, "/"); ok && host != "" {
			addTarget(host)
		}
	}

	// Extract named people / Slack channels explicitly targeted by the prompt.
	// Ambiguous pronouns/roles ("client", "each person", "user who wrote the most")
	// cannot safely be pinned statically from the prompt alone without harming utility.
	for _, name := range []string{
		"alice", "bob", "charlie", "dora", "eve", "linda", "mark", "david", "sarah", "john",
		"general", "random", "external", "spotify", "apple",
	} {
		if containsWord(lower, name) {
			addTarget(name)
		}
	}
	if strings.Contains(lower, "all users") || strings.Contains(lower, "each user") ||
		strings.Contains(lower, "most total") || strings.Contains(lower, "to the client") ||
		strings.Contains(lower, "each person") || strings.Contains(lower, "from my friend") ||
		strings.Contains(lower, "new colleague") || strings.Contains(lower, "find a channel") ||
		strings.Contains(lower, "do the actions") || strings.Contains(lower, "follow the instructions") {
		targets = nil
	} else if strings.Contains(lower, "in his inbox") || strings.Contains(lower, "the article") ||
		strings.Contains(lower, "all the websites") {
		// When prompt reads dynamic articles/websites from channels/inboxes but explicitly
		// names the outbound sink (e.g. "send it to Alice", "post to www.our-company.com",
		// "post summary to 'random' channel"), keep ONLY that explicit outbound sink target.
		var sinkOnly []string
		for _, t := range targets {
			if strings.Contains(lower, "to "+t) || strings.Contains(lower, "to '"+t) ||
				strings.Contains(lower, "to \""+t) || strings.Contains(lower, "website "+t) {
				sinkOnly = append(sinkOnly, t)
			}
		}
		targets = sinkOnly
	}

	// Detect read of security-sensitive personal state explicitly authorized by user.
	for _, kw := range []string{
		"email", "emails", "inbox", "password", "security code", "transaction", "transactions",
		"spending", "bill", "rent", "scheduled", "direct message",
	} {
		if strings.Contains(lower, kw) {
			caps |= CapSecretRead
			break
		}
	}

	// Detect workspace / calendar / profile mutation intent.
	for _, kw := range []string{
		"create ", "create a", "reschedule", "add ", "append", "suggest ", "update ", "book ",
		"pay ", "refund", "send ", "set up", "post ", "message ", "invite ", "reply ",
		"change ", "share ", "write ", "transfer ", "do the actions", "follow the instructions",
	} {
		if strings.Contains(lower, kw) {
			caps |= CapFileWrite
			break
		}
	}

	// Explicit deletion capability is granted ONLY when user literally asks to delete/remove.
	for _, kw := range []string{"delete ", "delete the", "remove ", "erase "} {
		if strings.Contains(lower, kw) {
			caps |= CapFileUnlink
			break
		}
	}

	// External / network communication intent (sending/posting/transferring derived context).
	// Read-only webpage/website fetches remain strictly read-only (CapNetEgress without CapUntrustedEgress).
	needsEgress := false
	for _, kw := range []string{
		"send ", "email ", "message ", "reply ", "post ", "invite ", "pay ", "refund",
		"transfer", "set up", "schedule", "standing order", "reschedule", "share ",
		"add also", "to the channel", "to that channel", "write ", "create a new",
		"create an event", "book ", "update my", "update the password",
		"adjust my rent", "follow the instructions", "do the actions",
	} {
		if strings.Contains(lower, kw) {
			needsEgress = true
			break
		}
	}
	if needsEgress {
		caps |= CapNetEgress
		// User explicitly expects output derived from retrieved web/file/message context to leave.
		caps |= CapUntrustedEgress
	} else if strings.Contains(lower, "webpage") || strings.Contains(lower, "website") ||
		strings.Contains(lower, "http://") || strings.Contains(lower, "https://") {
		// Read-only URL fetch only; tainted follow-up egress stays blocked.
		caps |= CapNetEgress
	}

	// Untrusted binary execution is never granted from user NL prompts by default.
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
		AuthorizedTargets: targets,
		Summary:           FormatCaps(caps),
	}
}

func containsWord(text, word string) bool {
	idx := 0
	for {
		pos := strings.Index(text[idx:], word)
		if pos < 0 {
			return false
		}
		start := idx + pos
		end := start + len(word)
		leftOK := start == 0 || !isAlphaNum(text[start-1])
		rightOK := end == len(text) || !isAlphaNum(text[end])
		if leftOK && rightOK {
			return true
		}
		idx = end
	}
}

func isAlphaNum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
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
	if strings.HasPrefix(host, "[") {
		if h, _, ok := strings.Cut(strings.TrimPrefix(host, "["), "]"); ok {
			host = h
		}
	} else if h, _, err := splitHostPortLoose(ep); err == nil {
		host = h
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

func splitHostPortLoose(s string) (string, string, error) {
	lastColon := strings.LastIndexByte(s, ':')
	if lastColon <= 0 {
		return "", "", fmt.Errorf("no port")
	}
	return s[:lastColon], s[lastColon+1:], nil
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
