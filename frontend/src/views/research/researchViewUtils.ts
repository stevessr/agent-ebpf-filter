import type {
  ResearchCount,
  ResearchEvent,
  ResearchSecurityEvaluationSampleRow,
} from "../../types/config";

const TOP_COUNT_LIMIT = 8;
const EVENT_FEATURE_LIMIT = 5;
const EVENT_FEATURE_VALUE_LIMIT = 40;

export const topCounts = (items?: ResearchCount[]) =>
  (items || []).slice(0, TOP_COUNT_LIMIT);

export const eventFeaturePreview = (event: ResearchEvent) => {
  const features = event.features || {};
  const keys = Object.keys(features).slice(0, EVENT_FEATURE_LIMIT);
  if (!keys.length) return "—";
  return keys
    .map(
      (key) =>
        `${key}=${String(features[key]).slice(0, EVENT_FEATURE_VALUE_LIMIT)}`,
    )
    .join(", ");
};

export const eventRowKey = (event: ResearchEvent) =>
  event.id || `${event.timestamp}:${event.source}:${event.eventType}`;

export const trainingLabelColor = (label?: string) => {
  switch ((label || "").toUpperCase()) {
    case "ALLOW":
      return "green";
    case "ALERT":
      return "orange";
    case "BLOCK":
      return "red";
    case "REWRITE":
      return "blue";
    case "UNLABELED":
      return "default";
    default:
      return "geekblue";
  }
};

export const securityPriorityColor = (priority?: string) => {
  switch ((priority || "").toLowerCase()) {
    case "critical":
      return "red";
    case "high":
      return "volcano";
    case "medium":
      return "orange";
    case "low":
      return "green";
    default:
      return "blue";
  }
};

export const securityActionColor = (action?: string) => {
  switch ((action || "").toUpperCase()) {
    case "ALLOW":
      return "green";
    case "ALERT":
      return "orange";
    case "BLOCK":
      return "red";
    case "REWRITE":
      return "blue";
    case "UNLABELED":
      return "default";
    default:
      return "geekblue";
  }
};

export const formatSecurityToken = (value: string) =>
  value.replaceAll("_", " ").replaceAll(":", ": ");

export const securityFindingColor = (finding?: string) => {
  switch ((finding || "").toLowerCase()) {
    case "false_positive":
      return "gold";
    case "false_negative":
      return "red";
    case "policy_gap":
      return "purple";
    case "high_confidence_disagreement":
      return "magenta";
    case "unlabeled_high_risk":
      return "volcano";
    default:
      return "default";
  }
};

export const securitySampleRowKey = (
  row: ResearchSecurityEvaluationSampleRow,
) => row.id || `${row.source}:${row.commandLine}:${row.expectedAction}`;
