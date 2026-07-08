package app

import (
	"log"

	"agent-ebpf-filter/redaction"
	"agent-ebpf-filter/redaction/rules"
	"agent-ebpf-filter/pb"
)

// ── Global redaction engine ──────────────────────────────────────────────────

var globalRedactionEngine *redaction.RedactionEngine

// initRedactionEngine creates and configures the global redaction engine.
func initRedactionEngine() {
	policy := runtimeSettingsStore.Snapshot().RedactionPolicy
	if policy.Level == "" {
		policy.Level = redaction.RedactionLevelStandard
	}
	if policy.DefaultPlaceholder == "" {
		policy.DefaultPlaceholder = "[REDACTED]"
	}
	globalRedactionEngine = redaction.NewRedactionEngine(policy)
	log.Printf("[REDACTION] engine initialized: level=%s, rules=%d", policy.Level, len(policy.Rules))
}

// ── Event-level redaction ────────────────────────────────────────────────────

// redactEvent applies the configured redaction policy to an event before broadcast.
func redactEvent(event *pb.Event, engine *redaction.RedactionEngine) {
	if event == nil || engine == nil {
		return
	}

	event.Comm = engine.ApplyRules(event.Comm, redaction.FieldCategoryIdentifier)
	event.Path = redactPathWithRules(event.Path, engine)
	event.ExtraPath = redactPathWithRules(event.ExtraPath, engine)
	event.ExtraInfo = applyContentRedaction(event.ExtraInfo, engine)
	event.NetEndpoint = engine.ApplyRules(event.NetEndpoint, redaction.FieldCategoryNetwork)
	event.Domain = engine.ApplyRules(event.Domain, redaction.FieldCategoryNetwork)
	event.DstIp = engine.ApplyRules(event.DstIp, redaction.FieldCategoryNetwork)
	event.SrcIp = engine.ApplyRules(event.SrcIp, redaction.FieldCategoryNetwork)
	event.DnsName = engine.ApplyRules(event.DnsName, redaction.FieldCategoryNetwork)
	event.Cwd = redactPathWithRules(event.Cwd, engine)

	if event.RedactionLevel == "" {
		event.RedactionLevel = string(engine.PolicyLevel())
	}
}

// redactEnvelopeEvent applies redaction to an EventEnvelope including its payload.
func redactEnvelopeEvent(envelope *pb.EventEnvelope, engine *redaction.RedactionEngine) {
	if envelope == nil || engine == nil {
		return
	}

	envelope.Comm = engine.ApplyRules(envelope.Comm, redaction.FieldCategoryIdentifier)
	envelope.Cwd = redactPathWithRules(envelope.Cwd, engine)

	if envelope.Payload != nil {
		switch v := envelope.Payload.(type) {
		case *pb.EventEnvelope_ExecEvent:
			v.ExecEvent.Path = redactPathWithRules(v.ExecEvent.Path, engine)
			v.ExecEvent.ExtraInfo = applyContentRedaction(v.ExecEvent.ExtraInfo, engine)
		case *pb.EventEnvelope_FileEvent:
			v.FileEvent.Path = redactPathWithRules(v.FileEvent.Path, engine)
			v.FileEvent.ExtraPath = redactPathWithRules(v.FileEvent.ExtraPath, engine)
			v.FileEvent.ExtraInfo = applyContentRedaction(v.FileEvent.ExtraInfo, engine)
		case *pb.EventEnvelope_NetworkEvent:
			v.NetworkEvent.Endpoint = engine.ApplyRules(v.NetworkEvent.Endpoint, redaction.FieldCategoryNetwork)
			v.NetworkEvent.DstIp = engine.ApplyRules(v.NetworkEvent.DstIp, redaction.FieldCategoryNetwork)
			v.NetworkEvent.SrcIp = engine.ApplyRules(v.NetworkEvent.SrcIp, redaction.FieldCategoryNetwork)
			v.NetworkEvent.DnsName = engine.ApplyRules(v.NetworkEvent.DnsName, redaction.FieldCategoryNetwork)
			v.NetworkEvent.Sni = engine.ApplyRules(v.NetworkEvent.Sni, redaction.FieldCategoryNetwork)
		case *pb.EventEnvelope_WrapperEvent:
			v.WrapperEvent.CommandLine = applyContentRedaction(v.WrapperEvent.CommandLine, engine)
		case *pb.EventEnvelope_TlsEvent:
			v.TlsEvent.Url = applyContentRedaction(v.TlsEvent.Url, engine)
			v.TlsEvent.Host = engine.ApplyRules(v.TlsEvent.Host, redaction.FieldCategoryNetwork)
		case *pb.EventEnvelope_HttpEvent:
			v.HttpEvent.Url = applyContentRedaction(v.HttpEvent.Url, engine)
			v.HttpEvent.Host = engine.ApplyRules(v.HttpEvent.Host, redaction.FieldCategoryNetwork)
		}
	}
}

// ── Field-level helpers ──────────────────────────────────────────────────────

func redactPathWithRules(path string, engine *redaction.RedactionEngine) string {
	if path == "" {
		return path
	}
	level := engine.PolicyLevel()
	redacted := rules.RedactPath(path, level)
	redacted = rules.RedactPII(redacted, level)
	return engine.ApplyRules(redacted, redaction.FieldCategoryPath)
}

// applyContentRedaction applies content-level redaction (PII, secrets, credentials, entropy).
func applyContentRedaction(text string, engine *redaction.RedactionEngine) string {
	if text == "" {
		return text
	}
	level := engine.PolicyLevel()

	redacted := text
	redacted = rules.RedactCredentials(redacted, level)
	redacted = rules.RedactPII(redacted, level)
	redacted = rules.RedactGitleaks(redacted, level)
	redacted = rules.RedactContextSecret(redacted, level)
	redacted = rules.RedactHighEntropy(redacted, level)
	return redacted
}