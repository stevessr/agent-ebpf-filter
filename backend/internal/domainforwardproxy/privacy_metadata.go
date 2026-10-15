package domainforwardproxy

import (
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// InspectRequestMetadata prevents leaks through URL paths, query parameters,
// and custom headers: checking only the JSON body misses these egress paths.
// Metadata cannot safely be rewritten without breaking signatures/routing;
// in redact/block mode matching metadata is denied before the upstream call.
// Authentication headers to configured AI providers are intentionally exempt
// from content inspection and must be guarded by explicit upstream routing.
func (e *PrivacyEngine) InspectRequestMetadata(request *http.Request) (PrivacyReport, error) {
	report := PrivacyReport{Action: e.settings.Action}
	if e == nil || !e.settings.Enabled || request == nil || request.URL == nil {
		return report, nil
	}
	if len(request.URL.RawQuery) > 1<<16 {
		return report, errors.New("privacy URL query too large")
	}
	if len(request.URL.EscapedPath()) > 1<<16 {
		return report, errors.New("privacy URL path too large")
	}
	check := func(name, value string, forceBlock bool) error {
		if len(value) > 1<<16 {
			return errors.New("privacy metadata field too large")
		}
		folded := privacyFoldKey(name)
		if e.settings.ContextMode == "strict" && privacyMemoryKeys[folded] {
			report.Findings = append(report.Findings, PrivacyFinding{Category: "memory_exfiltration"})
			report.Blocked = true
			return nil
		}
		if forceBlock {
			report.Findings = append(report.Findings, PrivacyFinding{Category: "metadata_sensitive"})
			report.Blocked = true
			return nil
		}
		force := e.settings.ContextMode == "strict" && privacySensitiveKeys[folded] && value != ""
		matches := e.detect(value)
		if !force && len(matches) == 0 {
			return nil
		}
		if force {
			report.Findings = append(report.Findings, PrivacyFinding{Category: "metadata_credential"})
		} else {
			for _, match := range matches {
				if len(report.Findings) >= maxPrivacyFindings {
					return errors.New("privacy metadata finding limit exceeded")
				}
				report.Findings = append(report.Findings, PrivacyFinding{Category: match.category})
			}
		}
		if e.settings.Action != "warn" {
			report.Blocked = true
		}
		return nil
	}
	if err := check("url_path", request.URL.Path, false); err != nil {
		return report, err
	}
	if report.Blocked {
		return report, nil
	}
	if request.URL.RawQuery != "" {
		parameters, err := url.ParseQuery(request.URL.RawQuery)
		if err != nil {
			return report, errors.New("privacy URL query parsing failed")
		}
		keys := make([]string, 0, len(parameters))
		for key := range parameters {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			for _, value := range parameters[key] {
				if err := check(key, value, false); err != nil {
					return report, err
				}
				if report.Blocked {
					return report, nil
				}
			}
		}
	}
	headerNames := make([]string, 0, len(request.Header))
	for key := range request.Header {
		headerNames = append(headerNames, key)
	}
	sort.Strings(headerNames)
	for _, key := range headerNames {
		folded := privacyFoldKey(key)
		switch folded {
		case "authorization", "xapikey", "apikey":
			// Provider auth must be forwarded unchanged. Do not treat these
			// fields as evidence that arbitrary headers are safe.
			continue
		case "cookie", "proxyauthorization", "setcookie":
			// Cookie material is not a normal requirement for local LLM APIs.
			// Avoid forwarding browser session credentials to a third party.
			if e.settings.ContextMode == "strict" {
				if err := check(key, "", true); err != nil {
					return report, err
				}
				return report, nil
			}
		}
		// Ignore ordinary transport metadata, not arbitrary X-* attributes.
		switch folded {
		case "accept", "acceptencoding", "contenttype", "contentlength",
			"useragent", "connection", "cachecontrol", "host",
			"contentencoding", "transferencoding", "xforwardedfor",
			"xforwardedhost", "xforwardedproto", "openaiorganization",
			"openaiproject":
			continue
		}
		for _, value := range request.Header.Values(key) {
			if err := check(key, value, false); err != nil {
				return report, err
			}
			if report.Blocked {
				return report, nil
			}
		}
	}
	return report, nil
}
