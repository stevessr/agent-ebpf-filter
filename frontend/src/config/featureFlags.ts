import { ALL_FEATURE_IDS, type FeatureID } from "../types/feature";

const normalizeFeatureName = (value: string): string =>
  value
    .trim()
    .toLowerCase()
    .replace(/^agentfeat_/, "")
    .replace(/[^a-z0-9_]+/g, "_")
    .replace(/^_+|_+$/g, "");

const parseFeatureList = (raw?: string) => {
  const normalized = (raw || "all").trim();
  if (!normalized || normalized === "all") {
    return { mode: "all" as const, features: new Set(ALL_FEATURE_IDS) };
  }
  if (normalized === "core") {
    return { mode: "core" as const, features: new Set<FeatureID>() };
  }
  const features = new Set<FeatureID>();
  for (const part of normalized.split(/[\s,]+/)) {
    const candidate = normalizeFeatureName(part);
    if ((ALL_FEATURE_IDS as string[]).includes(candidate)) {
      features.add(candidate as FeatureID);
    }
  }
  return { mode: "custom" as const, features };
};

const frontendBuildFeatures = parseFeatureList(
  import.meta.env.VITE_AGENT_BUILD_FEATURES,
);

export const FRONTEND_BUILD_FEATURE_MODE = frontendBuildFeatures.mode;
export const FRONTEND_BUILD_FEATURES = frontendBuildFeatures.features;

export const isFeatureIncludedInFrontendBuild = (feature: FeatureID) =>
  FRONTEND_BUILD_FEATURES.has(feature);

export const filterFeaturesForFrontendBuild = <
  T extends { feature?: FeatureID },
>(
  items: T[],
): T[] =>
  items.filter(
    (item) => !item.feature || isFeatureIncludedInFrontendBuild(item.feature),
  );
