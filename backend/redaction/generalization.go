package redaction

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"time"
)

// Generalizer implements data generalization techniques to reduce precision
// while maintaining statistical utility. This is GDPR-compliant anonymization.
type Generalizer struct {
	config GeneralizationConfig
}

// GeneralizationConfig configures generalization behavior.
type GeneralizationConfig struct {
	IPPrecision        IPPrecisionLevel        // How to generalize IP addresses
	TimePrecision      TimePrecisionLevel      // How to generalize timestamps
	PathGeneralization PathGeneralizationLevel // How to generalize file paths
	Enabled            bool
}

// IPPrecisionLevel controls IP address generalization.
type IPPrecisionLevel string

const (
	IPPrecisionFull   IPPrecisionLevel = "full"    // Keep full IP
	IPPrecisionSubnet IPPrecisionLevel = "subnet"  // Keep subnet (e.g., 192.168.1.0/24)
	IPPrecisionClass  IPPrecisionLevel = "class"   // Keep class (e.g., 192.168.0.0/16)
	IPPrecisionNone   IPPrecisionLevel = "none"    // Remove completely
)

// TimePrecisionLevel controls timestamp generalization.
type TimePrecisionLevel string

const (
	TimePrecisionFull   TimePrecisionLevel = "full"   // Keep full timestamp
	TimePrecisionMinute TimePrecisionLevel = "minute" // Round to minute
	TimePrecisionHour   TimePrecisionLevel = "hour"   // Round to hour
	TimePrecisionDay    TimePrecisionLevel = "day"    // Round to day
	TimePrecisionMonth  TimePrecisionLevel = "month"  // Round to month
)

// PathGeneralizationLevel controls file path generalization.
type PathGeneralizationLevel string

const (
	PathGeneralizationFull    PathGeneralizationLevel = "full"    // Keep full path
	PathGeneralizationPattern PathGeneralizationLevel = "pattern" // Replace specific parts with wildcards
	PathGeneralizationBase    PathGeneralizationLevel = "base"    // Keep only basename
	PathGeneralizationNone    PathGeneralizationLevel = "none"    // Remove completely
)

// NewGeneralizer creates a new generalizer with the given configuration.
func NewGeneralizer(config GeneralizationConfig) *Generalizer {
	return &Generalizer{
		config: config,
	}
}

// GeneralizeIP reduces precision of an IP address.
func (g *Generalizer) GeneralizeIP(ip string) string {
	if !g.config.Enabled {
		return ip
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip // Not a valid IP
	}

	switch g.config.IPPrecision {
	case IPPrecisionFull:
		return ip

	case IPPrecisionSubnet:
		// IPv4: Keep /24 (e.g., 192.168.1.100 → 192.168.1.0/24)
		// IPv6: Keep /64
		if parsed.To4() != nil {
			// IPv4
			parts := strings.Split(ip, ".")
			if len(parts) == 4 {
				return fmt.Sprintf("%s.%s.%s.0/24", parts[0], parts[1], parts[2])
			}
		} else {
			// IPv6 - keep first 64 bits
			return fmt.Sprintf("%s/64", parsed.String()[:19])
		}
		return ip

	case IPPrecisionClass:
		// IPv4: Keep /16 (e.g., 192.168.1.100 → 192.168.0.0/16)
		// IPv6: Keep /48
		if parsed.To4() != nil {
			parts := strings.Split(ip, ".")
			if len(parts) == 4 {
				return fmt.Sprintf("%s.%s.0.0/16", parts[0], parts[1])
			}
		} else {
			return fmt.Sprintf("%s/48", parsed.String()[:14])
		}
		return ip

	case IPPrecisionNone:
		return "[IP_GENERALIZED]"

	default:
		return ip
	}
}

// GeneralizeTimestamp reduces precision of a timestamp.
func (g *Generalizer) GeneralizeTimestamp(ts time.Time) time.Time {
	if !g.config.Enabled {
		return ts
	}

	switch g.config.TimePrecision {
	case TimePrecisionFull:
		return ts

	case TimePrecisionMinute:
		return time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), 0, 0, ts.Location())

	case TimePrecisionHour:
		return time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), 0, 0, 0, ts.Location())

	case TimePrecisionDay:
		return time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, ts.Location())

	case TimePrecisionMonth:
		return time.Date(ts.Year(), ts.Month(), 1, 0, 0, 0, 0, ts.Location())

	default:
		return ts
	}
}

// GeneralizePath reduces specificity of a file path.
func (g *Generalizer) GeneralizePath(path string) string {
	if !g.config.Enabled {
		return path
	}

	switch g.config.PathGeneralization {
	case PathGeneralizationFull:
		return path

	case PathGeneralizationPattern:
		// Replace username and specific identifiers with wildcards
		// /home/alice/projects/myapp/src/main.go → /home/*/projects/*/src/main.go
		return generalizePathPattern(path)

	case PathGeneralizationBase:
		// Keep only the filename
		// /home/alice/projects/myapp/src/main.go → main.go
		return filepath.Base(path)

	case PathGeneralizationNone:
		return "[PATH_GENERALIZED]"

	default:
		return path
	}
}

// generalizePathPattern replaces specific directory names with wildcards.
func generalizePathPattern(path string) string {
	// Common patterns to generalize
	patterns := []struct {
		match   string
		replace string
	}{
		{"/home/", "/home/*/"},
		{"/Users/", "/Users/*/"},
		{"/root/", "/root/*/"},
		{".local/share/", ".local/share/*/"},
		{".config/", ".config/*/"},
		{".cache/", ".cache/*/"},
	}

	result := path
	for _, pattern := range patterns {
		if strings.Contains(result, pattern.match) {
			// Find the next path separator after the pattern
			idx := strings.Index(result, pattern.match)
			if idx >= 0 {
				after := result[idx+len(pattern.match):]
				nextSep := strings.Index(after, "/")
				if nextSep > 0 {
					// Replace the username/identifier with *
					result = result[:idx] + pattern.replace + after[nextSep+1:]
				}
			}
		}
	}

	return result
}

// GeneralizeBatch processes multiple values efficiently.
func (g *Generalizer) GeneralizeBatch(values []string, valueType string) []string {
	if !g.config.Enabled || len(values) == 0 {
		return values
	}

	result := make([]string, len(values))
	for i, value := range values {
		switch valueType {
		case "ip":
			result[i] = g.GeneralizeIP(value)
		case "path":
			result[i] = g.GeneralizePath(value)
		default:
			result[i] = value
		}
	}
	return result
}

// DefaultGeneralizationConfig returns a sensible default configuration.
func DefaultGeneralizationConfig() GeneralizationConfig {
	return GeneralizationConfig{
		IPPrecision:        IPPrecisionSubnet,
		TimePrecision:      TimePrecisionHour,
		PathGeneralization: PathGeneralizationPattern,
		Enabled:            true,
	}
}

// StrictGeneralizationConfig returns a configuration for maximum privacy.
func StrictGeneralizationConfig() GeneralizationConfig {
	return GeneralizationConfig{
		IPPrecision:        IPPrecisionClass,
		TimePrecision:      TimePrecisionDay,
		PathGeneralization: PathGeneralizationBase,
		Enabled:            true,
	}
}
