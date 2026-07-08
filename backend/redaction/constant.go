package redaction

// ── Redaction levels ─────────────────────────────────────────────────────────

// RedactionLevel indicates how aggressively sensitive data should be masked.
type RedactionLevel string

const (
	RedactionLevelNone     RedactionLevel = "none"
	RedactionLevelBasic    RedactionLevel = "basic"
	RedactionLevelStandard RedactionLevel = "standard"
	RedactionLevelStrict   RedactionLevel = "strict"
)

// ── Field categories ─────────────────────────────────────────────────────────

// FieldCategory groups fields by the kind of sensitive data they may contain.
type FieldCategory string

const (
	FieldCategoryPath       FieldCategory = "path"
	FieldCategoryCommand    FieldCategory = "command"
	FieldCategoryNetwork    FieldCategory = "network"
	FieldCategoryCredential FieldCategory = "credential"
	FieldCategoryIdentifier FieldCategory = "identifier"
)

// ── Compliance standards ─────────────────────────────────────────────────────

// ComplianceStandard represents a data protection regulation or standard.
type ComplianceStandard string

const (
	ComplianceGDPR      ComplianceStandard = "GDPR"
	ComplianceCCPA      ComplianceStandard = "CCPA"
	ComplianceHIPAA     ComplianceStandard = "HIPAA"
	CompliancePCIDSS    ComplianceStandard = "PCI-DSS"
	ComplianceSOC2      ComplianceStandard = "SOC2"
	ComplianceISO27001  ComplianceStandard = "ISO27001"
)

// ── Generalization precision levels ──────────────────────────────────────────

// IPPrecisionLevel controls IP address generalization.
type IPPrecisionLevel string

const (
	IPPrecisionFull   IPPrecisionLevel = "full"
	IPPrecisionSubnet IPPrecisionLevel = "subnet"
	IPPrecisionClass  IPPrecisionLevel = "class"
	IPPrecisionNone   IPPrecisionLevel = "none"
)

// TimePrecisionLevel controls timestamp generalization.
type TimePrecisionLevel string

const (
	TimePrecisionFull   TimePrecisionLevel = "full"
	TimePrecisionMinute TimePrecisionLevel = "minute"
	TimePrecisionHour   TimePrecisionLevel = "hour"
	TimePrecisionDay    TimePrecisionLevel = "day"
	TimePrecisionMonth  TimePrecisionLevel = "month"
)

// PathGeneralizationLevel controls file path generalization.
type PathGeneralizationLevel string

const (
	PathGeneralizationFull    PathGeneralizationLevel = "full"
	PathGeneralizationPattern PathGeneralizationLevel = "pattern"
	PathGeneralizationBase    PathGeneralizationLevel = "base"
	PathGeneralizationNone    PathGeneralizationLevel = "none"
)

// ── Path mapping rule types ──────────────────────────────────────────────────

// PathRuleType indicates how to apply a path mapping rule.
type PathRuleType string

const (
	PathRuleExact    PathRuleType = "exact"
	PathRulePrefix   PathRuleType = "prefix"
	PathRuleSuffix   PathRuleType = "suffix"
	PathRuleWildcard PathRuleType = "wildcard"
	PathRuleRegex    PathRuleType = "regex"
)