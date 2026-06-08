package redaction

// RedactionLevel indicates how aggressively sensitive data should be masked.
type RedactionLevel string

const (
	RedactionLevelNone    RedactionLevel = "none"
	RedactionLevelBasic   RedactionLevel = "basic"
	RedactionLevelStandard RedactionLevel = "standard"
	RedactionLevelStrict  RedactionLevel = "strict"
)

// FieldCategory groups fields by the kind of sensitive data they may contain.
type FieldCategory string

const (
	FieldCategoryPath       FieldCategory = "path"
	FieldCategoryCommand    FieldCategory = "command"
	FieldCategoryNetwork    FieldCategory = "network"
	FieldCategoryCredential FieldCategory = "credential"
	FieldCategoryIdentifier FieldCategory = "identifier"
)

// SensitiveFieldRef references a field that should be redacted.
type SensitiveFieldRef struct {
	// Name is the field key or path within a payload.
	Name string `json:"name"`

	// Category describes the sensitive data class for the field.
	Category FieldCategory `json:"category"`

	// Pattern optionally narrows the match using a literal or regex pattern.
	Pattern string `json:"pattern,omitempty"`

	// Required marks the field as mandatory for a rule match.
	Required bool `json:"required,omitempty"`
}

// RedactionRule defines how a field or payload segment should be masked.
type RedactionRule struct {
	// ID is a stable rule identifier.
	ID string `json:"id"`

	// Description explains the rule's intent.
	Description string `json:"description,omitempty"`

	// Level is the redaction strength applied by this rule.
	Level RedactionLevel `json:"level"`

	// Categories restrict the rule to specific field categories.
	Categories []FieldCategory `json:"categories,omitempty"`

	// Fields lists the sensitive fields covered by this rule.
	Fields []SensitiveFieldRef `json:"fields,omitempty"`

	// ReplaceWith is the placeholder value used after redaction.
	ReplaceWith string `json:"replaceWith,omitempty"`

	// Enabled toggles the rule on or off.
	Enabled bool `json:"enabled,omitempty"`
}

// RedactionPolicy configures the active redaction behavior.
type RedactionPolicy struct {
	// Level is the default redaction level for the policy.
	Level RedactionLevel `json:"level"`

	// Rules contains custom rule overrides and additions.
	Rules []RedactionRule `json:"rules,omitempty"`

	// DefaultPlaceholder is used when a rule does not specify replacement text.
	DefaultPlaceholder string `json:"defaultPlaceholder,omitempty"`

	// PreserveLengths keeps redacted values aligned to their original length.
	PreserveLengths bool `json:"preserveLengths,omitempty"`

	// ExcludeCategories prevents redaction for the listed categories.
	ExcludeCategories []FieldCategory `json:"excludeCategories,omitempty"`
}
