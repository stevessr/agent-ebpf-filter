package redaction

// ComplianceStandard represents a data protection regulation or standard.
type ComplianceStandard string

const (
	ComplianceGDPR   ComplianceStandard = "GDPR"    // EU General Data Protection Regulation
	ComplianceCCPA   ComplianceStandard = "CCPA"    // California Consumer Privacy Act
	ComplianceHIPAA  ComplianceStandard = "HIPAA"   // Health Insurance Portability and Accountability Act
	CompliancePCIDSS ComplianceStandard = "PCI-DSS" // Payment Card Industry Data Security Standard
	ComplianceSOC2   ComplianceStandard = "SOC2"    // Service Organization Control 2
	ComplianceISO27001 ComplianceStandard = "ISO27001" // ISO/IEC 27001
)

// ComplianceMapping indicates which regulations a redaction level satisfies.
type ComplianceMapping struct {
	Level       RedactionLevel
	Standards   []ComplianceStandard
	Description string
	Requirements string
}

// ComplianceMatrix defines compliance coverage for each redaction level.
var ComplianceMatrix = []ComplianceMapping{
	{
		Level:       RedactionLevelNone,
		Standards:   []ComplianceStandard{},
		Description: "No redaction - raw data",
		Requirements: "Not suitable for any compliance requirement. Use only in fully trusted environments.",
	},
	{
		Level:       RedactionLevelBasic,
		Standards:   []ComplianceStandard{CompliancePCIDSS},
		Description: "Basic masking of obvious secrets (passwords, credit cards)",
		Requirements: "Meets PCI-DSS requirement to mask PANs (Primary Account Numbers) when displaying cardholder data.",
	},
	{
		Level:       RedactionLevelStandard,
		Standards: []ComplianceStandard{
			ComplianceGDPR,
			ComplianceCCPA,
			CompliancePCIDSS,
			ComplianceSOC2,
		},
		Description: "Comprehensive PII and credential masking (recommended for production)",
		Requirements: "Meets GDPR Article 32 (security of processing), CCPA data minimization, PCI-DSS data protection, and SOC2 CC6.7 (data classification and protection).",
	},
	{
		Level:       RedactionLevelStrict,
		Standards: []ComplianceStandard{
			ComplianceGDPR,
			ComplianceCCPA,
			ComplianceHIPAA,
			CompliancePCIDSS,
			ComplianceSOC2,
			ComplianceISO27001,
		},
		Description: "Maximum redaction with anonymization and generalization",
		Requirements: "Meets all above plus HIPAA §164.514 (de-identification of protected health information) and ISO/IEC 27001 A.8.2.3 (handling of assets). Suitable for highly regulated environments.",
	},
}

// GetCompliance returns the compliance mapping for a given redaction level.
func GetCompliance(level RedactionLevel) ComplianceMapping {
	for _, mapping := range ComplianceMatrix {
		if mapping.Level == level {
			return mapping
		}
	}
	return ComplianceMapping{Level: level}
}

// MeetsStandard checks if a redaction level meets a specific compliance standard.
func MeetsStandard(level RedactionLevel, standard ComplianceStandard) bool {
	mapping := GetCompliance(level)
	for _, s := range mapping.Standards {
		if s == standard {
			return true
		}
	}
	return false
}

// GetRecommendedLevel returns the minimum redaction level for a set of standards.
func GetRecommendedLevel(standards []ComplianceStandard) RedactionLevel {
	// Check if HIPAA or ISO27001 is required (need Strict)
	for _, std := range standards {
		if std == ComplianceHIPAA || std == ComplianceISO27001 {
			return RedactionLevelStrict
		}
	}

	// Check if GDPR, CCPA, SOC2 is required (need Standard)
	for _, std := range standards {
		if std == ComplianceGDPR || std == ComplianceCCPA || std == ComplianceSOC2 {
			return RedactionLevelStandard
		}
	}

	// Check if PCI-DSS only (Basic sufficient)
	for _, std := range standards {
		if std == CompliancePCIDSS {
			return RedactionLevelBasic
		}
	}

	// No specific requirements
	return RedactionLevelNone
}

// ComplianceReport generates a compliance report for current settings.
type ComplianceReport struct {
	CurrentLevel    RedactionLevel
	MetStandards    []ComplianceStandard
	UnmetStandards  []ComplianceStandard
	Recommendations []string
}

// GenerateComplianceReport creates a compliance report.
func GenerateComplianceReport(currentLevel RedactionLevel, requiredStandards []ComplianceStandard) ComplianceReport {
	mapping := GetCompliance(currentLevel)

	report := ComplianceReport{
		CurrentLevel:   currentLevel,
		MetStandards:   []ComplianceStandard{},
		UnmetStandards: []ComplianceStandard{},
		Recommendations: []string{},
	}

	// Check which standards are met
	for _, required := range requiredStandards {
		met := false
		for _, available := range mapping.Standards {
			if required == available {
				met = true
				report.MetStandards = append(report.MetStandards, required)
				break
			}
		}
		if !met {
			report.UnmetStandards = append(report.UnmetStandards, required)
		}
	}

	// Generate recommendations
	if len(report.UnmetStandards) > 0 {
		recommendedLevel := GetRecommendedLevel(requiredStandards)
		report.Recommendations = append(report.Recommendations,
			"Upgrade to "+string(recommendedLevel)+" level to meet all required standards")

		// Specific recommendations
		for _, std := range report.UnmetStandards {
			switch std {
			case ComplianceHIPAA:
				report.Recommendations = append(report.Recommendations,
					"HIPAA requires de-identification per §164.514 - use Strict level with anonymization")
			case ComplianceGDPR:
				report.Recommendations = append(report.Recommendations,
					"GDPR Article 32 requires appropriate technical measures - use Standard or Strict level")
			case ComplianceCCPA:
				report.Recommendations = append(report.Recommendations,
					"CCPA requires data minimization - use Standard or Strict level")
			case ComplianceISO27001:
				report.Recommendations = append(report.Recommendations,
					"ISO/IEC 27001 A.8.2.3 requires proper handling of assets - use Strict level")
			}
		}
	} else {
		report.Recommendations = append(report.Recommendations,
			"Current level meets all required compliance standards")
	}

	return report
}

// ComplianceInfo provides detailed information about a compliance standard.
type ComplianceInfo struct {
	Standard    ComplianceStandard
	FullName    string
	Description string
	KeyRequirements []string
	MinimumLevel RedactionLevel
}

// GetComplianceInfo returns detailed information about a compliance standard.
func GetComplianceInfo(standard ComplianceStandard) ComplianceInfo {
	switch standard {
	case ComplianceGDPR:
		return ComplianceInfo{
			Standard:    ComplianceGDPR,
			FullName:    "General Data Protection Regulation",
			Description: "EU regulation on data protection and privacy",
			KeyRequirements: []string{
				"Article 32: Security of processing (technical measures)",
				"Article 25: Data protection by design and by default",
				"Pseudonymization and encryption of personal data",
			},
			MinimumLevel: RedactionLevelStandard,
		}
	case ComplianceCCPA:
		return ComplianceInfo{
			Standard:    ComplianceCCPA,
			FullName:    "California Consumer Privacy Act",
			Description: "California state law regulating consumer data privacy",
			KeyRequirements: []string{
				"Data minimization principle",
				"Purpose limitation for data collection",
				"Reasonable security procedures",
			},
			MinimumLevel: RedactionLevelStandard,
		}
	case ComplianceHIPAA:
		return ComplianceInfo{
			Standard:    ComplianceHIPAA,
			FullName:    "Health Insurance Portability and Accountability Act",
			Description: "US law protecting sensitive patient health information",
			KeyRequirements: []string{
				"§164.514: De-identification of protected health information (PHI)",
				"§164.312: Technical safeguards",
				"Safe Harbor method or Expert Determination",
			},
			MinimumLevel: RedactionLevelStrict,
		}
	case CompliancePCIDSS:
		return ComplianceInfo{
			Standard:    CompliancePCIDSS,
			FullName:    "Payment Card Industry Data Security Standard",
			Description: "Security standard for organizations handling credit cards",
			KeyRequirements: []string{
				"Requirement 3: Protect stored cardholder data",
				"Requirement 3.3: Mask PAN when displayed",
				"Show only first 6 and last 4 digits",
			},
			MinimumLevel: RedactionLevelBasic,
		}
	case ComplianceSOC2:
		return ComplianceInfo{
			Standard:    ComplianceSOC2,
			FullName:    "Service Organization Control 2",
			Description: "Auditing standard for service organizations' security",
			KeyRequirements: []string{
				"CC6.7: Data classification and protection",
				"Logical and physical access controls",
				"Data at rest and in transit protection",
			},
			MinimumLevel: RedactionLevelStandard,
		}
	case ComplianceISO27001:
		return ComplianceInfo{
			Standard:    ComplianceISO27001,
			FullName:    "ISO/IEC 27001 Information Security Management",
			Description: "International standard for information security management systems",
			KeyRequirements: []string{
				"A.8.2.3: Handling of assets",
				"A.9.2.3: Management of privileged access rights",
				"A.18.1.4: Privacy and protection of personal data",
			},
			MinimumLevel: RedactionLevelStrict,
		}
	default:
		return ComplianceInfo{
			Standard:    standard,
			FullName:    string(standard),
			Description: "Unknown compliance standard",
		}
	}
}
