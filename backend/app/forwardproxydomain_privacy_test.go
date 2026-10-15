package app

import (
	"testing"

	"agent-ebpf-filter/core"
)

func TestDomainForwardContextPolicySurvivesCoreBoundary(t *testing.T) {
	settings := DomainForwardProxySettings{
		Privacy: core.DomainPrivacySettings{
			Enabled:     true,
			Action:      "redact",
			ContextMode: "strict",
			JSONFieldRules: []core.DomainPrivacyJSONFieldRule{{
				Path:      "/params/arguments/private_email",
				Action:    "allow",
				Recipient: "api.openai.com",
			}},
		},
	}
	internal := toInternalDomainForwardProxySettings(settings)
	if internal.Privacy.ContextMode != "strict" ||
		len(internal.Privacy.JSONFieldRules) != 1 ||
		internal.Privacy.JSONFieldRules[0].Recipient != "api.openai.com" {
		t.Fatalf("contextual disclosure settings lost in core-to-proxy conversion: %+v", internal.Privacy)
	}
	reversed := fromInternalDomainForwardProxySettings(internal)
	if reversed.Privacy.ContextMode != "strict" ||
		len(reversed.Privacy.JSONFieldRules) != 1 ||
		reversed.Privacy.JSONFieldRules[0].Path != "/params/arguments/private_email" {
		t.Fatalf("contextual disclosure settings lost in proxy-to-core conversion: %+v", reversed.Privacy)
	}
}
