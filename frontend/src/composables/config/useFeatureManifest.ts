import { computed, ref } from "vue";
import axios from "axios";
import {
  ALL_FEATURE_IDS,
  type FeatureID,
  type FeatureManifestEntry,
  type FeatureManifestResponse,
  type FeatureStatus,
} from "../../types/feature";
import { isFeatureIncludedInFrontendBuild } from "../../config/featureFlags";

const defaultFeatureName = (feature: FeatureID) =>
  feature
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");

const createFallbackEntry = (feature: FeatureID): FeatureManifestEntry => ({
  id: feature,
  name: defaultFeatureName(feature),
  compiledIn: isFeatureIncludedInFrontendBuild(feature),
  runtimeEnabled: false,
  authRequired: true,
  routePrefixes: [],
  dangerLevel: "medium",
  buildTag: `agentfeat_${feature}`,
});

export function useFeatureManifest() {
  const features = ref<FeatureManifestEntry[]>([]);
  const loading = ref(false);
  const loaded = ref(false);
  const error = ref("");

  const featuresById = computed(() =>
    features.value.reduce<Partial<Record<FeatureID, FeatureManifestEntry>>>(
      (acc, feature) => {
        acc[feature.id] = feature;
        return acc;
      },
      {},
    ),
  );

  const mergedFeatures = computed<FeatureManifestEntry[]>(() =>
    ALL_FEATURE_IDS.map((feature) => {
      const serverFeature = featuresById.value[feature];
      const frontendCompiledIn = isFeatureIncludedInFrontendBuild(feature);
      if (!serverFeature) {
        return createFallbackEntry(feature);
      }
      const compiledIn = frontendCompiledIn && serverFeature.compiledIn;
      return {
        ...serverFeature,
        compiledIn,
        runtimeEnabled: compiledIn && Boolean(serverFeature.runtimeEnabled),
      };
    }),
  );

  const mergedFeaturesById = computed(() =>
    mergedFeatures.value.reduce<Record<FeatureID, FeatureManifestEntry>>(
      (acc, feature) => {
        acc[feature.id] = feature;
        return acc;
      },
      {} as Record<FeatureID, FeatureManifestEntry>,
    ),
  );

  const fetchFeatureManifest = async () => {
    loading.value = true;
    error.value = "";
    try {
      const res = await axios.get<FeatureManifestResponse>("/system/features");
      features.value = Array.isArray(res.data.features)
        ? res.data.features
        : [];
      loaded.value = true;
    } catch (err) {
      error.value =
        err instanceof Error ? err.message : "Failed to fetch feature manifest";
    } finally {
      loading.value = false;
    }
  };

  const entryFor = (feature: FeatureID) =>
    mergedFeaturesById.value[feature] || createFallbackEntry(feature);

  const isCompiledIn = (feature: FeatureID) => entryFor(feature).compiledIn;

  const isRuntimeEnabled = (feature: FeatureID) =>
    isCompiledIn(feature) && entryFor(feature).runtimeEnabled;

  const featureStatus = (feature: FeatureID): FeatureStatus => {
    const entry = entryFor(feature);
    if (!entry.compiledIn) return "compiled-out";
    if (!loaded.value && !featuresById.value[feature]) return "unknown";
    if (!entry.runtimeEnabled) return "runtime-disabled";
    return "enabled";
  };

  const featureStatusLabel = (feature: FeatureID) => {
    switch (featureStatus(feature)) {
      case "compiled-out":
        return "未编译";
      case "runtime-disabled":
        return "运行时关闭";
      case "enabled":
        return "可用";
      default:
        return "未知";
    }
  };

  const featureStatusColor = (feature: FeatureID) => {
    switch (featureStatus(feature)) {
      case "compiled-out":
        return "red";
      case "runtime-disabled":
        return "orange";
      case "enabled":
        return "green";
      default:
        return "default";
    }
  };

  return {
    features,
    mergedFeatures,
    loading,
    loaded,
    error,
    fetchFeatureManifest,
    entryFor,
    isCompiledIn,
    isRuntimeEnabled,
    featureStatus,
    featureStatusLabel,
    featureStatusColor,
  };
}
