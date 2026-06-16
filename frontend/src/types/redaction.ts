// TypeScript definitions matching backend/redaction/types.go

export type RedactionLevel = "none" | "basic" | "standard" | "strict";

export type FieldCategory = "path" | "command" | "network" | "credential" | "identifier";

export interface SensitiveFieldRef {
  name: string;
  category: FieldCategory;
  pattern?: string;
  required?: boolean;
}

export interface RedactionRule {
  id: string;
  description?: string;
  level: RedactionLevel;
  categories?: FieldCategory[];
  fields?: SensitiveFieldRef[];
  replaceWith?: string;
  enabled?: boolean;
}

export interface RedactionPolicy {
  level: RedactionLevel;
  rules?: RedactionRule[];
  defaultPlaceholder?: string;
  preserveLengths?: boolean;
  excludeCategories?: FieldCategory[];
}
