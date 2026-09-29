// Package agentboundary implements a conservative finite, exact-grant subset
// check for offline policy review. It is not NVIDIA OpenShell's SMT prover and
// does not enforce any host/kernel permissions.
package agentboundary

import (
	"fmt"
	"net"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	MaxLineage         = 16
	MaxGrants          = 256
	MaxCounterexamples = 32
)

var nameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)
var domainRE = regexp.MustCompile(`^[a-z0-9-]+(?:\.[a-z0-9-]+)*$`)

type Grant struct {
	Domain    string `json:"domain"`            // filesystem, network, tool, model, credential
	Resource  string `json:"resource"`          // exact path, executable name or host:port
	Operation string `json:"operation"`         // read/write, CONNECT, METHOD:/path, EXEC, INFER, INJECT
	Subject   string `json:"subject,omitempty"` // required exact credential profile or binary for network/model
}

type Policy struct {
	ID          string  `json:"id"`
	ParentID    string  `json:"parentId,omitempty"`
	Generation  uint64  `json:"generation"`
	UID         uint32  `json:"uid"`
	ExpiresAtMS int64   `json:"expiresAtMs,omitempty"`
	Grants      []Grant `json:"grants"`
}

type Counterexample struct {
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
	Grant  *Grant `json:"grant,omitempty"`
}

type Report struct {
	SchemaVersion    string           `json:"schemaVersion"`
	Status           string           `json:"status"` // within_boundary / exceeds_boundary / unsupported
	Coverage         []string         `json:"coverage"`
	Counterexamples  []Counterexample `json:"counterexamples"`
	NewGrants        []Grant          `json:"newGrants,omitempty"`
	ReviewReasons    []string         `json:"reviewReasons,omitempty"`
	RequiresApproval bool             `json:"requiresApproval"`
	Applied          bool             `json:"applied"`
}

func validHostPort(raw string) bool {
	host, port, err := net.SplitHostPort(raw)
	if err != nil || host == "" || strings.ContainsAny(host, "*?@/") {
		return false
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil && (!domainRE.MatchString(host) || strings.ToLower(host) != host) {
		return false
	}
	if ip != nil && ip.String() != host {
		return false
	}
	return true
}

func validOperation(domain, operation string) bool {
	switch domain {
	case "filesystem":
		return operation == "read" || operation == "write"
	case "tool":
		return operation == "EXEC"
	case "model":
		return operation == "INFER"
	case "credential":
		return operation == "INJECT"
	case "network":
		if operation == "CONNECT" {
			return true
		}
		parts := strings.SplitN(operation, ":", 2)
		if len(parts) != 2 {
			return false
		}
		switch parts[0] {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		default:
			return false
		}
		p := parts[1]
		return strings.HasPrefix(p, "/") && !strings.ContainsAny(p, "*?#\\") && path.Clean(p) == p
	default:
		return false
	}
}

func validGrant(g Grant) bool {
	if !validOperation(g.Domain, g.Operation) {
		return false
	}
	if len(g.Resource) == 0 || len(g.Resource) > 256 || strings.ContainsAny(g.Resource, "*?") {
		return false
	}
	switch g.Domain {
	case "filesystem":
		if g.Subject != "" || !strings.HasPrefix(g.Resource, "/") {
			return false
		}
		return path.Clean(g.Resource) == g.Resource && !strings.Contains(g.Resource, "\\")
	case "network", "model":
		return validHostPort(g.Resource) && (g.Subject == "" || nameRE.MatchString(g.Subject))
	case "credential":
		return validHostPort(g.Resource) && nameRE.MatchString(g.Subject)
	case "tool":
		return nameRE.MatchString(g.Resource) && g.Subject == ""
	default:
		return false
	}
}

func grantKey(g Grant) string {
	return g.Domain + "\x00" + g.Resource + "\x00" + g.Operation + "\x00" + g.Subject
}

func validatePolicy(p Policy) string {
	if !nameRE.MatchString(p.ID) || (p.ParentID != "" && !nameRE.MatchString(p.ParentID)) || p.ID == p.ParentID {
		return "invalid policy or parent identity"
	}
	if p.Generation == 0 || p.UID == 0 || p.ExpiresAtMS < 0 {
		return "invalid generation, UID or expiry"
	}
	if len(p.Grants) > MaxGrants {
		return "too many grants"
	}
	seen := make(map[string]bool, len(p.Grants))
	for _, grant := range p.Grants {
		if !validGrant(grant) {
			return "unsupported or ambiguous grant"
		}
		key := grantKey(grant)
		if seen[key] {
			return "duplicate grant"
		}
		seen[key] = true
	}
	return ""
}

func newReport() Report {
	return Report{SchemaVersion: "agent-boundary.v1", Status: "within_boundary",
		Coverage:        []string{"filesystem_exact", "network_exact", "tool_exact", "model_exact", "credential_exact", "identity", "expiry"},
		Counterexamples: make([]Counterexample, 0)}
}

func addExample(r *Report, actor, reason string, grant *Grant) {
	r.Status = "exceeds_boundary"
	r.RequiresApproval = true
	if len(r.Counterexamples) < MaxCounterexamples {
		r.Counterexamples = append(r.Counterexamples, Counterexample{Actor: actor, Reason: reason, Grant: grant})
	}
}

func unsupported(r *Report, actor, reason string) {
	r.Status = "unsupported"
	r.RequiresApproval = true
	if len(r.Counterexamples) < MaxCounterexamples {
		r.Counterexamples = append(r.Counterexamples, Counterexample{Actor: actor, Reason: reason})
	}
}

// CheckLineage checks root -> descendant. It never merges grants across
// ancestors: each child must be an exact subset of its immediate parent.
func CheckLineage(chain []Policy) Report {
	r := newReport()
	if len(chain) == 0 || len(chain) > MaxLineage {
		unsupported(&r, "", "lineage must have 1-16 policies")
		return r
	}
	for i, p := range chain {
		if reason := validatePolicy(p); reason != "" {
			unsupported(&r, p.ID, reason)
			return r
		}
		if i == 0 {
			if p.ParentID != "" {
				unsupported(&r, p.ID, "root has an external parent")
				return r
			}
			continue
		}
		parent := chain[i-1]
		if p.ParentID != parent.ID {
			unsupported(&r, p.ID, "lineage is not a direct parent-to-child chain")
			return r
		}
		if p.UID != parent.UID {
			// Do not claim equivalence for a different user without OS proof.
			unsupported(&r, p.ID, "UID changes require sandbox identity proof")
			return r
		}
		if parent.ExpiresAtMS != 0 && (p.ExpiresAtMS == 0 || p.ExpiresAtMS > parent.ExpiresAtMS) {
			addExample(&r, p.ID, "child expiry exceeds parent authority", nil)
		}
		caps := make(map[string]struct{}, len(parent.Grants))
		for _, grant := range parent.Grants {
			caps[grantKey(grant)] = struct{}{}
		}
		for _, grant := range p.Grants {
			if _, ok := caps[grantKey(grant)]; !ok {
				g := grant
				addExample(&r, p.ID, "child adds authority absent from immediate parent", &g)
			}
		}
	}
	return r
}

// CheckUpdate checks both lineage and an exact-grant change of the final
// actor. New authority is reviewable even if within the operator's maximum.
func CheckUpdate(chain []Policy, previous *Policy) Report {
	r := CheckLineage(chain)
	if r.Status == "unsupported" || previous == nil || len(chain) == 0 {
		return r
	}
	candidate := chain[len(chain)-1]
	if reason := validatePolicy(*previous); reason != "" {
		unsupported(&r, previous.ID, "invalid previous policy: "+reason)
		return r
	}
	if previous.ID != candidate.ID || previous.ParentID != candidate.ParentID || previous.UID != candidate.UID {
		unsupported(&r, candidate.ID, "policy identity is immutable across revisions")
		return r
	}
	if candidate.Generation <= previous.Generation {
		addExample(&r, candidate.ID, "generation rollback or replay", nil)
	}
	seen := make(map[string]struct{}, len(previous.Grants))
	for _, grant := range previous.Grants {
		seen[grantKey(grant)] = struct{}{}
	}
	for _, grant := range candidate.Grants {
		if _, ok := seen[grantKey(grant)]; !ok {
			r.NewGrants = append(r.NewGrants, grant)
		}
	}
	sort.Slice(r.NewGrants, func(i, j int) bool { return grantKey(r.NewGrants[i]) < grantKey(r.NewGrants[j]) })
	if len(r.NewGrants) > 0 {
		r.RequiresApproval = true
		r.ReviewReasons = append(r.ReviewReasons, "new_access_grants")
	}
	if previous.ExpiresAtMS != 0 && (candidate.ExpiresAtMS == 0 || candidate.ExpiresAtMS > previous.ExpiresAtMS) {
		r.RequiresApproval = true
		r.ReviewReasons = append(r.ReviewReasons, "authority_lifetime_expanded")
	}
	return r
}

// HasGrant uses exact tuple matching: no wildcard, implicit CONNECT=>REST,
// path prefix or credential transposition can silently broaden access.
func HasGrant(p Policy, g Grant) bool {
	if !validGrant(g) {
		return false
	}
	for _, allowed := range p.Grants {
		if allowed == g {
			return true
		}
	}
	return false
}

func GrantString(g Grant) string {
	return fmt.Sprintf("%s %s %s [%s]", g.Domain, g.Operation, g.Resource, g.Subject)
}
